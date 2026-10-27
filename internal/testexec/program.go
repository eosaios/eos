// Package testexec builds native test processes without shell or Python dependencies.
package testexec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Scenario tests deliberately change PATH/HOME; fixture compilation must keep
// the original toolchain and cache environment, while launched peers receive
// the scenario's environment from their callers.
var (
	goExecutable, goExecutableErr = exec.LookPath("go")
	buildEnvironment              = os.Environ()
)

// Build compiles source into a native executable owned by this test.
func Build(t testing.TB, name, source string) string {
	t.Helper()
	if goExecutableErr != nil {
		t.Fatalf("locate fixture Go compiler: %v", goExecutableErr)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	name = Name(name)
	binary := filepath.Join(dir, name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goExecutable, "build", "-o", binary, src)
	cmd.Env = append([]string(nil), buildEnvironment...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build native fixture: %v\n%s", err, output)
	}
	return binary
}

// Name returns the executable filename for the native test platform.
func Name(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
