package adapter

import (
	"slices"
	"testing"

	"github.com/eosaios/eos/pkg/protocol"
)

func TestProtocolEnvelopeToEventNormalizesRustTurnEventAndKeepsTopLevelTurnID(t *testing.T) {
	event := protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeTurnItemDelta,
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "agent-1",
		Payload:   map[string]any{"text": "hello"},
	})

	if event.Kind() != string(protocol.EventTypeItemDelta) {
		t.Fatalf("Kind()=%q, want item.delta", event.Kind())
	}
	if event.EventType != string(protocol.EventTypeTurnItemDelta) {
		t.Fatalf("EventType=%q, want raw turn.item_delta", event.EventType)
	}
	if event.TurnID != "turn-1" {
		t.Fatalf("TurnID=%q, want turn-1", event.TurnID)
	}
	if event.AgentID != "agent-1" {
		t.Fatalf("AgentID=%q, want agent-1", event.AgentID)
	}
	if event.EffectiveMessage() != "hello" {
		t.Fatalf("EffectiveMessage()=%q, want hello", event.EffectiveMessage())
	}
	if got := stringValue(event.Payload, "original_event_type"); got != string(protocol.EventTypeTurnItemDelta) {
		t.Fatalf("original_event_type=%q, want %q", got, protocol.EventTypeTurnItemDelta)
	}
	if !runtimeJSONRPCEventMatchesFilter(event, runtimeJSONRPCEventFilter{SessionID: "session-1", TurnID: "turn-1", AgentID: "agent-1"}) {
		t.Fatal("event did not match filter by top-level turn_id/agent_id")
	}
}

func TestProtocolEnvelopeToEventExtractsToolNameAndDisplayFromNestedItem(t *testing.T) {
	event := protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeTurnItemCompleted,
		SessionID: "session-1",
		TurnID:    "turn-1",
		Payload: map[string]any{
			"item": map[string]any{
				"name":      "read_file",
				"arguments": `{"path": "src/main.go"}`,
				"result": map[string]any{
					"display": "读取主入口文件。",
					"status":  "success",
				},
			},
		},
	})

	if event.EventType != string(protocol.EventTypeTurnItemCompleted) {
		t.Fatalf("EventType=%q, want raw turn.item_completed", event.EventType)
	}
	if got := stringValue(event.Payload, "tool_name"); got != "read_file" {
		t.Fatalf("tool_name=%q, want read_file", got)
	}
	if got := stringValue(event.Payload, "message"); got != "读取主入口文件。" {
		t.Fatalf("message=%q, want 读取主入口文件。", got)
	}
	if got := event.EffectiveMessage(); got != "读取主入口文件。" {
		t.Fatalf("EffectiveMessage()=%q, want 读取主入口文件。", got)
	}
}

func TestRuntimeJSONRPCEventMatchesFilterUsesEventTypes(t *testing.T) {
	event := protocolEnvelopeToEvent(protocol.Envelope{
		EventType: protocol.EventTypeTurnCompleted,
		SessionID: "session-1",
		TurnID:    "turn-1",
		Payload:   map[string]any{"text": "done"},
	})

	if !runtimeJSONRPCEventMatchesFilter(event, runtimeJSONRPCEventFilter{EventTypes: []string{"turn.completed"}}) {
		t.Fatal("turn.completed event should match raw event_types filter")
	}
	if runtimeJSONRPCEventMatchesFilter(event, runtimeJSONRPCEventFilter{EventTypes: []string{"request.completed"}}) {
		t.Fatal("turn.completed event should not match normalized compatibility event_types filter")
	}
}

func TestRuntimeRPCSubscriptionEventTypesForGlobalStateSync(t *testing.T) {
	got := runtimeRPCSubscriptionEventTypes(runtimeJSONRPCEventFilter{})
	if len(got) == 0 {
		t.Fatal("expected non-empty event_types for global state sync subscription")
	}
	if slices.Contains(got, "turn.text_delta") {
		t.Fatal("global state sync subscription should not include turn.text_delta")
	}
	if !slices.Contains(got, "turn.completed") {
		t.Fatal("global state sync subscription should include turn.completed")
	}
	if nonGlobal := runtimeRPCSubscriptionEventTypes(runtimeJSONRPCEventFilter{TurnID: "turn-1"}); len(nonGlobal) != 0 {
		t.Fatalf("turn-scoped subscription event_types=%v, want nil", nonGlobal)
	}
}
