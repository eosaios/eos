package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// ApplyWithClient 编排壳批测：只覆盖 replaceBinary 之前的安全错误臂——
// 成功链会替换当前运行中的可执行文件，本文件绝不触达（本地替换语义已由
// TestApplyEndToEnd 直接串底层函数覆盖）。

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyWithClientGuardArms(t *testing.T) {
	t.Run("无可下载资产拒绝", func(t *testing.T) {
		if _, err := ApplyWithClient(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no downloadable asset") {
			t.Fatalf("Apply(nil) error = %v", err)
		}
		if _, err := ApplyWithClient(context.Background(), &CheckResult{}, nil, nil); err == nil {
			t.Fatal("Apply(empty result) error = nil")
		}
		if _, err := ApplyWithClient(context.Background(), &CheckResult{DownloadURL: "http://x/y.tgz"}, nil, nil); err == nil {
			t.Fatal("Apply(missing asset name) error = nil")
		}
	})

	t.Run("下载失败拒绝", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusNotFound)
		}))
		defer server.Close()
		res := &CheckResult{DownloadURL: server.URL + "/eos.tgz", AssetName: "eos.tgz", ChecksumURL: server.URL + "/SHA256SUMS.txt"}
		if _, err := ApplyWithClient(context.Background(), res, nil, server.Client()); err == nil || !strings.Contains(err.Error(), "download eos.tgz") {
			t.Fatalf("download failure error = %v", err)
		}
	})

	t.Run("缺校验清单拒绝", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("archive-bytes"))
		}))
		defer server.Close()
		res := &CheckResult{DownloadURL: server.URL + "/eos.tgz", AssetName: "eos.tgz", ChecksumURL: ""}
		if _, err := ApplyWithClient(context.Background(), res, nil, server.Client()); err == nil || !strings.Contains(err.Error(), "SHA256SUMS.txt") {
			t.Fatalf("missing sums error = %v", err)
		}
	})

	t.Run("校验和不匹配拒绝", func(t *testing.T) {
		var sums []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "SHA256SUMS.txt") {
				_, _ = w.Write(sums)
				return
			}
			_, _ = w.Write([]byte("archive-bytes"))
		}))
		defer server.Close()
		badSum := strings.Repeat("0", 64)
		sums = []byte(fmt.Sprintf("%s  eos.tgz\n", badSum))
		res := &CheckResult{DownloadURL: server.URL + "/eos.tgz", AssetName: "eos.tgz", ChecksumURL: server.URL + "/SHA256SUMS.txt"}
		if _, err := ApplyWithClient(context.Background(), res, nil, server.Client()); err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatalf("checksum mismatch error = %v", err)
		}
	})

	t.Run("坏归档拒绝", func(t *testing.T) {
		archive := []byte("not-a-tarball")
		sum := sha256.Sum256(archive)
		sums := fmt.Sprintf("%s  eos.tgz\n", hex.EncodeToString(sum[:]))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "SHA256SUMS.txt") {
				_, _ = w.Write([]byte(sums))
				return
			}
			_, _ = w.Write(archive)
		}))
		defer server.Close()
		res := &CheckResult{DownloadURL: server.URL + "/eos.tgz", AssetName: "eos.tgz", ChecksumURL: server.URL + "/SHA256SUMS.txt"}
		if _, err := ApplyWithClient(context.Background(), res, nil, server.Client()); err == nil || !strings.Contains(err.Error(), "extract") {
			t.Fatalf("bad archive error = %v", err)
		}
	})

	t.Run("归档缺二进制拒绝", func(t *testing.T) {
		// 合法 tar.gz，但没有 eos 二进制（只有 README）。
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		if err := tw.WriteHeader(&tar.Header{Name: "pkg/README", Mode: 0o644, Size: 6}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("hello!")); err != nil {
			t.Fatal(err)
		}
		tw.Close()
		gw.Close()
		archive := buf.Bytes()
		sum := sha256.Sum256(archive)
		sums := fmt.Sprintf("%s  eos.tgz\n", hex.EncodeToString(sum[:]))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "SHA256SUMS.txt") {
				_, _ = w.Write([]byte(sums))
				return
			}
			_, _ = w.Write(archive)
		}))
		defer server.Close()
		res := &CheckResult{DownloadURL: server.URL + "/eos.tgz", AssetName: "eos.tgz", ChecksumURL: server.URL + "/SHA256SUMS.txt"}
		if _, err := ApplyWithClient(context.Background(), res, nil, server.Client()); err == nil || !strings.Contains(err.Error(), "not found in archive") {
			t.Fatalf("missing binary error = %v", err)
		}
	})
}

func TestBuildCheckResultAndPlatformAsset(t *testing.T) {
	result := buildCheckResult("v1.0.0", "v2.3.4", "linux", "amd64")
	if !result.HasUpdate || result.CurrentVersion != "v1.0.0" || result.LatestVersion != "v2.3.4" {
		t.Fatalf("result = %+v", result)
	}
	if result.AssetName != "eos-cli_v2.3.4_linux-amd64.tar.gz" {
		t.Fatalf("asset name = %q", result.AssetName)
	}
	if !strings.HasSuffix(result.DownloadURL, "/v2.3.4/eos-cli_v2.3.4_linux-amd64.tar.gz") {
		t.Fatalf("download url = %q", result.DownloadURL)
	}
	if !strings.HasSuffix(result.ChecksumURL, "/v2.3.4/SHA256SUMS.txt") {
		t.Fatalf("checksum url = %q", result.ChecksumURL)
	}

	winName, winSuffix := platformAssetName(" v3.0.0 ", "windows", "arm64")
	if winName != "eos-cli_v3.0.0_windows-arm64.zip" || winSuffix != ".zip" {
		t.Fatalf("windows asset = %q %q", winName, winSuffix)
	}
	if newer := buildCheckResult("v9.9.9", "v1.0.0", "linux", "amd64"); newer.HasUpdate {
		t.Fatal("older latest must not flag HasUpdate")
	}
}

func TestTagFromRedirectTerminalArms(t *testing.T) {
	// Location 缺 /tag/ 标记 → 终态错误（含状态码与响应体摘要）。
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("rate limited")),
	}
	if _, err := tagFromRedirect(resp); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("non-redirect error = %v", err)
	}
	// Location 以 /tag/ 结尾无版本号 → 错误。
	resp2 := &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{"https://github.com/eosaios/eos/releases/tag/"}},
		Body:       io.NopCloser(strings.NewReader("")),
	}
	if _, err := tagFromRedirect(resp2); err == nil || !strings.Contains(err.Error(), "缺少版本号") {
		t.Fatalf("empty tag error = %v", err)
	}
}

// ---- 归档布局补充：zip 平台包 ----

func TestExtractZipNormalizesParentPaths(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// Windows 归档常见反斜杠路径 + ./ 前缀。
	for name, content := range map[string]string{
		`./pkg/eos`:       "bin",
		`pkg\core\m.json`: "{}",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := extractArchive(archive, out); err != nil {
		t.Fatalf("extractArchive error = %v", err)
	}
	for _, rel := range []string{filepath.Join("pkg", "eos"), filepath.Join("pkg", "core", "m.json")} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("extracted %s missing: %v", rel, err)
		}
	}
}
