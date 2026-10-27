package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestApplyEndToEnd 用本地构造的 tar.gz 归档走完 Apply 全流程：
// 校验清单 → 解压 → 替换二进制 → 同步 core/ → 清理残留。
// Windows 下的 .zip 分支由 TestApplyZipLayout 覆盖（构造 zip 走 extract 部分）。
func TestApplyEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows rename-in-place semantics differ; core swap logic covered by extract test")
	}

	tmp := t.TempDir()
	installDir := filepath.Join(tmp, "install")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 伪造当前安装：eos 二进制 + 旧 core
	exePath := filepath.Join(installDir, "eos")
	if err := os.WriteFile(exePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldCore := filepath.Join(installDir, "core", "x86_64-unknown-linux-musl")
	os.MkdirAll(oldCore, 0o755)
	if err := os.WriteFile(filepath.Join(oldCore, "eos-core"), []byte("old-core"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 构造新版归档：eos-cli_v9.9.9/eos + core/<triple>/eos-core
	archivePath, assetName, wantSum := buildTestArchive(t, tmp, "eos", "eos-cli_v9.9.9_test")
	sumsPath := filepath.Join(tmp, "SHA256SUMS.txt")
	if err := os.WriteFile(sumsPath, []byte(fmt.Sprintf("%s  %s\n", wantSum, assetName)), 0o644); err != nil {
		t.Fatal(err)
	}

	// 校验清单通过
	if err := verifyChecksum(archivePath, sumsPath, assetName); err != nil {
		t.Fatalf("verifyChecksum: %v", err)
	}
	// 篡改清单应失败
	badSums := filepath.Join(tmp, "bad.txt")
	os.WriteFile(badSums, []byte(strings.Repeat("0", 64)+"  "+assetName+"\n"), 0o644)
	if err := verifyChecksum(archivePath, badSums, assetName); err == nil {
		t.Fatal("checksum mismatch must fail")
	}

	// 解压 + 替换流程（Apply 的本地部分）
	extractDir := filepath.Join(tmp, "extract")
	if err := extractArchive(archivePath, extractDir); err != nil {
		t.Fatal(err)
	}
	stageRoot, err := singleRootDir(extractDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(stageRoot) != "eos-cli_v9.9.9_test" {
		t.Fatalf("stageRoot = %q, want wrapped dir", stageRoot)
	}
	if err := replaceBinary(filepath.Join(stageRoot, "eos"), exePath); err != nil {
		t.Fatal(err)
	}
	if err := swapCoreDir(filepath.Join(stageRoot, "core"), filepath.Join(installDir, "core")); err != nil {
		t.Fatal(err)
	}
	cleanupStaleCores(installDir)

	got, _ := os.ReadFile(exePath)
	if string(got) != "new-binary" {
		t.Fatalf("binary not replaced: %q", got)
	}
	newCore, err := os.ReadFile(filepath.Join(installDir, "core", "aarch64-test-triple", "eos-core"))
	if err != nil || string(newCore) != "new-core" {
		t.Fatalf("core not replaced: %v %q", err, newCore)
	}
	// 旧 core 残留目录应被清理
	entries, _ := os.ReadDir(installDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "core.old-") {
			t.Fatalf("stale core dir remains: %s", e.Name())
		}
	}
}

func TestApplyZipLayout(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "test.zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	files := map[string]string{
		"eos-cli_v9.9.9_win/eos.exe":             "new-binary",
		"eos-cli_v9.9.9_win/core/t/eos-core.exe": "new-core",
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()
	zf.Close()

	out := filepath.Join(tmp, "extract")
	if err := extractArchive(zipPath, out); err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

// TestExtractZipBackslashEntries 回归：Windows PowerShell Compress-Archive
// 历史产物以反斜杠写 zip 条目（eos-...\core\），解压器只按 "/" 识别目录
// 后缀（zip.FileInfo().IsDir()），不归一化会把目录条目当文件、解压失败
// （eos update 实测报「mkdir ...\core: The system cannot find the path」）。
func TestExtractZipBackslashEntries(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "backslash.zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	// 模拟 Compress-Archive 产物：条目名用反斜杠，目录条目带尾 "\"。
	entries := []struct{ name, content string }{
		{"eos-cli_v9.9.9_win\\core\\", ""},
		{"eos-cli_v9.9.9_win\\eos.exe", "new-binary"},
		{"eos-cli_v9.9.9_win\\core\\triple\\eos-core.exe", "new-core"},
	}
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()
	zf.Close()

	out := filepath.Join(tmp, "extract")
	if err := extractArchive(zipPath, out); err != nil {
		t.Fatalf("extractArchive backslash zip error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "eos-cli_v9.9.9_win", "eos.exe")); err != nil {
		t.Fatalf("eos.exe missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "eos-cli_v9.9.9_win", "core", "triple", "eos-core.exe")); err != nil {
		t.Fatalf("core binary missing: %v", err)
	}
}

func TestSafeJoin(t *testing.T) {
	if _, err := safeJoin("/tmp/x", "../escape.txt"); err == nil {
		t.Error("path traversal must be rejected")
	}
	if _, err := safeJoin("/tmp/x", "/abs.txt"); err == nil {
		t.Error("absolute path must be rejected")
	}
	if _, err := safeJoin("/tmp/x", "core/t/eos-core"); err != nil {
		t.Errorf("normal path must be accepted: %v", err)
	}
}

func buildTestArchive(t *testing.T, dir, binaryName, rootName string) (path, name, sum string) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	files := []struct {
		name, content string
		mode          int64
	}{
		{rootName + "/" + binaryName, "new-binary", 0o755},
		{rootName + "/core/aarch64-test-triple/eos-core", "new-core", 0o755},
		{rootName + "/core/aarch64-test-triple/manifest.json", "{}", 0o644},
	}
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: f.name, Mode: f.mode, Size: int64(len(f.content)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gw.Close()

	name = rootName + ".tar.gz"
	path = filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(buf.Bytes())
	return path, name, hex.EncodeToString(h[:])
}
