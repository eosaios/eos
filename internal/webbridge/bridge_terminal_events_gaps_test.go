package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// webbridge 批五批测：终端域全链（创建/关闭/写/resize/输出流/状态快照）+
// conversation_events 缺口 handler（item_completed / failure / text.final /
// turn.completed / token_usage / started / default 分支）。

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// ---- 终端 fake backend ----

type fakeTerminalBackend struct {
	mu       sync.Mutex
	chunks   []string // Read 依次吐出的数据块
	readErr  error    // 数据耗尽后返回（nil 视为 io.EOF）
	written  []string
	resized  [][2]int
	closed   int
	closeErr error
	waitErr  error
}

func (b *fakeTerminalBackend) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.chunks) > 0 {
		chunk := b.chunks[0]
		b.chunks = b.chunks[1:]
		n := copy(p, chunk)
		return n, nil
	}
	if b.readErr != nil {
		return 0, b.readErr
	}
	return 0, io.EOF
}

func (b *fakeTerminalBackend) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.written = append(b.written, string(p))
	return len(p), nil
}

func (b *fakeTerminalBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed++
	return b.closeErr
}

func (b *fakeTerminalBackend) Resize(cols, rows int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resized = append(b.resized, [2]int{cols, rows})
	return nil
}

func (b *fakeTerminalBackend) Wait(context.Context) error { return b.waitErr }

// newTerminalTestBridge 构造带 fake launcher 与终端会话表的桥。
func newTerminalTestBridge(t *testing.T, backend *fakeTerminalBackend, launchErr error) (*BridgeService, *TerminalService, *emitRecorder) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	gateway := &chatSessionsGatewayStub{defaultWorkspace: home}
	rec := &emitRecorder{}
	s := &BridgeService{
		runtimeGateway:   gateway,
		sessions:         map[string]*sessionState{},
		prompts:          map[string]*promptState{},
		terminalSessions: map[string]*terminalSessionHandle{},
		emitEvent:        rec.record,
	}
	s.terminalLauncher = func(string, int, int) (bridgeTerminalBackend, error) {
		if launchErr != nil {
			return nil, launchErr
		}
		return backend, nil
	}
	return s, NewTerminalService(s), rec
}

// ---- 终端 runtime helper ----

func TestTerminalRuntimeHelpers(t *testing.T) {
	if got := formatTerminalSessionTitle("  ", 2); got != "bash 2" {
		t.Fatalf("formatTerminalSessionTitle(blank) = %q", got)
	}
	if got := formatTerminalSessionTitle(string(os.PathSeparator), 3); got != "bash 3" {
		t.Fatalf("formatTerminalSessionTitle(sep) = %q", got)
	}
	if got := formatTerminalSessionTitle("/ws/proj", 4); got != "proj · bash 4" {
		t.Fatalf("formatTerminalSessionTitle = %q", got)
	}

	if got := latestTerminalSessionIDLocked(nil); got != "" {
		t.Fatalf("latestTerminalSessionIDLocked(nil) = %q", got)
	}
	items := map[string]*terminalSessionHandle{
		"t-1": {TerminalSessionCard: TerminalSessionCard{ID: "t-1"}, order: 1},
		"t-3": {TerminalSessionCard: TerminalSessionCard{ID: "t-3"}, order: 3},
		"t-2": {TerminalSessionCard: TerminalSessionCard{ID: "t-2"}, order: 2},
	}
	if got := latestTerminalSessionIDLocked(items); got != "t-3" {
		t.Fatalf("latestTerminalSessionIDLocked = %q, want t-3", got)
	}
	// nil 成员跳过。
	withNil := map[string]*terminalSessionHandle{"t-9": nil}
	if got := latestTerminalSessionIDLocked(withNil); got != "" {
		t.Fatalf("latestTerminalSessionIDLocked(nil member) = %q", got)
	}
}

func TestTerminalStateLockedSortsAndProjects(t *testing.T) {
	s, _, _ := newTerminalTestBridge(t, nil, nil)
	s.stateMu.Lock()
	s.terminalSessions["t-2"] = &terminalSessionHandle{TerminalSessionCard: TerminalSessionCard{ID: "t-2", Title: "b"}, order: 2}
	s.terminalSessions["t-1"] = &terminalSessionHandle{TerminalSessionCard: TerminalSessionCard{ID: "t-1", Title: "a"}, order: 1}
	s.terminalActiveSessionID = "t-2"
	state := s.terminalStateLocked()
	s.stateMu.Unlock()

	if len(state.Sessions) != 2 || state.Sessions[0].ID != "t-1" || state.Sessions[1].ID != "t-2" {
		t.Fatalf("sessions order = %+v", state.Sessions)
	}
	if state.ActiveSessionID != "t-2" {
		t.Fatalf("ActiveSessionID = %q", state.ActiveSessionID)
	}
	if state.Shell == nil {
		t.Fatal("Shell snapshot missing")
	}
}

