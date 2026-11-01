package adapter

import (
	"context"
	"encoding/json"
	"github.com/eosaios/eos/pkg/coreapi"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
	"testing"
	"time"
)

func TestStdioGatewayRawTerminalClosesWithoutRenaming(t *testing.T) {
	for _, name := range []string{"turn.completed", "turn.error", "turn.cancelled", "turn.interrupted", "request.completed", "request.failed", "request.done"} {
		t.Run(name, func(t *testing.T) {
			client := serveRawTerminal(t, name)
			t.Cleanup(func() {
				if err := client.Close(); err != nil {
					t.Error(err)
				}
			})
			g := NewStdioGateway(client)
			events, _, err := g.CoreStartTurnStreamWithRequestRPC(context.Background(), coreapi.StartTurnRequest{SessionID: "s-1", TurnID: "t-f", Input: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			timer := time.NewTimer(time.Second)
			defer timer.Stop()
			var kinds []string
			for {
				select {
				case e, ok := <-events:
					if !ok {
						if len(kinds) != 1 || kinds[0] != name {
							t.Fatalf("wire event changed: got %v want [%s]", kinds, name)
						}
						return
					}
					kinds = append(kinds, e.EventType)
				case <-timer.C:
					t.Fatalf("terminal did not close stream: %v", kinds)
				}
			}
		})
	}
}

func serveRawTerminal(t *testing.T, name string) *StdioClient {
	t.Helper()
	client, conn := newPipeStdioClient(t)
	go func() {
		stream := protocoljsonrpc.NewStream(conn, conn)
		for {
			msg, err := stream.ReadMessage()
			if err != nil {
				return
			}
			if msg.Request == nil {
				continue
			}
			result := json.RawMessage(`{}`)
			switch msg.Request.Method {
			case protocoljsonrpc.MethodEventSubscribe:
				result = json.RawMessage(`{"subscription_id":"sub-1"}`)
			case protocoljsonrpc.MethodTurnStart:
				result = json.RawMessage(`{"id":"t-f","session_id":"s-1","status":"running"}`)
			}
			if err := stream.WriteMessage(protocoljsonrpc.Response{ID: msg.Request.ID, Result: result}); err != nil {
				return
			}
			if msg.Request.Method == protocoljsonrpc.MethodTurnStart {
				payload, err := json.Marshal(map[string]any{"version": "v1", "event_id": "e-terminal", "event_type": name, "session_id": "s-1", "turn_id": "t-f", "source": "runtime", "payload": map[string]any{"text": "fixture"}})
				if err != nil {
					return
				}
				if err := stream.WriteMessage(protocoljsonrpc.Notification{Method: protocoljsonrpc.NotificationEvent, Params: payload}); err != nil {
					return
				}
			}
		}
	}()
	return client
}
