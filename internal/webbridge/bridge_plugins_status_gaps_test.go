package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// webbridge 批九批测：plugins 全家（CoreCallRPC 通道）/ finishConversation
// 终态分支与 prompt 收口 / ProbeInvoke·GetStatus / 更新下载循环（httptest）/
// server routes 注册 / posix pty 后端轻量真进程。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// ---- stub 扩展：CoreCallRPC / Invoke ----

func (g *chatSessionsGatewayStub) CoreCallRPC(_ context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.callRPCErr != nil {
		return nil, g.callRPCErr
	}
	g.callRPCs = append(g.callRPCs, [2]string{method, string(params)})
	return g.callRPCResult, nil
}

func (g *chatSessionsGatewayStub) Invoke(_ context.Context, _ string) (<-chan adapter.Event, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.invokeErr != nil {
		return nil, g.invokeErr
	}
	if g.invokeEvents == nil {
		g.invokeEvents = make(chan adapter.Event, 8)
	}
	return g.invokeEvents, nil
}

// ---- plugins 全家 ----

func TestPluginOperationsViaCoreCallRPC(t *testing.T) {
	newPluginBridge := func(t *testing.T) (*BridgeService, *chatSessionsGatewayStub) {
		gateway := &chatSessionsGatewayStub{}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		s := &BridgeService{
			runtimeGateway: gateway,
			sessions:       map[string]*sessionState{},
			prompts:        map[string]*promptState{},
			emitEvent:      func(string, any) {},
		}
		return s, gateway
	}

	t.Run("install/confirm/remove/search 转发", func(t *testing.T) {
		s, gateway := newPluginBridge(t)
		gateway.callRPCResult = json.RawMessage(`{"name":"demo","version":"1.0","installed":true,"mcp_registered":true,"skills_installed":["a"],"path":"/p"}`)
		result, err := s.PluginInstall(" /local/plugin ")
		if err != nil {
			t.Fatalf("PluginInstall error = %v", err)
		}
		if result.Name != "demo" || !result.Installed || len(result.SkillsInstalled) != 1 {
			t.Fatalf("result = %+v", result)
		}
		if _, err := s.PluginInstallConfirm("src"); err != nil {
			t.Fatalf("PluginInstallConfirm error = %v", err)
		}
		if err := s.PluginRemove(" demo "); err != nil {
			t.Fatalf("PluginRemove error = %v", err)
		}

		gateway.callRPCResult = json.RawMessage(`[{"name":"p1","enabled":true}]`)
		list, err := s.PluginList()
		if err != nil || len(list) != 1 || list[0].Name != "p1" {
			t.Fatalf("PluginList = %+v %v", list, err)
		}

		gateway.callRPCResult = json.RawMessage(`{"results":[{"name":"hit","tags":["t"]}],"total":1,"index_url":"https://idx"}`)
		search, err := s.PluginSearch(" query ")
		if err != nil || search.Total != 1 || len(search.Results) != 1 || search.Results[0].Name != "hit" {
			t.Fatalf("PluginSearch = %+v %v", search, err)
		}

		gateway.mu.Lock()
		calls := append([][2]string(nil), gateway.callRPCs...)
		gateway.mu.Unlock()
		methods := []string{}
		for _, c := range calls {
			methods = append(methods, c[0])
		}
		want := []string{"plugin/install", "plugin/install", "plugin/remove", "plugin/list", "plugin/search"}
		if strings.Join(methods, ",") != strings.Join(want, ",") {
			t.Fatalf("methods = %v", methods)
		}
		// source 原样透传（trim 由内核/前端负责），此处固化不裁剪行为。
		if !strings.Contains(calls[0][1], `"source":" /local/plugin "`) {
			t.Fatalf("install params = %s", calls[0][1])
		}
		if !strings.Contains(calls[1][1], `"confirm_permissions":true`) {
			t.Fatalf("confirm params = %s", calls[1][1])
		}
	})

	t.Run("RPC 失败与坏载荷包装", func(t *testing.T) {
		s, gateway := newPluginBridge(t)
		gateway.callRPCErr = errors.New("core down")
		if _, err := s.PluginInstall("x"); err == nil || !strings.Contains(err.Error(), "插件安装失败") {
			t.Fatalf("install failure = %v", err)
		}
		if _, err := s.PluginInstallConfirm("x"); err == nil {
			t.Fatal("confirm failure = nil")
		}
		if err := s.PluginRemove("x"); err == nil || !strings.Contains(err.Error(), "插件卸载失败") {
			t.Fatalf("remove failure = %v", err)
		}
		if _, err := s.PluginList(); err == nil {
			t.Fatal("list failure = nil")
		}
		if _, err := s.PluginSearch("x"); err == nil {
			t.Fatal("search failure = nil")
		}

		gateway.callRPCErr = nil
		gateway.callRPCResult = json.RawMessage(`not-json`)
		if _, err := s.PluginInstall("x"); err == nil || !strings.Contains(err.Error(), "解析安装结果失败") {
			t.Fatalf("bad install payload = %v", err)
		}
		if _, err := s.PluginList(); err == nil || !strings.Contains(err.Error(), "解析插件列表失败") {
			t.Fatalf("bad list payload = %v", err)
		}
		if _, err := s.PluginSearch("x"); err == nil || !strings.Contains(err.Error(), "解析搜索结果失败") {
			t.Fatalf("bad search payload = %v", err)
		}
	})

	t.Run("无 gateway 拒绝", func(t *testing.T) {
		s := &BridgeService{}
		if _, err := s.PluginInstall("x"); err == nil {
			t.Fatal("PluginInstall without gateway error = nil")
		}
	})
}

