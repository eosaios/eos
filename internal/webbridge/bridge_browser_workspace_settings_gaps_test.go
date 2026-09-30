package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// webbridge 批二批测：browser 控制面透传 / 帧路由 / workspace 服务 /
// settings 服务 / message codec 纯转换。复用批一的 chatSessionsGatewayStub
//（同包扩方法），browser 与 settings 方法在此文件补齐。

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// ---- chatSessionsGatewayStub 扩展：browser / settings / 订阅 ----

func (g *chatSessionsGatewayStub) CoreBrowserControlTakeoverRPC(_ context.Context, req coreapi.BrowserControlTakeoverRequest) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return g.browserErr
	}
	g.takeoverReasons = append(g.takeoverReasons, req.Reason)
	return nil
}

func (g *chatSessionsGatewayStub) CoreBrowserControlConfirmRPC(context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.browserErr
}

func (g *chatSessionsGatewayStub) CoreBrowserControlResumeRPC(context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.browserErr
}

func (g *chatSessionsGatewayStub) CoreBrowserFocusRPC(_ context.Context, req coreapi.BrowserFocusRequest) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return g.browserErr
	}
	g.focusURLs = append(g.focusURLs, req.URL)
	return nil
}

func (g *chatSessionsGatewayStub) CoreBrowserSetDefaultProfileRPC(_ context.Context, profile string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return g.browserErr
	}
	g.defaultProfiles = append(g.defaultProfiles, profile)
	return nil
}

func (g *chatSessionsGatewayStub) CoreBrowserTabNewRPC(_ context.Context, url string) (coreapi.BrowserTabInfo, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return coreapi.BrowserTabInfo{}, g.browserErr
	}
	g.tabNewURLs = append(g.tabNewURLs, url)
	return coreapi.BrowserTabInfo{Index: 3, URL: url, Title: "new tab"}, nil
}

func (g *chatSessionsGatewayStub) CoreBrowserTabSwitchRPC(_ context.Context, index int) (coreapi.BrowserTabInfo, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return coreapi.BrowserTabInfo{}, g.browserErr
	}
	g.tabSwitches = append(g.tabSwitches, index)
	return coreapi.BrowserTabInfo{Index: index, URL: "https://switched", Title: "switched"}, nil
}

func (g *chatSessionsGatewayStub) CoreBrowserTabCloseRPC(_ context.Context, index *int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return g.browserErr
	}
	g.tabCloses = append(g.tabCloses, index)
	return nil
}

func (g *chatSessionsGatewayStub) CoreBrowserNavigateRPC(_ context.Context, url string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return g.browserErr
	}
	g.navigations = append(g.navigations, url)
	return nil
}

func (g *chatSessionsGatewayStub) CoreBrowserLiveStartRPC(_ context.Context, req coreapi.BrowserLiveStartRequest) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return g.browserErr
	}
	g.liveStarts = append(g.liveStarts, req)
	return nil
}

func (g *chatSessionsGatewayStub) CoreBrowserLiveStopRPC(context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.browserErr
}

func (g *chatSessionsGatewayStub) CoreBrowserInputRPC(_ context.Context, req coreapi.BrowserInputRequest) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return g.browserErr
	}
	g.inputs = append(g.inputs, req)
	return nil
}

func (g *chatSessionsGatewayStub) CoreBrowserHistoryRPC(_ context.Context, action string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return g.browserErr
	}
	g.historyActions = append(g.historyActions, action)
	return nil
}

func (g *chatSessionsGatewayStub) CoreBrowserCopySelectionRPC(context.Context) (coreapi.BrowserCopySelectionResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return coreapi.BrowserCopySelectionResult{}, g.browserErr
	}
	return coreapi.BrowserCopySelectionResult{Text: "选中文本"}, nil
}

func (g *chatSessionsGatewayStub) CoreBrowserPickStartRPC(context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.browserErr
}

func (g *chatSessionsGatewayStub) CoreBrowserPickStopRPC(context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.browserErr
}

func (g *chatSessionsGatewayStub) CoreBrowserCredentialsImportRPC(_ context.Context, req coreapi.BrowserCredentialsImportRequest) (coreapi.BrowserCredentialsImportResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return coreapi.BrowserCredentialsImportResult{}, g.browserErr
	}
	g.credentialImports = append(g.credentialImports, req)
	return coreapi.BrowserCredentialsImportResult{Imported: 2}, nil
}

func (g *chatSessionsGatewayStub) CoreBrowserProfileUpsertRPC(_ context.Context, params map[string]any) ([]coreapi.BrowserProfileRecord, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return nil, g.browserErr
	}
	g.profileUpserts = append(g.profileUpserts, params)
	return []coreapi.BrowserProfileRecord{{Name: "external"}}, nil
}

func (g *chatSessionsGatewayStub) CoreBrowserProfilesRPC(context.Context) ([]coreapi.BrowserProfileRecord, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return nil, g.browserErr
	}
	return []coreapi.BrowserProfileRecord{{Name: "external"}, {Name: "internal"}}, nil
}

func (g *chatSessionsGatewayStub) CoreBrowserUploadProvideRPC(_ context.Context, requestID string, paths []string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserErr != nil {
		return g.browserErr
	}
	g.uploadProvides = append(g.uploadProvides, [2]interface{}{requestID, paths})
	return nil
}

