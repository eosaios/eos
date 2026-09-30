package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// webbridge 批四批测：turn 启动编排（start/resume/resolve-or-create/ensure）、
// SendChat 全链浅层臂与起跑-收尾循环、ResumeFailedTurn 错误臂、
// resolveSendSessionLocked / ensureWorkspaceSessionLocked / attachRunningTurn。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// ---- stub 扩展：turn 流 ----

func (g *chatSessionsGatewayStub) CoreStartTurnStreamWithRequestRPC(_ context.Context, req coreapi.StartTurnRequest) (<-chan adapter.Event, coreapi.Turn, error) {
	g.mu.Lock()
	// 失败调用也留痕：重试路径的测试靠请求计数定位第一次失败。
	g.turnRequests = append(g.turnRequests, req)
	if g.turnErr != nil {
		err := g.turnErr
		g.mu.Unlock()
		return nil, coreapi.Turn{}, err
	}
	if g.turnFailFirst > 0 {
		g.turnFailFirst--
		g.mu.Unlock()
		return nil, coreapi.Turn{}, errors.New("turn start flaky")
	}
	if g.turnEvents == nil {
		g.turnEvents = make(chan adapter.Event, 8)
	}
	g.mu.Unlock()
	return g.turnEvents, coreapi.Turn{ID: "turn-gen-1"}, nil
}

func (g *chatSessionsGatewayStub) CoreResumeTurnStreamRPC(_ context.Context, _, turnID string) (<-chan adapter.Event, coreapi.Turn, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.turnErr != nil {
		return nil, coreapi.Turn{}, g.turnErr
	}
	g.resumeTurnIDs = append(g.resumeTurnIDs, turnID)
	if g.turnEvents == nil {
		g.turnEvents = make(chan adapter.Event, 8)
	}
	return g.turnEvents, coreapi.Turn{ID: "turn-resume-1"}, nil
}

// ---- turn 启动编排 ----

