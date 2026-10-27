package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// conversation_runtime 余臂批测：resolveSendSession 各分支、
// ensureWorkspaceSessionLocked preferred 链、取消判定、attachRunningTurn
// 回填、invoke 双入口守卫、messageInTerminalState、finishConversation
// 四终态臂。

import (
	"context"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func TestResolveSendSessionLockedArms(t *testing.T) {
	t.Run("显式 id 命中与空 workspace 回填", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		known := &sessionState{ID: "s-1"}
		s.sessions["s-1"] = known
		got, err := s.resolveSendSessionLocked("s-1", "")
		if err != nil || got != known {
			t.Fatalf("命中 = %+v, %v", got, err)
		}
		if known.WorkspacePath != "" {
			t.Fatal("空 workspace 参数回填空串——保持原值")
		}
	})

	t.Run("workspace 不一致落入解析链", func(t *testing.T) {
		ws := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: ws}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		s.sessions["s-old"] = &sessionState{ID: "s-old", WorkspacePath: "/other"}
		// 显式 id 属旧工作区：不得复用，按 workspace 解析（stub 无会话→新建）。
		got, err := s.resolveSendSessionLocked("s-old", ws)
		if err != nil {
			t.Fatalf("解析链 = %v", err)
		}
		if got.ID == "s-old" {
			t.Fatal("跨工作区不应复用旧会话")
		}
	})

	t.Run("workspace 空回退 activeWorkspace", func(t *testing.T) {
		ws := t.TempDir()
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{defaultWorkspace: ws})
		s.stateMu.Lock()
		s.activeWorkspace = ws
		s.stateMu.Unlock()
		s.sessions["s-act"] = &sessionState{ID: "s-act", WorkspacePath: ws}
		got, err := s.resolveSendSessionLocked("", ws)
		if err != nil || got.ID != "s-act" {
			t.Fatalf("active 命中 = %+v, %v", got, err)
		}
	})
}

func TestEnsureWorkspaceSessionLockedPreferred(t *testing.T) {
	ws := t.TempDir()

	t.Run("preferred 恢复链", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		// preferred 命中既有：置 current。
		sess := &sessionState{ID: "s-p", WorkspacePath: ws}
		s.sessions["s-p"] = sess
		if got := s.ensureWorkspaceSessionLocked(ws, "s-p"); got != sess {
			t.Fatalf("preferred 命中 = %+v", got)
		}
		if s.currentSessionID != "s-p" {
			t.Fatalf("current = %q", s.currentSessionID)
		}
	})

	t.Run("空 preferred 与无会话 nil", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		if got := s.ensureWorkspaceSessionLocked("  ", ""); got != nil {
			t.Fatalf("空 workspace = %+v", got)
		}
		// 空会话列表 → ensureActive 静默新建（stub create 链）。
		empty := t.TempDir()
		got := s.ensureWorkspaceSessionLocked(empty, "")
		if got == nil || got.ID == "" {
			t.Fatalf("静默新建 = %+v", got)
		}
	})
}

func TestConversationCancelledAndAttachTurn(t *testing.T) {
	s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
	if s.conversationCancelledLocked("s-x", "m") {
		t.Fatal("无 running 恒 false")
	}
	s.runningConversations["s-x"] = &runningConversationState{AssistantMessageID: "m-1", Cancelled: true}
	if !s.conversationCancelledLocked("s-x", "m-1") {
		t.Fatal("匹配+已取消应 true")
	}
	if s.conversationCancelledLocked("s-x", "m-2") {
		t.Fatal("消息不匹配应 false")
	}

	// attachRunningTurn：id 缺位 no-op / 消息不匹配 no-op / 命中回填。
	s.attachRunningTurn("", "m", conversationStreamHandle{})
	s.attachRunningTurn("s-x", "", conversationStreamHandle{})
	s.attachRunningTurn("s-x", "m-other", conversationStreamHandle{TurnID: "t-9"})
	if s.runningConversations["s-x"].TurnID != "" {
		t.Fatal("不匹配不应回填")
	}
	s.sessions["s-x"] = &sessionState{ID: "s-x", Messages: []ChatMessage{{ID: "m-1"}}}
	s.attachRunningTurn("s-x", "m-1", conversationStreamHandle{TurnID: "t-1"})
	if s.runningConversations["s-x"].TurnID != "t-1" {
		t.Fatal("TurnID 未回填")
	}
	if s.sessions["s-x"].Messages[0].turnID != "t-1" {
		t.Fatal("消息 turnID 未回填")
	}
}