func TestResolveTerminalWorkspaceFallbackChain(t *testing.T) {
	gateway := &chatSessionsGatewayStub{}
	s, _, _ := newTerminalTestBridge(t, nil, nil)
	// 直接换掉 gateway 控制 foreground RPC 返回。
	s.runtimeGateway = gateway

	// 显式路径最优先。
	if got := s.resolveTerminalWorkspace(" /explicit "); got != "/explicit" {
		t.Fatalf("explicit = %q", got)
	}

	// 回退 activeWorkspace。
	s.stateMu.Lock()
	s.activeWorkspace = "/active"
	s.stateMu.Unlock()
	if got := s.resolveTerminalWorkspace(""); got != "/active" {
		t.Fatalf("activeWorkspace fallback = %q", got)
	}

	// activeWorkspace 空 → foreground RPC。
	s.stateMu.Lock()
	s.activeWorkspace = ""
	s.stateMu.Unlock()
	gateway.foreground = "/foreground"
	if got := s.resolveTerminalWorkspace(""); got != "/foreground" {
		t.Fatalf("foreground fallback = %q", got)
	}

	// foreground 不可用 → 默认工作区（HOME/.eos/workspace，ensure 兜底创建）。
	gateway.foreground = ""
	if got := s.resolveTerminalWorkspace(""); got == "" {
		t.Fatal("default workspace fallback empty")
	}
}

// ---- 终端生命周期 ----

