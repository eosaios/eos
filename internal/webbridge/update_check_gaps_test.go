package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// update_check 余臂批测：代理客户端矩阵、代理配置读取三态、重定向解析
// 终态错误、清单解析三态、httptest 代理打通 checkGitHubLatest 全链、
// 缓存 TTL 复用。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateHTTPClientMatrix(t *testing.T) {
	// 空 → nil,nil（默认客户端直连）。
	if c, err := updateHTTPClient("  "); c != nil || err != nil {
		t.Fatalf("空代理 = %v, %v", c, err)
	}
	for _, bad := range []string{
		"://bad",            // 解析失败
		"ftp://proxy.local", // 非 http/https
		"http://",           // 缺主机
	} {
		if _, err := updateHTTPClient(bad); err == nil {
			t.Fatalf("非法代理 %q 应报错", bad)
		}
	}
	c, err := updateHTTPClient("http://127.0.0.1:1")
	if err != nil || c == nil || c.Transport == nil {
		t.Fatalf("合法代理 = %v, %v", c, err)
	}
	if err := validateUpdateProxyURL("http://ok:1"); err != nil {
		t.Fatalf("校验合法地址 = %v", err)
	}
}

func TestUpdateProxyRawStates(t *testing.T) {
	// nil receiver 直连态。
	var nilBridge *BridgeService
	if on, u, err := nilBridge.updateProxyRaw(); on || u != "" || err != nil {
		t.Fatalf("nil bridge = %v,%q,%v", on, u, err)
	}
	// 配置路径指向目录（读失败）→ 错误臂。
	s := &BridgeService{runtimeGateway: &chatSessionsGatewayStub{
		configPathOverride: t.TempDir(), // 目录必读失败
	}}
	if _, _, err := s.updateProxyRaw(); err == nil {
		t.Fatal("配置不可读应报错")
	}
	// 正常配置：开关+地址读取。
	cfg := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfg, []byte(`{"update_proxy_enabled": true, "update_proxy_url": "http://p"}`), 0o644)
	s2 := &BridgeService{runtimeGateway: &chatSessionsGatewayStub{configPathOverride: cfg}}
	if on, u, err := s2.updateProxyRaw(); err != nil || !on || u != "http://p" {
		t.Fatalf("正常配置 = %v,%q,%v", on, u, err)
	}
}

func TestFetchReleaseTagRedirectArms(t *testing.T) {
	newSrv := func(handler http.HandlerFunc) *httptest.Server {
		return httptest.NewServer(handler)
	}
	// 302 带 tag → 解析成功。
	srv := newSrv(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://github.com/eosaios/eos-app/releases/tag/v9.9.9/")
		w.WriteHeader(http.StatusFound)
	})
	defer srv.Close()
	tag, err := fetchReleaseTag(contextBackground(), srv.URL, srv.Client())
	if err != nil || tag != "v9.9.9" {
		t.Fatalf("302 解析 = %q, %v", tag, err)
	}

	// 200 无重定向 → 终态错误（无重试睡眠）。
	srv2 := newSrv(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("rate limited page"))
	})
	defer srv2.Close()
	if _, err := fetchReleaseTag(contextBackground(), srv2.URL, srv2.Client()); err == nil ||
		!strings.Contains(err.Error(), "未返回版本重定向") {
		t.Fatalf("无重定向终态 = %v", err)
	}

	// Location 缺 /tag/ 标记。
	srv3 := newSrv(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.com/other")
		w.WriteHeader(http.StatusFound)
	})
	defer srv3.Close()
	if _, err := fetchReleaseTag(contextBackground(), srv3.URL, srv3.Client()); err == nil {
		t.Fatal("缺标记应报错")
	}

	// Location 空版本号。
	srv4 := newSrv(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://github.com/o/r/releases/tag/")
		w.WriteHeader(http.StatusFound)
	})
	defer srv4.Close()
	if _, err := fetchReleaseTag(contextBackground(), srv4.URL, srv4.Client()); err == nil ||
		!strings.Contains(err.Error(), "缺少版本号") {
		t.Fatalf("空版本 = %v", err)
	}
}