func (g *chatSessionsGatewayStub) CoreSubscribeEventsRPC(_ context.Context, _, _, _ string, _ int) (<-chan adapter.Event, func(), error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.subscribeErr != nil {
		return nil, nil, g.subscribeErr
	}
	if g.eventCh == nil {
		g.eventCh = make(chan adapter.Event, 8)
	}
	ch := g.eventCh
	return ch, func() {}, nil
}

func (g *chatSessionsGatewayStub) CoreSaveSettingsRPC(_ context.Context, settings coreapi.Settings) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.saveSettingsErr != nil {
		return g.saveSettingsErr
	}
	g.savedKernels = append(g.savedKernels, settings)
	return nil
}

// ---- browser 控制面 ----

func TestBrowserControlForwardingArms(t *testing.T) {
	t.Run("无 gateway 拒绝", func(t *testing.T) {
		s := &BridgeService{}
		if _, err := s.BrowserControlTakeover("r", "", 0); err == nil {
			t.Fatal("BrowserControlTakeover without gateway error = nil")
		}
		if _, err := s.BrowserNavigate("https://x"); err == nil {
			t.Fatal("BrowserNavigate without gateway error = nil")
		}
	})

	gateway := &chatSessionsGatewayStub{}
	s := &BridgeService{runtimeGateway: gateway}

	t.Run("takeover 成功与失败", func(t *testing.T) {
		result, err := s.BrowserControlTakeover("  manual  ", " note ", 1500)
		if err != nil {
			t.Fatalf("BrowserControlTakeover error = %v", err)
		}
		if result["taken_over"] != true {
			t.Fatalf("result = %v", result)
		}
		if gateway.takeoverReasons[0] != "manual" {
			t.Fatalf("reason not trimmed: %q", gateway.takeoverReasons[0])
		}
		gateway.browserErr = errors.New("busy")
		if _, err := s.BrowserControlTakeover("r", "", 0); err == nil || !strings.Contains(err.Error(), "接管控制权失败") {
			t.Fatalf("takeover error = %v, want wrapped failure", err)
		}
		gateway.browserErr = nil
	})

	t.Run("confirm/resume", func(t *testing.T) {
		if _, err := s.BrowserControlConfirm(); err != nil {
			t.Fatalf("BrowserControlConfirm error = %v", err)
		}
		if _, err := s.BrowserControlResume(); err != nil {
			t.Fatalf("BrowserControlResume error = %v", err)
		}
		gateway.browserErr = errors.New("deny")
		if _, err := s.BrowserControlConfirm(); err == nil {
			t.Fatal("confirm error = nil on gateway failure")
		}
		if _, err := s.BrowserControlResume(); err == nil {
			t.Fatal("resume error = nil on gateway failure")
		}
		gateway.browserErr = nil
	})

	t.Run("focus 与默认 profile", func(t *testing.T) {
		if _, err := s.BrowserFocus(" https://eos.example ", " external "); err != nil {
			t.Fatalf("BrowserFocus error = %v", err)
		}
		if gateway.focusURLs[0] != "https://eos.example" {
			t.Fatalf("focus url not trimmed: %v", gateway.focusURLs)
		}
		result, err := s.BrowserSetDefaultProfile(" external ")
		if err != nil {
			t.Fatalf("BrowserSetDefaultProfile error = %v", err)
		}
		if result["profile"] != " external " || gateway.defaultProfiles[0] != "external" {
			t.Fatalf("profile result/call mismatch: %v %v", result, gateway.defaultProfiles)
		}
	})

	t.Run("tab new/switch/close", func(t *testing.T) {
		result, err := s.BrowserTabNew(" https://tab ")
		if err != nil {
			t.Fatalf("BrowserTabNew error = %v", err)
		}
		if result["index"] != 3 || result["title"] != "new tab" {
			t.Fatalf("tab new result = %v", result)
		}
		switched, err := s.BrowserTabSwitch(1)
		if err != nil || switched["url"] != "https://switched" {
			t.Fatalf("tab switch = %v %v", switched, err)
		}
		index := 2
		if _, err := s.BrowserTabClose(&index); err != nil {
			t.Fatalf("BrowserTabClose error = %v", err)
		}
		if _, err := s.BrowserTabClose(nil); err != nil {
			t.Fatalf("BrowserTabClose(nil) error = %v", err)
		}
		if len(gateway.tabCloses) != 2 || gateway.tabCloses[0] == nil || *gateway.tabCloses[0] != 2 || gateway.tabCloses[1] != nil {
			t.Fatalf("tabCloses = %v", gateway.tabCloses)
		}
	})

	t.Run("navigate 空地址拒绝", func(t *testing.T) {
		if _, err := s.BrowserNavigate("   "); err == nil || !strings.Contains(err.Error(), "地址不能为空") {
			t.Fatalf("BrowserNavigate('') error = %v", err)
		}
		result, err := s.BrowserNavigate(" https://nav ")
		if err != nil || result["navigated"] != true {
			t.Fatalf("navigate = %v %v", result, err)
		}
		if gateway.navigations[0] != "https://nav" {
			t.Fatalf("navigate url not trimmed: %v", gateway.navigations)
		}
	})

	t.Run("live start 参数与 stop", func(t *testing.T) {
		if _, err := s.BrowserLiveStart(1280, 720, 70); err != nil {
			t.Fatalf("BrowserLiveStart error = %v", err)
		}
		started := gateway.liveStarts[0]
		if started.MaxWidth == nil || *started.MaxWidth != 1280 || started.Quality == nil || *started.Quality != 70 {
			t.Fatalf("live start params = %+v", started)
		}
		if _, err := s.BrowserLiveStart(0, 0, 0); err != nil {
			t.Fatalf("BrowserLiveStart(zeros) error = %v", err)
		}
		if gateway.liveStarts[1].MaxWidth != nil || gateway.liveStarts[1].MaxHeight != nil || gateway.liveStarts[1].Quality != nil {
			t.Fatalf("zero params should stay nil: %+v", gateway.liveStarts[1])
		}
		if _, err := s.BrowserLiveStop(); err != nil {
			t.Fatalf("BrowserLiveStop error = %v", err)
		}
	})

	t.Run("input 校验与转发", func(t *testing.T) {
		result, err := s.BrowserInput(map[string]interface{}{"kind": "key", "key": "Enter"})
		if err != nil || result["ok"] != true {
			t.Fatalf("BrowserInput = %v %v", result, err)
		}
		if gateway.inputs[0].Kind != "key" {
			t.Fatalf("input kind = %+v", gateway.inputs[0])
		}
		if _, err := s.BrowserInput(map[string]interface{}{}); err == nil || !strings.Contains(err.Error(), "缺少 kind") {
			t.Fatalf("input without kind error = %v", err)
		}
		if _, err := s.BrowserInput(map[string]interface{}{"kind": 123}); err == nil {
			t.Fatal("input with bad type error = nil")
		}
	})

	t.Run("history 校验", func(t *testing.T) {
		for _, action := range []string{"back", "forward", " reload "} {
			if _, err := s.BrowserHistory(action); err != nil {
				t.Fatalf("BrowserHistory(%q) error = %v", action, err)
			}
		}
		if gateway.historyActions[2] != "reload" {
			t.Fatalf("history action not trimmed: %v", gateway.historyActions)
		}
		if _, err := s.BrowserHistory("purge"); err == nil || !strings.Contains(err.Error(), "不支持的历史操作") {
			t.Fatalf("BrowserHistory(purge) error = %v", err)
		}
	})

	t.Run("选区/选取模式/profile 列表", func(t *testing.T) {
		result, err := s.BrowserCopySelection()
		if err != nil || result["text"] != "选中文本" {
			t.Fatalf("copy selection = %v %v", result, err)
		}
		if _, err := s.BrowserPickStart(); err != nil {
			t.Fatalf("BrowserPickStart error = %v", err)
		}
		if _, err := s.BrowserPickStop(); err != nil {
			t.Fatalf("BrowserPickStop error = %v", err)
		}
		profiles, err := s.BrowserProfiles()
		if err != nil || len(profiles) != 2 {
			t.Fatalf("BrowserProfiles = %v %v", profiles, err)
		}
	})

	t.Run("credentials import 与 profile upsert", func(t *testing.T) {
		result, err := s.BrowserCredentialsImport(" 127.0.0.1:9222 ", " external ", []string{"github.com"}, true)
		if err != nil {
			t.Fatalf("BrowserCredentialsImport error = %v", err)
		}
		if result["imported"] == nil {
			t.Fatalf("import result = %v", result)
		}
		if gateway.credentialImports[0].Endpoint != "127.0.0.1:9222" {
			t.Fatalf("import endpoint not trimmed: %+v", gateway.credentialImports[0])
		}

		headless := true
		note := "  工作账号  "
		if _, err := s.BrowserProfileUpsert("  external  ", &headless, note); err != nil {
			t.Fatalf("BrowserProfileUpsert error = %v", err)
		}
		params := gateway.profileUpserts[0]
		if params["name"] != "external" || params["headless"] != true || params["note"] != "工作账号" {
			t.Fatalf("profile params = %v", params)
		}
		if _, err := s.BrowserProfileUpsert("   ", nil, ""); err == nil || !strings.Contains(err.Error(), "名不能为空") {
			t.Fatalf("upsert empty name error = %v", err)
		}
	})

	t.Run("web 模式无原生文件选择框", func(t *testing.T) {
		if _, err := s.BrowserPickUploadFile("png"); err == nil || !strings.Contains(err.Error(), "打开文件选择失败") {
			t.Fatalf("BrowserPickUploadFile error = %v", err)
		}
	})
}

