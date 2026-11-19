package cli

import (
	"context"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
	"strings"
	"testing"
	"time"
)

func TestHeadlessSurfacesRequiredInteractionWithoutWaitingForTimeout(t *testing.T) {
	for _, kind := range []protocol.EventType{protocol.EventTypeApprovalReq, protocol.EventTypeTurnWaitingApproval, "turn.request_user_input"} {
		for _, format := range []string{"text", "json", "stream-json"} {
			t.Run(string(kind)+"/"+format, func(t *testing.T) {
				events := make(chan protocol.Envelope, 1)
				release := make(chan struct{})
				defer close(release)
				started := make(chan coreapi.StartTurnRequest, 1)
				engine := &printTestEngine{state: printTestStateService{}, sessions: printTestSessionService{session: coreapi.Session{ID: "requires-human"}}, turns: printTestTurnService{started: started, release: release}, events: printTestEventSubscriber{events: events}}
				go func() {
					req := <-started
					events <- protocol.NewEvent(kind, protocol.EventOptions{SessionID: req.SessionID, TurnID: req.TurnID, Payload: map[string]any{"approval_id": "approval-1"}})
				}()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				var err error
				output := captureStdout(t, func() {
					if format == "stream-json" {
						err = runStreamJSONTurn(ctx, engine, "local task", time.Now(), "")
					} else {
						_, err = runSingleTurn(ctx, engine, "local task", format, "")
					}
				})
				if err == nil || !strings.Contains(err.Error(), "requires-human") || ctx.Err() != nil {
					t.Fatalf("interaction not surfaced immediately: %v (context: %v)", err, ctx.Err())
				}
				if kind != "turn.request_user_input" && !strings.Contains(err.Error(), "--approval-mode never") {
					t.Fatalf("missing unattended guidance: %v", err)
				}
				if format == "stream-json" && (!strings.Contains(output, string(kind)) || strings.Contains(output, "turn.completed")) {
					t.Fatalf("invalid interaction stream: %s", output)
				}
			})
		}
	}
}
