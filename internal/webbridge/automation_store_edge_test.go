package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTempHome 把 HOME 指到临时目录，automationStorePath 随之隔离。
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows
	return home
}

func TestComputeNextRunAt(t *testing.T) {
	if got := computeNextRunAt("", true); got != "" {
		t.Fatalf("empty schedule = %q", got)
	}
	if got := computeNextRunAt("*/5 * * * *", false); got != "" {
		t.Fatalf("disabled = %q", got)
	}
	if got := computeNextRunAt("not-cron", true); got != "" {
		t.Fatalf("bad cron = %q", got)
	}
	got := computeNextRunAt("0 0 * * *", true)
	if got == "" {
		t.Fatal("valid cron should produce RFC3339")
	}
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Fatalf("not RFC3339: %q", got)
	}
}

func TestAutomationStoreSaveLoadRoundTrip(t *testing.T) {
	withTempHome(t)
	s := &BridgeService{}

	// 文件不存在 → 空状态
	st := s.loadAutomationStore()
	if len(st.Templates) != 0 || st.PresetEnabled == nil {
		t.Fatalf("empty load = %+v", st)
	}

	// 保存并回读
	want := automationStoreState{
		Templates: []automationStoreRecord{
			{
				ID:            "custom-1",
				Title:         "每日总结",
				Description:   "d",
				Prompt:        "p",
				Schedule:      "0 9 * * *",
				Enabled:       true,
				WorkspacePath: "/ws",
			},
			{ID: "   ", Title: "skip"}, // 空 ID 测试 hydrate 过滤
		},
		PresetEnabled: map[string]bool{"daily-report": true},
	}
	if err := s.saveAutomationStore(want); err != nil {
		t.Fatal(err)
	}
	got := s.loadAutomationStore()
	if len(got.Templates) != 2 {
		t.Fatalf("templates = %+v", got.Templates)
	}
	if got.Templates[0].ID != "custom-1" || !got.Templates[0].Enabled {
		t.Fatalf("rec = %+v", got.Templates[0])
	}
	if !got.PresetEnabled["daily-report"] {
		t.Fatal("preset enabled")
	}

	// 损坏 JSON → 空状态
	path := s.automationStorePath()
	if err := os.WriteFile(path, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	st = s.loadAutomationStore()
	if len(st.Templates) != 0 || st.PresetEnabled == nil {
		t.Fatalf("broken load = %+v", st)
	}
}

func TestHydrateAndAllTemplatesReadOnly(t *testing.T) {
	withTempHome(t)
	s := &BridgeService{}
	_ = s.saveAutomationStore(automationStoreState{
		Templates: []automationStoreRecord{
			{ID: "c1", Title: "T", Prompt: "P", Schedule: "0 0 * * *", Enabled: true},
			{ID: "  "},
		},
		PresetEnabled: map[string]bool{},
	})

	s.hydrateAutomationTemplates()
	if len(s.customAutomationTemplates) != 1 {
		t.Fatalf("hydrated = %+v", s.customAutomationTemplates)
	}
	if s.customAutomationTemplates[0].Preset {
		t.Fatal("custom should not be preset")
	}

	all := s.allAutomationTemplatesReadOnly()
	if len(all) < 1 {
		t.Fatal("all empty")
	}
	// 至少包含自定义 c1
	found := false
	for _, item := range all {
		if item.ID == "c1" {
			found = true
			if item.NextRunAt == "" {
				t.Fatal("enabled schedule should have NextRunAt")
			}
		}
	}
	if !found {
		t.Fatalf("c1 missing: %+v", all)
	}

	if _, ok := s.automationTemplateByIDReadOnly("c1"); !ok {
		t.Fatal("find c1")
	}
	if _, ok := s.automationTemplateByIDReadOnly("nope"); ok {
		t.Fatal("missing id")
	}

	// presetEnabledMapReadOnly
	m := s.presetEnabledMapReadOnly()
	if m == nil {
		t.Fatal("preset map")
	}
}

func TestMaybeReloadAutomationFromFile(t *testing.T) {
	withTempHome(t)
	s := &BridgeService{}
	// 非目标路径：不触发（scheduler 为 nil 时 reload 也安全）
	s.maybeReloadAutomationFromFile("/tmp/other.json")
	// 目标路径
	s.maybeReloadAutomationFromFile(s.automationStorePath())
}

func TestNormalizeSandboxModeAliases(t *testing.T) {
	cases := map[string]string{
		"read-only":          "read-only",
		"readonly":           "read-only",
		"RO":                 "read-only",
		"workspace-write":    "workspace-write",
		"workspace":          "workspace-write",
		"ww":                 "workspace-write",
		"danger-full-access": "danger-full-access",
		"full":               "danger-full-access",
		"完全访问":               "danger-full-access",
		"nope":               defaultSandboxMode,
		"":                   defaultSandboxMode,
	}
	for in, want := range cases {
		if got := NormalizeSandboxMode(in); got != want {
			t.Fatalf("NormalizeSandboxMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAutomationStorePathUsesHome(t *testing.T) {
	home := withTempHome(t)
	s := &BridgeService{}
	got := s.automationStorePath()
	want := filepath.Join(home, ".eos", automationStoreFileName)
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, automationStoreFileName) {
		t.Fatalf("name = %q", got)
	}
}