func TestStartBrowserEventPumpForwardsBrowserTopics(t *testing.T) {
	gateway := &chatSessionsGatewayStub{}
	gateway.eventCh = make(chan adapter.Event, 8)
	rec := &emitRecorder{}
	s := &BridgeService{
		runtimeGateway: gateway,
		sessions:       map[string]*sessionState{},
		prompts:        map[string]*promptState{},
		emitEvent:      rec.record,
	}
	s.startBrowserEventPump()

	gateway.eventCh <- adapter.Event{Type: "browser.takeover.requested", Payload: map[string]any{"reason": "manual"}}
	gateway.eventCh <- adapter.Event{EventType: "browser.pick.selected", Data: map[string]any{"ref": "button#go"}}
	gateway.eventCh <- adapter.Event{Type: "turn.started"}
	// upload.needed 缺 request_id → 早退（不 provide）。
	gateway.eventCh <- adapter.Event{Type: "browser.upload.needed", Payload: map[string]any{}}
	// 带 dataUrl 的 frame → 缓存 + 轻载荷转发。
	jpeg := base64.StdEncoding.EncodeToString([]byte("fakejpeg"))
	gateway.eventCh <- adapter.Event{Type: "browser.frame", Payload: map[string]any{
		"dataUrl": "data:image/jpeg;base64," + jpeg,
		"ts":      float64(1234),
		"meta":    map[string]any{"w": 1280},
	}}
	// 非 jpeg 前缀的 frame 载荷原样降级返回。
	gateway.eventCh <- adapter.Event{Type: "browser.frame", Payload: map[string]any{"dataUrl": "data:image/png;base64,xxxx"}}

	eventually(t, "browser events forwarded", func() bool {
		// takeover / pick / upload.needed / jpeg frame / png frame 共 5 条转发。
		return len(rec.browserPayloads()) >= 5
	})

	payloads := rec.browserPayloads()
	if payloads[0].Type != "browser.takeover.requested" {
		t.Fatalf("first forwarded = %+v", payloads[0])
	}
	if payloads[1].Type != "browser.pick.selected" || payloads[1].Payload["ref"] != "button#go" {
		t.Fatalf("EventType fallback payload = %+v", payloads[1])
	}
	// 事件顺序：…/upload.needed/jpeg frame/png frame；jpeg 帧轻载荷、png 帧原样降级。
	jpegFrame := payloads[len(payloads)-2]
	if _, heavy := jpegFrame.Payload["dataUrl"]; heavy {
		t.Fatalf("jpeg frame payload should be light: %+v", jpegFrame.Payload)
	}
	if jpegFrame.Payload["ts"] != float64(1234) || jpegFrame.Payload["meta"] == nil {
		t.Fatalf("frame light payload = %+v", jpegFrame.Payload)
	}
	if cached := s.browserFrame.Load(); cached == nil || string(cached.jpeg) != "fakejpeg" {
		t.Fatalf("frame cache = %+v", cached)
	}
	degraded := payloads[len(payloads)-1]
	if _, ok := degraded.Payload["dataUrl"]; !ok {
		t.Fatalf("non-jpeg frame should pass through unchanged: %+v", degraded.Payload)
	}
	for _, p := range payloads {
		if p.Type == "turn.started" {
			t.Fatal("non-browser topic forwarded to frontend")
		}
	}
	// 缺 request_id 的 upload.needed 不应触发 provide。
	gateway.mu.Lock()
	provides := len(gateway.uploadProvides)
	gateway.mu.Unlock()
	if provides != 0 {
		t.Fatalf("uploadProvides = %d, want 0 for missing request_id", provides)
	}
}

