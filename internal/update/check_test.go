package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestFetchLatestTag_ParsesRedirectTag 验证从 302 Location 解析 tag（含尾斜杠）。
func TestFetchLatestTag_ParsesRedirectTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r,
			"https://github.com/eosaios/eos/releases/tag/v1.2.3/",
			http.StatusFound)
	}))
	defer srv.Close()

	tag, err := fetchLatestTag(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("fetchLatestTag() error = %v", err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("tag = %q, want v1.2.3", tag)
	}
}

// TestFetchLatestTag_NonRedirectIsTerminalError 验证无重定向（如限流页）
// 返回带状态码的错误且不重试。
func TestFetchLatestTag_NonRedirectIsTerminalError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("API rate limit exceeded"))
	}))
	defer srv.Close()

	_, err := fetchLatestTag(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("expected error for non-redirect response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error should carry status code, got: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("terminal error should not retry, attempts = %d", attempts)
	}
}

// TestFetchLatestTag_RetriesTransientNetworkErrors 验证瞬断（连接被重置）
// 时按退避重试并最终成功。
func TestFetchLatestTag_RetriesTransientNetworkErrors(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			panic(http.ErrAbortHandler) // 模拟瞬断：连接被服务端中断
		}
		http.Redirect(w, r,
			"https://github.com/eosaios/eos/releases/tag/v2.0.0",
			http.StatusFound)
	}))
	defer srv.Close()

	tag, err := fetchLatestTag(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("fetchLatestTag() error = %v", err)
	}
	if tag != "v2.0.0" {
		t.Fatalf("tag = %q, want v2.0.0", tag)
	}
	if attempts < 2 {
		t.Fatalf("expected retry after transient failure, attempts = %d", attempts)
	}
}

// TestFetchLatestTag_TimeoutGetsFreshBudgetPerAttempt 验证每次尝试持有
// 独立超时预算：首次请求整个超时（弱网黑洞，非瞬断）后，第二次尝试
// 仍能拿到完整预算并成功。共享总预算的旧实现在此场景下必然失败。
func TestFetchLatestTag_TimeoutGetsFreshBudgetPerAttempt(t *testing.T) {
	old := checkTimeout
	checkTimeout = 100 * time.Millisecond
	t.Cleanup(func() { checkTimeout = old })

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			<-r.Context().Done() // 模拟黑洞：挂死到本次尝试超时
			return
		}
		http.Redirect(w, r,
			"https://github.com/eosaios/eos/releases/tag/v3.0.0",
			http.StatusFound)
	}))
	defer srv.Close()

	tag, err := fetchLatestTag(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("fetchLatestTag() error = %v", err)
	}
	if tag != "v3.0.0" {
		t.Fatalf("tag = %q, want v3.0.0", tag)
	}
	if attempts < 2 {
		t.Fatalf("expected retry after timeout, attempts = %d", attempts)
	}
}

// TestBuildCheckResult_ConstructsDeterministicURLs 验证资产/校验/发布页 URL
// 按命名约定确定性拼接，无需 API 枚举。
func TestBuildCheckResult_ConstructsDeterministicURLs(t *testing.T) {
	r := buildCheckResult("v1.0.0-beta.9", "v1.0.0-beta.10", "windows", "amd64")

	if !r.HasUpdate {
		t.Fatal("HasUpdate should be true from beta.9 to beta.10")
	}
	wantAsset := "eos-cli_v1.0.0-beta.10_windows-amd64.zip"
	if r.AssetName != wantAsset {
		t.Fatalf("AssetName = %q, want %q", r.AssetName, wantAsset)
	}
	base := "https://github.com/eosaios/eos/releases/download/v1.0.0-beta.10"
	if r.DownloadURL != base+"/"+wantAsset {
		t.Fatalf("DownloadURL = %q", r.DownloadURL)
	}
	if r.ChecksumURL != base+"/SHA256SUMS.txt" {
		t.Fatalf("ChecksumURL = %q", r.ChecksumURL)
	}
	if r.ReleaseURL != "https://github.com/eosaios/eos/releases/tag/v1.0.0-beta.10" {
		t.Fatalf("ReleaseURL = %q", r.ReleaseURL)
	}
}

// TestBuildCheckResult_SameVersionNoUpdate 验证同版本不提示更新，非 Windows
// 平台归档后缀为 .tar.gz。
func TestBuildCheckResult_SameVersionNoUpdate(t *testing.T) {
	r := buildCheckResult("v1.0.0-beta.10", "v1.0.0-beta.10", "linux", "arm64")
	if r.HasUpdate {
		t.Fatal("same version should not report update")
	}
	if !strings.HasSuffix(r.AssetName, "_linux-arm64.tar.gz") {
		t.Fatalf("non-windows archive should be .tar.gz, got %q", r.AssetName)
	}
}
