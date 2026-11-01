package sidecar

import (
	"context"
	"errors"
	"github.com/eosaios/eos/internal/testexec"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
	"os"
	"strings"
	"testing"
	"time"
)

type blockedStreamWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w *blockedStreamWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *blockedStreamWriter) Close() error                { close(w.entered); <-w.release; return nil }

func TestProcessExitCanFinishWhileStreamCloseWaits(t *testing.T) {
	writer := &blockedStreamWriter{make(chan struct{}), make(chan struct{})}
	defer close(writer.release)
	client := protocoljsonrpc.NewStreamClient(protocoljsonrpc.NewStream(strings.NewReader(""), writer))
	process := &ProcessClient{client: client, waitCh: make(chan error, 1), waitDone: make(chan struct{})}
	go func() { <-writer.entered; close(process.waitDone) }()
	done := make(chan error, 1)
	go func() { done <- process.CloseWithTimeout(50 * time.Millisecond) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("process close deadline is blocked by stream close")
	}
}

func TestProcessCloseDeadlineKillsChildIgnoringEOF(t *testing.T) {
	binary := testexec.Build(t, "ignore-close-eof", `package main
import "time"
func main(){time.Sleep(10*time.Minute)}`)
	process, err := StartProcess(context.Background(), ProcessOptions{BinaryPath: binary})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := process.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Log(err)
		}
	})
	done := make(chan error, 1)
	go func() { done <- process.CloseWithTimeout(50 * time.Millisecond) }()
	select {
	case <-done:
		select {
		case <-process.waitDone:
		default:
			t.Fatal("child was not reaped")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pipe close prevented the child termination deadline")
	}
}