func TestStartConversationTurnRPCArms(t *testing.T) {
	t.Run("无 gateway 拒绝", func(t *testing.T) {
		if _, err := (&BridgeService{}).startConversationTurnRPC(context.Background(), "", "hi", nil, "t1"); err == nil {
			t.Fatal("startConversationTurnRPC without gateway error = nil")
		}
		if _, err := (&BridgeService{}).startResumeConversationTurnRPC(context.Background(), "", "t1"); err == nil {
			t.Fatal("startResumeConversationTurnRPC without gateway error = nil")
		}
	})

	t.Run("空 session 解析失败拒绝", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s := &BridgeService{runtimeGateway: gateway}
		if _, err := s.startConversationTurnRPC(context.Background(), "", "hi", nil, ""); err == nil || !strings.Contains(err.Error(), "session_id is required") {
			t.Fatalf("start turn error = %v", err)
		}
		if _, err := s.startResumeConversationTurnRPC(context.Background(), "", "t1"); err == nil || !strings.Contains(err.Error(), "session_id is required") {
			t.Fatalf("resume turn error = %v", err)
		}
	})

	t.Run("resolveOrCreateSession 三态", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{currentMeta: adapter.SessionMeta{ID: "current-1"}}
		s := &BridgeService{runtimeGateway: gateway}
		s.stateMu.Lock()
		s.activeWorkspace = "/ws"
		s.stateMu.Unlock()
		if got := s.resolveOrCreateSessionRPC(); got != "current-1" {
			t.Fatalf("resolveOrCreateSessionRPC = %q, want current-1", got)
		}

		// 无 current → 新建。
		gateway.currentMeta = adapter.SessionMeta{}
		if got := s.resolveOrCreateSessionRPC(); got == "" {
			t.Fatal("resolveOrCreateSessionRPC should create on missing current")
		}

		// activeWorkspace 空 → 直接空。
		s.stateMu.Lock()
		s.activeWorkspace = ""
		s.stateMu.Unlock()
		if got := s.resolveOrCreateSessionRPC(); got != "" {
			t.Fatalf("resolveOrCreateSessionRPC without workspace = %q, want empty", got)
		}
	})

	t.Run("start 首败后 resume 同 session 重试", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		turnCh := make(chan adapter.Event, 4)
		gateway.turnEvents = turnCh
		gateway.turnFailFirst = 1 // 首次 turn/start 失败，重试放行
		s := &BridgeService{runtimeGateway: gateway}
		s.stateMu.Lock()
		s.activeWorkspace = "/ws"
		s.stateMu.Unlock()
		handle, err := s.startConversationTurnRPC(context.Background(), "sess-1", "hi", nil, "t-9")
		if err != nil {
			t.Fatalf("startConversationTurnRPC retry error = %v", err)
		}
		if handle.SessionID != "sess-1" || handle.TurnID != "turn-gen-1" {
			t.Fatalf("handle = %+v", handle)
		}
		if handle.Interrupt == nil {
			t.Fatal("handle.Interrupt missing for valid turn")
		}
		gateway.mu.Lock()
		requests := len(gateway.turnRequests)
		gateway.mu.Unlock()
		if requests != 2 {
			t.Fatalf("turnRequests = %d, want 2 (fail + retry)", requests)
		}
		close(turnCh)
	})

	t.Run("ensureSessionForTurnRPC resume 失败降级新建并 rekey", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{
			turnErr:   nil,
			resumeErr: errors.New("archived"),
			createErr: nil,
		}
		gateway.mu.Lock()
		gateway.currentMeta = adapter.SessionMeta{}
		gateway.mu.Unlock()
		turnCh := make(chan adapter.Event, 4)
		gateway.turnEvents = turnCh
		s := &BridgeService{
			runtimeGateway: gateway,
			sessions:       map[string]*sessionState{},
			prompts:        map[string]*promptState{},
		}
		s.stateMu.Lock()
		s.sessions["sess-old"] = &sessionState{ID: "sess-old", WorkspacePath: "/ws"}
		s.stateMu.Unlock()

		newID := s.ensureSessionForTurnRPC("sess-old")
		if newID == "" || newID == "sess-old" {
			t.Fatalf("ensureSessionForTurnRPC = %q, want fresh id", newID)
		}
		s.stateMu.RLock()
		_, oldGone := s.sessions["sess-old"]
		_, newHere := s.sessions[newID]
		s.stateMu.RUnlock()
		if oldGone || !newHere {
			t.Fatal("session not rekeyed to new core id")
		}
		close(turnCh)
	})

	t.Run("resume turn 成功与失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		turnCh := make(chan adapter.Event, 4)
		gateway.turnEvents = turnCh
		s := &BridgeService{runtimeGateway: gateway}
		handle, err := s.startResumeConversationTurnRPC(context.Background(), "sess-1", " t-1 ")
		if err != nil {
			t.Fatalf("startResumeConversationTurnRPC error = %v", err)
		}
		if handle.TurnID != "turn-resume-1" {
			t.Fatalf("handle = %+v", handle)
		}
		gateway.mu.Lock()
		ids := append([]string(nil), gateway.resumeTurnIDs...)
		gateway.mu.Unlock()
		if len(ids) != 1 || ids[0] != "t-1" {
			t.Fatalf("resumeTurnIDs = %v", ids)
		}
		gateway.turnErr = errors.New("turn gone")
		if _, err := s.startResumeConversationTurnRPC(context.Background(), "sess-1", "t-1"); err == nil {
			t.Fatal("resume turn error = nil on gateway failure")
		}
		close(turnCh)
	})

	t.Run("gatewayTurnHandle 空 turnID 无中断钩子", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		handle := conversationGatewayTurnHandle(gateway, "s", nil, "  ")
		if handle.Interrupt != nil {
			t.Fatal("Interrupt should be nil for blank turn id")
		}
		withID := conversationGatewayTurnHandle(gateway, "s", nil, " t-2 ")
		if withID.Interrupt == nil {
			t.Fatal("Interrupt missing for valid turn id")
		}
		if err := withID.Interrupt(context.Background()); err != nil {
			t.Fatalf("Interrupt error = %v", err)
		}
	})
}

