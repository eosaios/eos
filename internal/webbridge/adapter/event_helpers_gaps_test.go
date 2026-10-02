package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// event_helpers 纯函数域全臂批测：事件构造回填、信封转换、turn.* 归一、
// 消息提取与别名归一。

import (
	"testing"

	"github.com/eosaios/eos/pkg/protocol"
)

func TestNewAdapterEventFallbackFill(t *testing.T) {
	// delta/final/reasoning：payload 缺 text 时用 message 回填（原样不 trim）。
	ev := newAdapterEvent(protocol.EventTypeItemDelta, "TextDelta", "r1", " 消息体 ", nil)
	if ev.Payload["text"] != " 消息体 " {
		t.Fatalf("text 回填 = %v", ev.Payload["text"])
	}
	ev = newAdapterEvent(protocol.EventTypeTextFinal, "", "", "", map[string]any{"text": "已有"})
	if ev.Payload["text"] != "已有" {
		t.Fatalf("已有 text 不覆盖: %v", ev.Payload["text"])
	}
	// 审批：approval_id + message 双回填。
	ev = newAdapterEvent(protocol.EventTypeApprovalReq, "ConfirmRequired", "ap-1", "请确认", nil)
	if ev.Payload["approval_id"] != "ap-1" || ev.Payload["message"] != "请确认" {
		t.Fatalf("审批回填 = %v", ev.Payload)
	}
	ev = newAdapterEvent(protocol.EventTypeApprovalReq, "", "", "", map[string]any{"approval_id": "x", "message": "y"})
	if ev.Payload["approval_id"] != "x" || ev.Payload["message"] != "y" {
		t.Fatalf("审批已有键不覆盖: %v", ev.Payload)
	}
	// 询问：inquiry_id + question 双回填。
	ev = newAdapterEvent(protocol.EventTypeInquiryReq, "Inquiry", "q-1", "问题", nil)
	if ev.Payload["inquiry_id"] != "q-1" || ev.Payload["question"] != "问题" {
		t.Fatalf("询问回填 = %v", ev.Payload)
	}
	ev = newAdapterEvent(protocol.EventTypeInquiryReq, "", "", "", map[string]any{"inquiry_id": "x", "question": "y"})
	if ev.Payload["inquiry_id"] != "x" || ev.Payload["question"] != "y" {
		t.Fatalf("询问已有键不覆盖: %v", ev.Payload)
	}
	// 失败：error 回填。
	ev = newAdapterEvent(protocol.EventTypeRequestFailed, "Error", "r", " 炸了 ", nil)
	if ev.Payload["error"] != " 炸了 " {
		t.Fatalf("error 回填 = %v", ev.Payload["error"])
	}
	ev = newAdapterEvent(protocol.EventTypeRequestFailed, "", "", "", map[string]any{"error": "已有"})
	if ev.Payload["error"] != "已有" {
		t.Fatalf("已有 error 不覆盖: %v", ev.Payload)
	}
	// 默认臂：message 回填。
	ev = newAdapterEvent(protocol.EventTypeRequestDone, "", "", "完成说明", nil)
	if ev.Payload["message"] != "完成说明" {
		t.Fatalf("默认 message 回填 = %v", ev.Payload["message"])
	}
	// message 空时从 payload 提取；legacyType 空时从协议类型派生。
	ev = newAdapterEvent(protocol.EventTypeItemDelta, "", "r", "", map[string]any{"delta": "增量"})
	if ev.Message != "增量" {
		t.Fatalf("Message 提取 = %q", ev.Message)
	}
	if ev.Type != "TextDelta" {
		t.Fatalf("legacyType 派生 = %q", ev.Type)
	}
}

