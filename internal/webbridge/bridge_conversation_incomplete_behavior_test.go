package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// 一轮「没做完」的收尾行为：内容保留、内核的说明行照常展示、不报故障，
// 也不宣布「请求已完成」。
func TestIncompleteTurnKeepsContentAndExplainsWithoutAnnouncingCompletion(t *testing.T) {
	for _, reason := range []string{"max_output_tokens", "content_filter", "iteration_limit", "something_new"} {
		s, session := incompleteBridge(textItems("半截的回答")...)
		// 内核先下发说明行，再下发带 status=incomplete 的终态。
		s.handleConversationEventLocked(incompleteFrame(session, "turn.item_completed", kernelNotice("turn-1", reason)))
		s.handleConversationEventLocked(incompleteFrame(session, "turn.completed", map[string]any{
			"status": "incomplete", "reason": reason,
		}))
		msg := findSessionMessageByID(session, "assistant-1")
		if msg.State != "completed" {
			t.Fatalf("%s: state = %q, want completed (terminal, content kept)", reason, msg.State)
		}
		if msg.Items[0].Text != "半截的回答" {
			t.Fatalf("%s: content changed: %q", reason, msg.Items[0].Text)
		}
		row := lastStatusItem(msg)
		if row == nil || row.Level != "warning" || !strings.Contains(row.Text, "内容不完整") {
			t.Fatalf("%s: explanation row = %+v, want the kernel warning row", reason, row)
		}
		if session.Running || session.NeedsAttention {
			t.Fatalf("%s: incomplete is not a failure: running=%v needsAttention=%v", reason, session.Running, session.NeedsAttention)
		}
		for _, ev := range msg.RuntimeEvents {
			if strings.Contains(ev.Title, "请求已完成") {
				t.Fatalf("%s: an incomplete turn must not announce completion: %+v", reason, ev)
			}
		}
	}
}

// 内核发起的取消/中断：按「已停止」收尾。此前这类事件落到空分支，会话停在
// 「进行中」，随后流关闭被判成「流式响应异常结束」——一次正常的停止被展示成故障。
func TestKernelInterruptedEndsAsStoppedNotStreamError(t *testing.T) {
	for _, kind := range []string{"turn.interrupted", "turn.cancelled"} {
		s, session := incompleteBridge(textItems("说到一半")...)
		s.handleConversationEventLocked(incompleteFrame(session, kind, map[string]any{"status": "interrupted"}))
		msg := findSessionMessageByID(session, "assistant-1")
		if session.Running || session.NeedsAttention {
			t.Fatalf("%s: running=%v needsAttention=%v, want a settled, non-error turn", kind, session.Running, session.NeedsAttention)
		}
		if !messageInTerminalState(session, "assistant-1") {
			t.Fatalf("%s: message left non-terminal; the stream-close fallback would report it as a stream error", kind)
		}
		row := lastStatusItem(msg)
		if row == nil || row.Level != "warning" || row.Text != "已停止" {
			t.Fatalf("%s: want a plain stopped row (warning level), got %+v", kind, row)
		}
		if msg.Items[0].Text != "说到一半" {
			t.Fatalf("%s: content changed: %q", kind, msg.Items[0].Text)
		}
	}
}

// incompleteBridge 造一个正在产出、带初始正文的 assistant 消息。
func incompleteBridge(items ...ThreadItem) (*BridgeService, *sessionState) {
	s := &BridgeService{
		prompts:              map[string]*promptState{},
		runningConversations: map[string]*runningConversationState{},
	}
	session := &sessionState{
		ID:      "session-incomplete",
		Title:   "未完成的一轮",
		Running: true,
		Messages: []ChatMessage{
			// turnID 让保存与重载按轮次合并消息（与线上一致）。
			{ID: "assistant-1", Role: "assistant", State: "streaming", Items: items, turnID: "turn-1"},
		},
	}
	return s, session
}

func textItems(texts ...string) []ThreadItem {
	out := make([]ThreadItem, 0, len(texts))
	for i, text := range texts {
		out = append(out, ThreadItem{ID: "msg-" + string(rune('a'+i)), Kind: "agent_message", Text: text, Status: "streaming"})
	}
	return out
}

func lastStatusItem(message *ChatMessage) *ThreadItem {
	for i := len(message.Items) - 1; i >= 0; i-- {
		if message.Items[i].Kind == "status" {
			return &message.Items[i]
		}
	}
	return nil
}

// kernelNotice 按内核的线上形状构造说明行的 item_completed 载荷。
func kernelNotice(turnID, reason string) map[string]any {
	return map[string]any{"item": map[string]any{
		"id": turnID + "_incomplete_" + reason, "kind": "status",
		"text": "输出达到长度上限，本轮到此停止，内容不完整。", "level": "warning", "status": "completed",
	}}
}

func incompleteFrame(session *sessionState, kind string, payload map[string]any) conversationEventFrame {
	return conversationEventFrame{
		session:            session,
		sessionID:          session.ID,
		assistantMessageID: "assistant-1",
		kind:               kind,
		event:              adapter.Event{Type: kind, EventType: kind, Payload: payload},
	}
}
