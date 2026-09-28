package panels

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// ---------- LSP ----------

func TestLSPPanelStatusAndTable(t *testing.T) {
	p := NewLSPPanel(testStyles(), "zh")
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}
	// 表格宽度为 0 时不渲染数据行，先 SetSize
	p.SetSize(100, 40)
	p.SetStatus(LSPPanelSummary{
		Enabled:          true,
		AutoDetect:       true,
		ConfigFile:       "/ws/.eos/lsp.toml",
		Workspace:        "/ws",
		DetectedLanguage: "go",
		ActiveLanguage:   "go",
		ActiveServer:     "gopls",
		ActiveRoot:       "/ws",
		Message:          "ready",
	}, []LSPServerRow{
		{Language: "go", Command: "gopls", Found: true},
		{Language: "rust", Found: false},
		{Language: "py", Command: "", Found: true},
	})

	view := p.View()
	for _, want := range []string{"LSP", "ready", "/ws/.eos/lsp.toml", "gopls", "not found", "found", "* go"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}

	// 空 Detected/Active 回落
	p.SetStatus(LSPPanelSummary{}, nil)
	view = p.View()
	if !strings.Contains(view, "(unknown)") || !strings.Contains(view, "(not running)") {
		t.Fatalf("fallback labels missing:\n%s", view)
	}
	if !strings.Contains(view, "(empty)") {
		t.Fatalf("empty table row missing:\n%s", view)
	}

	p.SetSize(4, 4) // 宽高下限夹到 0
	p.SetSize(100, 40)
}

func TestLSPPanelDetailAndKeys(t *testing.T) {
	p := NewLSPPanel(testStyles(), "zh")
	p.SetSize(100, 40)
	p.SetStatus(LSPPanelSummary{
		ActiveLanguage: "go",
		ActiveServer:   "gopls",
		ActiveRoot:     "/ws",
	}, []LSPServerRow{
		{Language: "go", Command: "gopls", Found: true},
		{Language: "rust", Command: "  ", Found: false},
	})

	// enter 打开详情（带 Active 段）
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !p.viewing {
		t.Fatal("enter should open detail")
	}
	view := p.View()
	if !strings.Contains(view, "Language: go") || !strings.Contains(view, "Active:") {
		t.Fatalf("detail view:\n%s", view)
	}

	// viewing 时 esc 关闭
	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if p.viewing {
		t.Fatal("esc should close detail")
	}

	// 第二行 Command 空白 → 详情写 (empty)（列表列则按 Found 显示 not found）
	p.table.SetCursor(1)
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(p.detail.View(), "(empty)") {
		t.Fatalf("detail empty cmd:\n%s", p.detail.View())
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	// r → 刷新
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if _, ok := cmd().(LSPRefreshMsg); !ok {
		t.Fatalf("r = %T", cmd())
	}

	// 越界 enter 不打开
	p.servers = nil
	p.table.SetCursor(5)
	_, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.viewing {
		t.Fatal("oob enter should not open")
	}

	// 语言切换
	out, _ := p.Update(LanguageChangeMsg{Language: "en"})
	if lp := out.(*LSPPanel); lp.language != "en" {
		t.Fatalf("lang = %q", lp.language)
	}
}

// ---------- MCP ----------

func TestMCPPanelServersAndBrowserSummary(t *testing.T) {
	p := NewMCPPanel(testStyles(), "zh")
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}
	p.SetSize(100, 40)
	p.SetServers([]MCPServer{
		{Name: "fs", Type: "stdio", Enabled: true},
		{Name: "web", Type: "sse", Enabled: false},
		{Name: "other", Type: "custom", Enabled: true},
	})
	p.SetBrowserSummary(BrowserSummary{Running: true, Kind: "chrome", Version: "120"})

	view := p.View()
	for _, want := range []string{"fs", "web", "chrome 120"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}

	// 空 Kind 回落 chrome
	p.SetBrowserSummary(BrowserSummary{Running: true, Version: ""})
	if !strings.Contains(p.View(), "chrome") {
		t.Fatalf("blank kind fallback:\n%s", p.View())
	}

	// 错误状态
	p.SetBrowserSummary(BrowserSummary{LastError: "boom"})
	if !strings.Contains(p.View(), "boom") {
		t.Fatalf("error status missing:\n%s", p.View())
	}

	// 空列表
	p.SetServers(nil)
	if !strings.Contains(p.View(), "无 MCP 服务器") {
		t.Fatalf("empty servers:\n%s", p.View())
	}

	if got := blankOr("  x  ", "y"); got != "x" {
		t.Fatalf("blankOr trim = %q", got)
	}
	if got := blankOr("", "y"); got != "y" {
		t.Fatalf("blankOr fallback = %q", got)
	}

	p.SetSize(90, 36)
}