func TestEventEffectiveAccessors(t *testing.T) {
	// EffectiveRequestID 四臂。
	if got := (Event{RequestID: " rid "}).EffectiveRequestID(); got != "rid" {
		t.Fatalf("直接 rid = %q", got)
	}
	ev := Event{Payload: map[string]any{"approval_id": "ap"}, EventType: string(protocol.EventTypeApprovalReq)}
	if got := ev.EffectiveRequestID(); got != "ap" {
		t.Fatalf("审批 id 提取 = %q", got)
	}
	ev = Event{Payload: map[string]any{"inquiry_id": "iq"}, EventType: string(protocol.EventTypeInquiryReq)}
	if got := ev.EffectiveRequestID(); got != "iq" {
		t.Fatalf("询问 id 提取 = %q", got)
	}
	ev = Event{Payload: map[string]any{}, EventType: "turn.delta"}
	if got := ev.EffectiveRequestID(); got != "" {
		t.Fatalf("默认应空 = %q", got)
	}
	// EffectiveMessage：直接值 + payload 提取。
	if got := (Event{Message: " msg "}).EffectiveMessage(); got != "msg" {
		t.Fatalf("直接 message = %q", got)
	}
	ev = Event{Payload: map[string]any{"error": "失败原因"}, EventType: string(protocol.EventTypeRequestFailed)}
	if got := ev.EffectiveMessage(); got != "失败原因" {
		t.Fatalf("payload message = %q", got)
	}
	// payloadMap：Payload 优先、空时回落 Data。
	ev = Event{Payload: map[string]any{"k": "p"}, Data: map[string]any{"k": "d"}}
	if got := ev.payloadMap()["k"]; got != "p" {
		t.Fatalf("Payload 优先 = %v", got)
	}
	ev = Event{Data: map[string]any{"k": "d"}}
	if got := ev.payloadMap()["k"]; got != "d" {
		t.Fatalf("Data 回落 = %v", got)
	}
}

func TestProtocolEnvelopeToEventNormalization(t *testing.T) {
	// turn.item_completed → item.completed：工具名提升 + result.display 提升为 message。
	ev := protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeTurnItemCompleted,
		TurnID:    "",
		AgentID:   "",
		Payload: map[string]any{
			"turn_id":  "t-9",
			"agent_id": "a-9",
			"item": map[string]any{
				"name":   "shell",
				"result": map[string]any{"display": "命令完成"},
			},
		},
	})
	if ev.Payload["original_event_type"] != string(protocol.EventTypeTurnItemCompleted) {
		t.Fatalf("original_event_type = %v", ev.Payload["original_event_type"])
	}
	if ev.Payload["tool_name"] != "shell" {
		t.Fatalf("tool_name 提升 = %v", ev.Payload["tool_name"])
	}
	if ev.Payload["message"] != "命令完成" {
		t.Fatalf("display 提升 = %v", ev.Payload["message"])
	}
	if ev.TurnID != "t-9" || ev.AgentID != "a-9" {
		t.Fatalf("turn/agent id 回填 = %q/%q", ev.TurnID, ev.AgentID)
	}

	// 顶层 name/tool 提升两臂。
	ev = protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeTurnItemStarted,
		Payload:   map[string]any{"name": "editor"},
	})
	if ev.Payload["tool_name"] != "editor" {
		t.Fatalf("顶层 name 提升 = %v", ev.Payload["tool_name"])
	}
	ev = protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeTurnItemDelta,
		Payload:   map[string]any{"item": map[string]any{"tool": "fs"}},
	})
	if ev.Payload["tool_name"] != "fs" {
		t.Fatalf("嵌套 item.tool 提升 = %v", ev.Payload["tool_name"])
	}
	// 已有 tool_name 不覆盖；已有 message 不再提升 display。
	ev = protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeTurnItemCompleted,
		Payload: map[string]any{
			"tool_name": "既有",
			"message":   "既有说明",
			"item":      map[string]any{"result": map[string]any{"display": "不应覆盖"}},
		},
	})
	if ev.Payload["tool_name"] != "既有" || ev.Payload["message"] != "既有说明" {
		t.Fatalf("既有键不覆盖: %v", ev.Payload)
	}

	// turn.cancelled/interrupted → request.failed 的错误文案兜底。
	ev = protocolEnvelopeToEvent(protocol.Envelope{EventType: protocol.EventTypeTurnCancelled, Payload: map[string]any{}})
	if ev.Payload["error"] != "request cancelled" {
		t.Fatalf("cancelled 兜底 = %v", ev.Payload["error"])
	}
	ev = protocolEnvelopeToEvent(protocol.Envelope{EventType: protocol.EventTypeTurnInterrupted, Payload: map[string]any{}})
	if ev.Payload["error"] != "request interrupted" {
		t.Fatalf("interrupted 兜底 = %v", ev.Payload["error"])
	}
	ev = protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeTurnCancelled,
		Payload:   map[string]any{"error": "已有错误"},
	})
	if ev.Payload["error"] != "已有错误" {
		t.Fatalf("已有 error 不覆盖: %v", ev.Payload["error"])
	}

	// 审批/询问信封：request_id 从 payload 提取。
	ev = protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeApprovalReq,
		Payload:   map[string]any{"approval_id": "ap-2"},
	})
	if ev.RequestID != "ap-2" {
		t.Fatalf("审批 requestID = %q", ev.RequestID)
	}
	ev = protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeInquiryReq,
		Payload:   map[string]any{"inquiry_id": "iq-2"},
	})
	if ev.RequestID != "iq-2" {
		t.Fatalf("询问 requestID = %q", ev.RequestID)
	}
	// turn 等待审批归一 + 信封字段透传。
	env := protocol.Envelope{
		EventType: protocol.EventTypeTurnWaitingApproval,
		EventID:   "e-1", SessionID: "s-1", ThreadID: "th-1", TurnID: "t-1",
		AgentID: "a-1", CorrelationID: "c-1", RequestID: "rq-1", Source: "runtime",
		Version: protocol.VersionV1,
		Payload: map[string]any{"approval_id": "ap-3"},
	}
	ev = protocolEnvelopeToEvent(env)
	if ev.EventID != "e-1" || ev.SessionID != "s-1" || ev.ThreadID != "th-1" ||
		ev.TurnID != "t-1" || ev.AgentID != "a-1" || ev.CorrelationID != "c-1" ||
		ev.Source != "runtime" || ev.RequestID != "rq-1" {
		t.Fatalf("信封字段透传不符: %+v", ev)
	}
}

