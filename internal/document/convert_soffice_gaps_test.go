package document

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// convert.go 余臂批测：同格式复制的 IO 错误族、高保真链（PATH 前置假
// soffice 拦截：成功/失败/产物改名搬运/缺二进制三态）。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertSameFormatIOErrors(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	os.WriteFile(src, []byte("x"), 0o644)

	// 源不可读。
	if _, err := Convert(filepath.Join(dir, "missing.txt"), ConversionOptions{TargetFormat: "txt"}); err == nil {
		t.Fatal("缺源应报错")
	}
	// 目标目录建在文件之下。
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("f"), 0o644)
	if _, err := Convert(src, ConversionOptions{
		TargetFormat: "txt", DestinationPath: filepath.Join(blocker, "sub", "a.txt"),
	}); err == nil {
		t.Fatal("目录被文件占位应报错")
	}
	// 目标写入失败：目录本身作目标。
	if _, err := Convert(src, ConversionOptions{TargetFormat: "txt", DestinationPath: dir}); err == nil {
		t.Fatal("目录作目标应报错")
	}
}

// newFakeSoffice 写一个假 soffice：按 --outdir 与源文件推演产物名并落盘。
// mode=fail 直接退 1 带 stderr。
func newFakeSoffice(t *testing.T, mode string) string {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
outdir=""
src=""
fmt=""
prev=""
for arg in "$@"; do
  case "$prev" in
    --outdir) outdir="$arg" ;;
    --convert-to) fmt="$arg" ;;
  esac
  prev="$arg"
  case "$arg" in
    /*|./*) src="$arg" ;;
  esac
done
if [ "` + mode + `" = "fail" ]; then
  echo "soffice boom" >&2
  exit 1
fi
base=$(basename "$src")
name="${base%.*}"
printf 'fake-converted' > "$outdir/$name.$fmt"
echo "convert ok"
`
	path := filepath.Join(bin, "soffice")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func withPath(t *testing.T, dir string) {
	t.Helper()
	if dir == "" {
		t.Setenv("PATH", t.TempDir()) // 空 PATH：soffice 缺失
		return
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestConvertHighFidelityWithFakeSoffice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.docx")
	if err := WriteDOCX(src, DocumentFromText("标题", "内容")); err != nil {
		t.Fatal(err)
	}

	t.Run("soffice 缺失", func(t *testing.T) {
		withPath(t, "")
		_, err := Convert(src, ConversionOptions{TargetFormat: "pdf", Fidelity: "high"})
		if err == nil || !strings.Contains(err.Error(), "soffice not found") {
			t.Fatalf("缺失 = %v", err)
		}
	})

	t.Run("soffice 失败透传", func(t *testing.T) {
		withPath(t, newFakeSoffice(t, "fail"))
		_, err := Convert(src, ConversionOptions{TargetFormat: "pdf", Fidelity: "high"})
		if err == nil || !strings.Contains(err.Error(), "high-fidelity conversion failed") ||
			!strings.Contains(err.Error(), "soffice boom") {
			t.Fatalf("失败 = %v", err)
		}
	})

	t.Run("成功与产物改名搬运", func(t *testing.T) {
		withPath(t, newFakeSoffice(t, "ok"))
		// 目标名与源名不同：触发 produced != destination 搬运臂。
		dst := filepath.Join(dir, "renamed.pdf")
		res, err := Convert(src, ConversionOptions{TargetFormat: "pdf", Fidelity: "high", DestinationPath: dst})
		if err != nil {
			t.Fatalf("成功链 = %v", err)
		}
		if res.UsedEngine != "soffice" {
			t.Fatalf("engine = %q", res.UsedEngine)
		}
		data, err := os.ReadFile(dst)
		if err != nil || string(data) != "fake-converted" {
			t.Fatalf("搬运产物 = %q, %v", data, err)
		}
		// 原产物（源名.pdf）应被清理。
		if _, err := os.Stat(filepath.Join(dir, "src.pdf")); !os.IsNotExist(err) {
			t.Fatal("原产物应清理")
		}
		if len(res.Warnings) == 0 {
			t.Fatal("soffice 输出应作 warning")
		}
	})

	t.Run("产物即目标直落", func(t *testing.T) {
		withPath(t, newFakeSoffice(t, "ok"))
		dst := filepath.Join(dir, "src.pdf")
		if _, err := os.Stat(dst); err == nil {
			os.Remove(dst)
		}
		res, err := Convert(src, ConversionOptions{TargetFormat: "pdf", Fidelity: "high", DestinationPath: dst})
		if err != nil {
			t.Fatalf("直落 = %v", err)
		}
		if res.UsedEngine != "soffice" {
			t.Fatalf("engine = %q", res.UsedEngine)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Fatalf("直落产物: %v", err)
		}
	})
}