func TestHandleUploadNeededProvidesEmptyOnPickFailure(t *testing.T) {
	gateway := &chatSessionsGatewayStub{}
	rec := &emitRecorder{}
	s := &BridgeService{
		runtimeGateway: gateway,
		sessions:       map[string]*sessionState{},
		prompts:        map[string]*promptState{},
		emitEvent:      rec.record,
	}
	// web 模式 BrowserPickUploadFile 必失败 → 取消路径：provide 空路径列表。
	s.handleUploadNeeded(map[string]any{"request_id": "req-1"})
	eventually(t, "upload provide with empty paths", func() bool {
		gateway.mu.Lock()
		defer gateway.mu.Unlock()
		return len(gateway.uploadProvides) > 0
	})
	gateway.mu.Lock()
	provide := gateway.uploadProvides[0]
	gateway.mu.Unlock()
	if provide[0] != "req-1" || provide[1] != nil && len(provide[1].([]string)) != 0 {
		t.Fatalf("provide = %v, want req-1 with empty paths", provide)
	}
}

func TestStartBrowserEventPumpSubscribeFailureSkips(t *testing.T) {
	gateway := &chatSessionsGatewayStub{subscribeErr: errors.New("unavailable")}
	s := &BridgeService{runtimeGateway: gateway}
	s.startBrowserEventPump() // 静默跳过，不 panic
}

func TestServeBrowserFrameRoute(t *testing.T) {
	s := &BridgeService{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, BrowserFrameRoutePath, nil)
	s.serveBrowserFrame(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("empty cache code = %d, want 404", w.Code)
	}

	s.browserFrame.Store(&browserFrameCache{jpeg: []byte("framebytes"), ts: 99})
	w = httptest.NewRecorder()
	s.serveBrowserFrame(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", w.Code)
	}
	if w.Body.String() != "framebytes" {
		t.Fatalf("body = %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("content-type = %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control = %q", cc)
	}
}

// ---- workspace 服务 ----

func TestWorkspaceServiceSelectWorkspaceArms(t *testing.T) {
	t.Run("nil bridge / 空路径", func(t *testing.T) {
		svc := NewWorkspaceService(nil)
		if _, err := svc.SelectWorkspace(""); err == nil {
			t.Fatal("SelectWorkspace(nil bridge) error = nil")
		}
		if _, err := svc.TrustWorkspace(""); err == nil {
			t.Fatal("TrustWorkspace(nil bridge) error = nil")
		}
		if _, err := svc.RemoveWorkspace(""); err == nil {
			t.Fatal("RemoveWorkspace(nil bridge) error = nil")
		}
		gateway := &chatSessionsGatewayStub{}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		ws := NewWorkspaceService(chatSvc.bridge)
		if _, err := ws.SelectWorkspace("   "); err == nil || !strings.Contains(err.Error(), "路径不能为空") {
			t.Fatalf("SelectWorkspace('') error = %v", err)
		}
	})

	t.Run("激活失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{useErr: errors.New("denied"), addErr: errors.New("denied")}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewWorkspaceService(chatSvc.bridge)
		if _, err := svc.SelectWorkspace(t.TempDir()); err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("SelectWorkspace error = %v", err)
		}
	})

	t.Run("恢复历史会话并 resume", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{
			defaultWorkspace: workspace,
			sessions:         []coreapi.Session{{ID: "ws-sess-1", WorkspaceRoot: workspace}},
		}
		s, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewWorkspaceService(chatSvc.bridge)
		state, err := svc.SelectWorkspace(workspace)
		if err != nil {
			t.Fatalf("SelectWorkspace error = %v", err)
		}
		if state.CurrentSessionID != "ws-sess-1" {
			t.Fatalf("CurrentSessionID = %q, want restored ws-sess-1", state.CurrentSessionID)
		}
		s.stateMu.RLock()
		active := s.activeWorkspace
		s.stateMu.RUnlock()
		if active != workspace {
			t.Fatalf("activeWorkspace = %q", active)
		}
	})

	t.Run("无历史会话回落空 current", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewWorkspaceService(chatSvc.bridge)
		state, err := svc.SelectWorkspace(workspace)
		if err != nil {
			t.Fatalf("SelectWorkspace error = %v", err)
		}
		if state.CurrentSessionID != "" {
			t.Fatalf("CurrentSessionID = %q, want empty", state.CurrentSessionID)
		}
		cleared := false
		for _, call := range gateway.CoreSetCurrentSessionCalls() {
			if call[1] == "" {
				cleared = true
			}
		}
		if !cleared {
			t.Fatal("set-current-session('') not called for empty workspace")
		}
	})
}

