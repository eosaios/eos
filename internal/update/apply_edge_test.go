package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleRootDir(t *testing.T) {
	dir := t.TempDir()
	// 空目录 → 自身
	got, err := singleRootDir(dir)
	if err != nil || got != dir {
		t.Fatalf("empty = %q %v", got, err)
	}
	// 单一子目录 → 该子目录
	sub := filepath.Join(dir, "pkg")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = singleRootDir(dir)
	if err != nil || got != sub {
		t.Fatalf("single dir = %q %v", got, err)
	}
	// 两个条目 → 自身
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = singleRootDir(dir)
	if err != nil || got != dir {
		t.Fatalf("multi = %q %v", got, err)
	}
	// 不存在
	if _, err := singleRootDir(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("missing")
	}
}

func TestCopyDirAndSwapCoreDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(filepath.Join(src, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a", "b", "c.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "a", "b", "c.txt"))
	if err != nil || string(data) != "hello" {
		t.Fatalf("copied = %q %v", data, err)
	}

	// swapCoreDir 新目标
	stage := t.TempDir()
	target := filepath.Join(t.TempDir(), "core")
	if err := os.WriteFile(filepath.Join(stage, "eos-core"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := swapCoreDir(stage, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "eos-core")); err != nil {
		t.Fatal(err)
	}
	// 二次 swap 旋转旧目录
	stage2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(stage2, "eos-core"), []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := swapCoreDir(stage2, target); err != nil {
		t.Fatal(err)
	}
	// cleanupStaleCores 清理 core.old-*
	old := filepath.Join(filepath.Dir(target), "core.old-123")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	cleanupStaleCores(filepath.Dir(target))
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("stale dir remains: %v", err)
	}
	// 缺失目录不 panic
	cleanupStaleCores(filepath.Join(t.TempDir(), "nope"))
}

func TestReplaceBinaryAndCurrentExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "app")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	newBin := filepath.Join(dir, "new")
	if err := os.WriteFile(newBin, []byte("newbin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(newBin, exe); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil || string(data) != "newbin" {
		t.Fatalf("replaced = %q %v", data, err)
	}

	// 缺失源
	if err := replaceBinary(filepath.Join(dir, "nope"), exe); err == nil {
		t.Fatal("missing source")
	}

	if _, err := currentExecutable(); err != nil {
		t.Fatalf("currentExecutable = %v", err)
	}
}

func TestProgressReaderAndLatestReleasesURL(t *testing.T) {
	var got int64
	r := &progressReader{r: strings.NewReader("hello"), total: 5, fn: func(done, total int64) {
		got = done
	}}
	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if err != nil || n != 5 || got != 5 {
		t.Fatalf("read = %d %v got=%d", n, err, got)
	}
	if !strings.Contains(latestReleasesPageURL(), "releases/latest") {
		t.Fatalf("url = %q", latestReleasesPageURL())
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(archive, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sums := filepath.Join(dir, "SHA256SUMS.txt")
	// 正确 checksum
	if err := os.WriteFile(sums, []byte("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824  a.bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(archive, sums, "a.bin"); err != nil {
		t.Fatal(err)
	}

	// 错误 checksum
	if err := os.WriteFile(sums, []byte("deadbeef  a.bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(archive, sums, "a.bin"); err == nil {
		t.Fatal("mismatch")
	}

	// 未列出
	if err := verifyChecksum(archive, sums, "other.bin"); err == nil {
		t.Fatal("not listed")
	}

	// 缺失 sums
	if err := verifyChecksum(archive, filepath.Join(dir, "nope"), "a.bin"); err == nil {
		t.Fatal("missing sums")
	}
}

func TestExtractArchive(t *testing.T) {
	dir := t.TempDir()
	// 非法 zip
	bad := filepath.Join(dir, "bad.zip")
	if err := os.WriteFile(bad, []byte("not zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(bad, dir); err == nil {
		t.Fatal("bad zip")
	}
	// 非法 tar.gz
	badtgz := filepath.Join(dir, "bad.tar.gz")
	if err := os.WriteFile(badtgz, []byte("not gz"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(badtgz, dir); err == nil {
		t.Fatal("bad tgz")
	}
	// extractArchive 按扩展名分发
	if err := extractArchive(bad, dir); err == nil {
		t.Fatal("zip dispatch")
	}
	if err := extractArchive(badtgz, dir); err == nil {
		t.Fatal("tgx dispatch")
	}
}
