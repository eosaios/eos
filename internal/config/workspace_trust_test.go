package config

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceTrustPathUsesWorkspaceLocalEOSDir(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "repo")
	got := WorkspaceTrustPath(workspace)
	want := filepath.Join(NormalizeWorkspacePath(workspace), ".eos", "trusted.json")
	if got != want {
		t.Fatalf("WorkspaceTrustPath()=%q, want %q", got, want)
	}
}

func TestTrustWorkspaceLocalPersistsTrustedState(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if IsWorkspaceTrustedLocal(workspace) {
		t.Fatal("workspace should not be trusted before marker exists")
	}
	if err := TrustWorkspaceLocal(workspace); err != nil {
		t.Fatalf("TrustWorkspaceLocal() error = %v", err)
	}
	if !IsWorkspaceTrustedLocal(workspace) {
		t.Fatal("workspace should be trusted after marker is written")
	}

	state, err := LoadWorkspaceTrust(workspace)
	if err != nil {
		t.Fatalf("LoadWorkspaceTrust() error = %v", err)
	}
	if !state.Trusted {
		t.Fatalf("Trusted=%v, want true", state.Trusted)
	}
	if state.TrustedAt == "" {
		t.Fatal("TrustedAt should be populated")
	}
}

func TestTrustWorkspaceLocalIsIdempotent(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "repo")
	if err := TrustWorkspaceLocal(workspace); err != nil {
		t.Fatalf("first TrustWorkspaceLocal() error = %v", err)
	}
	if err := TrustWorkspaceLocal(workspace); err != nil {
		t.Fatalf("second TrustWorkspaceLocal() error = %v", err)
	}
	if !IsWorkspaceTrustedLocal(workspace) {
		t.Fatal("workspace should remain trusted")
	}
}
