package config

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBoolFlagsDefaults(t *testing.T) {
	if !NextMessagePredictionEnabled(nil) {
		t.Fatal("nil pred")
	}
	if !MemoryInjectionEnabled(nil) {
		t.Fatal("nil mem")
	}
	if !GitCommitReminderEnabled(nil) {
		t.Fatal("nil git")
	}
	cfg := &Config{}
	if !NextMessagePredictionEnabled(cfg) || !MemoryInjectionEnabled(cfg) || !GitCommitReminderEnabled(cfg) {
		t.Fatal("unset defaults true")
	}
	f := boolPtr(false)
	cfg.NextMessagePredictionEnabled = f
	cfg.MemoryInjectionEnabled = f
	cfg.GitCommitReminder = f
	if NextMessagePredictionEnabled(cfg) || MemoryInjectionEnabled(cfg) || GitCommitReminderEnabled(cfg) {
		t.Fatal("explicit false")
	}
}

func TestDiffThemeAndProxy(t *testing.T) {
	if DiffHighlightTheme(nil) != "" {
		t.Fatal("nil theme")
	}
	cfg := &Config{DiffTheme: "  monokai  "}
	if DiffHighlightTheme(cfg) != "monokai" {
		t.Fatalf("theme = %q", DiffHighlightTheme(cfg))
	}
	if EffectiveUpdateProxyURL(nil) != "" {
		t.Fatal("nil proxy")
	}
	if EffectiveUpdateProxyURL(&Config{}) != "" {
		t.Fatal("disabled proxy")
	}
	cfg = &Config{UpdateProxyEnabled: true, UpdateProxyURL: "  http://p  "}
	if EffectiveUpdateProxyURL(cfg) != "http://p" {
		t.Fatalf("proxy = %q", EffectiveUpdateProxyURL(cfg))
	}
}

func TestResolveLogDir(t *testing.T) {
	// 空 → 默认
	if ResolveLogDir("") == "" {
		t.Fatal("default")
	}
	// ~ 展开
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got := ResolveLogDir("~/logs")
	if !strings.HasPrefix(got, filepath.Clean(home)) {
		t.Fatalf("tilde = %q", got)
	}
	if ResolveLogDir("~") == "" {
		t.Fatal("tilde root")
	}
	// 相对 → 绝对
	got = ResolveLogDir("rel/logs")
	if !filepath.IsAbs(got) {
		t.Fatalf("rel = %q", got)
	}
	// 绝对
	if ResolveLogDir("/tmp/x") != filepath.Clean("/tmp/x") {
		t.Fatal("abs")
	}

	if DefaultLogDir() == "" {
		t.Fatal("DefaultLogDir")
	}
	_ = ConfiguredLogDir()
	_ = Path()
}

func TestParseLegacyMCPServersJSON(t *testing.T) {
	// 非法 JSON
	if _, err := ParseLegacyMCPServersJSON([]byte("{")); err == nil {
		t.Fatal("bad json")
	}
	// 缺 mcpServers
	if _, err := ParseLegacyMCPServersJSON([]byte(`{"x":1}`)); err == nil {
		t.Fatal("missing key")
	}

	// 完整条目
	raw := []byte(`{
      "mcpServers": {
        "fs": {
          "command": "npx",
          "args": ["-y", "fs"],
          "envs": {"A": "1"},
          "enabled": true,
          "type": "stdio"
        },
        "web": {
          "url": "https://mcp.example",
          "approval_mode": "on-request"
        },
        "  ": {"command": "x"}
      }
    }`)
	entries, err := ParseLegacyMCPServersJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	byName := map[string]MCPEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if e := byName["fs"]; e.Command != "npx" || len(e.Args) != 2 || e.Envs["A"] != "1" {
		t.Fatalf("fs = %+v", e)
	}
	if e := byName["web"]; e.BaseURL != "https://mcp.example" {
		t.Fatalf("web = %+v", e)
	}

	// base_url 优先于 url
	entries, err = ParseLegacyMCPServersJSON([]byte(`{"mcpServers":{"a":{"base_url":"B","url":"U"}}}`))
	if err != nil || entries[0].BaseURL != "B" {
		t.Fatalf("base_url = %+v %v", entries, err)
	}
	// env 回落
	entries, err = ParseLegacyMCPServersJSON([]byte(`{"mcpServers":{"a":{"env":{"K":"v"}}}}`))
	if err != nil || entries[0].Envs["K"] != "v" {
		t.Fatalf("env = %+v %v", entries, err)
	}
}

func TestLSPEnabledValues(t *testing.T) {
	var c LSPConfig
	if !c.EnabledValue() || !c.AutoDetectValue() {
		t.Fatal("defaults")
	}
	f := false
	c.Enabled = &f
	c.AutoDetect = &f
	if c.EnabledValue() || c.AutoDetectValue() {
		t.Fatal("explicit false")
	}
}
