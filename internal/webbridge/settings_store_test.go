package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStayInTrayDefaultsTrueAndRoundTrips(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	globalPath := filepath.Join(home, ".eos.json")
	if err := os.WriteFile(globalPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	// 旧配置无 stay_in_tray：默认开。
	initial, err := LoadGUISettings(globalPath, "", GUISettingsDefaults{Language: "zh", Theme: "system"})
	if err != nil {
		t.Fatalf("initial LoadGUISettings() error = %v", err)
	}
	if !initial.Workspace.StayInTray {
		t.Fatalf("legacy config should default StayInTray=true")
	}

	// 关闭并保存：落盘 stay_in_tray=false，重读保持。
	if _, err := SaveGUISettings(globalPath, "", GUISettingsSaveInput{
		Language:             "zh",
		LogDir:               filepath.Join(root, "logs"),
		Theme:                "light",
		AutoContext:          true,
		DesktopNotifications: true,
		GitCommitReminder:    true,
		StayInTray:           false,
		UseMemory:            true,
		MaxInjectKB:          48,
		WatchDebounceMs:      500,
		PollIntervalSec:      5,
		PlanPromptStyle:      "concise",
	}, GUISettingsDefaults{Language: "zh", Theme: "system"}); err != nil {
		t.Fatalf("SaveGUISettings() error = %v", err)
	}
	after, err := LoadGUISettings(globalPath, "", GUISettingsDefaults{Language: "zh", Theme: "system"})
	if err != nil {
		t.Fatalf("after LoadGUISettings() error = %v", err)
	}
	if after.Workspace.StayInTray {
		t.Fatalf("StayInTray should persist as false after save")
	}
}

func TestSaveGUISettingsKeepsCLILanguageSeparate(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// 初始：CLI 已经把 language=en 写进 ~/.eos.json。
	globalPath := filepath.Join(home, ".eos.json")
	logDir := filepath.Join(root, "logs")
	if err := os.WriteFile(globalPath, []byte("{\n  \"language\": \"en\",\n  \"models\": [{\"name\": \"demo\"}],\n  \"trusted_workspaces\": [\"repo-a\"]\n}\n"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	// 桌面端把语言改成 zh：必须落到 gui_language，不能动 CLI 的 language。
	snapshot, err := SaveGUISettings(globalPath, "", GUISettingsSaveInput{
		Language:             "zh",
		LogDir:               logDir,
		Theme:                "light",
		AutoContext:          false,
		DesktopNotifications: false,
		MaxInjectKB:          72,
		WatchDebounceMs:      900,
		PollIntervalSec:      11,
		PlanPromptStyle:      "concise",
	}, GUISettingsDefaults{
		Language: "en",
		Theme:    "system",
	})
	if err != nil {
		t.Fatalf("SaveGUISettings() error = %v", err)
	}
	if snapshot.Language != "zh" {
		t.Fatalf("snapshot.Language=%q, want zh", snapshot.Language)
	}

	raw, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("read global config: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal global config: %v", err)
	}
	// CLI 的 language 必须原样保留为 en，不被桌面端覆盖。
	if got := doc["language"]; got != "en" {
		t.Fatalf("language=%v, want en (CLI 字段不能被桌面端覆盖)", got)
	}
	// 桌面端的语言落到独立的 gui_language。
	if got := doc["gui_language"]; got != "zh" {
		t.Fatalf("gui_language=%v, want zh", got)
	}
	if got := doc["log_dir"]; got != logDir {
		t.Fatalf("log_dir=%v, want %q", got, logDir)
	}
	if _, ok := doc["models"]; !ok {
		t.Fatal("expected models to be preserved")
	}
	if _, ok := doc["trusted_workspaces"]; !ok {
		t.Fatal("expected trusted_workspaces to be preserved")
	}

	// 再次读取：桌面端应该拿到 gui_language（zh），而不是 CLI 的 language（en）。
	loaded, err := LoadGUISettings(globalPath, "", GUISettingsDefaults{
		Language: "zh",
		Theme:    "system",
	})
	if err != nil {
		t.Fatalf("LoadGUISettings() error = %v", err)
	}
	if loaded.Language != "zh" {
		t.Fatalf("loaded.Language=%q, want zh (应该优先读 gui_language)", loaded.Language)
	}
}

// 旧配置迁移：用户升级到新版本后，第一次启动还没有 gui_language 字段，
// 此时桌面端应该 fallback 到 language，让历史设置不丢失。
func TestLoadGUISettingsFallsBackToCLILanguageWhenGuiLanguageMissing(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	globalPath := filepath.Join(home, ".eos.json")
	// 旧配置只有 language=en，没有 gui_language。
	if err := os.WriteFile(globalPath, []byte("{\n  \"language\": \"en\"\n}\n"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	snapshot, err := LoadGUISettings(globalPath, "", GUISettingsDefaults{
		Language: "zh",
		Theme:    "system",
	})
	if err != nil {
		t.Fatalf("LoadGUISettings() error = %v", err)
	}
	if snapshot.Language != "en" {
		t.Fatalf("snapshot.Language=%q, want en (没有 gui_language 时应 fallback 到 language)", snapshot.Language)
	}
}

func TestSaveGUISettingsPreservesUnknownWorkspaceKeys(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "repo-a")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".eos"), 0o755); err != nil {
		t.Fatalf("mkdir workspace settings dir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	globalPath := filepath.Join(home, ".eos.json")
	if err := os.WriteFile(globalPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	workspacePath := ResolveWorkspaceSettingsPath(workspace)
	if err := os.WriteFile(workspacePath, []byte("{\n  \"language\": \"legacy\",\n  \"theme\": \"dark\",\n  \"watch_mode\": \"manual\",\n  \"custom_flag\": true,\n  \"trusted\": true,\n  \"trusted_at\": \"2026-01-02T03:04:05Z\"\n}\n"), 0o644); err != nil {
		t.Fatalf("write workspace settings: %v", err)
	}

	snapshot, err := SaveGUISettings(globalPath, workspace, GUISettingsSaveInput{
		Language:             "en",
		LogDir:               filepath.Join(root, "global-logs"),
		Theme:                "light",
		SandboxMode:          "danger-full-access",
		AutoContext:          false,
		DesktopNotifications: true,
		MaxInjectKB:          64,
		WatchDebounceMs:      750,
		PollIntervalSec:      9,
		PlanPromptStyle:      "structured",
	}, GUISettingsDefaults{
		Language: "zh",
		Theme:    "system",
	})
	if err != nil {
		t.Fatalf("SaveGUISettings() error = %v", err)
	}

	if snapshot.WorkspaceSettingsPath != workspacePath {
		t.Fatalf("WorkspaceSettingsPath=%q, want %q", snapshot.WorkspaceSettingsPath, workspacePath)
	}
	if snapshot.LogDir != filepath.Join(root, "global-logs") {
		t.Fatalf("LogDir=%q, want %q", snapshot.LogDir, filepath.Join(root, "global-logs"))
	}
	if !snapshot.Workspace.Trusted {
		t.Fatal("expected trusted flag to be preserved")
	}
	if snapshot.Workspace.TrustedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("TrustedAt=%q, want preserved timestamp", snapshot.Workspace.TrustedAt)
	}
	if snapshot.Workspace.SandboxMode != "danger-full-access" {
		t.Fatalf("SandboxMode=%q, want danger-full-access", snapshot.Workspace.SandboxMode)
	}

	raw, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatalf("read workspace settings: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal workspace settings: %v", err)
	}
	if got := doc["theme"]; got != "light" {
		t.Fatalf("theme=%v, want light", got)
	}
	if got := doc["sandbox_mode"]; got != "danger-full-access" {
		t.Fatalf("sandbox_mode=%v, want danger-full-access", got)
	}
	if got := doc["auto_context"]; got != false {
		t.Fatalf("auto_context=%v, want false", got)
	}
	if got := doc["watch_mode"]; got != "manual" {
		t.Fatalf("watch_mode=%v, want manual", got)
	}
	if got := doc["custom_flag"]; got != true {
		t.Fatalf("custom_flag=%v, want true", got)
	}
	if got := doc["language"]; got != "legacy" {
		t.Fatalf("workspace language=%v, want preserved legacy value", got)
	}
	if _, exists := doc["log_dir"]; exists {
		t.Fatal("expected log_dir to stay out of workspace settings")
	}
}

func TestLoadGUISettingsDefaultsLegacyWorkspaceSandboxModeToWorkspace(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "repo-a")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".eos"), 0o755); err != nil {
		t.Fatalf("mkdir workspace settings dir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	globalPath := filepath.Join(home, ".eos.json")
	if err := os.WriteFile(globalPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	workspacePath := ResolveWorkspaceSettingsPath(workspace)
	if err := os.WriteFile(workspacePath, []byte("{\n  \"theme\": \"dark\",\n  \"auto_context\": false\n}\n"), 0o644); err != nil {
		t.Fatalf("write legacy workspace settings: %v", err)
	}

	snapshot, err := LoadGUISettings(globalPath, workspace, GUISettingsDefaults{
		Language: "zh",
		Theme:    "system",
	})
	if err != nil {
		t.Fatalf("LoadGUISettings() error = %v", err)
	}
	if snapshot.Workspace.SandboxMode != "workspace-write" {
		t.Fatalf("SandboxMode=%q, want workspace-write (legacy default normalized)", snapshot.Workspace.SandboxMode)
	}
	if snapshot.Workspace.Theme != "dark" {
		t.Fatalf("Theme=%q, want dark", snapshot.Workspace.Theme)
	}
	if snapshot.Workspace.AutoContext {
		t.Fatal("expected auto_context=false to be preserved")
	}
}

func TestResolveWorkspaceSettingsPathFallsBackToHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got := ResolveWorkspaceSettingsPath("")
	want := filepath.Join(home, ".eos", "settings.json")
	if got != want {
		t.Fatalf("ResolveWorkspaceSettingsPath(\"\")=%q, want %q", got, want)
	}
}

// 记忆注入开关：旧配置（无 use_memory key）默认开；关闭后持久化并回读为关。
func TestSaveGUISettingsUseMemoryRoundTrip(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	globalPath := filepath.Join(home, ".eos.json")
	if err := os.WriteFile(globalPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	defaults := GUISettingsDefaults{Language: "zh", Theme: "system"}

	loaded, err := LoadGUISettings(globalPath, "", defaults)
	if err != nil {
		t.Fatalf("LoadGUISettings() error = %v", err)
	}
	if !loaded.Workspace.UseMemory {
		t.Fatal("expected UseMemory to default to true when key is missing")
	}

	snapshot, err := SaveGUISettings(globalPath, "", GUISettingsSaveInput{
		Language:             "zh",
		Theme:                "system",
		AutoContext:          true,
		DesktopNotifications: true,
		UseMemory:            false,
		MaxInjectKB:          48,
		WatchDebounceMs:      500,
		PollIntervalSec:      5,
	}, defaults)
	if err != nil {
		t.Fatalf("SaveGUISettings() error = %v", err)
	}
	if snapshot.Workspace.UseMemory {
		t.Fatal("expected UseMemory=false after save")
	}

	reloaded, err := LoadGUISettings(globalPath, "", defaults)
	if err != nil {
		t.Fatalf("LoadGUISettings() error = %v", err)
	}
	if reloaded.Workspace.UseMemory {
		t.Fatal("expected UseMemory=false to persist across reload")
	}
}
