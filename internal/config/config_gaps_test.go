package config

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// config.go 余臂批测：Path 的 HOME 缺失回落、Load 的坏 JSON/工作区归一
// 回写/legacy MCP 迁移三链、legacy 解析错误族。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPathNoHomeFallback(t *testing.T) {
	t.Setenv("HOME", "")
	if got := Path(); got != ".eos.json" {
		t.Fatalf("无 HOME = %q", got)
	}
}

func writeHomeConfig(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if content != "" {
		if err := os.WriteFile(filepath.Join(home, ".eos.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestLoadBadJSONTolerates(t *testing.T) {
	writeHomeConfig(t, "{not json")
	cfg, p := Load()
	if filepath.Base(p) != ".eos.json" {
		t.Fatalf("path = %q", p)
	}
	// 坏 JSON 容错为零值 + 归一化默认工作区（不崩、不报错）。
	if len(cfg.MCP) != 0 || cfg.LastWorkspace == "" {
		t.Fatalf("坏 JSON 容错形态 = MCP:%d last:%q", len(cfg.MCP), cfg.LastWorkspace)
	}
}

func TestLoadNormalizeWritesBack(t *testing.T) {
	// LastWorkspace 指向不存在的目录 → NormalizeWorkspaceState 会改写并回存。
	home := writeHomeConfig(t, `{"last_workspace": "/definitely/missing/ws"}`)
	_, _ = Load()
	raw, err := os.ReadFile(filepath.Join(home, ".eos.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("回写产物非法 JSON: %v\n%s", err, raw)
	}
}

func TestLoadMigratesLegacyMCPServers(t *testing.T) {
	home := writeHomeConfig(t, `{"mcpServers": {"fetch": {"type": "stdio", "command": "mcp-fetch", "args": ["--x"], "envs": {"A": "B"}, "enabled": true}}}`)
	cfg, _ := Load()
	if len(cfg.MCP) != 1 || cfg.MCP[0].Name != "fetch" || cfg.MCP[0].Command != "mcp-fetch" {
		t.Fatalf("迁移结果 = %+v", cfg.MCP)
	}
	// 迁移后回写为新结构。
	raw, _ := os.ReadFile(filepath.Join(home, ".eos.json"))
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["mcpServers"]; ok {
		t.Fatal("legacy 键不应残留")
	}
}

func TestParseLegacyMCPServersJSONErrors(t *testing.T) {
	if _, err := ParseLegacyMCPServersJSON([]byte("{not json")); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
	if _, err := ParseLegacyMCPServersJSON([]byte(`{"other": 1}`)); err == nil {
		t.Fatal("缺 mcpServers 应报错")
	}
	if entries, err := ParseLegacyMCPServersJSON([]byte(`{"mcpServers": {}}`)); err != nil || len(entries) != 0 {
		t.Fatalf("空列表 = %+v, %v", entries, err)
	}
	// 非对象形态的条目值：按名字收录为默认条目（行为固化，不报错）。
	if entries, err := ParseLegacyMCPServersJSON([]byte(`{"mcpServers": {"a": "not-object"}}`)); err != nil || len(entries) != 1 || entries[0].Name != "a" {
		t.Fatalf("非对象条目 = %+v, %v", entries, err)
	}
}