func TestFetchAssetDigestArms(t *testing.T) {
	sums := "aaaa1111bbbb2222  eos-app_1.0.0_darwin-arm64.dmg\ncccc3333  other\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	}))
	defer srv.Close()
	got, err := fetchAssetDigest(contextBackground(), srv.URL, "eos-app_1.0.0_darwin-arm64.dmg", srv.Client())
	if err != nil || got != "aaaa1111bbbb2222" {
		t.Fatalf("命中条目 = %q, %v", got, err)
	}
	if _, err := fetchAssetDigest(contextBackground(), srv.URL, "missing.dmg", srv.Client()); err == nil ||
		!strings.Contains(err.Error(), "未列入") {
		t.Fatalf("缺条目 = %v", err)
	}

	// 非 200 终态。
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv2.Close()
	if _, err := fetchAssetDigest(contextBackground(), srv2.URL, "x", srv2.Client()); err == nil ||
		!strings.Contains(err.Error(), "403") {
		t.Fatalf("非 200 = %v", err)
	}
}

func TestCheckGitHubLatestEarlyAndProxyArms(t *testing.T) {
	origVersion := BuildVersion
	defer func() { BuildVersion = origVersion }()

	// dev 版早期返回（不打网络）。
	BuildVersion = "dev"
	res, err := checkGitHubLatest("")
	if err != nil || res.HasUpdate {
		t.Fatalf("dev 早期返回 = %+v, %v", res, err)
	}

	// 非法代理地址：构造期 fail-fast（不打网络）。
	BuildVersion = "0.1.0"
	if _, err := checkGitHubLatest("ftp://bad"); err == nil {
		t.Fatal("非法代理应报错")
	}
	// 注：checkGitHubLatest 中段（重定向解析/资产拼接/清单拉取）目标 URL
	// 硬编码 https://github.com 且 https 过代理走 CONNECT 隧道，本地
	// httptest 无法中间人——归网络绑定豁免；各子函数已单独覆盖。
}

func TestCheckForUpdatesCacheAndErrorArms(t *testing.T) {
	// 隔离包级缓存。
	withUpdateCache(t, nil)
	t.Cleanup(func() { withUpdateCache(t, nil) })

	// dev 版：早期返回且写缓存 → 第二次命中缓存（无网络）。
	origVersion := BuildVersion
	defer func() { BuildVersion = origVersion }()
	BuildVersion = "dev"
	s := &BridgeService{runtimeGateway: &chatSessionsGatewayStub{}}
	first := s.CheckForUpdates()
	if first.Error != "" {
		t.Fatalf("dev 检查 = %+v", first)
	}
	second := s.CheckForUpdates()
	if second.LatestVersion != first.LatestVersion || second.CheckedAt != first.CheckedAt {
		t.Fatalf("缓存未复用: %+v vs %+v", first, second)
	}

	// github 错误臂：代理返回 200 无重定向 → 终态错误、不缓存。
	withUpdateCache(t, nil)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("blocked"))
	}))
	defer proxy.Close()
	cfg := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfg, []byte(`{"update_proxy_enabled": true, "update_proxy_url": "`+proxy.URL+`"}`), 0o644)
	s2 := &BridgeService{runtimeGateway: &chatSessionsGatewayStub{configPathOverride: cfg}}
	BuildVersion = "0.1.0"
	res := s2.CheckForUpdates()
	if res.Error == "" || res.HasUpdate {
		t.Fatalf("错误臂 = %+v", res)
	}
	// 失败不缓存：缓存指针仍空。
	updateCacheMu.Lock()
	cachedNil := cachedUpdateResult == nil
	updateCacheMu.Unlock()
	if !cachedNil {
		t.Fatal("失败结果不应入缓存")
	}

	// 代理配置错误臂：开关开启但地址非法。
	cfgBad := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfgBad, []byte(`{"update_proxy_enabled": true, "update_proxy_url": "ftp://bad"}`), 0o644)
	s3 := &BridgeService{runtimeGateway: &chatSessionsGatewayStub{configPathOverride: cfgBad}}
	res = s3.CheckForUpdates()
	if res.Error == "" {
		t.Fatal("非法代理配置应报错")
	}
}

func TestTrimTagPrefixAndAssetName(t *testing.T) {
	if got := trimTagPrefix(" v1.2.3 "); got != "1.2.3" {
		t.Fatalf("trimTagPrefix = %q", got)
	}
	// Windows setup 资产名保留 v 前缀（与其发布命名约定一致）。
	name, ok := desktopAssetName("v1.2.3", "windows", "amd64")
	if !ok || name != "eos-app-setup-v1.2.3.exe" {
		t.Fatalf("windows 资产 = %q", name)
	}
	if _, ok := desktopAssetName("v1", "plan9", "amd64"); ok {
		t.Fatal("未知平台应不可用")
	}
}
