package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// 应用内更新下载：发现新版本后不跳浏览器，直接在应用内下载当前平台
// 安装包并用 GitHub Release API 的 asset digest 做 sha256 校验，就绪后
// 一键安装（Windows 提权拉起 setup 安装器；macOS 原地替换 bundle 后
// 自动重启；Linux 打开下载目录）。进度经 eos:bridge:update-download
// 事件推给前端。

const updateDownloadEventName = "eos:bridge:update-download"

// 更新下载阶段：idle → downloading → verifying → ready / failed。
const (
	updateStageIdle        = "idle"
	updateStageDownloading = "downloading"
	updateStageVerifying   = "verifying"
	updateStageReady       = "ready"
	updateStageFailed      = "failed"
)

// updateDownloadEmitInterval 进度事件的节流下限：流式回调每 64KB 一次，
// 不加节流会对事件总线刷出每秒数百条消息。
const updateDownloadEmitInterval = 150 * time.Millisecond

type UpdateDownloadState struct {
	Stage         string `json:"stage"`
	Version       string `json:"version,omitempty"`
	AssetName     string `json:"assetName,omitempty"`
	ReceivedBytes int64  `json:"receivedBytes"`
	TotalBytes    int64  `json:"totalBytes"`
	Percent       int    `json:"percent"`
	LocalPath     string `json:"localPath,omitempty"`
	Error         string `json:"error,omitempty"`
}

// UpdateInstallState 是状态快照（bridge_types.go）的 DTO 字段，恒为 idle：
// 应用内更新走 StartUpdateDownload/InstallUpdate 的状态机，此结构仅维持
// 既有桥接契约。
type UpdateInstallState struct {
	Status      string `json:"status"`
	Version     string `json:"version,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
	FilePath    string `json:"filePath,omitempty"`
	Progress    int    `json:"progress,omitempty"`
	Error       string `json:"error,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
}

func (s *BridgeService) GetUpdateDownloadState() UpdateDownloadState {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	return s.updateDownload
}

// CancelUpdateDownload 取消进行中的下载（幂等，无下载时返回当前状态）。
func (s *BridgeService) CancelUpdateDownload() UpdateDownloadState {
	s.updateMu.Lock()
	cancel := s.updateCancel
	s.updateMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return s.GetUpdateDownloadState()
}

// StartUpdateDownload 启动当前平台更新包的应用内下载（幂等）：
// 下载中返回当前进度；已就绪且文件仍在则直接返回就绪态；其余状态重新发起。
func (s *BridgeService) StartUpdateDownload() UpdateDownloadState {
	s.updateMu.Lock()
	switch s.updateDownload.Stage {
	case updateStageDownloading:
		state := s.updateDownload
		s.updateMu.Unlock()
		return state
	case updateStageReady:
		if _, err := os.Stat(s.updateDownload.LocalPath); err == nil {
			state := s.updateDownload
			s.updateMu.Unlock()
			return state
		}
	}
	s.updateMu.Unlock()

	info := s.CheckForUpdates()
	if info.Error != "" {
		s.failUpdateDownload("检查更新失败：" + info.Error)
		return s.GetUpdateDownloadState()
	}
	if !info.HasUpdate {
		s.failUpdateDownload("已是最新版本，无需下载")
		return s.GetUpdateDownloadState()
	}
	if info.DownloadURL == "" || info.AssetName == "" {
		s.failUpdateDownload("Release 未提供当前平台的安装包")
		return s.GetUpdateDownloadState()
	}
	if info.AssetDigest == "" {
		// 无法校验完整性的包不落地（不留兜底：宁可失败也不装未校验的二进制）
		s.failUpdateDownload("Release 资产缺少 sha256 digest，拒绝下载")
		return s.GetUpdateDownloadState()
	}

	dest := filepath.Join(updateDownloadDir(), info.AssetName)

	// 下载客户端按代理开关构建（开关关 = 默认客户端）。地址非法在构造期
	// fail-fast：不等下载超时才暴露配置错误。
	_, proxyURL, proxyErr := s.updateProxyRaw()
	if proxyErr != nil {
		s.failUpdateDownload("读取代理配置失败：" + proxyErr.Error())
		return s.GetUpdateDownloadState()
	}
	downloadClient, err := updateHTTPClient(proxyURL)
	if err != nil {
		s.failUpdateDownload("更新代理地址非法：" + err.Error())
		return s.GetUpdateDownloadState()
	}

	ctx, cancel := context.WithCancel(context.Background())

	s.updateMu.Lock()
	s.updateCancel = cancel
	s.updateDownload = UpdateDownloadState{
		Stage:      updateStageDownloading,
		Version:    info.LatestVersion,
		AssetName:  info.AssetName,
		TotalBytes: info.AssetSizeBytes,
	}
	state := s.updateDownload
	s.updateMu.Unlock()
	s.emitUpdateDownloadState(state)

	go s.runUpdateDownload(ctx, downloadClient, info, dest)
	return state
}

