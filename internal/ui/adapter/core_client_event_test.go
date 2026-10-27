package adapter

import (
	"testing"

	"github.com/eosaios/eos/pkg/protocol"
)

func TestRuntimeEventFromEnvelopeNormalizesRustTurnEvent(t *testing.T) {
	event := runtimeEventFromEnvelope(protocol.Envelope{
		EventType: protocol.EventTypeTurnItemDelta,
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "agent-1",
		Payload:   map[string]any{"text": "hello"},
	})

	if event.Type != string(protocol.EventTypeItemDelta) {
		t.Fatalf("Type=%q, want text.delta", event.Type)
	}
	if event.RID != "turn-1" {
		t.Fatalf("RID=%q, want turn-1", event.RID)
	}
	if event.Content != "hello" {
		t.Fatalf("Content=%q, want hello", event.Content)
	}
	if got, _ := event.Data["agent_id"].(string); got != "agent-1" {
		t.Fatalf("agent_id=%q, want agent-1", got)
	}
	if got, _ := event.Data["original_event_type"].(string); got != string(protocol.EventTypeTurnItemDelta) {
		t.Fatalf("original_event_type=%q, want %q", got, protocol.EventTypeTurnItemDelta)
	}
}