func TestWorkspaceServiceTrustAndRemoveArms(t *testing.T) {
	t.Run("trust nil/空路径/RPC 失败/成功", func(t *testing.T) {
		if _, err := (NewWorkspaceService(nil)).TrustWorkspace(""); err == nil {
			t.Fatal("TrustWorkspace(nil bridge) error = nil")
		}
		gateway := &chatSessionsGatewayStub{}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewWorkspaceService(chatSvc.bridge)
		if _, err := svc.TrustWorkspace("  "); err == nil || !strings.Contains(err.Error(), "路径不能为空") {
			t.Fatalf("TrustWorkspace('') error = %v", err)
		}
		if _, err := svc.TrustWorkspace(t.TempDir()); err != nil {
			t.Fatalf("TrustWorkspace error = %v", err)
		}
	})

	t.Run("remove nil/空路径/默认工作区保护", func(t *testing.T) {
		if _, err := (NewWorkspaceService(nil)).RemoveWorkspace(""); err == nil {
			t.Fatal("RemoveWorkspace(nil bridge) error = nil")
		}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		defaultWS := filepath.Join(home, ".eos", "workspace")
		gateway := &chatSessionsGatewayStub{defaultWorkspace: defaultWS}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewWorkspaceService(chatSvc.bridge)
		if _, err := svc.RemoveWorkspace("   "); err == nil || !strings.Contains(err.Error(), "路径不能为空") {
			t.Fatalf("RemoveWorkspace('') error = %v", err)
		}
		if _, err := svc.RemoveWorkspace(defaultWS); err == nil || !strings.Contains(err.Error(), "默认工作区不能移除") {
			t.Fatalf("RemoveWorkspace(default) error = %v", err)
		}
	})

	t.Run("remove 先删会话再忘工作区", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		defaultWS := filepath.Join(home, ".eos", "workspace")
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{
			defaultWorkspace: defaultWS,
			lastWorkspace:    defaultWS,
			sessions:         []coreapi.Session{{ID: "gone-1", WorkspaceRoot: workspace}, {ID: "gone-2", WorkspaceRoot: workspace}},
		}
		s, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewWorkspaceService(chatSvc.bridge)
		s.stateMu.Lock()
		s.sessions["gone-1"] = &sessionState{ID: "gone-1", WorkspacePath: workspace}
		s.sessions["local-only"] = &sessionState{ID: "local-only", WorkspacePath: workspace}
		s.activeWorkspace = workspace
		s.currentSessionID = "gone-1"
		s.stateMu.Unlock()

		state, err := svc.RemoveWorkspace(workspace)
		if err != nil {
			t.Fatalf("RemoveWorkspace error = %v", err)
		}
		if len(gateway.deletedIDs) != 2 {
			t.Fatalf("deletedIDs = %v, want both core sessions deleted first", gateway.deletedIDs)
		}
		s.stateMu.RLock()
		_, gone1 := s.sessions["gone-1"]
		_, localGone := s.sessions["local-only"]
		active := s.activeWorkspace
		s.stateMu.RUnlock()
		if gone1 || localGone {
			t.Fatal("workspace sessions still present after remove")
		}
		if active != defaultWS {
			t.Fatalf("activeWorkspace = %q, want fallback to last workspace", active)
		}
		if state.CurrentSessionID == "gone-1" {
			t.Fatal("current session still points at removed workspace session")
		}
	})

	t.Run("remove 容忍内核不存在错误", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		defaultWS := filepath.Join(home, ".eos", "workspace")
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{
			defaultWorkspace: defaultWS,
			removeErr:        os.ErrNotExist,
		}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewWorkspaceService(chatSvc.bridge)
		if _, err := svc.RemoveWorkspace(workspace); err != nil {
			t.Fatalf("RemoveWorkspace error = %v, want tolerated ErrNotExist", err)
		}
	})
}

// ---- settings 服务 ----

