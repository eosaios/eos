package sidecar

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const startupExitFixtureEnv = "EOS_FIX_DE12_STARTUP_EXIT_FIXTURE"

func TestMain(m *testing.M) {
	if os.Getenv(startupExitFixtureEnv) == "exit-7" {
		os.Exit(7)
	}
	os.Exit(m.Run())
}

func TestCoreStartupExitIncludesBinaryAndExitStatus(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = StartRemoteEngine(ctx, ProcessOptions{
		BinaryPath: binary,
		Args:       []string{"-test.run=^$"},
		Env:        map[string]string{startupExitFixtureEnv: "exit-7"},
	})
	if err == nil || !strings.Contains(err.Error(), binary) {
		t.Fatalf("startup error = %v, want diagnostic containing %q", err, binary)
	}
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 7 {
		t.Fatalf("startup error = %v, want wrapped exit status 7", err)
	}
}
