package filedialog

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestIsUnavailableAndCanceled(t *testing.T) {
	if !IsUnavailable(ErrUnavailable) {
		t.Fatal("unavailable")
	}
	if !IsCanceled(ErrCanceled) {
		t.Fatal("canceled")
	}
	if IsUnavailable(ErrCanceled) || IsCanceled(ErrUnavailable) {
		t.Fatal("cross")
	}
	wrapped := errors.Join(ErrUnavailable, errors.New("x"))
	if !IsUnavailable(wrapped) {
		t.Fatal("wrapped")
	}
}

func TestNormalizeDirectory(t *testing.T) {
	if _, err := normalizeDirectory("  "); !errors.Is(err, ErrCanceled) {
		t.Fatalf("blank = %v", err)
	}
	if _, err := normalizeDirectory("\r\n"); !errors.Is(err, ErrCanceled) {
		t.Fatal("crlf blank")
	}
	got, err := normalizeDirectory("  ./rel  \n")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("not abs = %q", got)
	}
}

func TestLookPathAny(t *testing.T) {
	if _, ok := lookPathAny("", "   "); ok {
		t.Fatal("blank names")
	}
	if _, ok := lookPathAny("definitely-not-a-binary-xyz"); ok {
		t.Fatal("missing binary")
	}
	// sh 应存在于 unix
	if _, ok := lookPathAny("definitely-not", "sh"); !ok {
		t.Skip("sh not on PATH")
	}
}