func TestTerminalLifecycleArms(t *testing.T) {
	t.Run("nil bridge 与空 workspace", func(t *testing.T) {
		if _, err := NewTerminalService(nil).CreateTerminalSession(""); err == nil {
			t.Fatal("CreateTerminalSession(nil) error = nil")
		}
		backend := &fakeTerminalBackend{}
		s, svc, _ := newTerminalTestBridge(t, backend, nil)
		// 空 workspace 无任何显式来源时兜底到 HOME/.eos/workspace（ensure 兜底
		// 创建，总是成功）——固化「空输入不拒绝而是落默认工作区」语义。
		gateway := &chatSessionsGatewayStub{}
		s.runtimeGateway = gateway
		state, err := svc.CreateTerminalSession("")
		if err != nil {
			t.Fatalf("CreateTerminalSession('') error = %v", err)
		}
		// Windows 下 Cwd 是反斜杠分隔：统一 ToSlash 后断言后缀。
		if !strings.HasSuffix(filepath.ToSlash(state.Sessions[0].Cwd), ".eos/workspace") {
			t.Fatalf("empty workspace should fall back to default, cwd = %q", state.Sessions[0].Cwd)
		}
		if _, err := NewTerminalService(nil).CloseTerminalSession("x"); err == nil {
			t.Fatal("CloseTerminalSession(nil) error = nil")
		}
	})

	t.Run("无 launcher 与 launcher 失败", func(t *testing.T) {
		s, svc, _ := newTerminalTestBridge(t, nil, nil)
		s.terminalLauncher = nil
		if _, err := svc.CreateTerminalSession("/ws"); err == nil || !strings.Contains(err.Error(), "launcher unavailable") {
			t.Fatalf("CreateTerminalSession(no launcher) error = %v", err)
		}

		_, svc2, _ := newTerminalTestBridge(t, nil, errors.New("no pty"))
		if _, err := svc2.CreateTerminalSession("/ws"); err == nil || !strings.Contains(err.Error(), "no pty") {
			t.Fatalf("CreateTerminalSession(launch error) error = %v", err)
		}
	})

	t.Run("创建成功挂会话并起输出流", func(t *testing.T) {
		backend := &fakeTerminalBackend{chunks: []string{"hello\r\n"}}
		s, svc, rec := newTerminalTestBridge(t, backend, nil)
		state, err := svc.CreateTerminalSession("/ws/proj")
		if err != nil {
			t.Fatalf("CreateTerminalSession error = %v", err)
		}
		if len(state.Sessions) != 1 || state.Sessions[0].Status != "running" {
			t.Fatalf("state = %+v", state)
		}
		if state.Sessions[0].Title != "proj · bash 1" {
			t.Fatalf("title = %q", state.Sessions[0].Title)
		}
		sessionID := state.Sessions[0].ID
		eventually(t, "terminal output emitted", func() bool { return rec.has(terminalOutputEventName) })
		// 流读到 EOF 后会话转 exited。
		eventually(t, "terminal exited", func() bool {
			s.stateMu.RLock()
			defer s.stateMu.RUnlock()
			handle := s.terminalSessions[sessionID]
			return handle != nil && handle.Status == "exited"
		})
	})

	t.Run("读错误标记 failed", func(t *testing.T) {
		backend := &fakeTerminalBackend{readErr: errors.New("pty broken")}
		s, svc, _ := newTerminalTestBridge(t, backend, nil)
		state, err := svc.CreateTerminalSession("/ws")
		if err != nil {
			t.Fatalf("CreateTerminalSession error = %v", err)
		}
		sessionID := state.Sessions[0].ID
		eventually(t, "terminal failed", func() bool {
			s.stateMu.RLock()
			defer s.stateMu.RUnlock()
			handle := s.terminalSessions[sessionID]
			return handle != nil && handle.Status == "failed"
		})
	})

	t.Run("close 不存在与成功切换 active", func(t *testing.T) {
		backend := &fakeTerminalBackend{}
		_, svc, rec := newTerminalTestBridge(t, backend, nil)
		if _, err := svc.CloseTerminalSession("missing"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("CloseTerminalSession(missing) error = %v", err)
		}
		if _, err := svc.CloseTerminalSession("  "); err == nil || !strings.Contains(err.Error(), "id required") {
			t.Fatalf("CloseTerminalSession('') error = %v", err)
		}

		first, err := svc.CreateTerminalSession("/ws")
		if err != nil {
			t.Fatal(err)
		}
		firstID := first.ActiveSessionID
		second, err := svc.CreateTerminalSession("/ws")
		if err != nil {
			t.Fatal(err)
		}
		// 关掉第二个（当前 active）→ active 应回切到第一个。
		state, err := svc.CloseTerminalSession(second.ActiveSessionID)
		if err != nil {
			t.Fatalf("CloseTerminalSession error = %v", err)
		}
		if state.ActiveSessionID != firstID {
			t.Fatalf("ActiveSessionID = %q, want first session %q", state.ActiveSessionID, firstID)
		}
		backend.mu.Lock()
		closed := backend.closed
		backend.mu.Unlock()
		if closed != 1 {
			t.Fatalf("backend closed = %d, want 1", closed)
		}
		// 两次创建 + 一次 close 各 emit 一波 shellUpdated（goroutine 内
		// loadBootstrap 写 HOME 目录），全部落地后再结束防清理竞态。
		eventually(t, "close emits settled", func() bool { return rec.count() >= 3 })
	})

	t.Run("closeAll 清空并关后端", func(t *testing.T) {
		backend := &fakeTerminalBackend{}
		s, svc, rec := newTerminalTestBridge(t, backend, nil)
		if _, err := svc.CreateTerminalSession("/ws"); err != nil {
			t.Fatal(err)
		}
		svc.CloseAllTerminalSessions()
		s.stateMu.RLock()
		count := len(s.terminalSessions)
		active := s.terminalActiveSessionID
		s.stateMu.RUnlock()
		if count != 0 || active != "" {
			t.Fatalf("after closeAll: count=%d active=%q", count, active)
		}
		backend.mu.Lock()
		closed := backend.closed
		backend.mu.Unlock()
		if closed != 1 {
			t.Fatalf("backend closed = %d, want 1", closed)
		}
		// 创建 + closeAll 的 emit（goroutine 内 loadBootstrap 写 HOME 目录）
		// 落地后再结束防清理竞态。
		eventually(t, "closeAll emits settled", func() bool { return rec.count() >= 1 })
	})
}

// ---- 终端控制 ----

