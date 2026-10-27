package config

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"os"
	"path/filepath"
	"testing"
)

func setTestHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestLooksMaskedAPIKey(t *testing.T) {
	if LooksMaskedAPIKey("") || LooksMaskedAPIKey("   ") {
		t.Fatal("empty")
	}
	if !LooksMaskedAPIKey("****") {
		t.Fatal("all stars")
	}
	if !LooksMaskedAPIKey("abcd...wxyz") {
		t.Fatal("mask form")
	}
	if LooksMaskedAPIKey("sk-real-key-here") {
		t.Fatal("real key")
	}
	if LooksMaskedAPIKey("abcd.wxyz") {
		t.Fatal("not enough dots")
	}
}

func TestSnapshotAndRestorePlaintextAPIKeys(t *testing.T) {
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgPath := filepath.Join(home, ".eos.json")

	cfg := Config{
		Active: "m1",
		Models: []ModelEntry{
			{Name: "m1", APIKey: "sk-plain"},
			{Name: "m2", APIKey: "abcd...wxyz"},
			{Name: "m3"},
		},
	}
	if err := Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	snap := SnapshotPlaintextAPIKeys()
	if snap["m1"] != "sk-plain" {
		t.Fatalf("snap = %v", snap)
	}
	if _, ok := snap["m2"]; ok {
		t.Fatal("masked should not snapshot")
	}

	// 覆盖 masked 后回补
	cfg.Models[1].APIKey = "efgh...ijkl"
	if err := Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	RestorePlaintextAPIKeys(map[string]string{"m2": "sk-restored", "m1": "sk-keep"})
	cfg2, _ := Load()
	if cfg2.Models[1].APIKey != "sk-restored" {
		t.Fatalf("restored = %q", cfg2.Models[1].APIKey)
	}
	if cfg2.Models[0].APIKey != "sk-plain" {
		t.Fatalf("plain untouched = %q", cfg2.Models[0].APIKey)
	}

	// 空快照 no-op
	RestorePlaintextAPIKeys(nil)
}

func TestWorkspacesHelpers(t *testing.T) {
	setTestHome(t)

	if DefaultWorkspacePath() == "" {
		t.Fatal("default path")
	}
	if err := EnsureDefaultWorkspaceDir(); err != nil {
		t.Fatal(err)
	}

	// ResolveWorkspacePath
	if _, err := ResolveWorkspacePath("  "); err == nil {
		t.Fatal("empty")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err := ResolveWorkspacePath("~/proj")
	if err != nil || !filepath.IsAbs(got) {
		t.Fatalf("tilde = %q %v", got, err)
	}

	// NormalizeWorkspacePath
	if NormalizeWorkspacePath("  ") != "" {
		t.Fatal("blank")
	}
	if NormalizeWorkspacePath("./a/../b") == "" {
		t.Fatal("clean")
	}

	// PathsEqual
	if !PathsEqual("/a/b", "/a/b") {
		t.Fatal("equal")
	}
	if PathsEqual("", "/a") || PathsEqual("/a", "") {
		t.Fatal("blank")
	}

	// containsWorkspace / sameWorkspaceSlice / NormalizeWorkspaceState / ResolveForegroundWorkspace
	if containsWorkspace([]string{"/a"}, "/b") {
		t.Fatal("contains")
	}
	if !containsWorkspace([]string{"/a"}, "/a") {
		t.Fatal("contains hit")
	}
	if sameWorkspaceSlice([]string{"/a"}, []string{"/a"}) != true {
		t.Fatal("same slice")
	}
	if sameWorkspaceSlice([]string{"/a"}, []string{"/b"}) {
		t.Fatal("diff slice")
	}
	NormalizeWorkspaceState(&Config{})
	_ = ResolveForegroundWorkspace(Config{}, "")
	_ = ResolveForegroundWorkspace(Config{KnownWorkspaces: []string{"/ws"}, LastWorkspace: "/ws"}, "")
}

func TestActiveModelAndSave(t *testing.T) {
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, "sub", ".eos.json")

	cfg := Config{
		Active: "m1",
		Models: []ModelEntry{{Name: "m1", Model: "x"}, {Name: "m2"}},
	}
	if err := Save(cfg, path); err != nil {
		t.Fatal(err)
	}
	got, ok := ActiveModel(cfg)
	if !ok || got.Name != "m1" {
		t.Fatalf("active = %+v %v", got, ok)
	}
	if _, ok := ActiveModel(Config{Active: "nope"}); ok {
		t.Fatal("missing active")
	}

	// Load 往返
	loaded, p := Load()
	_ = loaded
	_ = p

	// tryMigrateLegacyMCPServers
	_, _ = tryMigrateLegacyMCPServers([]byte(`{"mcpServers":{}}`), &Config{})
}

func TestWorkspaceTrust(t *testing.T) {
	setTestHome(t)
	dir := t.TempDir()

	// 缺失 → 未信任
	if IsWorkspaceTrustedLocal(dir) {
		t.Fatal("not trusted yet")
	}
	if err := TrustWorkspaceLocal(dir); err != nil {
		t.Fatal(err)
	}
	if !IsWorkspaceTrustedLocal(dir) {
		t.Fatal("should be trusted")
	}
	state, err := LoadWorkspaceTrust(dir)
	if err != nil || !state.Trusted || state.TrustedAt == "" {
		t.Fatalf("state = %+v %v", state, err)
	}

	// 空 path
	if WorkspaceTrustPath("") == "" {
		t.Fatal("empty path fallback")
	}

	// 坏 JSON
	bad := filepath.Join(dir, ".eos")
	if err := os.WriteFile(filepath.Join(bad, workspaceTrustFileName), []byte("{"), 0o644); err != nil {
		// 目录可能已存在
		_ = err
	}
}