// ---- SendChat / ResumeFailedTurn ----

func TestSendChatArms(t *testing.T) {
	t.Run("nil bridge 与空输入", func(t *testing.T) {
		if _, err := NewChatService(nil).SendChat("", "", "", nil); err == nil {
			t.Fatal("SendChat(nil) error = nil")
		}
		gateway := &chatSessionsGatewayStub{}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		if _, err := svc.SendChat("s", "w", "   ", nil); err == nil || !strings.Contains(err.Error(), "message content is required") {
			t.Fatalf("SendChat('') error = %v", err)
		}
		// 附件路径也算有效输入。
		if _, err := svc.SendChat("s", "w", "  ", []string{"/tmp/a.png"}); err == nil {
			t.Fatal("attachment-only send unexpectedly failed earlier than expected")
		}
	})

	t.Run("进行中会话拒绝", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace, Running: true}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()
		if _, err := svc.SendChat("sess-1", workspace, "hi", nil); err == nil || !strings.Contains(err.Error(), "still processing") {
			t.Fatalf("SendChat(running) error = %v", err)
		}
	})

	t.Run("workspace 与 session 不一致回落当前", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		s, svc, rec := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()
		// workspace 传别的路径 → session 不复用，按 workspace 新链路走。
		// turn 流注入失败让 runConversation 走同步错误收尾，避免测试
		// 结束后仍有后台 goroutine 写 HOME 目录（tempdir 清理竞态）。
		gateway.mu.Lock()
		gateway.turnErr = errors.New("stream unavailable")
		gateway.mu.Unlock()
		_, err := svc.SendChat("sess-1", t.TempDir(), "hi", nil)
		if err != nil {
			t.Fatalf("SendChat(mismatched workspace) error = %v", err)
		}
		// 开头「确认工作区」+ 错误收尾各 emit 一波 shellUpdated；
		// 两波都落地后 runConversation 不再产生新的写盘 goroutine。
		eventually(t, "both shellUpdated waves", func() bool {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			count := 0
			for _, name := range rec.events {
				if name == shellUpdatedEventName {
					count++
				}
			}
			return count >= 2
		})
		s.conversationWG.Wait()
	})

	t.Run("persist 失败软着陆", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace, saveErr: errors.New("disk full")}
		s, svc, rec := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()
		response, err := svc.SendChat("sess-1", workspace, "hi", nil)
		if err != nil {
			t.Fatalf("SendChat persist failure should soft-land, got %v", err)
		}
		if response.CurrentSessionID == "" {
			t.Fatal("soft-land response missing session")
		}
		s.stateMu.RLock()
		needsAttention := s.sessions["sess-1"].NeedsAttention
		s.stateMu.RUnlock()
		if !needsAttention {
			t.Fatal("session should flag NeedsAttention after persist failure")
		}
		// 软着陆路径的 emit goroutine 会写 HOME 目录，等事件落地防清理竞态。
		eventually(t, "shellUpdated after soft-land", func() bool { return rec.has(shellUpdatedEventName) })
	})

	t.Run("成功起跑与流收尾", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		turnCh := make(chan adapter.Event, 8)
		gateway.turnEvents = turnCh
		s, svc, rec := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()

		response, err := svc.SendChat("sess-1", workspace, " 你好 ", nil)
		if err != nil {
			t.Fatalf("SendChat error = %v", err)
		}
		if response.CurrentSessionID != "sess-1" {
			t.Fatalf("CurrentSessionID = %q", response.CurrentSessionID)
		}

		// 起跑态：running 挂上、user+assistant 占位消息入列、turn 请求已发。
		eventually(t, "running conversation attached", func() bool {
			s.stateMu.RLock()
			defer s.stateMu.RUnlock()
			_, running := s.runningConversations["sess-1"]
			return running && len(s.sessions["sess-1"].Messages) >= 2
		})
		s.stateMu.RLock()
		first := s.sessions["sess-1"].Messages[0]
		second := s.sessions["sess-1"].Messages[1]
		s.stateMu.RUnlock()
		if first.Role != "user" || first.Content != "你好" {
			t.Fatalf("user message = %+v", first)
		}
		if second.Role != "assistant" || second.State != "streaming" {
			t.Fatalf("assistant placeholder = %+v", second)
		}

		// 流收尾：close 后 runConversation 归一，会话退出 running。
		close(turnCh)
		eventually(t, "conversation finished", func() bool {
			s.stateMu.RLock()
			defer s.stateMu.RUnlock()
			session := s.sessions["sess-1"]
			_, running := s.runningConversations["sess-1"]
			return session != nil && !session.Running && !running
		})
		eventually(t, "shellUpdated emitted", func() bool { return rec.has(shellUpdatedEventName) })
	})
}

