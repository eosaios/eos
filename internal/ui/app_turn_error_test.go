package ui

import (
	"errors"
	"strings"
	"testing"
)

func TestFailedTurnEventAndRPCErrorRenderOnce(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "event-first", true: "rpc-first"}[reverse], func(t *testing.T) {
			setTestHome(t)
			app := newTestAppModel(t)
			app.appendHistory(historyEntry{kind: "user", content: "hello"})
			event := func() { app.handleAIResponseMsg(AIResponseMsg{Type: "error", Content: "HTTP 401: Invalid API Key"}) }
			rpc := func() { app.handleErrorMsg(ErrorMsg{Err: errors.New("HTTP 401: Invalid API Key")}) }
			if reverse {
				rpc()
				event()
			} else {
				event()
				rpc()
			}
			if len(app.history) != 2 {
				t.Fatalf("duplicate errors: %#v", app.history)
			}
			if !strings.Contains(app.history[1].content, "/model") {
				t.Fatal("missing recovery hint")
			}
			app.appendHistory(historyEntry{kind: "user", content: "retry"})
			rpc()
			if len(app.history) != 4 {
				t.Fatal("next turn failure must remain visible")
			}
		})
	}
}