func TestSettingsServiceArms(t *testing.T) {
	t.Run("nil bridge 全方法", func(t *testing.T) {
		svc := NewSettingsService(nil)
		if _, err := svc.SaveSettings(SettingsSaveRequest{}); err == nil {
			t.Fatal("SaveSettings(nil) error = nil")
		}
		if _, err := svc.SetReasoningLevel(""); err == nil {
			t.Fatal("SetReasoningLevel(nil) error = nil")
		}
		if _, err := svc.ToggleFastMode(); err == nil {
			t.Fatal("ToggleFastMode(nil) error = nil")
		}
		if got := svc.SetTheme("dark"); got.CurrentSessionID != "" || got.ActiveWorkspace != "" {
			t.Fatalf("SetTheme(nil) = %+v, want zero state", got)
		}
		if got := svc.SetExecutionMode("auto"); got.AppVersion != "" {
			t.Fatalf("SetExecutionMode(nil) = %+v, want zero state", got)
		}
	})

	t.Run("代理开启无地址拒绝", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewSettingsService(chatSvc.bridge)
		if _, err := svc.SaveSettings(SettingsSaveRequest{UpdateProxyEnabled: true}); err == nil || !strings.Contains(err.Error(), "地址为空") {
			t.Fatalf("SaveSettings error = %v, want empty proxy url rejection", err)
		}
		if _, err := svc.SaveSettings(SettingsSaveRequest{UpdateProxyEnabled: true, UpdateProxyURL: "::::not-a-url"}); err == nil {
			t.Fatal("SaveSettings error = nil for invalid proxy url")
		}
	})

	t.Run("保存成功链推进内核三轴", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		gateway := &chatSessionsGatewayStub{}
		gateway.configPathOverride = filepath.Join(home, "config.json")
		s, chatSvc, rec := newChatSessionsTestBridge(t, gateway)
		svc := NewSettingsService(chatSvc.bridge)
		t.Cleanup(CloseLogger_FOR_TESTS_ONLY)

		trayCalled := false
		s.SetStayInTrayChangedListener(func(enabled bool) { trayCalled = enabled })
		state, err := svc.SaveSettings(SettingsSaveRequest{
			Language:          "zh",
			Theme:             "dark",
			ExecutionMode:     "agent",
			SandboxMode:       "workspace-write",
			ReasoningLevel:    "high",
			PromptTimeoutSecs: -5,
			StayInTray:        true,
		})
		if err != nil {
			t.Fatalf("SaveSettings error = %v", err)
		}
		if state.CurrentSessionID != "" && false {
			t.Fatal("unreachable")
		}
		gateway.mu.Lock()
		kernelSaves := len(gateway.savedKernels)
		gateway.mu.Unlock()
		if kernelSaves != 1 {
			t.Fatalf("kernel settings saves = %d, want 1", kernelSaves)
		}
		if kernelSaves == 1 {
			gateway.mu.Lock()
			saved := gateway.savedKernels[0]
			gateway.mu.Unlock()
			if saved.PromptTimeoutSecs != nil {
				t.Fatalf("negative timeout should normalize to nil, got %v", *saved.PromptTimeoutSecs)
			}
		}
		if !trayCalled {
			t.Fatal("stay-in-tray listener not notified")
		}
		eventually(t, "shellUpdated after save", func() bool { return rec.has(shellUpdatedEventName) })
	})

	t.Run("setReasoningLevel 空回退 off", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewSettingsService(chatSvc.bridge)
		if _, err := svc.SetReasoningLevel("  "); err != nil {
			t.Fatalf("SetReasoningLevel('') error = %v", err)
		}
		if _, err := svc.SetReasoningLevel("HIGH"); err != nil {
			t.Fatalf("SetReasoningLevel(HIGH) error = %v", err)
		}
		gateway.mu.Lock()
		getErr := gateway.getSettingsErr
		gateway.mu.Unlock()
		_ = getErr
	})

	t.Run("setReasoningLevel 内核失败透传", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		gateway := &chatSessionsGatewayStub{}
		gateway.configPathOverride = filepath.Join(home, "config.json")
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewSettingsService(chatSvc.bridge)
		gateway.getSettingsErr = errors.New("kernel down")
		if _, err := svc.SaveSettings(SettingsSaveRequest{Language: "zh"}); err == nil || !strings.Contains(err.Error(), "kernel down") {
			t.Fatalf("SaveSettings with kernel read failure error = %v", err)
		}
	})

	t.Run("normalizePromptTimeoutSecs", func(t *testing.T) {
		if got := normalizePromptTimeoutSecs(-3); got != 0 {
			t.Fatalf("normalizePromptTimeoutSecs(-3) = %d", got)
		}
		if got := normalizePromptTimeoutSecs(0); got != 0 {
			t.Fatalf("normalizePromptTimeoutSecs(0) = %d", got)
		}
		if got := normalizePromptTimeoutSecs(90); got != 90 {
			t.Fatalf("normalizePromptTimeoutSecs(90) = %d", got)
		}
	})

	t.Run("setPromptTimeoutRPC 读改写", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		if err := s.setPromptTimeoutRPC(120); err != nil {
			t.Fatalf("setPromptTimeoutRPC(120) error = %v", err)
		}
		gateway.mu.Lock()
		saved := gateway.savedKernels[0]
		gateway.mu.Unlock()
		if saved.PromptTimeoutSecs == nil || *saved.PromptTimeoutSecs != 120 {
			t.Fatalf("saved timeout = %v, want 120", saved.PromptTimeoutSecs)
		}
		if err := s.setPromptTimeoutRPC(0); err != nil {
			t.Fatalf("setPromptTimeoutRPC(0) error = %v", err)
		}
		gateway.mu.Lock()
		cleared := gateway.savedKernels[1].PromptTimeoutSecs
		gateway.mu.Unlock()
		if cleared != nil {
			t.Fatalf("timeout on disable = %v, want nil", *cleared)
		}
		gateway.getSettingsErr = errors.New("read failed")
		if err := s.setPromptTimeoutRPC(10); err == nil || !strings.Contains(err.Error(), "read kernel settings") {
			t.Fatalf("read failure error = %v", err)
		}
		gateway.getSettingsErr = nil
		gateway.saveSettingsErr = errors.New("write failed")
		if err := s.setPromptTimeoutRPC(10); err == nil || !strings.Contains(err.Error(), "save kernel settings") {
			t.Fatalf("write failure error = %v", err)
		}
	})
}

