package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintStartupFailureUsesExistingErrorFormat(t *testing.T) {
	for _, format := range []string{"json", "text", "stream-json"} {
		t.Run(format, func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "missing-core")
			t.Setenv("EOS_CORE_PATH", missing)
			stdout, stderr := capturePrintStartupFailure(t, func() error {
				return RunPrintMode(PrintOptions{Query: "hi", OutputFormat: format})
			})
			if format == "json" {
				var document map[string]string
				if err := json.Unmarshal([]byte(stdout), &document); err != nil {
					t.Fatalf("invalid error JSON %q: %v", stdout, err)
				}
				if len(document) != 1 || !strings.Contains(document["error"], missing) {
					t.Fatalf("error document = %v, want only error with missing path", document)
				}
				if stderr != "" {
					t.Fatalf("RunPrintMode JSON diagnostic leaked to stderr: %q", stderr)
				}
			} else if stdout != "" || !strings.Contains(stderr, missing) {
				t.Fatalf("stdout=%q stderr=%q, want diagnostic with missing path on stderr", stdout, stderr)
			}
		})
	}
}

func TestRootPrintFailureReturnsError(t *testing.T) {
	isolateHome(t)
	t.Setenv("EOS_CORE_PATH", filepath.Join(t.TempDir(), "missing-core"))
	previousQuery, previousFormat := printQuery, outputFormat
	printQuery, outputFormat = "hi", "json"
	t.Cleanup(func() { printQuery, outputFormat = previousQuery, previousFormat })
	if rootCmd.RunE == nil {
		t.Fatal("root command must propagate execution errors")
	}
	stdout, _ := capturePrintStartupFailure(t, func() error {
		return rootCmd.RunE(rootCmd, nil)
	})
	var document map[string]string
	if err := json.Unmarshal([]byte(stdout), &document); err != nil || document["error"] == "" {
		t.Fatalf("root print diagnostic = %q, decode error = %v", stdout, err)
	}
}

func capturePrintStartupFailure(t *testing.T, run func() error) (string, string) {
	t.Helper()
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutReader.Close()
	defer stdoutWriter.Close()
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stderrReader.Close()
	defer stderrWriter.Close()
	previousStdout, previousStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	defer func() { os.Stdout, os.Stderr = previousStdout, previousStderr }()
	if err := run(); err == nil {
		t.Fatal("missing core must return an error")
	}
	if err := stdoutWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatal(err)
	}
	stdout, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatal(err)
	}
	return string(stdout), string(stderr)
}