// InstallUpdate 启动已就绪更新包的安装（仅 ready 状态可用）。
func (s *BridgeService) InstallUpdate() error {
	s.updateMu.Lock()
	state := s.updateDownload
	s.updateMu.Unlock()
	if state.Stage != updateStageReady || state.LocalPath == "" {
		err := errors.New("更新包尚未就绪，请先完成下载")
		slog.Error("update install failed", "error", err)
		return err
	}
	if _, err := os.Stat(state.LocalPath); err != nil {
		err = fmt.Errorf("更新包文件已不存在（可能被清理），请重新下载: %w", err)
		slog.Error("update install failed", "error", err)
		return err
	}
	if err := launchUpdateInstaller(state.LocalPath); err != nil {
		slog.Error("update install failed", "path", state.LocalPath, "error", err)
		return err
	}
	// 拉起安装器成功后主动退出：macOS 原地替换与 Windows Inno 覆盖安装都
	// 需要旧进程让位（iss [Code] 的 taskkill 只兜残留的孤儿内核），装完由
	// 安装器侧（iss [Run]）自动拉起新版本。Linux 只打开下载目录，不退出。
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		scheduleUpdateQuit(s)
	}
	return nil
}

// scheduleUpdateQuit 延迟退出当前进程：给桥接 RPC 响应留出返回前端的
// 时间，再交给安装器（Windows Inno / macOS 已调度的新版本）接管。
// web 模式没有 Wails app，直接退出进程。
func scheduleUpdateQuit(s *BridgeService) {
	go func() {
		time.Sleep(800 * time.Millisecond)
		RequestShutdown()
		os.Exit(0)
	}()
}

func (s *BridgeService) runUpdateDownload(ctx context.Context, client *http.Client, info UpdateCheckResult, dest string) {
	// 下载时限给足余量（拉 GitHub 资产在国内网络下普遍偏慢）
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	var lastEmit time.Time
	err := downloadHTTPToFile(ctx, client, info.DownloadURL, dest, info.AssetSizeBytes, func(received, total int64) {
		now := time.Now()
		if now.Sub(lastEmit) < updateDownloadEmitInterval {
			return
		}
		lastEmit = now
		s.updateUpdateDownloadState(func(st *UpdateDownloadState) {
			st.ReceivedBytes, st.TotalBytes = received, total
			st.Percent = downloadPercent(received, total)
		})
	})
	if err != nil {
		_ = os.Remove(dest)
		s.failUpdateDownload("下载失败：" + err.Error())
		return
	}

	s.updateUpdateDownloadState(func(st *UpdateDownloadState) { st.Stage = updateStageVerifying })
	if err := verifyFileDigest(dest, info.AssetDigest); err != nil {
		_ = os.Remove(dest)
		s.failUpdateDownload("完整性校验失败，已丢弃下载文件：" + err.Error())
		return
	}

	s.updateUpdateDownloadState(func(st *UpdateDownloadState) {
		st.Stage = updateStageReady
		st.Percent = 100
		st.LocalPath = dest
	})
}

// downloadPercent 计算下载进度（封顶 99%——100% 留给校验/就绪阶段语义）。
// 从 windows-only 的 bridge_terminal_shell_install.go 迁出：应用内更新
// 是全平台功能，非 Windows 构建原先引用不到该符号导致编译失败。
func downloadPercent(received, total int64) int {
	if total <= 0 {
		return 0
	}
	percent := int(float64(received) / float64(total) * 100)
	if percent > 99 {
		percent = 99
	}
	return percent
}

func (s *BridgeService) failUpdateDownload(message string) {
	// 完整错误（含网络协议细节、URL）在这里落日志；UI 状态条只显示单行摘要。
	slog.Error("update download failed", "error", message)
	s.updateUpdateDownloadState(func(st *UpdateDownloadState) {
		st.Stage = updateStageFailed
		st.Error = message
		st.ReceivedBytes, st.TotalBytes, st.Percent = 0, 0, 0
		st.LocalPath = ""
	})
}

// updateUpdateDownloadState 在锁内修改状态并向事件总线推送快照。
func (s *BridgeService) updateUpdateDownloadState(mutate func(*UpdateDownloadState)) {
	s.updateMu.Lock()
	mutate(&s.updateDownload)
	state := s.updateDownload
	s.updateMu.Unlock()
	s.emitUpdateDownloadState(state)
}

func (s *BridgeService) emitUpdateDownloadState(state UpdateDownloadState) {
	emitter := s.currentEmitter()
	if emitter == nil {
		return
	}
	emitter(updateDownloadEventName, state)
}

// updateDownloadDir 下载缓存目录：与终端 shell 安装同一约定（Windows 下
// os.UserCacheDir() == %LOCALAPPDATA%，即 %LOCALAPPDATA%\eos\updates）。
// UserCacheDir 失败回落 TempDir：该调用只在系统环境严重异常时失败，
// 回落目录仅影响缓存位置，不影响功能正确性。
func updateDownloadDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "eos-updates")
	}
	return filepath.Join(base, "eos", "updates")
}

// verifyFileDigest 校验文件 sha256 与 GitHub digest（"sha256:<hex>"）一致。
func verifyFileDigest(path, digest string) error {
	want := strings.TrimPrefix(strings.TrimSpace(digest), "sha256:")
	if want == "" || len(want) != sha256.Size*2 {
		return fmt.Errorf("digest 格式非法: %q", digest)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("sha256 不匹配（期望 %s，实际 %s）", want, got)
	}
	return nil
}