func TestInvokeConversationGuards(t *testing.T) {
	s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
	// invokeSession 注入但 core 缺位：runtime unavailable。
	s.invokeSession = func(adapter.Core, context.Context, string, []string) (<-chan adapter.Event, error) {
		return nil, nil
	}
	if _, err := s.invokeConversationTurnStream(context.Background(), "s-x", "hi", nil, ""); err == nil ||
		!strings.Contains(err.Error(), "runtime unavailable") {
		t.Fatalf("core 缺位 = %v", err)
	}
	// resume 走 invokeSession 注入路径：明确拒绝。
	if _, err := s.invokeConversationResumeStream(context.Background(), "s-x", "t-1"); err == nil ||
		!strings.Contains(err.Error(), "runtime gateway path") {
		t.Fatalf("resume 拒绝 = %v", err)
	}
}

func TestFinishConversationTerminalStates(t *testing.T) {
	newReady := func(t *testing.T) *BridgeService {
		s, _, rec := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		session := &sessionState{
			ID: "s-f", Running: true,
			Messages: []ChatMessage{{ID: "msg-f", Role: "assistant", State: "streaming"}},
		}
		s.sessions["s-f"] = session
		s.runningConversations["s-f"] = &runningConversationState{AssistantMessageID: "msg-f"}
		t.Cleanup(func() { waitForEmitsToSettle(t, rec) })
		return s
	}
	run := func(s *BridgeService, ctx context.Context) {
		s.finishConversation("s-f", "msg-f", ctx)
	}

	t.Run("完成收口", func(t *testing.T) {
		s := newReady(t)
		findSessionMessageByID(s.sessions["s-f"], "msg-f").State = "completed"
		run(s, context.Background())
		if s.sessions["s-f"].Running {
			t.Fatal("完成应复位 Running")
		}
		if _, ok := s.runningConversations["s-f"]; ok {
			t.Fatal("running 应清理")
		}
	})

	t.Run("看门狗静默", func(t *testing.T) {
		s := newReady(t)
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errTurnWatchdogTripped)
		run(s, ctx)
		msg := findSessionMessageByID(s.sessions["s-f"], "msg-f")
		if msg.State != "failed" || s.sessions["s-f"].NeedsAttention != true {
			t.Fatalf("看门狗终态 = %+v", msg)
		}
	})

	t.Run("取消停止", func(t *testing.T) {
		s := newReady(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		run(s, ctx)
		msg := findSessionMessageByID(s.sessions["s-f"], "msg-f")
		if msg.State != "failed" {
			t.Fatalf("取消终态 = %+v", msg)
		}
		if s.sessions["s-f"].NeedsAttention {
			t.Fatal("用户主动停止不需注意")
		}
	})

	t.Run("超时", func(t *testing.T) {
		s := newReady(t)
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		<-ctx.Done()
		run(s, ctx)
		if !s.sessions["s-f"].NeedsAttention {
			t.Fatal("超时应需注意")
		}
	})

	t.Run("流异常断开（default）", func(t *testing.T) {
		s := newReady(t)
		run(s, context.Background())
		if !s.sessions["s-f"].NeedsAttention {
			t.Fatal("异常断开应需注意")
		}
	})

	t.Run("会话未运行+消息非终态：异常分支", func(t *testing.T) {
		s := newReady(t)
		s.sessions["s-f"].Running = false
		run(s, context.Background())
		if !s.sessions["s-f"].NeedsAttention {
			t.Fatal("非运行会话的异常收尾应需注意")
		}
	})
}

func TestMessageInTerminalState(t *testing.T) {
	session := &sessionState{Messages: []ChatMessage{
		{ID: "a", State: "completed"}, {ID: "b", State: "failed"}, {ID: "c", State: "streaming"},
	}}
	if !messageInTerminalState(session, "a") || !messageInTerminalState(session, "b") {
		t.Fatal("completed/failed 是终态")
	}
	if messageInTerminalState(session, "c") {
		t.Fatal("streaming 非终态")
	}
	if !messageInTerminalState(session, "missing") {
		t.Fatal("缺位消息按终态处理")
	}
}