// ---- finishConversation 终态分支 ----

func newFinishCtx(t *testing.T) (*BridgeService, *sessionState) {
	t.Helper()
	gateway := &chatSessionsGatewayStub{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	s := &BridgeService{
		runtimeGateway:       gateway,
		sessions:             map[string]*sessionState{},
		runningConversations: map[string]*runningConversationState{},
		prompts:              map[string]*promptState{},
		emitEvent:            func(string, any) {},
	}
	session := &sessionState{
		ID:       "sess-f",
		Title:    "会话",
		Running:  true,
		Messages: []ChatMessage{{ID: "a-f", Role: "assistant", State: "streaming", IsPlaceholder: true}},
	}
	s.sessions["sess-f"] = session
	s.runningConversations["sess-f"] = &runningConversationState{AssistantMessageID: "a-f"}
	return s, session
}

func TestFinishConversationTerminalBranches(t *testing.T) {
	t.Run("正常完成（占位已被 text.final 归一）", func(t *testing.T) {
		s, session := newFinishCtx(t)
		findSessionMessageByID(session, "a-f").State = "completed"
		s.finishConversation("sess-f", "a-f", context.Background())
		if session.Running {
			t.Fatal("session still running after finish")
		}
		if session.NeedsAttention {
			t.Fatal("completed finish should not flag attention")
		}
		s.stateMu.RLock()
		_, running := s.runningConversations["sess-f"]
		s.stateMu.RUnlock()
		if running {
			t.Fatal("running conversation not removed")
		}
	})

	t.Run("用户取消分支", func(t *testing.T) {
		s, session := newFinishCtx(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s.finishConversation("sess-f", "a-f", ctx)
		if session.Running || session.NeedsAttention {
			t.Fatalf("cancel branch state wrong: running=%v attention=%v", session.Running, session.NeedsAttention)
		}
		msg := findSessionMessageByID(session, "a-f")
		if msg.State != "failed" || !strings.Contains(msg.Items[len(msg.Items)-1].Text, "已停止") {
			t.Fatalf("cancel message = %+v", msg)
		}
	})

	t.Run("超时分支", func(t *testing.T) {
		s, session := newFinishCtx(t)
		ctx, cancel := context.WithTimeout(context.Background(), -time.Second)
		defer cancel()
		<-ctx.Done()
		s.finishConversation("sess-f", "a-f", ctx)
		if !session.NeedsAttention {
			t.Fatal("timeout should flag attention")
		}
		msg := findSessionMessageByID(session, "a-f")
		if msg.State != "failed" {
			t.Fatalf("timeout message state = %q", msg.State)
		}
	})

	t.Run("watchdog 静默分支", func(t *testing.T) {
		s, session := newFinishCtx(t)
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errTurnWatchdogTripped)
		s.finishConversation("sess-f", "a-f", ctx)
		msg := findSessionMessageByID(session, "a-f")
		if !session.NeedsAttention || msg.State != "failed" {
			t.Fatalf("watchdog state = attention=%v msg=%+v", session.NeedsAttention, msg)
		}
		found := false
		for _, item := range msg.Items {
			if strings.Contains(item.Text, "会话无响应") {
				found = true
			}
		}
		if !found {
			t.Fatalf("watchdog notice missing: %+v", msg.Items)
		}
	})

	t.Run("流异常结束分支收起挂起审批", func(t *testing.T) {
		s, session := newFinishCtx(t)
		s.prompts["apr-x"] = &promptState{PromptCard: PromptCard{ID: "apr-x", SessionID: "sess-f"}, AssistantMessageID: "a-f"}
		s.finishConversation("sess-f", "a-f", context.Background()) // ctx 无错 → default 流异常分支
		if _, still := s.prompts["apr-x"]; still {
			t.Fatal("pending prompt not dismissed on stream failure")
		}
		if !session.NeedsAttention {
			t.Fatal("stream failure should flag attention")
		}
	})

	t.Run("会话未跑时的终态幂等", func(t *testing.T) {
		s, session := newFinishCtx(t)
		session.Running = false
		findSessionMessageByID(session, "a-f").State = "completed"
		s.finishConversation("sess-f", "a-f", context.Background())
		// 非 running + 已终态：不应追加异常事件。
		msg := findSessionMessageByID(session, "a-f")
		if msg.State != "completed" {
			t.Fatalf("terminal message flipped: %q", msg.State)
		}
	})

	t.Run("不存在会话不 panic", func(t *testing.T) {
		s, _ := newFinishCtx(t)
		s.finishConversation("missing", "nope", context.Background())
	})
}

func TestDismissPendingPromptsLocked(t *testing.T) {
	s, session := newFinishCtx(t)
	s.prompts["apr-1"] = &promptState{PromptCard: PromptCard{ID: "apr-1", SessionID: "sess-f"}, AssistantMessageID: "a-f"}
	s.prompts["apr-other"] = &promptState{PromptCard: PromptCard{ID: "apr-other", SessionID: "sess-2"}, AssistantMessageID: "a-f"}
	s.dismissPendingPromptsLocked(" sess-f ")
	if _, mine := s.prompts["apr-1"]; mine {
		t.Fatal("session prompt not dismissed")
	}
	if _, other := s.prompts["apr-other"]; !other {
		t.Fatal("other-session prompt dismissed incorrectly")
	}
	// 状态行被改写为失效提示。
	msg := findSessionMessageByID(session, "a-f")
	found := false
	for _, item := range msg.Items {
		if strings.Contains(item.Text, "审批失效") {
			found = true
		}
	}
	if !found {
		t.Fatalf("invalidation notice missing: %+v", msg.Items)
	}
	// 空 sessionID 早退。
	s.dismissPendingPromptsLocked("  ")
}

// ---- ProbeInvoke / GetStatus ----

func TestProbeInvokeArms(t *testing.T) {
	t.Run("nil bridge / 空 input 默认提示语", func(t *testing.T) {
		probe := NewSystemService(nil).ProbeInvoke("  x  ")
		if probe.Source != "unavailable" || probe.Input != "x" {
			t.Fatalf("probe = %+v", probe)
		}
	})

	t.Run("Invoke 失败返回错误探针", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{invokeErr: errors.New("no engine")}
		s := &BridgeService{runtimeGateway: gateway}
		probe := NewSystemService(s).ProbeInvoke("")
		if probe.Error == "" || !strings.Contains(probe.Input, "summarize") {
			t.Fatalf("probe = %+v", probe)
		}
	})

	t.Run("事件流驱动完成", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		ch := make(chan adapter.Event, 8)
		gateway.invokeEvents = ch
		s := &BridgeService{runtimeGateway: gateway}
		ch <- adapter.Event{Type: "turn.item_started", Data: map[string]any{"message": "工作"}}
		ch <- adapter.Event{EventType: "text.final", Data: map[string]any{"message": "完成"}}
		close(ch)
		probe := NewSystemService(s).ProbeInvoke("诊断")
		if !probe.Completed || len(probe.Events) != 2 {
			t.Fatalf("probe = %+v", probe)
		}
		if probe.Events[1].EventType != "text.final" {
			t.Fatalf("events = %+v", probe.Events)
		}
	})

	t.Run("无 gateway 探针降级", func(t *testing.T) {
		s := &BridgeService{}
		probe := NewSystemService(s).ProbeInvoke("x")
		if probe.Error == "" {
			t.Fatal("probe without gateway should carry error")
		}
	})
}

