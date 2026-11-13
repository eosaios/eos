package cli

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
)

// Exercise the real exec entrypoint with a framed sidecar that sends text deltas.
// A footer must never replay text already streamed to stdout.
func TestExecTextOutputPrintsStreamOnce(t *testing.T) {
	newFakeKernelEnv(t)
	output := captureStdout(t, func() {
		if err := runExec(context.Background(), execOptions{Prompt: "hi", Output: "text"}); err != nil {
			t.Fatal(err)
		}
	})
	if output != "你好，世界\n" {
		t.Fatalf("stdout = %q, want one streamed reply", output)
	}
}

func TestExecTextOutputKeepsFinalOnlyReply(t *testing.T) {
	events := make(chan protocol.Envelope, 3)
	release := make(chan struct{})
	started := make(chan coreapi.StartTurnRequest, 1)
	defer close(release)
	engine := &printTestEngine{
		state:    printTestStateService{},
		sessions: printTestSessionService{session: coreapi.Session{ID: "final-only"}},
		turns:    printTestTurnService{started: started, release: release},
		events:   printTestEventSubscriber{events: events},
	}
	go func() {
		req := <-started
		events <- protocol.NewEvent(protocol.EventType("turn.retry"), protocol.EventOptions{
			SessionID: req.SessionID, TurnID: req.TurnID,
			Payload: map[string]any{"message": "正在重试（1/10）"},
		})
		events <- protocol.NewEvent(protocol.EventTypeTurnItemCompleted, protocol.EventOptions{
			SessionID: req.SessionID, TurnID: req.TurnID,
			Payload: map[string]any{"item": map[string]any{"kind": "agent_message", "text": "final only"}},
		})
		events <- protocol.NewEvent(protocol.EventTypeTurnCompleted, protocol.EventOptions{
			SessionID: req.SessionID, TurnID: req.TurnID,
		})
	}()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	previousStderr := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = previousStderr; writer.Close() }()
	output := captureStdout(t, func() {
		content, err := runSingleTurn(context.Background(), engine, "hello", "text", "")
		if err != nil {
			t.Fatal(err)
		}
		writeExecOutput("text", ExecResult{Content: content})
	})
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = previousStderr
	diagnostics, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if output != "final only\n" {
		t.Fatalf("stdout = %q, want the final-only reply once", output)
	}
	if !strings.Contains(string(diagnostics), "正在重试（1/10）") {
		t.Fatalf("retry progress missing from stderr: %q", diagnostics)
	}
}