func TestTerminalControlArms(t *testing.T) {
	t.Run("write 校验与转发", func(t *testing.T) {
		backend := &fakeTerminalBackend{}
		s, svc, _ := newTerminalTestBridge(t, backend, nil)
		if err := NewTerminalService(nil).WriteTerminalInput("t", "x"); err == nil {
			t.Fatal("WriteTerminalInput(nil) error = nil")
		}
		if err := svc.WriteTerminalInput("  ", "x"); err == nil || !strings.Contains(err.Error(), "id required") {
			t.Fatalf("WriteTerminalInput('') error = %v", err)
		}
		if err := svc.WriteTerminalInput("t-1", ""); err != nil {
			t.Fatalf("WriteTerminalInput(empty data) error = %v", err)
		}
		if err := svc.WriteTerminalInput("missing", "x"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("WriteTerminalInput(missing) error = %v", err)
		}

		s.stateMu.Lock()
		s.terminalSessions["t-1"] = &terminalSessionHandle{TerminalSessionCard: TerminalSessionCard{ID: "t-1"}, backend: backend}
		s.stateMu.Unlock()
		if err := svc.WriteTerminalInput("t-1", "ls\r"); err != nil {
			t.Fatalf("WriteTerminalInput error = %v", err)
		}
		backend.mu.Lock()
		written := strings.Join(backend.written, "")
		backend.mu.Unlock()
		if written != "ls\r" {
			t.Fatalf("written = %q", written)
		}
		s.stateMu.RLock()
		active := s.terminalActiveSessionID
		s.stateMu.RUnlock()
		if active != "t-1" {
			t.Fatalf("write should mark session active, got %q", active)
		}
	})

	t.Run("resize 校验与转发", func(t *testing.T) {
		backend := &fakeTerminalBackend{}
		s, svc, _ := newTerminalTestBridge(t, backend, nil)
		if err := NewTerminalService(nil).ResizeTerminalSession("t", 80, 24); err == nil {
			t.Fatal("ResizeTerminalSession(nil) error = nil")
		}
		if err := svc.ResizeTerminalSession(" ", 80, 24); err == nil || !strings.Contains(err.Error(), "id required") {
			t.Fatalf("ResizeTerminalSession('') error = %v", err)
		}
		// 非法尺寸静默忽略（返回 nil）。
		if err := svc.ResizeTerminalSession("t-1", 0, 24); err != nil {
			t.Fatalf("ResizeTerminalSession(cols=0) error = %v", err)
		}
		if err := svc.ResizeTerminalSession("t-1", 80, -1); err != nil {
			t.Fatalf("ResizeTerminalSession(rows<0) error = %v", err)
		}
		if err := svc.ResizeTerminalSession("missing", 80, 24); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ResizeTerminalSession(missing) error = %v", err)
		}

		s.stateMu.Lock()
		s.terminalSessions["t-1"] = &terminalSessionHandle{TerminalSessionCard: TerminalSessionCard{ID: "t-1"}, backend: backend}
		s.stateMu.Unlock()
		if err := svc.ResizeTerminalSession("t-1", 100, 30); err != nil {
			t.Fatalf("ResizeTerminalSession error = %v", err)
		}
		backend.mu.Lock()
		resizes := append([][2]int(nil), backend.resized...)
		backend.mu.Unlock()
		if len(resizes) != 1 || resizes[0] != [2]int{100, 30} {
			t.Fatalf("resized = %v", resizes)
		}
	})
}

func TestFinishTerminalSessionArms(t *testing.T) {
	backend := &fakeTerminalBackend{}
	s, _, rec := newTerminalTestBridge(t, backend, nil)
	// 不存在的会话早退（不 emit）。
	s.finishTerminalSession("missing", "exited")
	if rec.has(shellUpdatedEventName) {
		t.Fatal("finishTerminalSession(missing) should not emit")
	}

	s.stateMu.Lock()
	s.terminalSessions["t-1"] = &terminalSessionHandle{TerminalSessionCard: TerminalSessionCard{ID: "t-1"}, backend: backend}
	s.stateMu.Unlock()
	s.finishTerminalSession("t-1", "exited")
	s.stateMu.RLock()
	status := s.terminalSessions["t-1"].Status
	s.stateMu.RUnlock()
	if status != "exited" {
		t.Fatalf("status = %q", status)
	}
	// emitShellUpdated 走 goroutine，等事件落地再断言。
	eventually(t, "shellUpdated after terminal finish", func() bool { return rec.has(shellUpdatedEventName) })
}

// ---- conversation events 缺口 handler ----

type eventsTestCtx struct {
	s       *BridgeService
	rec     *emitRecorder
	session *sessionState
}

