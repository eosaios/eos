package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHandleGlobalKeyBranches(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// ctrl+c 空闲 → Quit
	cmd := app.handleGlobalKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c idle")
	}
	// ctrl+c 处理中 → 取消不 Quit
	app.state.Processing = true
	app.handleGlobalKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	app.state.Processing = false

	// f2 切换模式
	app.handleGlobalKey(tea.KeyPressMsg{Code: tea.KeyF2})
	if app.state.Mode == "" {
		t.Fatal("f2 mode")
	}
	app.handleGlobalKey(tea.KeyPressMsg{Code: tea.KeyF2})

	// ? 帮助
	app.handleGlobalKey(tea.KeyPressMsg{Code: '?', Text: "?"})
	if app.activeView != "help" {
		t.Fatalf("help view = %q", app.activeView)
	}

	// esc 空闲清空
	app.activeView = "shell"
	app.shell.SetInputValue("x")
	app.handleGlobalKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if app.shell.GetInputValue() != "" {
		t.Fatal("esc clear")
	}

	// esc 处理中 + 有 cancel
	app.state.Processing = true
	app.setActiveCancel(func() {})
	app.handleGlobalKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	app.state.Processing = false

	// tab 无预测 → 切换思考
	app.activeView = "shell"
	app.state.Mode = "ai"
	app.handleGlobalKey(tea.KeyPressMsg{Code: tea.KeyTab})

	// ctrl+o 循环 live 面板
	app.handleGlobalKey(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})

	// alt+v 粘贴图片
	app.handleGlobalKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})

	// alt+h 无 thinking 内容 → 不切换
	app.state.Thinking = true
	app.handleGlobalKey(tea.KeyPressMsg{Code: 'h', Mod: tea.ModAlt})

	// 未知键
	if cmd := app.handleGlobalKey(tea.KeyPressMsg{Code: 'z', Text: "z"}); cmd != nil {
		t.Fatal("unknown key")
	}
}

func TestRenderHistoryEntryKinds(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	for _, e := range []historyEntry{
		{kind: "user", content: "hi"},
		{kind: "ai", content: "reply", executionMode: "plan", rawMarkdown: "# p"},
		{kind: "system", content: "sys", level: "error"},
		{kind: "tool", toolName: "Bash", toolOutput: "ok", toolStatus: "success"},
		{kind: "agent.task", agentName: "sub", task: "do", agentEvent: "dispatch"},
		{kind: "agent.final", agentName: "sub", content: "done", agentEvent: "result"},
		{kind: "unknown-kind", content: "x"},
	} {
		if out := app.renderHistoryEntry(e); out == "" {
			t.Fatalf("empty render for %q", e.kind)
		}
	}
}
