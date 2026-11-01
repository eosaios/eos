package headless

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/protocol"
)

func TestAwaitTurnRequiresTerminalBeforeStreamCloses(t *testing.T) {
	for _, name := range []string{"closed_events_before_start_ack", "closed_events_after_start_ack", "nil_events_after_start_ack"} {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				start := make(chan TurnStartResult, 1)
				var events chan protocol.Envelope
				switch name {
				case "closed_events_before_start_ack":
					events = make(chan protocol.Envelope)
					close(events)
					go func() { start <- TurnStartResult{} }()
				case "closed_events_after_start_ack":
					start <- TurnStartResult{}
					events = make(chan protocol.Envelope)
					close(events)
				case "nil_events_after_start_ack":
					start <- TurnStartResult{}
				}
				err := AwaitTurn(ctx, start, events, nil)
				cancel()
				if err == nil || !strings.Contains(err.Error(), "event stream closed before turn completed") {
					t.Fatalf("round %d: got %v; want explicit incomplete-stream error", i, err)
				}
			}
		})
	}
}

func TestAwaitTurnBufferedCompletionWinsOverFollowingEOF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := make(chan TurnStartResult, 1)
	start <- TurnStartResult{}
	events := make(chan protocol.Envelope, 1)
	events <- protocol.Envelope{EventType: protocol.EventTypeRequestDone}
	close(events)
	err := AwaitTurn(ctx, start, events, func(kind protocol.EventType, _ map[string]any, _ protocol.EventType) (bool, error) {
		return kind == protocol.EventTypeRequestDone, nil
	})
	if err != nil {
		t.Fatalf("completed turn rejected: %v", err)
	}
}
