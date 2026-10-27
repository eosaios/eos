package setup

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/styles"

	tea "charm.land/bubbletea/v2"
)

func newMCPEditor(edit bool, name, initial string) *MCPConfigEditorView {
	return NewMCPConfigEditorView(styles.NewStyles(styles.DefaultDarkTheme()), "zh", initial, edit, name)
}

func TestMCPConfigEditorConstructorAndSize(t *testing.T) {
	v := newMCPEditor(true, "demo", `{"cmd":["echo"]}`)
	if !v.edit || v.originalName != "demo" {
		t.Fatalf("edit/originalName = %v/%q", v.edit, v.originalName)
	}
	if v.textarea.Value() != `{"cmd":["echo"]}` {
		t.Fatalf("textarea value = %q", v.textarea.Value())
	}
	if !v.textarea.Focused() {
		t.Fatal("textarea should be focused")
	}

	v.SetSize(100, 40)
	if v.width != 100 || v.height != 40 {
		t.Fatalf("size = %dx%d", v.width, v.height)
	}
	if cmd := v.Init(); cmd == nil {
		t.Fatal("Init should delegate to textarea")
	}
}

func TestMCPConfigEditorEscCancels(t *testing.T) {
	v := newMCPEditor(false, "", "")
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should return cancel cmd")
	}
	if _, ok := cmd().(MCPConfigCancelMsg); !ok {
		t.Fatalf("esc cmd() = %T, want MCPConfigCancelMsg", cmd())
	}
}

func TestMCPConfigEditorCtrlSSubmitsAndRecordsHistory(t *testing.T) {
	v := newMCPEditor(true, "srv", `{"a":1}`)
	_, cmd := v.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+s should return submit cmd")
	}
	msg, ok := cmd().(MCPConfigSubmitMsg)
	if !ok {
		t.Fatalf("ctrl+s cmd() = %T, want MCPConfigSubmitMsg", cmd())
	}
	if msg.Text != `{"a":1}` || !msg.Edit || msg.OriginalName != "srv" {
		t.Fatalf("submit = %+v", msg)
	}
	hist := v.textarea.GetHistory()
	if len(hist) != 1 || hist[0] != `{"a":1}` {
		t.Fatalf("history = %v, want one entry", hist)
	}
}

func TestMCPConfigEditorCtrlSSubmitAddMode(t *testing.T) {
	v := newMCPEditor(false, "", "plain")
	_, cmd := v.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	msg := cmd().(MCPConfigSubmitMsg)
	if msg.Edit || msg.OriginalName != "" || msg.Text != "plain" {
		t.Fatalf("add-mode submit = %+v", msg)
	}
}

func TestMCPConfigEditorForwardsOtherKeys(t *testing.T) {
	v := newMCPEditor(false, "", "")
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if got := v.textarea.Value(); !strings.Contains(got, "h") {
		t.Fatalf("textarea = %q, want contains h", got)
	}
	// 非快捷键路径可能返回 textarea 的 cmd（也可能 nil），这里只校验值变更
	_ = cmd
}

func TestMCPConfigEditorViewTitles(t *testing.T) {
	add := newMCPEditor(false, "", "")
	view := add.View()
	if !strings.Contains(view, "新增 MCP 服务器") {
		t.Fatalf("add title missing:\n%s", view)
	}
	if !strings.Contains(view, "Ctrl+S: 保存") {
		t.Fatalf("help line missing:\n%s", view)
	}

	edit := newMCPEditor(true, "x", "")
	edit.SetSize(90, 30)
	view = edit.View()
	if !strings.Contains(view, "编辑 MCP 服务器") {
		t.Fatalf("edit title missing:\n%s", view)
	}
	if !strings.Contains(view, "输入 MCP 配置 JSON") {
		t.Fatalf("desc missing:\n%s", view)
	}
}