func newEventsTestCtx(t *testing.T) *eventsTestCtx {
	t.Helper()
	gateway := &chatSessionsGatewayStub{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	rec := &emitRecorder{}
	s := &BridgeService{
		runtimeGateway: gateway,
		sessions:       map[string]*sessionState{},
		prompts:        map[string]*promptState{},
		emitEvent:      rec.record,
	}
	session := &sessionState{
		ID:    "sess-1",
		Title: "新对话",
		Messages: []ChatMessage{
			{ID: "u1", Role: "user", Content: "hi"},
			{ID: "a1", Role: "assistant", Content: "思考中", State: "streaming", IsPlaceholder: true},
		},
		Running: true,
	}
	s.sessions["sess-1"] = session
	return &eventsTestCtx{s: s, rec: rec, session: session}
}

func (c *eventsTestCtx) frame(kind, message string, event adapter.Event) conversationEventFrame {
	return conversationEventFrame{
		session:            c.session,
		sessionID:          "sess-1",
		assistantMessageID: "a1",
		input:              "hi",
		source:             "runtime",
		kind:               kind,
		message:            message,
		event:              event,
	}
}

func TestHandleConversationFailureArms(t *testing.T) {
	t.Run("手动取消分支", func(t *testing.T) {
		ctx := newEventsTestCtx(t)
		ctx.s.runningConversations = map[string]*runningConversationState{
			"sess-1": {AssistantMessageID: "a1", Cancelled: true},
		}
		result := ctx.s.handleConversationEventLocked(ctx.frame("request.failed", "boom", adapter.Event{}))
		if !result.persist {
			t.Fatal("failure should persist")
		}
		msg := findSessionMessageByID(ctx.session, "a1")
		if msg.State != "failed" || msg.IsPlaceholder {
			t.Fatalf("message = %+v", msg)
		}
		if ctx.session.NeedsAttention {
			t.Fatal("manual cancel should not flag NeedsAttention")
		}
		if !strings.Contains(msg.Items[len(msg.Items)-1].Text, "停止") {
			t.Fatalf("cancel notice missing: %+v", msg.Items)
		}
	})

	t.Run("普通失败分支", func(t *testing.T) {
		ctx := newEventsTestCtx(t)
		ctx.s.handleConversationEventLocked(ctx.frame("turn.error", "model 403", adapter.Event{}))
		if !ctx.session.NeedsAttention {
			t.Fatal("failure should flag NeedsAttention")
		}
		msg := findSessionMessageByID(ctx.session, "a1")
		if msg.State != "failed" {
			t.Fatalf("message state = %q", msg.State)
		}
		ctx.s.stateMu.RLock()
		notifications := len(ctx.s.notifications)
		ctx.s.stateMu.RUnlock()
		if notifications == 0 {
			t.Fatal("failure should push notification")
		}
	})
}

func TestHandleConversationTextFinalAndTurnCompleted(t *testing.T) {
	t.Run("text.final 归一并补标题", func(t *testing.T) {
		ctx := newEventsTestCtx(t)
		ctx.s.handleConversationEventLocked(ctx.frame("text.final", "回答完毕", adapter.Event{}))
		msg := findSessionMessageByID(ctx.session, "a1")
		if ctx.session.Running || msg.State != "completed" {
			t.Fatalf("session running=%v msg=%+v", ctx.session.Running, msg)
		}
		if isAutoSessionPlaceholderTitle(ctx.session.Title) {
			t.Fatalf("auto title not filled: %q", ctx.session.Title)
		}
	})

	t.Run("turn.completed 读取 payload", func(t *testing.T) {
		ctx := newEventsTestCtx(t)
		event := adapter.Event{Payload: map[string]any{"reason": "stop"}}
		ctx.s.handleConversationEventLocked(ctx.frame("turn.completed", "", event))
		msg := findSessionMessageByID(ctx.session, "a1")
		if msg.State != "completed" {
			t.Fatalf("message state = %q", msg.State)
		}
		if ctx.session.Running {
			t.Fatal("session still running after turn.completed")
		}
	})
}

func TestHandleConversationTokenUsageForwarding(t *testing.T) {
	ctx := newEventsTestCtx(t)
	event := adapter.Event{
		TurnID: "t-1",
		Payload: map[string]any{
			"prompt_tokens":     100,
			"completion_tokens": 40,
			"total_tokens":      140,
		},
	}
	result := ctx.s.handleConversationEventLocked(ctx.frame("turn.token_usage", "", event))
	if result.persist {
		t.Fatal("token_usage must not persist")
	}
	ctx.rec.mu.Lock()
	var usage TurnUsagePayload
	found := false
	// emitRecorder 只记 browser payload，这里直接从事件名判断。
	for _, name := range ctx.rec.events {
		if name == usageUpdatedEventName {
			found = true
		}
	}
	ctx.rec.mu.Unlock()
	if !found {
		t.Fatal("usage-updated event not emitted")
	}
	_ = usage
}

func TestHandleConversationStartedItemCompletedDefault(t *testing.T) {
	t.Run("request.started 追加 runtime 事件", func(t *testing.T) {
		ctx := newEventsTestCtx(t)
		// runtime 事件按 event.EventType 提取（与 frame.kind 解耦）：
		// turn.retry 带 attempt/max 进度时标题应带 (1/3)。
		event := adapter.Event{
			EventType: "turn.retry",
			Payload:   map[string]any{"attempt": 1, "max_attempts": 3},
		}
		ctx.s.handleConversationEventLocked(ctx.frame("request.started", "", event))
		msg := findSessionMessageByID(ctx.session, "a1")
		if len(msg.RuntimeEvents) == 0 {
			t.Fatal("runtime events not appended")
		}
		found := false
		for _, item := range msg.RuntimeEvents {
			if strings.Contains(item.Title, "1/3") {
				found = true
			}
		}
		if !found {
			t.Fatalf("retry progress title missing: %+v", msg.RuntimeEvents)
		}
	})

	t.Run("tool.result 置回流式态", func(t *testing.T) {
		ctx := newEventsTestCtx(t)
		findSessionMessageByID(ctx.session, "a1").State = "waiting"
		ctx.s.handleConversationEventLocked(ctx.frame("tool.result", "", adapter.Event{}))
		msg := findSessionMessageByID(ctx.session, "a1")
		if msg.State != "streaming" {
			t.Fatalf("message state = %q, want streaming", msg.State)
		}
	})

	t.Run("未知 kind 走 default 分支", func(t *testing.T) {
		ctx := newEventsTestCtx(t)
		findSessionMessageByID(ctx.session, "a1").State = "waiting"
		ctx.s.handleConversationEventLocked(ctx.frame("goal.updated", "", adapter.Event{Type: "goal.updated"}))
		msg := findSessionMessageByID(ctx.session, "a1")
		// goal.updated 不在 runtime 提取表 → 不追加事件、不翻状态。
		if len(msg.RuntimeEvents) != 0 {
			t.Fatalf("unknown kind should not append runtime events: %+v", msg.RuntimeEvents)
		}
		if msg.State != "waiting" {
			t.Fatalf("unknown kind should not flip state, got %q", msg.State)
		}
	})

	t.Run("assistant 已完成时 item_completed 早退", func(t *testing.T) {
		ctx := newEventsTestCtx(t)
		findSessionMessageByID(ctx.session, "a1").State = "completed"
		before := len(findSessionMessageByID(ctx.session, "a1").RuntimeEvents)
		ctx.s.handleConversationEventLocked(conversationEventFrame{
			session:            ctx.session,
			sessionID:          "sess-1",
			assistantMessageID: "a1",
			kind:               "turn.item_completed",
			assistantCompleted: true,
			event:              adapter.Event{},
		})
		msg := findSessionMessageByID(ctx.session, "a1")
		if len(msg.RuntimeEvents) != before {
			t.Fatal("assistantCompleted should short-circuit item_completed")
		}
		if msg.State != "completed" {
			t.Fatalf("completed message state flipped to %q", msg.State)
		}
	})
}

func TestApprovalHelpersEdgeCases(t *testing.T) {
	if isApprovalPromptKind("approval.required") != true || isApprovalPromptKind("tool.approval_required") != true {
		t.Fatal("approval prompt kinds mismatch")
	}
	if isApprovalPromptKind("turn.completed") {
		t.Fatal("non-approval kind flagged")
	}

	if got := approvalIDFromEvent(adapter.Event{Payload: map[string]any{"approval_id": " apr-1 "}}); got != "apr-1" {
		t.Fatalf("approvalIDFromEvent(payload) = %q", got)
	}
	if got := approvalIDFromEvent(adapter.Event{RequestID: "req-1"}); got != "req-1" {
		t.Fatalf("approvalIDFromEvent(requestID fallback) = %q", got)
	}
}
