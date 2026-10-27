package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 最小合法 PNG（1x1）：MIME 魔数嗅探只需要头部字节。
var testPNGBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52,
}

func newAttachmentRouteBridge(t *testing.T) (*BridgeService, string) {
	t.Helper()
	root := t.TempDir()
	return &BridgeService{activeWorkspace: root}, root
}

func serveImageRequest(t *testing.T, bridge *BridgeService, path string) *httptest.ResponseRecorder {
	t.Helper()
	handler := bridge.AttachmentImageMiddleware(http.NotFoundHandler())
	target := AttachmentImageRoutePath + "?path=" + url.QueryEscape(filepath.ToSlash(path))
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAttachmentImageMiddlewareServesWorkspaceAttachment(t *testing.T) {
	bridge, root := newAttachmentRouteBridge(t)
	image := filepath.Join(root, ".eos", "attachments", "shot.png")
	if err := os.MkdirAll(filepath.Dir(image), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(image, testPNGBytes, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec := serveImageRequest(t, bridge, image)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "image/png") {
		t.Fatalf("content-type = %q, want image/png", got)
	}
	if rec.Body.Len() != len(testPNGBytes) {
		t.Fatalf("body length = %d, want %d", rec.Body.Len(), len(testPNGBytes))
	}
}

func TestAttachmentImageMiddlewareRejectsOutsideRoots(t *testing.T) {
	bridge, _ := newAttachmentRouteBridge(t)
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, testPNGBytes, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec := serveImageRequest(t, bridge, outside)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404（白名单外的路径不得经路由读出）", rec.Code)
	}
}

func TestAttachmentImageMiddlewareRejectsNonImage(t *testing.T) {
	bridge, root := newAttachmentRouteBridge(t)
	notes := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(notes, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec := serveImageRequest(t, bridge, notes)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404（非图片扩展名拒绝）", rec.Code)
	}
}

func TestAttachmentImageMiddlewarePassesThroughOtherRoutes(t *testing.T) {
	bridge, _ := newAttachmentRouteBridge(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := bridge.AttachmentImageMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/some/frontend/route", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418（非附件路由必须原样交给默认 handler）", rec.Code)
	}
}

func TestPreviewAttachmentReturnsRouteURL(t *testing.T) {
	bridge, root := newAttachmentRouteBridge(t)
	image := filepath.Join(root, ".eos", "attachments", "shot.png")
	if err := os.MkdirAll(filepath.Dir(image), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(image, testPNGBytes, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc := NewAttachmentService(bridge)
	preview, err := svc.PreviewAttachment(image)
	if err != nil {
		t.Fatalf("PreviewAttachment: %v", err)
	}
	if preview.MIME != "image/png" {
		t.Fatalf("mime = %q, want image/png", preview.MIME)
	}
	// url 必须指向路由且能被路由解析回同一文件。
	if !strings.HasPrefix(preview.URL, AttachmentImageRoutePath+"?path=") {
		t.Fatalf("url = %q, want prefix %q", preview.URL, AttachmentImageRoutePath+"?path=")
	}
	rec := serveImageRequest(t, bridge, preview.Path)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview.Path 经路由加载 status = %d, want 200", rec.Code)
	}
}
