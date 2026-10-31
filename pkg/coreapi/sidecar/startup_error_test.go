package sidecar

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	// Windows 上 os.Executable 与 exec.Cmd.Path 可能分别落到短路径 / 长路径
	// 或大小写不同的盘符形式上；诊断断言按 Clean+Base 比对，避免假阴性。
	binary = filepath.Clean(binary)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = StartRemoteEngine(ctx, ProcessOptions{
		BinaryPath: binary,
		Args:       []string{"-test.run=^$"},
		Env:        map[string]string{startupExitFixtureEnv: "exit-7"},
	})
	if err == nil {
		t.Fatal("startup error = nil, want diagnostic with binary path and exit status 7")
	}
	msg := err.Error()
	if !errorMentionsBinary(msg, binary) {
		t.Fatalf("startup error = %v, want diagnostic containing binary %q (or its base name)", err, binary)
	}
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 7 {
		t.Fatalf("startup error = %v, want wrapped exit status 7", err)
	}
}

// errorMentionsBinary 判断启动诊断是否点名了被拉起的二进制。
// 接受完整路径（大小写不敏感）或其 basename，覆盖 Windows 短/长路径差异。
func errorMentionsBinary(msg, binary string) bool {
	if strings.Contains(msg, binary) {
		return true
	}
	if strings.Contains(strings.ToLower(msg), strings.ToLower(binary)) {
		return true
	}
	base := filepath.Base(binary)
	return base != "" && base != "." && strings.Contains(msg, base)
}
