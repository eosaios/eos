package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type UpdateCheckResult struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	DownloadURL    string `json:"downloadUrl,omitempty"`
	// 应用内更新下载用：sha256 校验 + 进度总量。
	AssetName      string `json:"assetName,omitempty"`
	AssetDigest    string `json:"assetDigest,omitempty"`
	AssetSizeBytes int64  `json:"assetSizeBytes,omitempty"`
	ReleaseNotes   string `json:"releaseNotes,omitempty"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	CheckedAt      string `json:"checkedAt"`
	Error          string `json:"error,omitempty"`
}

var (
	updateCacheMu      sync.Mutex
	cachedUpdateResult *UpdateCheckResult
	lastCheckAt        time.Time
)

// updateCacheTTL 成功结果的短时缓存窗口：避免每次手动检查都打网络请求。
// 失败结果不缓存，下一次手动检查会重新尝试（这是修掉的旧 bug：sync.Once
// 让首次失败被永久缓存，之后永远返回"已是最新"）。
const updateCacheTTL = 15 * time.Minute

// checkTimeout 是单次网络尝试的超时预算。var 以便测试注入短超时。
var checkTimeout = 15 * time.Second

func (s *BridgeService) CheckForUpdates() UpdateCheckResult {
	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()

	// 成功结果在 TTL 内直接复用；失败/超时结果不缓存，允许重试。
	if cachedUpdateResult != nil && time.Since(lastCheckAt) < updateCacheTTL {
		return *cachedUpdateResult
	}

	_, proxyURL, err := s.updateProxyRaw()
	if err != nil {
		// 完整错误落日志；UI 状态条只展示单行摘要（URL/签名细节是噪音）。
		slog.Error("update check failed", "source", "proxy_config", "error", err)
		return UpdateCheckResult{
			CurrentVersion: BuildVersion,
			HasUpdate:      false,
			Error:          err.Error(),
			CheckedAt:      time.Now().Format(time.RFC3339),
		}
	}
	result, err := checkGitHubLatest(proxyURL)
	if err != nil {
		slog.Error("update check failed", "source", "github", "error", err)
		return UpdateCheckResult{
			CurrentVersion: BuildVersion,
			HasUpdate:      false,
			Error:          err.Error(),
			CheckedAt:      time.Now().Format(time.RFC3339),
		}
	}
	cachedUpdateResult = result
	lastCheckAt = time.Now()
	return *result
}

// checkGitHubLatest 用 releases/latest 网页重定向解析最新版本，不走 api.github.com：
// 未认证 GitHub API 限流 60 次/小时/IP，共享代理出口（国内常态）极易触发 403
// 被误读为「无更新」；网页会 302 到 releases/tag/<版本>，从 Location 解析 tag
// 不占 API 配额（与 eos-cli internal/update/check.go 同一方案）。
// 资产地址与 sha256 走确定性拼接 + SHA256SUMS.txt，不再请求 API 枚举 assets。
func checkGitHubLatest(proxyURL string) (*UpdateCheckResult, error) {
	current := strings.TrimSpace(BuildVersion)
	if current == "" || current == "dev" {
		return &UpdateCheckResult{CurrentVersion: current, HasUpdate: false, CheckedAt: time.Now().Format(time.RFC3339)}, nil
	}

	// 代理 URL 非法在构造期 fail-fast（配置被手工改坏时立即报错，不静默直连）。
	client, err := updateHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()

	latest, err := fetchReleaseTag(ctx, releaseLatestPageURL(), client)
	if err != nil {
		return nil, err
	}

	result := &UpdateCheckResult{
		CurrentVersion: current,
		LatestVersion:  latest,
		HasUpdate:      isNewerVersion(current, latest),
		ReleaseURL:     fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", githubAppOwner, githubAppRepo, latest),
		CheckedAt:      time.Now().Format(time.RFC3339),
	}
	if !result.HasUpdate {
		return result, nil
	}

	assetName, available := desktopAssetName(latest, runtime.GOOS, runtime.GOARCH)
	if !available {
		// 当前平台无安装包：前端回落 Release 页链接（与旧行为一致）。
		return result, nil
	}
	result.AssetName = assetName
	base := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s", githubAppOwner, githubAppRepo, latest)
	result.DownloadURL = base + "/" + assetName

	digest, err := fetchAssetDigest(ctx, base+"/SHA256SUMS.txt", assetName, client)
	if err != nil {
		return nil, fmt.Errorf("获取发布校验清单失败: %w", err)
	}
	result.AssetDigest = digest
	return result, nil
}

const githubAppOwner = "eosaios"
const githubAppRepo = "eos-app"

// releaseLatestPageURL 返回 releases/latest 网页地址（非 API 端点）。
func releaseLatestPageURL() string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/latest", githubAppOwner, githubAppRepo)
}

// fetchReleaseTag 通过 releases/latest 网页重定向解析最新版本号。
// 弱网下瞬断常见（EOF/RST/整段黑洞），带退避重试；每次尝试持有独立的
// checkTimeout 预算——若共享一个总预算，首次超时（最长 checkTimeout）
// 就几乎耗尽窗口，后续重试拿不到完整预算形同虚设。
func fetchReleaseTag(ctx context.Context, page string, client *http.Client) (string, error) {
	// client 非 nil 时取其 Transport（显式代理），否则用默认 Transport
	//（遵循环境代理约定）；两种源统一包裹重定向陷阱——不跟随 302、
	// 直接取 Location 解析 tag。
	transport := http.DefaultTransport
	if client != nil && client.Transport != nil {
		transport = client.Transport
	}
	trapClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 不跟随重定向，直接取 302 的 Location
		},
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, page, nil)
		if err != nil {
			cancel()
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("User-Agent", "EOS-App-Update-Check")

		resp, err := trapClient.Do(req)
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("resolve latest release: %w", err)
		} else {
			tag, err := tagFromRedirect(resp)
			if err == nil {
				return tag, nil
			}
			// 非 3xx/缺 Location 是终态错误（如限流页），短时间窗内结果
			// 不变，重试无意义，直接失败。
			return "", err
		}
		if attempt == 3 {
			break
		}
		select {
		case <-ctx.Done():
			return "", lastErr
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	return "", lastErr
}

// tagFromRedirect 从 releases/latest 的重定向响应中解析版本号。
// 非 3xx/缺 Location 视为终态错误（如限流页），不重试。
func tagFromRedirect(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	loc := strings.TrimSpace(resp.Header.Get("Location"))
	const marker = "/tag/"
	if loc == "" || !strings.Contains(loc, marker) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("releases/latest 未返回版本重定向（HTTP %d）: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}
	tag := strings.TrimSuffix(loc[strings.LastIndex(loc, marker)+len(marker):], "/")
	if tag == "" {
		return "", fmt.Errorf("releases/latest 重定向缺少版本号: %s", loc)
	}
	return tag, nil
}

// fetchAssetDigest 从发布资产的 SHA256SUMS.txt 中解析指定资产的 sha256。
// 网络瞬断带重试；服务端响应（非 200/清单缺条目）视为终态错误不重试。
func fetchAssetDigest(ctx context.Context, checksumURL, assetName string, client *http.Client) (string, error) {
	active := client
	if active == nil {
		active = http.DefaultClient
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, checksumURL, nil)
		if err != nil {
			cancel()
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("User-Agent", "EOS-App-Update-Check")

		resp, err := active.Do(req)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("fetch SHA256SUMS.txt: %w", err)
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			cancel()
			if readErr != nil {
				lastErr = fmt.Errorf("read SHA256SUMS.txt: %w", readErr)
			} else if resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("SHA256SUMS.txt 返回 HTTP %d", resp.StatusCode)
			} else if digest := assetDigestFromChecksums(body, assetName); digest == "" {
				return "", fmt.Errorf("%s 未列入 SHA256SUMS.txt（清单缺失校验值，拒绝下载）", assetName)
			} else {
				return digest, nil
			}
		}
		if attempt == 3 {
			break
		}
		select {
		case <-ctx.Done():
			return "", lastErr
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	return "", lastErr
}

// assetDigestFromChecksums 解析 sha256sum 格式清单（"<hex>  <name>"）中
// 指定资产的小写 hex 摘要；未列出返回空串。
func assetDigestFromChecksums(sums []byte, assetName string) string {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[1] == assetName {
			return strings.ToLower(fields[0])
		}
	}
	return ""
}

// desktopAssetName 按发布资产命名约定构造当前平台的安装包名
// （与 scripts/build_release.go 归档命名 + installer.iss 一致）：
//   - Windows：eos-app-setup-<版本>.exe（Inno Setup 安装器，tag 带 v 前缀）；
//   - darwin/linux：eos-app_<去 v 版本>_<goos>-<goarch>.dmg / .tar.gz。
//
// 返回 available=false 表示该平台无安装包（前端回落 Release 页链接）。
func desktopAssetName(tag, goos, goarch string) (string, bool) {
	switch desktopAssetPattern(goos, goarch) {
	case "":
		return "", false
	case "setup":
		return fmt.Sprintf("eos-app-setup-%s.exe", tag), true
	case "darwin-arm64.dmg":
		return fmt.Sprintf("eos-app_%s_darwin-arm64.dmg", trimTagPrefix(tag)), true
	case "darwin-amd64.dmg":
		return fmt.Sprintf("eos-app_%s_darwin-amd64.dmg", trimTagPrefix(tag)), true
	case "linux-amd64.tar.gz":
		return fmt.Sprintf("eos-app_%s_linux-amd64.tar.gz", trimTagPrefix(tag)), true
	case "linux-arm64.tar.gz":
		return fmt.Sprintf("eos-app_%s_linux-arm64.tar.gz", trimTagPrefix(tag)), true
	default:
		return "", false
	}
}

func trimTagPrefix(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "v")
}

// desktopAssetPattern 返回 eos-app Release 资产名中能唯一定位当前平台
// 安装包的片段（与 build_release.go 的归档命名约定一致）。
// Windows setup 资产名形如 eos-app-setup-<版本>.exe（不含平台段），
// 匹配关键词 "windows-amd64" 会落空导致「未提供当前平台的安装包」。
func desktopAssetPattern(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "darwin/arm64":
		return "darwin-arm64.dmg"
	case "darwin/amd64":
		return "darwin-amd64.dmg"
	case "linux/amd64":
		return "linux-amd64.tar.gz"
	case "linux/arm64":
		return "linux-arm64.tar.gz"
	case "windows/amd64":
		return "setup"
	default:
		return ""
	}
}

func isNewerVersion(current, latest string) bool {
	c := strings.TrimPrefix(strings.TrimSpace(current), "v")
	l := strings.TrimPrefix(strings.TrimSpace(latest), "v")
	if l == "" || c == "" {
		return false
	}
	// 数值化比较：把 X.Y.Z[-pre] 拆段，数字段按数值比、 prerelease 内
	// 数字标识也按数值比（修复 beta.10 < beta.3 的字典序误判）。
	cv, cpre := splitVer(c)
	lv, lpre := splitVer(l)
	for i := 0; i < 3; i++ {
		if cv[i] != lv[i] {
			return lv[i] > cv[i]
		}
	}
	if cpre == lpre {
		return false
	}
	if cpre == "" {
		return false // 当前是正式版，latest 是预发布
	}
	if lpre == "" {
		return true // latest 是正式版
	}
	return newerPre(lpre, cpre)
}

func splitVer(v string) ([3]uint64, string) {
	var out [3]uint64
	if idx := strings.Index(v, "-"); idx >= 0 {
		copyNums(&out, v[:idx])
		return out, v[idx+1:]
	}
	copyNums(&out, v)
	return out, ""
}

func copyNums(out *[3]uint64, s string) {
	parts := strings.SplitN(s, ".", 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.ParseUint(parts[i], 10, 64)
		out[i] = n
	}
}

// newerPre 判断 prerelease a 是否比 b 新（逐点分段，数字段数值比较）。
func newerPre(a, b string) bool {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.ParseUint(as[i], 10, 64)
		bn, berr := strconv.ParseUint(bs[i], 10, 64)
		if aerr == nil && berr == nil && an != bn {
			return an > bn
		}
		if aerr != nil || berr != nil {
			// 任一非纯数字：比较字符串兜底
			if as[i] != bs[i] {
				return as[i] > bs[i]
			}
		}
	}
	return len(as) > len(bs)
}
