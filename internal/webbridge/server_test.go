package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func newDispatchTestServer() *Server {
	return &Server{bridge: &BridgeService{}, hub: newEventHub()}
}

func TestDispatchCallUnknownMethod(t *testing.T) {
	s := newDispatchTestServer()
	_, err := s.dispatchCall("main.BridgeService.DoesNotExist", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown bridge method") {
		t.Fatalf("expected unknown method error, got %v", err)
	}
}

func TestDispatchCallArityMismatch(t *testing.T) {
	s := newDispatchTestServer()
	_, err := s.dispatchCall("BridgeService.RenameSession", nil)
	if err == nil || !strings.Contains(err.Error(), "expects 2 argument(s)") {
		t.Fatalf("expected arity error, got %v", err)
	}
}

func TestDispatchCallDeniedLifecycleMethods(t *testing.T) {
	s := newDispatchTestServer()
	for _, method := range []string{"Start", "Close"} {
		if _, err := s.dispatchCall(method, nil); err == nil {
			t.Fatalf("expected %q to be denied", method)
		}
	}
}

func TestDispatchCallGetStats(t *testing.T) {
	s := newDispatchTestServer()
	result, err := s.dispatchCall("BridgeService.GetStats", nil)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	raw, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("marshal result: %v", marshalErr)
	}
	var stats SessionStats
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatalf("unmarshal SessionStats: %v", err)
	}
	if stats.SessionCount != 0 {
		t.Fatalf("expected zero sessions on bare bridge, got %d", stats.SessionCount)
	}
}

func TestDispatchCallArgumentDecoding(t *testing.T) {
	s := newDispatchTestServer()
	// ReadClipboardText 无参数返回空串（web 模式剪贴板不支持），验证
	// 空参数列表 + 单返回值的分发路径。
	result, err := s.dispatchCall("github.com/eosaios/eos-gui/internal/app.BridgeService.ReadClipboardText", []json.RawMessage{})
	if err != nil {
		t.Fatalf("ReadClipboardText failed: %v", err)
	}
	if text, ok := result.(string); !ok || text != "" {
		t.Fatalf("expected empty string result, got %#v", result)
	}
}

func TestInjectWebRuntimeMarker(t *testing.T) {
	marked := injectWebRuntimeMarker("<html><head><title>x</title></head></html>")
	if !strings.Contains(marked, "<head>"+webRuntimeMarker) {
		t.Fatalf("marker not injected right after <head>: %s", marked)
	}
	if injectWebRuntimeMarker("<html><body></body></html>") != "<html><body></body></html>" {
		t.Fatalf("marker must not be injected when <head> is missing")
	}
}

func writeTestUIDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html><head></head><body>app</body></html>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	return dir
}

func TestResolveWebUIDirExplicit(t *testing.T) {
	dir := writeTestUIDir(t)
	got, err := resolveWebUIDir(dir)
	if err != nil {
		t.Fatalf("explicit ui dir rejected: %v", err)
	}
	if got != dir {
		t.Fatalf("expected %s, got %s", dir, got)
	}
}

func TestResolveWebUIDirExplicitMissing(t *testing.T) {
	if _, err := resolveWebUIDir(t.TempDir()); err == nil {
		t.Fatal("expected error for dir without index.html")
	}
}

func TestResolveWebUIDirSearchMissListsCandidates(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := resolveWebUIDir("")
	if err == nil || !strings.Contains(err.Error(), "--ui-dir") {
		t.Fatalf("expected fail-fast error mentioning --ui-dir, got %v", err)
	}
}

func TestHandleStaticInjectsMarker(t *testing.T) {
	s := newDispatchTestServer()
	s.uiDir = writeTestUIDir(t)
	rec := httptest.NewRecorder()
	s.handleStatic(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), webRuntimeMarker) {
		t.Fatalf("marker missing from served index.html")
	}
}

func TestHandleStaticServesAsset(t *testing.T) {
	s := newDispatchTestServer()
	s.uiDir = writeTestUIDir(t)
	if err := os.MkdirAll(filepath.Join(s.uiDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.uiDir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	rec := httptest.NewRecorder()
	// 回归：r.URL.Path 已带前导斜杠，handler 若再拼 "/" 会得到 "//assets/…"，
	// Clean 拒绝 → 所有静态资产 404、页面全白（曾全线中招）。
	s.handleStatic(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for asset, got %d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "console.log(1)") {
		t.Fatalf("asset body mismatch: %q", rec.Body.String())
	}
}

func TestHandleStaticRejectsTraversal(t *testing.T) {
	s := newDispatchTestServer()
	s.uiDir = writeTestUIDir(t)
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	req.URL.Path = "/../../etc/passwd"
	rec := httptest.NewRecorder()
	s.handleStatic(rec, req)
	// 可疑路径（净化前后不一致）必须 404，绝不逃出 ui 目录。
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for traversal path, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "root:") {
		t.Fatalf("traversal escaped ui dir")
	}
}

func TestHandleRuntimeJS(t *testing.T) {
	s := newDispatchTestServer()
	rec := httptest.NewRecorder()
	s.handleRuntimeJS(rec, httptest.NewRequest(http.MethodGet, "/wails/runtime.js", nil))
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/javascript") {
		t.Fatalf("unexpected content type %q", ct)
	}
	for _, fragment := range []string{"export const Call", "export const Events", "export const Window", "__EOS_WEB_RUNTIME__"} {
		if !strings.Contains(rec.Body.String(), fragment) {
			t.Fatalf("runtime.js missing %q", fragment)
		}
	}
}

func TestEventHubBroadcastsToWSClient(t *testing.T) {
	s := newDispatchTestServer()
	backend := httptest.NewServer(http.HandlerFunc(s.handleWS))
	defer backend.Close()

	wsURL := "ws" + strings.TrimPrefix(backend.URL, "http")
	ctx := context.Background()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.CloseNow()

	// Dial 返回先于服务端 goroutine 的 hub.add 完成；过早 emit 会因 hub
	// 尚无订阅者而丢帧，Read 随即永久阻塞（CI 上 5m 超时 panic）。
	// 先轮询等连接注册进 hub 再 emit。
	registered := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		s.hub.mu.Lock()
		n := len(s.hub.clients)
		s.hub.mu.Unlock()
		if n > 0 {
			registered = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !registered {
		t.Fatal("ws client not registered in hub before deadline")
	}

	s.hub.emit("eos:bridge:shell-updated", map[string]any{"hello": "world"})
	// 带超时读：帧丢失时快速失败，避免整包测试挂死到全局 5m 超时。
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, raw, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var frame struct {
		Name string         `json:"name"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatalf("unmarshal frame: %v", err)
	}
	if frame.Name != "eos:bridge:shell-updated" || frame.Data["hello"] != "world" {
		t.Fatalf("unexpected frame: %s", raw)
	}
}
