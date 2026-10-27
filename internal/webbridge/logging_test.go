package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUsesWorkspaceStateDefaultsToFalseWithoutDevServer(t *testing.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "")
	t.Setenv("EOS_WORKSPACE_STATE", "")

	if UsesWorkspaceState() {
		t.Fatal("expected workspace state to be disabled without dev server or override")
	}
}

func TestUsesWorkspaceStateUsesDevServerWhenPresent(t *testing.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "http://127.0.0.1:9245")
	t.Setenv("EOS_WORKSPACE_STATE", "")

	if !UsesWorkspaceState() {
		t.Fatal("expected workspace state to be enabled when the frontend dev server is configured")
	}
}

func TestUsesWorkspaceStateRespectsOverride(t *testing.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "")
	t.Setenv("EOS_WORKSPACE_STATE", "true")

	if !UsesWorkspaceState() {
		t.Fatal("expected workspace state override to enable workspace state")
	}

	t.Setenv("FRONTEND_DEVSERVER_URL", "http://127.0.0.1:9245")
	t.Setenv("EOS_WORKSPACE_STATE", "false")

	if UsesWorkspaceState() {
		t.Fatal("expected workspace state override to disable workspace state")
	}
}

func TestDefaultLogDirIgnoresWorkspaceState(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("FRONTEND_DEVSERVER_URL", "http://127.0.0.1:9245")
	t.Setenv("EOS_WORKSPACE_STATE", "")

	var want string
	switch runtime.GOOS {
	case "windows":
		local := filepath.Join(root, "localappdata")
		t.Setenv("LOCALAPPDATA", local)
		t.Setenv("APPDATA", "")
		want = filepath.Join(local, "EOS", "logs")
	case "darwin":
		want = filepath.Join(home, "Library", "Logs", "EOS")
	default:
		state := filepath.Join(root, "state")
		t.Setenv("XDG_STATE_HOME", state)
		want = filepath.Join(state, "eos", "logs")
	}

	if got := defaultLogDir(); got != want {
		t.Fatalf("defaultLogDir()=%q, want %q", got, want)
	}
}
