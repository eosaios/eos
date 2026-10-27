package webbridge

import (
	"testing"
)

// bridge_message_codec_test.go 验证 ChatMessage ↔ SessionMessage 的持久化往返
// 契约：assistant 消息按 Items 展开为多条带 item_id/turn_id/kind 的 SessionMessage，
// 重载时 chatMessagesFromRuntime 必须能按 turn_id 合并、按 kind 完整重建 Items。
// 这是对 "AI 回复重启后消失" bug 的回归保护。

const codecTurnID = "turn_test_1"

func chatMessageWithItems(role, id string) ChatMessage {
	return ChatMessage{
		ID:        id,
		Role:      role,
		State:     "completed",
		CreatedAt: "2026-07-15T12:00:00+08:00",
		UpdatedAt: "2026-07-15T12:00:00+08:00",
	}
}

// assistant 消息带多种 item（正文/思考/工具调用+结果）展开后再读回，
// Items 的种类、文本、工具参数与结果必须完整保留。
func TestSessionMessagesFromChatMessage_AssistantItemsRoundTrip(t *testing.T) {
	src := chatMessageWithItems("assistant", "asst-1")
	src.turnID = codecTurnID
	src.Items = []ThreadItem{
		{ID: "i_reason", Kind: "reasoning", Reasoning: "思考一下", Status: "completed"},
		{ID: "i_msg", Kind: "agent_message", Text: "这是回复正文", Status: "completed"},
		{
			ID:       "i_tool",
			Kind:     "tool_call",
			ToolName: "read_file",
			ToolArgs: `{"path":"/a"}`,
			ToolResult: &ItemToolResult{
				Status: "success",
				Output: "file content",
			},
			Status: "completed",
		},
	}

	encoded := sessionMessagesFromChatMessage(src)
	if len(encoded) < 4 { // reasoning + agent_message + tool_call + tool result
		t.Fatalf("encoded messages = %d, want >= 4 (items 未正确展开)", len(encoded))
	}
	// tool_call 必须额外产出一条 role=tool 结果消息。
	var hasToolResult bool
	for _, m := range encoded {
		if m.Role == "tool" {
			hasToolResult = true
			if got := metadataString(m.Metadata["tool_call_id"]); got != "i_tool" {
				t.Errorf("tool result tool_call_id = %q, want i_tool", got)
			}
		}
	}
	if !hasToolResult {
		t.Errorf("缺少 role=tool 的工具结果消息")
	}

	// 读回：chatMessagesFromRuntime 按 turn_id 合并为单条 assistant ChatMessage。
	decoded := chatMessagesFromRuntime(encoded)
	if len(decoded) != 1 {
		t.Fatalf("decoded messages = %d, want 1 (应按 turn_id 合并为单条)", len(decoded))
	}
	got := decoded[0]
	if got.Role != "assistant" {
		t.Errorf("decoded role = %q, want assistant", got.Role)
	}
	if got.turnID != codecTurnID {
		t.Errorf("decoded turnID = %q, want %q", got.turnID, codecTurnID)
	}
	// 至少应重建出 reasoning / agent_message / tool_call 三种 item。
	kinds := map[string]bool{}
	textByKind := map[string]string{}
	for _, it := range got.Items {
		kinds[it.Kind] = true
		switch it.Kind {
		case "reasoning":
			textByKind["reasoning"] = it.Reasoning
		case "agent_message":
			textByKind["agent_message"] = it.Text
		case "tool_call":
			textByKind["tool_name"] = it.ToolName
			textByKind["tool_args"] = it.ToolArgs
			if it.ToolResult == nil {
				t.Errorf("tool_call %s 的 ToolResult 未回填", it.ID)
			} else if it.ToolResult.Output != "file content" {
				t.Errorf("tool result output = %q, want %q", it.ToolResult.Output, "file content")
			}
		}
	}
	for _, want := range []string{"reasoning", "agent_message", "tool_call"} {
		if !kinds[want] {
			t.Errorf("重建后缺少 kind=%s 的 item", want)
		}
	}
	if textByKind["reasoning"] != "思考一下" {
		t.Errorf("reasoning 文本 = %q, want %q", textByKind["reasoning"], "思考一下")
	}
	if textByKind["agent_message"] != "这是回复正文" {
		t.Errorf("agent_message 文本 = %q, want %q", textByKind["agent_message"], "这是回复正文")
	}
	if textByKind["tool_name"] != "read_file" {
		t.Errorf("tool_name = %q, want read_file", textByKind["tool_name"])
	}
	if textByKind["tool_args"] != `{"path":"/a"}` {
		t.Errorf("tool_args = %q, want 原值", textByKind["tool_args"])
	}
}

// user 消息仍单条持久化，Content 是唯一载体，且无 turn_id（独立气泡）。
func TestSessionMessagesFromChatMessage_UserStaysSingle(t *testing.T) {
	src := chatMessageWithItems("user", "user-1")
	src.Content = "用户提问"
	encoded := sessionMessagesFromChatMessage(src)
	if len(encoded) != 1 {
		t.Fatalf("user encoded = %d, want 1", len(encoded))
	}
	if encoded[0].Content != "用户提问" {
		t.Errorf("user content = %q, want 用户提问", encoded[0].Content)
	}
	if turnID := metadataString(encoded[0].Metadata["turn_id"]); turnID != "" {
		t.Errorf("user 消息不应带 turn_id, got %q", turnID)
	}
}

// 占位 assistant（IsPlaceholder 且无 Items）退化为单条 Content，
// 不应因展开逻辑丢消息。
func TestSessionMessagesFromChatMessage_PlaceholderFallback(t *testing.T) {
	src := chatMessageWithItems("assistant", "asst-ph")
	src.Content = "思考中"
	src.IsPlaceholder = true
	encoded := sessionMessagesFromChatMessage(src)
	if len(encoded) != 1 {
		t.Fatalf("placeholder encoded = %d, want 1", len(encoded))
	}
	if encoded[0].Content != "思考中" {
		t.Errorf("placeholder content = %q, want 思考中", encoded[0].Content)
	}
}
