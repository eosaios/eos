package webbridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func contextBackground() context.Context {
	return context.Background()
}

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v1.0.0-beta.3", "v1.0.0-beta.10", true},  // 字典序会误判的经典场景
		{"v1.0.0-beta.10", "v1.0.0-beta.3", false}, //
		{"v1.0.0-beta.4", "v1.0.0-beta.5", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.9", "v1.0.10", true},
		{"v1.0.0-beta.5", "v1.0.0", true},  // 预发布 → 正式版是升级
		{"v1.0.0", "v1.0.0-beta.9", false}, // 正式版不回退到预发布
		{"", "v1.0.0", false},
	}
	for _, c := range cases {
		if got := isNewerVersion(c.current, c.latest); got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestDesktopAssetPattern(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}: "darwin-arm64.dmg",
		{"darwin", "amd64"}: "darwin-amd64.dmg",
		{"linux", "amd64"}:  "linux-amd64.tar.gz",
		{"linux", "arm64"}:  "linux-arm64.tar.gz",
		// Windows setup 资产名形如 eos-app-setup-<版本>.exe（不含平台段），
		// 匹配 "setup" 而非 "windows-amd64"（后者会漏选 setup 导致
		// 「Release 未提供当前平台的安装包」）。
		{"windows", "amd64"}: "setup",
	}
	for k, want := range cases {
		if got := desktopAssetPattern(k[0], k[1]); got != want {
			t.Errorf("desktopAssetPattern(%s,%s) = %q, want %q", k[0], k[1], got, want)
		}
	}
}

// TestDesktopAssetName 验证按发布命名约定构造的资产名与 build_release.go/
// installer.iss 产物一致（Windows setup 带 v 前缀，其余平台剥 v）。
func TestDesktopAssetName(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         string
	}{
		{"windows", "amd64", "eos-app-setup-v1.0.0-beta.17.exe"},
		{"darwin", "arm64", "eos-app_1.0.0-beta.17_darwin-arm64.dmg"},
		{"darwin", "amd64", "eos-app_1.0.0-beta.17_darwin-amd64.dmg"},
		{"linux", "amd64", "eos-app_1.0.0-beta.17_linux-amd64.tar.gz"},
		{"linux", "arm64", "eos-app_1.0.0-beta.17_linux-arm64.tar.gz"},
	}
	for _, c := range cases {
		got, ok := desktopAssetName("v1.0.0-beta.17", c.goos, c.goarch)
		if !ok || got != c.want {
			t.Errorf("desktopAssetName(%s,%s) = %q/%v, want %q", c.goos, c.goarch, got, ok, c.want)
		}
	}
	if _, ok := desktopAssetName("v1.0.0", "wasm", "wasm32"); ok {
		t.Errorf("未知平台应返回 available=false")
	}
}

// TestAssetDigestFromChecksums 验证 sha256sum 格式清单解析（双空格分隔、
// 大小写不回写，命中资产名返回小写 hex）。
func TestAssetDigestFromChecksums(t *testing.T) {
	sums := []byte("0123456789abcdef  eos-app-setup-v1.0.0.exe\n" +
		"abcdef0123456789  eos-app_1.0.0_linux-amd64.tar.gz\n")
	if got := assetDigestFromChecksums(sums, "eos-app-setup-v1.0.0.exe"); got != "0123456789abcdef" {
		t.Errorf("digest = %q, want 0123456789abcdef", got)
	}
	if got := assetDigestFromChecksums(sums, "eos-app_1.0.0_linux-amd64.tar.gz"); got != "abcdef0123456789" {
		t.Errorf("digest = %q, want abcdef0123456789", got)
	}
	if got := assetDigestFromChecksums(sums, "missing.exe"); got != "" {
		t.Errorf("未列入清单的资产应返回空串, got %q", got)
	}
}

// TestTagFromRedirect_ParsesRedirectTag 验证从 302 Location 解析 tag（含尾斜杠）。
func TestTagFromRedirect_ParsesRedirectTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r,
			"https://github.com/eosaios/eos-app/releases/tag/v1.2.3/",
			http.StatusFound)
	}))
	defer srv.Close()

	trap := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := trap.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := tagFromRedirect(resp)
	if err != nil {
		t.Fatalf("tagFromRedirect: %v", err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("tag = %q, want v1.2.3", tag)
	}
}

// TestTagFromRedirect_NonRedirectError 验证非 3xx 响应直接报错（终态，不重试）。
func TestTagFromRedirect_NonRedirectError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tagFromRedirect(resp)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got %v", err)
	}
}

// TestFetchReleaseTag_RetriesTransientNetworkErrors 验证瞬断（连接被重置）
// 时按退避重试并最终成功。
func TestFetchReleaseTag_RetriesTransientNetworkErrors(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			panic(http.ErrAbortHandler) // 模拟瞬断：连接被服务端中断
		}
		http.Redirect(w, r,
			"https://github.com/eosaios/eos-app/releases/tag/v2.0.0",
			http.StatusFound)
	}))
	defer srv.Close()

	tag, err := fetchReleaseTag(contextBackground(), srv.URL, nil)
	if err != nil {
		t.Fatalf("fetchReleaseTag: %v", err)
	}
	if tag != "v2.0.0" {
		t.Fatalf("tag = %q, want v2.0.0", tag)
	}
	if attempts < 2 {
		t.Fatalf("expected retry after transient failure, attempts = %d", attempts)
	}
}
