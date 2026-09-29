package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareOneForSession(t *testing.T) {
	s := &BridgeService{}
	svc := NewAttachmentService(s)

	// nil bridge
	if _, _, err := NewAttachmentService(nil).PrepareOneForSession("x", ""); err == nil {
		t.Fatal("nil bridge")
	}
	// 空路径
	if _, _, err := svc.PrepareOneForSession("  ", ""); err == nil {
		t.Fatal("empty path")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 相对路径 + workspace
	ref, rt, err := svc.PrepareOneForSession("a.txt", dir)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Name != "a.txt" || ref.Kind == "" {
		t.Fatalf("ref = %+v", ref)
	}
	if rt.Path == "" {
		t.Fatalf("runtime = %+v", rt)
	}

	// 目录报错
	if _, _, err := svc.PrepareOneForSession(dir, ""); err == nil {
		t.Fatal("dir")
	}
	// 缺失文件
	if _, _, err := svc.PrepareOneForSession(filepath.Join(dir, "nope"), ""); err == nil {
		t.Fatal("missing")
	}

	// PrepareForSession 多路径
	refs, rts, err := svc.PrepareForSession([]string{src, ""}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || len(rts) != 1 {
		t.Fatalf("refs = %d rts = %d", len(refs), len(rts))
	}
}

func TestAttachmentServiceNilBridge(t *testing.T) {
	svc := NewAttachmentService(nil)
	if _, err := svc.OpenAttachmentDialog(); err == nil {
		// 可能成功也可能失败，不 panic 即可
		_ = err
	}
}

func TestBrowserControlNoGateway(t *testing.T) {
	s := &BridgeService{}
	if _, err := s.BrowserControlTakeover("r", "n", 0); err == nil {
		t.Fatal("no gateway")
	}
	if _, err := s.BrowserControlConfirm(); err == nil {
		t.Fatal("no gateway")
	}
	if _, err := s.BrowserControlResume(); err == nil {
		t.Fatal("no gateway")
	}
	if _, err := s.BrowserFocus("", ""); err == nil {
		t.Fatal("no gateway")
	}
}