func TestResumeFailedTurnArms(t *testing.T) {
	t.Run("nil bridge 与进行中拒绝", func(t *testing.T) {
		if _, err := NewChatService(nil).ResumeFailedTurn(""); err == nil {
			t.Fatal("ResumeFailedTurn(nil) error = nil")
		}
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace, Running: true}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()
		if _, err := svc.ResumeFailedTurn("sess-1"); err == nil || !strings.Contains(err.Error(), "still processing") {
			t.Fatalf("ResumeFailedTurn(running) error = %v", err)
		}
	})

	t.Run("成功续跑只加 assistant 占位", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		turnCh := make(chan adapter.Event, 8)
		gateway.turnEvents = turnCh
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace, Messages: []ChatMessage{
			{ID: "u1", Role: "user", Content: "重试我"},
			{ID: "a1", Role: "assistant", State: "failed"},
		}}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()

		if _, err := svc.ResumeFailedTurn("sess-1"); err != nil {
			t.Fatalf("ResumeFailedTurn error = %v", err)
		}
		eventually(t, "resume running", func() bool {
			s.stateMu.RLock()
			defer s.stateMu.RUnlock()
			return s.sessions["sess-1"] != nil && s.sessions["sess-1"].Running
		})
		s.stateMu.RLock()
		messages := s.sessions["sess-1"].Messages
		s.stateMu.RUnlock()
		if len(messages) != 3 || messages[2].Role != "assistant" || !messages[2].IsPlaceholder {
			t.Fatalf("resume should append exactly one assistant placeholder, got %+v", messages)
		}
		close(turnCh)
		eventually(t, "resume finished", func() bool {
			s.stateMu.RLock()
			defer s.stateMu.RUnlock()
			return s.sessions["sess-1"] != nil && !s.sessions["sess-1"].Running
		})
	})
}

// ---- conversation runtime 纯 helper ----