func TestMCPPanelActions(t *testing.T) {
	p := NewMCPPanel(testStyles(), "zh")
	p.SetServers([]MCPServer{{Name: "fs", Type: "stdio", Enabled: true}})

	// 左右环绕
	p.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if p.GetCurrentAction() != "Reload" {
		t.Fatalf("left wrap = %q", p.GetCurrentAction())
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if p.GetCurrentAction() != "Toggle" {
		t.Fatalf("right wrap = %q", p.GetCurrentAction())
	}

	// enter Toggle
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if msg := cmd().(MCPToggleMsg); msg.Name != "fs" {
		t.Fatalf("toggle = %+v", msg)
	}

	// 移到 Browser / Add / Reload
	for i := 0; i < 5; i++ {
		p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	// 从 Toggle 右 5 次 → Reload（0→1 Browser, 2 Add, 3 Edit, 4 Delete, 5 Reload）
	if p.GetCurrentAction() != "Reload" {
		t.Fatalf("after 5 rights = %q", p.GetCurrentAction())
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := cmd().(MCPSaveMsg); !ok {
		t.Fatalf("Reload enter = %T", cmd())
	}

	// 快捷键
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if _, ok := cmd().(MCPAddMsg); !ok {
		t.Fatalf("a = %T", cmd())
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if _, ok := cmd().(MCPAddBrowserMsg); !ok {
		t.Fatalf("b = %T", cmd())
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if _, ok := cmd().(MCPToggleMsg); !ok {
		t.Fatalf("t = %T", cmd())
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if msg := cmd().(MCPEditMsg); msg.Name != "fs" {
		t.Fatalf("e = %+v", msg)
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if msg := cmd().(MCPDeleteMsg); msg.Name != "fs" {
		t.Fatalf("d = %+v", msg)
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if _, ok := cmd().(MCPSaveMsg); !ok {
		t.Fatalf("r = %T", cmd())
	}

	// 空列表 Toggle/Edit/Delete 快捷键不产生消息
	empty := NewMCPPanel(testStyles(), "zh")
	if _, cmd = empty.Update(tea.KeyPressMsg{Code: 't', Text: "t"}); cmd != nil {
		t.Fatalf("empty t = %T", cmd())
	}

	// 语言切换重译列
	out, _ := p.Update(LanguageChangeMsg{Language: "en"})
	if mp := out.(*MCPPanel); mp.language != "en" {
		t.Fatalf("lang = %q", mp.language)
	}

	// 越界 actionIndex
	p.actionIndex = 99
	if p.GetCurrentAction() != "" {
		t.Fatal("oob action should be empty")
	}
}

// ---------- Rules ----------

func TestRulesPanelScopesAndEdit(t *testing.T) {
	p := NewRulesPanel(testStyles(), "zh")
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}
	p.SetSize(80, 30)
	p.SetData("/ws", "/ws/EOS.md", "project rules", true, "/home/EOS.md", "global rules", true)

	if view := p.View(); !strings.Contains(view, "project rules") {
		t.Fatalf("project content missing:\n%s", view)
	}
	// tab 切到 global
	p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if p.scope != 1 || p.scopeLabel() != "Global" {
		t.Fatalf("scope = %d %q", p.scope, p.scopeLabel())
	}
	if view := p.View(); !strings.Contains(view, "global rules") {
		t.Fatalf("global content missing:\n%s", view)
	}

	// shift+tab 回 project
	p.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if p.scope != 0 {
		t.Fatalf("scope = %d", p.scope)
	}

	// e 进入编辑
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Fatal("e should return editor Init cmd")
	}
	if !p.IsEditing() {
		t.Fatal("should be editing")
	}

	// ctrl+s 保存 project
	p.editor.SetValue("saved project")
	_, cmd = p.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	save := cmd().(RulesSaveMsg)
	if save.Scope != "project" || save.Content != "saved project" {
		t.Fatalf("save = %+v", save)
	}
	if p.IsEditing() {
		t.Fatal("should exit editing")
	}
	if p.projectContent != "saved project" || !p.projectExists {
		t.Fatalf("project content = %q", p.projectContent)
	}

	// global 保存
	p.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // → global
	p.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	p.editor.SetValue("saved global")
	_, cmd = p.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	save = cmd().(RulesSaveMsg)
	if save.Scope != "global" {
		t.Fatalf("save scope = %q", save.Scope)
	}

	// CancelEdit
	p.enterEdit()
	p.CancelEdit()
	if p.IsEditing() {
		t.Fatal("CancelEdit should exit editing")
	}

	// r 刷新
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if _, ok := cmd().(RulesRefreshMsg); !ok {
		t.Fatalf("r = %T", cmd())
	}

	// 空路径 / 文件不存在占位
	p2 := NewRulesPanel(testStyles(), "zh")
	p2.SetSize(80, 30)
	p2.SetData("", "", "", false, "/missing/EOS.md", "", false)
	// scope=0（project）路径为空 → (empty)
	if !strings.Contains(p2.view.View(), "(empty)") {
		t.Fatalf("empty project body:\n%s", p2.view.View())
	}
	// 切到 global：路径非空但文件不存在 → (file not found)
	p2.scope = 1
	p2.updateViewContent()
	if !strings.Contains(p2.view.View(), "(file not found)") {
		t.Fatalf("not-found body:\n%s", p2.view.View())
	}
	if view := p2.View(); !strings.Contains(view, "missing") {
		t.Fatalf("missing status:\n%s", view)
	}
	// global 路径为空且 exists=false：path 空 → (empty)
	p2.SetData("", "", "", false, "", "", false)
	p2.scope = 1
	p2.updateViewContent()
	if !strings.Contains(p2.view.View(), "(empty)") {
		t.Fatalf("empty global body:\n%s", p2.view.View())
	}

	// 语言切换
	out, _ := p.Update(LanguageChangeMsg{Language: "en"})
	if rp := out.(*RulesPanel); rp.language != "en" {
		t.Fatalf("lang = %q", rp.language)
	}

	// nil 安全
	var nilP *RulesPanel
	nilP.CancelEdit()
	if nilP.IsEditing() {
		t.Fatal("nil IsEditing should be false")
	}
}
