package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"
)

// 未完成的原因挂在消息上，并随会话保存、重载不丢：原因编在内核说明行的
// item id 里，重载时据此还原。
func TestIncompleteReasonIsMarkedAndSurvivesReload(t *testing.T) {
	s, session := incompleteBridge(textItems("半截的回答")...)
	s.handleConversationEventLocked(incompleteFrame(session, "turn.item_completed", kernelNotice("turn-1", "max_output_tokens")))
	s.handleConversationEventLocked(incompleteFrame(session, "turn.completed", map[string]any{
		"status": "incomplete", "reason": "max_output_tokens",
	}))
	msg := findSessionMessageByID(session, "assistant-1")
	if msg.IncompleteReason != "max_output_tokens" {
		t.Fatalf("IncompleteReason = %q, want max_output_tokens", msg.IncompleteReason)
	}

	restored := chatMessagesFromRuntime(sessionMessagesFromChatMessage(*msg))
	if len(restored) != 1 {
		t.Fatalf("restored %d messages, want 1", len(restored))
	}
	got := restored[0]
	if got.IncompleteReason != "max_output_tokens" {
		t.Fatalf("reason lost on reload: %q", got.IncompleteReason)
	}
	row := lastStatusItem(&got)
	if row == nil || row.Level != "warning" || !strings.Contains(row.Text, "内容不完整") {
		t.Fatalf("explanation row lost on reload: %+v", got.Items)
	}
	if got.Items[0].Text != "半截的回答" {
		t.Fatalf("content lost on reload: %q", got.Items[0].Text)
	}
}

// 没见过的原因不猜含义（归为 unknown），但仍然按未完成处理。
func TestUnknownIncompleteReasonIsNormalizedNotGuessed(t *testing.T) {
	s, session := incompleteBridge(textItems("半截的回答")...)
	s.handleConversationEventLocked(incompleteFrame(session, "turn.completed", map[string]any{
		"status": "incomplete", "reason": "something_new",
	}))
	msg := findSessionMessageByID(session, "assistant-1")
	if msg.IncompleteReason != "unknown" {
		t.Fatalf("IncompleteReason = %q, want unknown", msg.IncompleteReason)
	}
}

func TestNormalCompletionHasNoIncompleteMark(t *testing.T) {
	s, session := incompleteBridge(textItems("完整的回答")...)
	s.handleConversationEventLocked(incompleteFrame(session, "turn.completed", map[string]any{"status": "completed"}))
	msg := findSessionMessageByID(session, "assistant-1")
	if msg.IncompleteReason != "" || lastStatusItem(msg) != nil {
		t.Fatalf("a normal completion must stay unmarked: %+v", msg)
	}
}

func TestIncompleteReasonFromItemID(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"turn-7_incomplete_max_output_tokens", "max_output_tokens"},
		{"turn-7_incomplete_Something-New", "unknown"},
		{"turn-7_incomplete_", ""},
		{"turn-7_msg_1", ""},
		{"status", ""},
	} {
		if got := incompleteReasonFromItemID(tc.id); got != tc.want {
			t.Fatalf("incompleteReasonFromItemID(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}