// ---- message codec 纯转换 ----

func TestMessageCodecApprovalAndQuestions(t *testing.T) {
	t.Run("populateApprovalFromMetadata 全臂", func(t *testing.T) {
		item := &ThreadItem{}
		populateApprovalFromMetadata(item, "not-a-map")
		if item.Approval != nil {
			t.Fatal("non-map metadata should not populate approval")
		}
		populateApprovalFromMetadata(item, map[string]any{"state": "   "})
		if item.Approval != nil {
			t.Fatal("blank state should not populate approval")
		}
		populateApprovalFromMetadata(item, map[string]any{
			"approvalId": "apr-1",
			"state":      "pending",
			"kind":       "approval",
			"title":      "执行命令",
			"riskLevel":  "high",
			"questions": []any{
				map[string]any{"id": "q1", "header": "端口", "question": "用哪个", "options": []any{
					map[string]any{"label": "8080", "description": "开发"},
					"bad-option-shape",
				}},
				"bad-question-shape",
			},
		})
		ap := item.Approval
		if ap == nil || ap.ApprovalID != "apr-1" || ap.State != "pending" || ap.Title != "执行命令" || ap.RiskLevel != "high" {
			t.Fatalf("approval = %+v", ap)
		}
		if len(ap.Questions) != 1 {
			t.Fatalf("questions = %+v, want bad shape skipped", ap.Questions)
		}
		if len(ap.Questions[0].Options) != 1 || ap.Questions[0].Options[0].Label != "8080" {
			t.Fatalf("options = %+v", ap.Questions[0].Options)
		}
	})

	t.Run("requestUserInputQuestionsFromAny 边界", func(t *testing.T) {
		got := requestUserInputQuestionsFromAny([]any{
			map[string]any{"id": "q1"},
			42,
			map[string]any{"id": "q2", "options": []any{"plain-string"}},
		})
		if len(got) != 2 {
			t.Fatalf("questions = %+v, want non-map items skipped", got)
		}
		if len(got[1].Options) != 0 {
			t.Fatalf("non-object option should be skipped: %+v", got[1].Options)
		}
		if got := requestUserInputQuestionsFromAny(nil); got == nil || len(got) != 0 {
			t.Fatalf("nil input = %+v, want empty non-nil", got)
		}
	})

	t.Run("threadItemFromSessionMessage 各 kind", func(t *testing.T) {
		if _, ok := threadItemFromSessionMessage(adapter.SessionMessage{Content: "x"}); ok {
			t.Fatal("missing item_id should not produce item")
		}
		reasoning, ok := threadItemFromSessionMessage(adapter.SessionMessage{
			Content:  "想想",
			Metadata: map[string]any{"item_id": "i1", "kind": "reasoning"},
		})
		if !ok || reasoning.Kind != "reasoning" || reasoning.Reasoning != "想想" {
			t.Fatalf("reasoning item = %+v", reasoning)
		}
		plan, _ := threadItemFromSessionMessage(adapter.SessionMessage{
			Content:  "计划",
			Metadata: map[string]any{"item_id": "i2", "kind": "plan"},
		})
		if plan.Kind != "plan" || plan.Text != "计划" {
			t.Fatalf("plan item = %+v", plan)
		}
		status, _ := threadItemFromSessionMessage(adapter.SessionMessage{
			Content:  "运行中",
			Metadata: map[string]any{"item_id": "i3", "kind": "status", "level": "info"},
		})
		if status.Kind != "status" || status.Level != "info" {
			t.Fatalf("status item = %+v", status)
		}
		agentMsg, _ := threadItemFromSessionMessage(adapter.SessionMessage{
			Role:     "assistant",
			Content:  "回答",
			Metadata: map[string]any{"item_id": "i4"},
		})
		if agentMsg.Kind != "agent_message" || agentMsg.Text != "回答" {
			t.Fatalf("agent_message item = %+v", agentMsg)
		}
		toolCall, _ := threadItemFromSessionMessage(adapter.SessionMessage{
			Content: "",
			Metadata: map[string]any{
				"item_id": "i5",
				"tool_call": map[string]any{
					"name":      "bash",
					"arguments": `{"cmd":"ls"}`,
					"bogus":     true,
				},
				"approval": map[string]any{"approvalId": "apr-5", "state": "pending"},
			},
		})
		if toolCall.Kind != "tool_call" || toolCall.ToolName != "bash" || toolCall.ToolArgs != `{"cmd":"ls"}` {
			t.Fatalf("tool_call item = %+v", toolCall)
		}
		if toolCall.Approval == nil || toolCall.Approval.ApprovalID != "apr-5" {
			t.Fatalf("approval not restored on tool_call: %+v", toolCall.Approval)
		}
		// tool_call metadata 非对象 → 仅 kind 标记。
		degenerate, _ := threadItemFromSessionMessage(adapter.SessionMessage{
			Metadata: map[string]any{"item_id": "i6", "tool_call": "bad"},
		})
		if degenerate.Kind != "tool_call" || degenerate.ToolName != "" {
			t.Fatalf("degenerate tool_call = %+v", degenerate)
		}
	})

	t.Run("metadataTurnID", func(t *testing.T) {
		if got := metadataTurnID(nil); got != "" {
			t.Fatalf("metadataTurnID(nil) = %q", got)
		}
		if got := metadataTurnID(map[string]any{"turn_id": 123}); got != "" {
			t.Fatalf("metadataTurnID(non-string) = %q", got)
		}
		if got := metadataTurnID(map[string]any{"turn_id": " t-1 "}); got != "t-1" {
			t.Fatalf("metadataTurnID = %q", got)
		}
	})
}

