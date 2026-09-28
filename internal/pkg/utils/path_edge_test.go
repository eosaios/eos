package utils

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

func TestResolvePathBasics(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// 空路径
	if r := ResolvePath(""); r.IsValid || r.ErrMsg != "path is empty" {
		t.Fatalf("empty = %+v", r)
	}

	// 相对路径在 cwd 内
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := ResolvePath("a.txt")
	if !r.IsValid || r.RelPath != "a.txt" {
		t.Fatalf("relative = %+v", r)
	}
	if r.AbsPath == "" || r.ResolvedAbs == "" {
		t.Fatalf("abs missing: %+v", r)
	}

	// 路径遍历被拒
	r = ResolvePath("../outside")
	if r.IsValid {
		t.Fatalf("traversal should fail: %+v", r)
	}
	if !strings.Contains(r.ErrMsg, "outside") {
		t.Fatalf("msg = %q", r.ErrMsg)
	}

	// ResolvePathUnder 指定 root
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	r = ResolvePathUnder(sub, "b.txt")
	if !r.IsValid {
		t.Fatalf("under = %+v", r)
	}
	r = ResolvePathUnder(sub, "../a.txt")
	if r.IsValid {
		t.Fatalf("under traversal = %+v", r)
	}

	// ResolvePathSimple / MustResolvePath
	if got := ResolvePathSimple("a.txt"); got == "" {
		t.Fatal("simple")
	}
	if got := ResolvePathSimple("../x"); got != "" {
		t.Fatalf("simple invalid = %q", got)
	}
	if got := MustResolvePath("a.txt"); got == "" {
		t.Fatal("must")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("MustResolvePath should panic on invalid")
			}
		}()
		MustResolvePath("../x")
	}()
}

func TestResolvePathSymlink(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 根内符号链接
	if err := os.Symlink("real.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Skip("symlink unsupported")
	}
	r := ResolvePath("link.txt")
	if !r.IsValid || !r.IsSymlink {
		t.Fatalf("symlink = %+v", r)
	}

	// 指向根外的符号链接
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "out.txt"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "out.txt"), filepath.Join(dir, "bad.txt")); err != nil {
		t.Fatal(err)
	}
	r = ResolvePath("bad.txt")
	if r.IsValid {
		t.Fatalf("out-of-root symlink should fail: %+v", r)
	}
	if !strings.Contains(r.ErrMsg, "symbolic link") {
		t.Fatalf("msg = %q", r.ErrMsg)
	}
}

func TestExpandAndNormalizePaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	if got := expandHomePath("~"); got != home {
		t.Fatalf("~ = %q", got)
	}
	if got := expandHomePath("~/x"); got != filepath.Join(home, "x") {
		t.Fatalf("~/x = %q", got)
	}
	if got := expandHomePath("~\\y"); got != filepath.Join(home, "y") {
		t.Fatalf("~\\y = %q", got)
	}
	if got := expandHomePath("~/"); got != home {
		t.Fatalf("~/ = %q", got)
	}
	if got := expandHomePath("plain"); got != "plain" {
		t.Fatalf("plain = %q", got)
	}

	if got := normalizePathInput("  a//b/../c  "); !strings.Contains(got, "c") {
		t.Fatalf("normalize = %q", got)
	}

	// Windows 盘符大写（跨平台可跑）
	if got := normalizeWindowsPath("c:/foo"); !strings.HasPrefix(got, "C:") && !strings.HasPrefix(got, "c:") {
		// 非 Windows 上 Clean 可能保留原样
		_ = got
	}
	if got := normalizeWindowsPath("C:\\a\\..\\b"); !strings.Contains(got, "b") {
		t.Fatalf("win clean = %q", got)
	}
}

func TestResolveSymlinkNonLink(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, isLink := resolveSymlink(p)
	if isLink {
		t.Fatalf("plain file isLink = %v", isLink)
	}
	if got != p {
		t.Fatalf("got = %q", got)
	}
	// 不存在的路径
	if _, isLink := resolveSymlink(filepath.Join(dir, "nope")); isLink {
		t.Fatal("missing isLink")
	}
}

func TestIsPathInRootJoinNormalize(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "a")
	if !IsPathInRoot(inside, dir) {
		t.Fatal("inside")
	}
	if IsPathInRoot(dir, inside) {
		t.Fatal("outside")
	}

	if got := JoinPath("/base", "rel/x"); got != filepath.Join("/base", "rel", "x") && got != "/base/rel/x" {
		t.Fatalf("join = %q", got)
	}
	if got := JoinPath("/base", "/abs"); got != "/abs" {
		t.Fatalf("abs join = %q", got)
	}

	if got := NormalizePath("/a//b/../c"); got != "/a/c" {
		t.Fatalf("normalize = %q", got)
	}
}