func TestGetStatusProjection(t *testing.T) {
	if got := NewSystemService(nil).GetStatus(); got.BridgeMode != "" && got.SessionCount != 0 {
		t.Fatalf("GetStatus(nil) = %+v", got)
	}
	gateway := &chatSessionsGatewayStub{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := &BridgeService{
		runtimeGateway: gateway,
		sessions:       map[string]*sessionState{"a": {}, "b": {}},
		prompts:        map[string]*promptState{},
		notifications:  []NotificationItem{{ID: "n1"}},
		emitEvent:      func(string, any) {},
	}
	status := NewSystemService(s).GetStatus()
	if status.SessionCount != 2 || status.NotificationCount != 1 {
		t.Fatalf("status = %+v", status)
	}
	if status.Uptime == "" {
		t.Fatal("uptime missing")
	}
}

// ---- 更新下载循环 ----

func TestRunUpdateDownloadLifecycle(t *testing.T) {
	newDownloadBridge := func(t *testing.T) *BridgeService {
		t.Helper()
		gateway := &chatSessionsGatewayStub{}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		return &BridgeService{
			runtimeGateway: gateway,
			sessions:       map[string]*sessionState{},
			prompts:        map[string]*promptState{},
			emitEvent:      func(string, any) {},
		}
	}

	t.Run("下载失败走 failed", func(t *testing.T) {
		s := newDownloadBridge(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gone", http.StatusNotFound)
		}))
		defer server.Close()
		s.runUpdateDownload(context.Background(), server.Client(), UpdateCheckResult{DownloadURL: server.URL + "/x.zip"}, t.TempDir()+"/x.zip")
		if state := s.GetUpdateDownloadState(); state.Stage != updateStageFailed || !strings.Contains(state.Error, "下载失败") {
			t.Fatalf("state = %+v", state)
		}
	})

	t.Run("digest 校验失败丢弃文件", func(t *testing.T) {
		s := newDownloadBridge(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("payload"))
		}))
		defer server.Close()
		dest := t.TempDir() + "/x.zip"
		s.runUpdateDownload(context.Background(), server.Client(), UpdateCheckResult{
			DownloadURL: server.URL + "/x.zip",
			AssetDigest: strings.Repeat("0", 64),
		}, dest)
		if state := s.GetUpdateDownloadState(); state.Stage != updateStageFailed || !strings.Contains(state.Error, "完整性校验失败") {
			t.Fatalf("state = %+v", state)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatalf("downloaded file should be removed on digest failure, err = %v", err)
		}
	})

	t.Run("成功到 ready", func(t *testing.T) {
		s := newDownloadBridge(t)
		payload := []byte("update-package-bytes")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(payload)
		}))
		defer server.Close()
		digest := sha256.Sum256(payload)
		sum := hex.EncodeToString(digest[:])
		dest := t.TempDir() + "/ok.zip"
		s.runUpdateDownload(context.Background(), server.Client(), UpdateCheckResult{
			DownloadURL:    server.URL + "/ok.zip",
			AssetDigest:    sum,
			AssetSizeBytes: int64(len(payload)),
		}, dest)
		state := s.GetUpdateDownloadState()
		if state.Stage != updateStageReady || state.Percent != 100 || state.LocalPath != dest {
			t.Fatalf("state = %+v", state)
		}
	})

	t.Run("downloadPercent 封顶 99", func(t *testing.T) {
		if got := downloadPercent(100, 100); got != 99 {
			t.Fatalf("downloadPercent(100,100) = %d", got)
		}
		if got := downloadPercent(50, 200); got != 25 {
			t.Fatalf("downloadPercent(50,200) = %d", got)
		}
		if got := downloadPercent(10, 0); got != 0 {
			t.Fatalf("downloadPercent(10,0) = %d", got)
		}
	})
}

