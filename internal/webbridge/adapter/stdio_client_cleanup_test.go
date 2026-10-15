package adapter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eosaios/eos/internal/testexec"
)

func TestFailedProcessStartReleasesLogsAndAllowsRetry(t *testing.T) {
	dir := t.TempDir()
	badBinary := filepath.Join(dir, "invalid.exe")
	if err := os.WriteFile(badBinary, []byte("not a native executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	client := NewStdioClient(StdioClientOptions{CorePath: badBinary, CoreLogDir: dir})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := client.Start(context.Background()); err == nil {
		t.Fatal("invalid executable must fail")
	}
	if err := os.RemoveAll(filepath.Join(dir, "core")); err != nil {
		t.Fatalf("failed start retained its log handle: %v", err)
	}
	client.opts.CorePath = testexec.Build(t, "eos-core", `package main
import("io";"os")
func main(){io.Copy(io.Discard,os.Stdin)}
`)
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("retry after failed spawn: %v", err)
	}
	logFile, ok := client.cmd.Stderr.(*os.File)
	if !ok {
		t.Fatal("expected an open log file")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := logFile.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("Close returned before releasing log: %v", err)
	}
	select {
	case <-client.Done():
	default:
		t.Fatal("Close returned before process completion")
	}
}
