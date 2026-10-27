//go:build windows

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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// PortableGit 按需安装器：探测三层全落空时，下载官方 PortableGit
// 自解压包到程序指定目录（%LOCALAPPDATA%\eos\shell\git），装完即可用。
//
// 版本与哈希钉死在官方 release（v2.55.0.windows.4）：从网络获取可执行
// 内容必须校验 sha256，不符即拒绝安装（fail-fast，不静默降级）。

const (
	portableGitVersion     = "2.55.0.4"
	portableGitReleaseTag  = "v2.55.0.windows.4"
	portableGitAssetName   = "PortableGit-2.55.0.4-64-bit.7z.exe"
	portableGitSizeBytes   = int64(58915456)
	portableGitDownloadURL = "https://github.com/git-for-windows/git/releases/download/" +
		portableGitReleaseTag + "/" + portableGitAssetName
	portableGitSHA256 = "016e84230a3767f0c6b3788e79ba0c58a17377086801719d46700fca4f7b36b5"
	// EOS_PORTABLEGIT_MIRROR 覆盖下载 URL 前缀（默认 github 官方），供网络
	// 受限环境自救。镜像仅改前缀，资产名与哈希校验不变，安全性等价。
	portableGitMirrorEnv = "EOS_PORTABLEGIT_MIRROR"
	// 安装目录名（位于 %LOCALAPPDATA%\eos\shell\ 下）。
	terminalShellDirName = "git"
)

type terminalShellProgress struct {
	Stage    string `json:"stage"` // downloading | extracting | done | failed
	Percent  int    `json:"percent"`
	Received int64  `json:"receivedBytes"`
	Total    int64  `json:"totalBytes"`
	Message  string `json:"message,omitempty"`
}

// shellAssetFetcher 抽象下载层（测试注入本地文件源，不打网络）。
type shellAssetFetcher func(ctx context.Context, url string, progress func(received, total int64)) (string, error)

// InstallTerminalShell 触发后台安装；已在安装中或 bash 已可用时直接返回。
func (s *BridgeService) InstallTerminalShell() error {
	if s == nil {
		return errors.New("bridge service is not available")
	}
	s.terminalShell.mu.Lock()
	if s.terminalShell.installing {
		s.terminalShell.mu.Unlock()
		return errors.New("terminal shell installation already in progress")
	}
	s.terminalShell.installing = true
	s.terminalShell.mu.Unlock()

	go s.runTerminalShellInstall(terminalShellInstaller{
		fetch:       httpShellAssetFetcher,
		extract:     extractPortableGit,
		ws:          newTerminalShellWorkspace(),
		expectedSHA: portableGitSHA256,
	})

	s.emitTerminalShellProgress(terminalShellProgress{Stage: "downloading", Total: portableGitSizeBytes})
	return nil
}

// terminalShellInstaller 安装流程依赖集：生产注入网络下载 + SFX 执行，
// 测试注入本地文件源 + 文件操作模拟，流程编排逻辑不变。
type terminalShellInstaller struct {
	fetch       shellAssetFetcher
	extract     func(sfxPath string, stagingDir string) error
	ws          terminalShellWorkspace
	expectedSHA string
}

// terminalShellWorkspace 安装的目录布局（下载缓存/暂存/最终目录）。
type terminalShellWorkspace struct {
	shellRoot   string // %LOCALAPPDATA%\eos\shell
	installDir  string // .../shell/git
	downloadDir string
	stagingDir  string // .../shell/.staging
}

func newTerminalShellWorkspace() terminalShellWorkspace {
	localAppData := os.Getenv("LOCALAPPDATA")
	root := filepath.Join(localAppData, "eos", "shell")
	return terminalShellWorkspace{
		shellRoot:   root,
		installDir:  filepath.Join(root, terminalShellDirName),
		downloadDir: filepath.Join(root, ".download"),
		stagingDir:  filepath.Join(root, ".staging"),
	}
}