func TestLegacyTypeFromProtocolMatrix(t *testing.T) {
	cases := []struct {
		in   protocol.EventType
		want string
	}{
		{protocol.EventTypeItemDelta, "TextDelta"},
		{protocol.EventTypeTextFinal, "TextFinal"},
		{protocol.EventTypeApprovalReq, "ConfirmRequired"},
		{protocol.EventTypeInquiryReq, "Inquiry"},
		{protocol.EventTypeRequestFailed, "Error"},
		{protocol.EventTypeItemStarted, "ToolStep"},
		{protocol.EventTypeItemCompleted, "ToolStep"},
		{protocol.EventTypeTextReasoning, "ToolStep"},
		{protocol.EventTypeAgentStarted, "ToolStep"},
		{protocol.EventTypeAgentFailed, "ToolStep"},
		{protocol.EventTypeTaskUpdated, "ToolStep"},
		{protocol.EventTypeTaskFailed, "ToolStep"},
		{protocol.EventTypeRequestDone, ""},
	}
	for _, tc := range cases {
		if got := legacyTypeFromProtocol(tc.in); got != tc.want {
			t.Fatalf("legacyTypeFromProtocol(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEventMessageFromPayloadMatrix(t *testing.T) {
	// delta/final/reasoning 共用键序。
	if got := eventMessageFromPayload("item.delta", map[string]any{"delta": " d "}); got != "d" {
		t.Fatalf("delta = %q", got)
	}
	if got := eventMessageFromPayload(string(protocol.EventTypeTextReasoning), map[string]any{"text": "t"}); got != "t" {
		t.Fatalf("reasoning = %q", got)
	}
	// 审批/询问/失败。
	if got := eventMessageFromPayload(string(protocol.EventTypeApprovalReq), map[string]any{"title": "标题"}); got != "标题" {
		t.Fatalf("approval = %q", got)
	}
	if got := eventMessageFromPayload(string(protocol.EventTypeInquiryReq), map[string]any{"question": "问"}); got != "问" {
		t.Fatalf("inquiry = %q", got)
	}
	if got := eventMessageFromPayload(string(protocol.EventTypeRequestFailed), map[string]any{"message": "错"}); got != "错" {
		t.Fatalf("failed = %q", got)
	}
	// 注：ItemStarted/ItemCompleted 专臂在 normalizeEventKind 下不可达——
	// 该函数恒把 item.started/completed 归一为 item.delta，工具文案实际经
	// normalizeTurnEventPayload 的 display/message 提升链路展示（勿在这里
	// 断言「调用工具:/工具完成:」前缀文案）。
	// agent/task 族键序。
	if got := eventMessageFromPayload(string(protocol.EventTypeAgentProgress), map[string]any{"label": "标签"}); got != "标签" {
		t.Fatalf("agent = %q", got)
	}
	if got := eventMessageFromPayload(string(protocol.EventTypeTaskDone), map[string]any{"tool_name": "dev"}); got != "dev" {
		t.Fatalf("task = %q", got)
	}
	// 兜底键序（未知 kind 走最后 return）。
	if got := eventMessageFromPayload("turn.started", map[string]any{"title": "标题兜底"}); got != "标题兜底" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestNormalizeEventKindAliases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"TextDelta", string(protocol.EventTypeItemDelta)},
		{"item.delta", string(protocol.EventTypeItemDelta)},
		{"TextFinal", string(protocol.EventTypeTextFinal)},
		{"TextReasoning", string(protocol.EventTypeTextReasoning)},
		{"ConfirmRequired", string(protocol.EventTypeApprovalReq)},
		{"Inquiry", string(protocol.EventTypeInquiryReq)},
		{"Error", string(protocol.EventTypeRequestFailed)},
		{"ToolStep", string(protocol.EventTypeItemDelta)},
		{"", ""},
		{"custom.kind", "custom.kind"},
	}
	for _, tc := range cases {
		if got := normalizeEventKind(tc.in); got != tc.want {
			t.Fatalf("normalizeEventKind(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Kind()：EventType 优先，空则回落 Type。
	if got := (Event{EventType: " item.delta ", Type: ""}).Kind(); got != string(protocol.EventTypeItemDelta) {
		t.Fatalf("Kind from EventType = %q", got)
	}
	if got := (Event{EventType: "", Type: "TextFinal"}).Kind(); got != string(protocol.EventTypeTextFinal) {
		t.Fatalf("Kind from Type = %q", got)
	}
}

func TestNormalizeTurnEventPayloadNilArm(t *testing.T) {
	normalizeTurnEventPayload(nil, protocol.EventTypeTurnItemDelta, protocol.EventTypeItemDelta) // 不 panic
	// item.result 提取：item 缺位臂。
	if got := nestedItemResultString(map[string]any{"other": 1}, "display"); got != "" {
		t.Fatalf("item 缺位 = %q", got)
	}
}

func TestNestedItemExtraction(t *testing.T) {
	if got := nestedItemString(nil, "name"); got != "" {
		t.Fatalf("nil payload = %q", got)
	}
	if got := nestedItemString(map[string]any{"item": "非map"}, "name"); got != "" {
		t.Fatalf("非 map item = %q", got)
	}
	if got := nestedItemString(map[string]any{"item": map[string]any{"name": "shell"}}, "name"); got != "shell" {
		t.Fatalf("item.name = %q", got)
	}
	if got := nestedItemResultString(map[string]any{"item": map[string]any{"result": map[string]any{"display": "d"}}}, "display"); got != "d" {
		t.Fatalf("item.result.display = %q", got)
	}
	if got := nestedItemResultString(map[string]any{"item": map[string]any{"result": "非map"}}, "display"); got != "" {
		t.Fatalf("非 map result = %q", got)
	}
}

func TestStringValueSkips(t *testing.T) {
	if got := stringValue(map[string]any{"k": "   "}, "k"); got != "" {
		t.Fatalf("空白串应跳过: %q", got)
	}
	if got := stringValue(map[string]any{"k": 42}, "k"); got != "" {
		t.Fatalf("非字符串应跳过: %q", got)
	}
	if got := stringValue(map[string]any{"k": "v"}, "missing", "k"); got != "v" {
		t.Fatalf("键序命中 = %q", got)
	}
	if got := stringValue(nil, "k"); got != "" {
		t.Fatalf("nil = %q", got)
	}
}
