package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// conversation_events 余臂批测：事件分发 switch 的 token_usage /
// request.started / item_delta 流式态 / item_completed 附加事件、
// markItemApproval 幂等与边界、审批去重与 ToolCall 标记链。

import (
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func newEventsBridge(t *testing.T) (*BridgeService, *sessionState) {
	t.Helper()
	s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
	session := &sessionState{
		ID: "sess-evt", WorkspacePath: t.TempDir(),
		Messages: []ChatMessage{{ID: "msg-evt", Role: "assistant", State: "waiting"}},
	}
	s.sessions["sess-evt"] = session
	return s, session
}

func TestConversationEventDispatchArms(t *testing.T) {
	t.Run("token_usage 分发", func(t *testing.T) {
		s, session := newEventsBridge(t)
		frame := conversationEventFrame{
			session: session, sessionID: "sess-evt", assistantMessageID: "msg-evt",
			kind: "turn.token_usage",
			event: adapter.Event{EventType: "turn.token_usage", Payload: map[string]any{
				"input_tokens":  11,
				"output_tokens": 22,
			}},
		}
		result := s.handleConversationEventLocked(frame)
		_ = result
	})

	t.Run("request.started 分发与流式态", func(t *testing.T) {
		s, session := newEventsBridge(t)
		frame := conversationEventFrame{
			session: session, sessionID: "sess-evt", assistantMessageID: "msg-evt",
			kind:  "request.started",
			event: adapter.Event{EventType: "request.started", Payload: map[string]any{}},
		}
		_ = s.handleConversationEventLocked(frame)
		// delta 分支置 streaming。
		deltaFrame := conversationEventFrame{
			session: session, sessionID: "sess-evt", assistantMessageID: "msg-evt",
			kind:  "turn.item_delta",
			event: adapter.Event{EventType: "turn.item_delta", Payload: map[string]any{"delta": "片段"}},
		}
		_ = s.handleConversationEventLocked(deltaFrame)
		if msg := findSessionMessageByID(session, "msg-evt"); msg.State != "streaming" {
			t.Fatalf("delta 后状态 = %q", msg.State)
		}
	})

	t.Run("item_completed 追加 runtime 事件", func(t *testing.T) {
		s, session := newEventsBridge(t)
		frame := conversationEventFrame{
			session: session, sessionID: "sess-evt", assistantMessageID: "msg-evt",
			kind: "turn.item_completed",
			event: adapter.Event{EventType: "turn.item_completed", Payload: map[string]any{
				"tool_name": "shell",
				"message":   "命令完成",
			}},
		}
		s.handleConversationItemCompletedLocked(frame)
		if msg := findSessionMessageByID(session, "msg-evt"); msg.State != "streaming" {
			t.Fatalf("item_completed 后状态 = %q", msg.State)
		}
		// assistantCompleted 短路臂。
		frame.assistantCompleted = true
		s.handleConversationItemCompletedLocked(frame)
	})
}

func TestMarkItemApprovalIdempotentAndBounds(t *testing.T) {
	s, session := newEventsBridge(t)
	msg := findSessionMessageByID(session, "msg-evt")
	msg.Items = []ThreadItem{{ID: "call-1", Kind: "tool_call"}}

	approval := &ItemApprovalState{ApprovalID: "ap-1", State: "pending"}
	if got := s.markItemApprovalLocked(session, "msg-evt", "call-1", approval); got == nil {
		t.Fatal("首次标记应返回 item")
	}
	// 幂等：同 id 同 state 不重复覆盖。
	if got := s.markItemApprovalLocked(session, "msg-evt", "call-1", approval); got != nil {
		t.Fatal("同状态重复标记应幂等 nil")
	}
	// 状态变化重新标记。
	approval2 := &ItemApprovalState{ApprovalID: "ap-1", State: "approved"}
	if got := s.markItemApprovalLocked(session, "msg-evt", "call-1", approval2); got == nil {
		t.Fatal("状态变化应重新标记")
	}
	// 边界：nil session / 空 callID / nil approval / 消息缺位 / item 缺位。
	if s.markItemApprovalLocked(nil, "msg-evt", "call-1", approval) != nil {
		t.Fatal("nil session 应 nil")
	}
	if s.markItemApprovalLocked(session, "msg-evt", "  ", approval) != nil {
		t.Fatal("空 callID 应 nil")
	}
	if s.markItemApprovalLocked(session, "msg-evt", "call-1", nil) != nil {
		t.Fatal("nil approval 应 nil")
	}
	if s.markItemApprovalLocked(session, "missing", "call-1", approval) != nil {
		t.Fatal("消息缺位应 nil")
	}
	if s.markItemApprovalLocked(session, "msg-evt", "call-x", approval) != nil {
		t.Fatal("item 缺位兜底应 nil")
	}
}

func TestHandleConversationApprovalLockedFlow(t *testing.T) {
	s, session := newEventsBridge(t)
	msg := findSessionMessageByID(session, "msg-evt")
	msg.Items = []ThreadItem{{ID: "call-ap", Kind: "tool_call"}}

	frame := conversationEventFrame{
		session: session, sessionID: "sess-evt", assistantMessageID: "msg-evt",
		kind: "tool.approval_required",
		event: adapter.Event{
			EventType: "tool.approval_required",
			RequestID: "call-ap",
			Payload:   map[string]any{"message": "要执行 rm -rf 吗"},
		},
	}
	s.handleConversationApprovalLocked(frame)
	prompt, ok := s.prompts["call-ap"]
	if !ok {
		t.Fatal("审批应建 prompt 反向索引")
	}
	if prompt.Kind != "approval" || prompt.Status != "pending" {
		t.Fatalf("prompt = %+v", prompt)
	}
	// ToolCall item 标记。
	if msg.Items[0].Approval == nil || msg.Items[0].Approval.ApprovalID != "call-ap" {
		t.Fatalf("item approval = %+v", msg.Items[0].Approval)
	}
	// 状态行 + 通知。
	if len(s.notifications) == 0 {
		t.Fatal("审批应留通知")
	}

	// 按 approval_id 去重：同 id 再来不再生成。
	s.stateMu.Lock()
	notifs := len(s.notifications)
	s.stateMu.Unlock()
	s.handleConversationApprovalLocked(frame)
	s.stateMu.Lock()
	notifs2 := len(s.notifications)
	s.stateMu.Unlock()
	if notifs2 != notifs {
		t.Fatalf("重复审批应去重: %d → %d", notifs, notifs2)
	}
}