// ---- server routes ----

func TestServerRoutesRegistered(t *testing.T) {
	s := &Server{bridge: &BridgeService{}, hub: newEventHub()}
	handler := s.routes()
	if handler == nil {
		t.Fatal("routes() nil")
	}
	// 注册存在性验证：GET-only 路由用 POST 打 → 405（方法不匹配）；
	// 注意 "GET /" 静态兜底会吃掉所有 GET，故不能用 GET 判定 404。
	for _, path := range []string{"/wails/runtime.js", AttachmentImageRoutePath, BrowserFrameRoutePath} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: POST code = %d, want 405 (route registered)", path, rec.Code)
		}
	}
	// POST /wails/call 已注册：POST 到达 handler（解析失败 400 而非 404/405）。
	callReq := httptest.NewRequest(http.MethodPost, "/wails/call", strings.NewReader(`[]`))
	callRec := httptest.NewRecorder()
	handler.ServeHTTP(callRec, callReq)
	if callRec.Code == http.StatusNotFound || callRec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("POST /wails/call code = %d — route not registered", callRec.Code)
	}
	// WS 升级失败应达 handler（400/200 皆可，非 404）。
	wsReq := httptest.NewRequest(http.MethodGet, "/wails/ws", nil)
	wsRec := httptest.NewRecorder()
	handler.ServeHTTP(wsRec, wsReq)
	if wsRec.Code == http.StatusNotFound {
		t.Fatalf("GET /wails/ws code = 404 — route not registered")
	}
}

