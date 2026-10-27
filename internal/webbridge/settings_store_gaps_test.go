package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// settings_store 余臂批测：HOME 缺失回落、坏 JSON/目录路径加载错误、
// 空 JSON 文件容错、空输入归一化。setJSONValue 对 map 文档恒成功，
// 其错误检查臂为防御性死臂（json.Marshal 基本类型不失败）。

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkspaceSettingsPathNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	got := ResolveWorkspaceSettingsPath("")
	if got != filepath.Join(".eos", workspaceSettingsFileName) {
		t.Fatalf("无 HOME 回落 = %q", got)
	}
}

func TestLoadGUISettingsBadInputs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	def := GUISettingsDefaults{Language: "zh", Theme: "system"}

	// 全局配置是目录 → 读失败。
	dirPath := t.TempDir()
	if _, err := LoadGUISettings(dirPath, "", def); err == nil {
		t.Fatal("全局配置目录应报错")
	}
	// 工作区设置文件位置本身是目录 → 读失败（不存在则容错为空文档）。
	globalOK := filepath.Join(t.TempDir(), "global.json")
	os.WriteFile(globalOK, []byte("{}"), 0o644)
	ws := t.TempDir()
	os.MkdirAll(filepath.Join(ws, ".eos", workspaceSettingsFileName+"-block"), 0o755)
	os.MkdirAll(filepath.Join(ws, ".eos"), 0o755)
	os.Mkdir(filepath.Join(ws, ".eos", workspaceSettingsFileName), 0o755) // 同名目录 → ReadFile 报错
	if _, err := LoadGUISettings(globalOK, ws, def); err == nil {
		t.Fatal("工作区设置目录应报错")
	}
	// 坏 JSON → 解析错误。
	badJSON := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(badJSON, []byte("{not json"), 0o644)
	if _, err := LoadGUISettings(badJSON, "", def); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
	// 空 JSON 文件（全空白）→ 容错为空文档。
	emptyJSON := filepath.Join(t.TempDir(), "empty.json")
	os.WriteFile(emptyJSON, []byte("   \n  "), 0o644)
	snap, err := LoadGUISettings(emptyJSON, "", def)
	if err != nil || snap.Language != "zh" {
		t.Fatalf("空文件容错 = %+v, %v", snap, err)
	}
}

func TestSaveGUISettingsBadInputs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	def := GUISettingsDefaults{Language: "zh", Theme: "system"}

	// 全局配置目录 → 读失败。
	if _, err := SaveGUISettings(t.TempDir(), "", GUISettingsSaveInput{}, def); err == nil {
		t.Fatal("全局配置目录应报错")
	}
	// 工作区设置文件位置是目录 → 读失败。
	globalOK := filepath.Join(t.TempDir(), "global.json")
	os.WriteFile(globalOK, []byte("{}"), 0o644)
	ws := t.TempDir()
	os.MkdirAll(filepath.Join(ws, ".eos"), 0o755)
	os.Mkdir(filepath.Join(ws, ".eos", workspaceSettingsFileName), 0o755)
	if _, err := SaveGUISettings(globalOK, ws, GUISettingsSaveInput{}, def); err == nil {
		t.Fatal("工作区设置目录应报错")
	}
	// 坏 JSON → 报错。
	badJSON := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(badJSON, []byte("[1,2"), 0o644)
	if _, err := SaveGUISettings(badJSON, "", GUISettingsSaveInput{}, def); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
}

func TestNormalizeSettingsDefaultsAndInput(t *testing.T) {
	// 空默认回落。
	def := normalizeGUISettingsDefaults(GUISettingsDefaults{})
	if def.Language != defaultLanguage || def.Theme != defaultTheme {
		t.Fatalf("默认回落 = %+v", def)
	}
	// 非空默认保持。
	def = normalizeGUISettingsDefaults(GUISettingsDefaults{Language: "en", Theme: "dark"})
	if def.Language != "en" || def.Theme != "dark" {
		t.Fatalf("非空保持 = %+v", def)
	}
	// 空输入回落默认；空白 trim。
	in := normalizeGUISettingsInput(GUISettingsSaveInput{
		Language: "  ", Theme: "  ", DiffTheme: " gh-dark ",
	}, GUISettingsDefaults{Language: "zh", Theme: "light"})
	if in.Language != "zh" || in.Theme != "light" || in.DiffTheme != "gh-dark" {
		t.Fatalf("输入归一 = %+v", in)
	}
}