func TestConversationRuntimeHelpers(t *testing.T) {
	t.Run("resolveSendSessionLocked 各态", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s := &BridgeService{
			runtimeGateway:       gateway,
			sessions:             map[string]*sessionState{},
			runningConversations: map[string]*runningConversationState{},
			prompts:              map[string]*promptState{},
		}

		// 显式 session 命中。
		workspace := t.TempDir()
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace}
		s.stateMu.Unlock()
		session, err := s.resolveSendSessionLocked(" sess-1 ", workspace)
		if err != nil || session == nil || session.ID != "sess-1" {
			t.Fatalf("resolveSendSessionLocked = %+v %v", session, err)
		}

		// workspace 不一致 → 不复用旧 session，走 workspace 解析（ensure 新会话）。
		other := t.TempDir()
		session, err = s.resolveSendSessionLocked("sess-1", other)
		if err != nil {
			t.Fatalf("mismatch resolve error = %v", err)
		}
		if session != nil && sameWorkspacePath(session.WorkspacePath, workspace) && session.ID == "sess-1" {
			t.Fatal("stale session reused across workspaces")
		}
	})

	t.Run("ensureWorkspaceSessionLocked preferred 优先", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s := &BridgeService{runtimeGateway: gateway, sessions: map[string]*sessionState{}, prompts: map[string]*promptState{}}
		if got := s.ensureWorkspaceSessionLocked("", "x"); got != nil {
			t.Fatalf("empty workspace = %+v", got)
		}
		workspace := t.TempDir()
		s.stateMu.Lock()
		s.sessions["preferred-1"] = &sessionState{ID: "preferred-1", WorkspacePath: workspace}
		s.stateMu.Unlock()
		if got := s.ensureWorkspaceSessionLocked(workspace, " preferred-1 "); got == nil || got.ID != "preferred-1" {
			t.Fatalf("preferred = %+v", got)
		}
	})

	t.Run("conversationCancelledLocked 与 attachRunningTurn", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s := &BridgeService{runtimeGateway: gateway, sessions: map[string]*sessionState{}, runningConversations: map[string]*runningConversationState{}, prompts: map[string]*promptState{}}
		if s.conversationCancelledLocked("s", "a") {
			t.Fatal("no running conversation should report cancelled=false")
		}
		s.stateMu.Lock()
		s.runningConversations["sess-1"] = &runningConversationState{AssistantMessageID: "msg-1"}
		s.stateMu.Unlock()
		if s.conversationCancelledLocked("sess-1", "msg-other") {
			t.Fatal("different assistant id should not report cancelled")
		}

		// attach：sessionID/assistantID 为空早退；不匹配早退；匹配回填 turnID 与 Interrupt。
		s.attachRunningTurn("", "msg-1", conversationStreamHandle{TurnID: "t-1"})
		s.attachRunningTurn("sess-1", "nope", conversationStreamHandle{TurnID: "t-1"})

		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", Messages: []ChatMessage{{ID: "msg-1", Role: "assistant"}}}
		s.stateMu.Unlock()
		interrupt := func(context.Context) error { return nil }
		s.attachRunningTurn("sess-1", "msg-1", conversationStreamHandle{TurnID: " t-9 ", Interrupt: interrupt})
		s.stateMu.RLock()
		running := s.runningConversations["sess-1"]
		msgTurn := s.sessions["sess-1"].Messages[0].turnID
		s.stateMu.RUnlock()
		if running.TurnID != "t-9" || running.Interrupt == nil {
			t.Fatalf("running = %+v", running)
		}
		if msgTurn != "t-9" {
			t.Fatalf("assistant message turnID = %q", msgTurn)
		}
	})

	t.Run("messageInTerminalState", func(t *testing.T) {
		if !messageInTerminalState(nil, "missing") {
			t.Fatal("missing message should be terminal")
		}
		session := &sessionState{Messages: []ChatMessage{
			{ID: "m1", State: "completed"},
			{ID: "m2", State: "streaming"},
			{ID: "m3", State: "failed"},
		}}
		if !messageInTerminalState(session, "m1") || !messageInTerminalState(session, "m3") {
			t.Fatal("completed/failed should be terminal")
		}
		if messageInTerminalState(session, "m2") {
			t.Fatal("streaming should not be terminal")
		}
	})

	t.Run("invokeSession 注入路径短路", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{turnErr: errors.New("stream unavailable")}
		s := &BridgeService{runtimeGateway: gateway}
		// invokeSession 未注入 → 走 gateway 编排（nil ctx 归一 coreCtx）。
		if _, err := s.invokeConversationTurnStream(nil, "sess-1", "hi", nil, "t-1"); err == nil {
			t.Fatal("gateway path with turn failure should error")
		}
		// resume 路径注入 invokeSession 时显式拒绝。
		s.invokeSession = func(adapter.Core, context.Context, string, []string) (<-chan adapter.Event, error) {
			return nil, nil
		}
		if _, err := s.invokeConversationResumeStream(context.Background(), "s", "t"); err == nil || !strings.Contains(err.Error(), "runtime gateway path") {
			t.Fatalf("resume with invokeSession error = %v", err)
		}
	})
}
