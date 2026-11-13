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
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	}()
	previousStderr := os.Stderr
	os.Stderr = writer
	writerClosed := false
	defer func() {
		os.Stderr = previousStderr
		if !writerClosed {
			if err := writer.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	output := captureStdout(t, func() {
		content, err := runSingleTurn(context.Background(), engine, "hello", "text", "")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeExecOutput("text", ExecResult{Content: content}); err != nil {
			t.Fatal(err)
		}
	})
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	writerClosed = true
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

func TestExecOutputReportsClosedStdout(t *testing.T) {
	closed, err := os.CreateTemp(t.TempDir(), "closed-output")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = closed
	defer func() { os.Stdout = previous }()
	for _, format := range []string{"text", "json"} {
		if err := writeExecOutput(format, ExecResult{Content: "reply"}); err == nil {
			t.Fatalf("%s output silently ignored closed stdout", format)
		}
	}
}