func (s *BridgeService) runTerminalShellInstall(inst terminalShellInstaller) {
	fail := func(message string) {
		s.terminalShellSetInstalling(false)
		s.emitTerminalShellProgress(terminalShellProgress{Stage: "failed", Message: message})
	}

	// 幂等：已装好（bin\bash.exe 就位）则直接完成。
	if _, err := os.Stat(filepath.Join(inst.ws.installDir, "bin", "bash.exe")); err == nil {
		s.terminalShellSetInstalling(false)
		s.emitTerminalShellProgress(terminalShellProgress{Stage: "done"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	s.emitTerminalShellProgress(terminalShellProgress{Stage: "downloading", Total: portableGitSizeBytes})
	sfxPath, err := inst.fetch(ctx, portableGitFetchURL(), func(received, total int64) {
		s.emitTerminalShellProgress(terminalShellProgress{
			Stage: "downloading", Received: received, Total: total,
			Percent: downloadPercent(received, total),
		})
	})
	if err != nil {
		fail(fmt.Sprintf("下载 PortableGit 失败：%v", err))
		return
	}

	if err := verifyFileSHA256(sfxPath, inst.expectedSHA); err != nil {
		// 校验失败必须拒绝安装并清理：不完整的文件绝不能执行。
		_ = os.Remove(sfxPath)
		fail(fmt.Sprintf("PortableGit 校验失败：%v", err))
		return
	}

	s.emitTerminalShellProgress(terminalShellProgress{Stage: "extracting", Percent: 0})
	if err := inst.extract(sfxPath, inst.ws.stagingDir); err != nil {
		fail(fmt.Sprintf("解压 PortableGit 失败：%v", err))
		return
	}
	_ = os.Remove(sfxPath)
	// staging 校验通过后原子切换到最终目录（同卷 rename；失败即报错重试）。
	if _, err := os.Stat(filepath.Join(inst.ws.stagingDir, "bin", "bash.exe")); err != nil {
		fail("PortableGit 解压完成但未找到 bin\bash.exe")
		return
	}
	if err := os.RemoveAll(inst.ws.installDir); err != nil {
		fail(fmt.Sprintf("清理旧安装目录失败：%v", err))
		return
	}
	if err := os.Rename(inst.ws.stagingDir, inst.ws.installDir); err != nil {
		fail(fmt.Sprintf("安装目录切换失败：%v", err))
		return
	}

	if _, err := os.Stat(filepath.Join(inst.ws.installDir, "bin", "bash.exe")); err != nil {
		fail("安装完成但未找到 bin\\bash.exe")
		return
	}

	s.terminalShellInvalidate()
	s.terminalShellSetInstalling(false)
	s.emitTerminalShellProgress(terminalShellProgress{Stage: "done"})
}

func (s *BridgeService) terminalShellSetInstalling(installing bool) {
	s.terminalShell.mu.Lock()
	s.terminalShell.installing = installing
	s.terminalShell.mu.Unlock()
}

func (s *BridgeService) emitTerminalShellProgress(progress terminalShellProgress) {
	emitter := s.currentEmitter()
	if emitter == nil {
		return
	}
	emitter(terminalShellEventName, progress)
}

func portableGitFetchURL() string {
	if mirror := strings.TrimSpace(os.Getenv(portableGitMirrorEnv)); mirror != "" {
		return strings.TrimRight(mirror, "/") + "/" + portableGitAssetName
	}
	return portableGitDownloadURL
}

// httpShellAssetFetcher 下载到临时文件并回调进度（流式实现归一到
// downloadHTTPToFile，此处只负责临时文件命名与体积提示）。
func httpShellAssetFetcher(ctx context.Context, url string, progress func(received, total int64)) (string, error) {
	tmp, err := os.CreateTemp("", "eos-portablegit-*.7z.exe")
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	tmp.Close()
	if err := downloadHTTPToFile(ctx, nil, url, path, portableGitSizeBytes, progress); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// extractPortableGit 运行自解压（-o 输出目录 -y 全部确认）到 staging；
// HideWindow + CREATE_NO_WINDOW 防止 SFX 闪出进度窗。
func extractPortableGit(sfxPath string, stagingDir string) error {
	if err := os.RemoveAll(stagingDir); err != nil {
		return err
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(sfxPath, "-o"+stagingDir, "-y")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sfx exit: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func verifyFileSHA256(path string, expectedHex string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, expectedHex) {
		return fmt.Errorf("sha256 mismatch: got %s", actual)
	}
	return nil
}