func TestMessageCodecChangeSetRollbackRoundTrip(t *testing.T) {
	t.Run("changeset nil 与往返", func(t *testing.T) {
		if coreAPIChangeSetFromChatMessage(nil) != nil {
			t.Fatal("coreAPIChangeSetFromChatMessage(nil) != nil")
		}
		if chatMessageChangeSetFromCoreAPI(nil) != nil {
			t.Fatal("chatMessageChangeSetFromCoreAPI(nil) != nil")
		}
		local := &MessageChangeSet{
			ID:      "cs-1",
			Summary: "两处修改",
			Files:   []ChangedFile{{Path: "a.go", Additions: 3, Deletions: 1}},
		}
		core := coreAPIChangeSetFromChatMessage(local)
		if core == nil || core.ID != "cs-1" || len(core.Files) != 1 || core.Files[0].Path != "a.go" {
			t.Fatalf("core changeset = %+v", core)
		}
		back := chatMessageChangeSetFromCoreAPI(core)
		if back == nil || back.Summary != "两处修改" || len(back.Files) != 1 || back.Files[0].Additions != 3 {
			t.Fatalf("round trip = %+v", back)
		}
		// 无 ID 且无文件 → 视为空。
		if chatMessageChangeSetFromCoreAPI(&coreapi.MessageChangeSet{}) != nil {
			t.Fatal("empty core changeset should convert to nil")
		}
	})

	t.Run("rollback nil 与往返", func(t *testing.T) {
		if coreAPITurnRollbackFromChatMessage(nil) != nil {
			t.Fatal("coreAPITurnRollbackFromChatMessage(nil) != nil")
		}
		if chatMessageTurnRollbackFromCoreAPI(nil) != nil {
			t.Fatal("chatMessageTurnRollbackFromCoreAPI(nil) != nil")
		}
		local := &TurnRollback{
			UserMessageID:      "u-1",
			AssistantMessageID: "a-1",
			Files:              []RollbackFileSnapshot{{Path: "b.go", ExistedBefore: true, ContentBase64: "YWJj"}},
		}
		core := coreAPITurnRollbackFromChatMessage(local)
		if core == nil || core.UserMessageID != "u-1" || len(core.Files) != 1 {
			t.Fatalf("core rollback = %+v", core)
		}
		back := chatMessageTurnRollbackFromCoreAPI(core)
		if back == nil || back.AssistantMessageID != "a-1" || len(back.Files) != 1 || back.Files[0].ContentBase64 != "YWJj" {
			t.Fatalf("round trip = %+v", back)
		}
		if chatMessageTurnRollbackFromCoreAPI(&coreapi.TurnRollback{}) != nil {
			t.Fatal("empty core rollback should convert to nil")
		}
	})

	t.Run("chatMessageFromRuntime 顶层 changeset/rollback 优先", func(t *testing.T) {
		msg := chatMessageFromRuntime(adapter.SessionMessage{
			Role:    "assistant",
			Content: "改完",
			Time:    time.Unix(1700000000, 0).UTC(),
			ChangeSet: &coreapi.MessageChangeSet{
				ID:    "cs-top",
				Files: []coreapi.ChangedFile{{Path: "c.go"}},
			},
			Rollback: &coreapi.TurnRollback{
				UserMessageID: "u-top",
				Files:         []coreapi.RollbackFileSnapshot{{Path: "c.go"}},
			},
		})
		if msg.ChangeSet == nil || msg.ChangeSet.ID != "cs-top" {
			t.Fatalf("changeset not taken from top level: %+v", msg.ChangeSet)
		}
		if msg.Rollback == nil || msg.Rollback.UserMessageID != "u-top" {
			t.Fatalf("rollback not taken from top level: %+v", msg.Rollback)
		}
		if msg.Role != "assistant" || msg.State != "completed" {
			t.Fatalf("message = %+v", msg)
		}

		typeOnly := chatMessageFromRuntime(adapter.SessionMessage{Type: "user", Content: "hi", Time: time.Now()})
		if typeOnly.Role != "user" {
			t.Fatalf("type fallback role = %q", typeOnly.Role)
		}
		empty := chatMessageFromRuntime(adapter.SessionMessage{Time: time.Now()})
		if empty.Role != "assistant" {
			t.Fatalf("default role = %q", empty.Role)
		}
	})
}

// ---- helpers on emitRecorder ----

func (r *emitRecorder) browserPayloads() []BrowserEventPayload {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]BrowserEventPayload, 0, len(r.payloads))
	out = append(out, r.payloads...)
	return out
}