// ---- posix pty 后端（轻量真进程） ----

func TestPosixPtyBackendLifecycle(t *testing.T) {
	backend, err := startBridgeTerminalBackend(t.TempDir(), 80, 24)
	if err != nil {
		t.Skipf("pty unavailable in this environment: %v", err)
	}
	if err := backend.Resize(100, 30); err != nil {
		t.Fatalf("Resize error = %v", err)
	}
	if _, err := backend.Write([]byte("\n")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	// 读回显（bash 启动横幅/回显，轮询至读到任意字节）。
	buf := make([]byte, 256)
	deadline := time.Now().Add(3 * time.Second)
	read := 0
	for time.Now().Before(deadline) && read == 0 {
		n, readErr := backend.Read(buf)
		if n > 0 {
			read += n
			break
		}
		if readErr != nil {
			t.Fatalf("Read error = %v", readErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if read == 0 {
		t.Log("pty produced no output within 3s; lifecycle still verified via write/resize/close")
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	waitErr := backend.Wait(context.Background())
	// Close 发 SIGHUP：bash 以 signal: hangup 退出属正常收尾（生产侧忽略
	// Wait 返回值），此处同样接受。
	if waitErr != nil && !shouldIgnoreTerminalProcessError(waitErr) && !strings.Contains(waitErr.Error(), "hangup") {
		t.Fatalf("Wait error = %v", waitErr)
	}
}

func TestShouldIgnoreTerminalProcessError(t *testing.T) {
	if !shouldIgnoreTerminalProcessError(nil) {
		t.Fatal("nil should be ignored")
	}
	if !shouldIgnoreTerminalProcessError(os.ErrProcessDone) {
		t.Fatal("process done should be ignored")
	}
	if shouldIgnoreTerminalProcessError(errors.New("boom")) {
		t.Fatal("generic error should not be ignored")
	}
	// context 错误不属「进程已终结」词表，不忽略。
	if shouldIgnoreTerminalProcessError(context.Canceled) {
		t.Fatal("context.Canceled should not be ignored")
	}
}
