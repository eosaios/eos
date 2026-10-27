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

func TestMemoryPanelInitCancelAndSelectProjectScope(t *testing.T) {
	p := newTestMemoryPanel()
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}

	// SelectProjectScope 无项目时 no-op
	p.SelectProjectScope()
	if p.scope != 0 {
		t.Fatalf("scope = %d, want 0", p.scope)
	}

	p.SetSize(80, 24)
	p.SetData(
		[]MemoryDoc{
			{Scope: "memory_summary.md", Content: "g-sum", Exists: true},
			{Scope: "MEMORY.md", Content: "g-hand", Exists: true},
		},
		&MemoryProject{
			Key: "k",
			Docs: [2]MemoryDoc{
				{Scope: "memory_summary.md", Content: "p-sum", Exists: true},
				{Scope: "MEMORY.md", Content: "p-hand", Exists: true},
			},
		},
	)

	p.SelectProjectScope()
	if p.scope != 1 || p.tab != 0 {
		t.Fatalf("after SelectProjectScope scope=%d tab=%d", p.scope, p.tab)
	}
	if !strings.Contains(p.View(), "p-sum") {
		t.Fatalf("project content:\n%s", p.View())
	}
	// 已在项目作用域时再调用不重置 tab
	p.tab = 1
	p.SelectProjectScope()
	if p.tab != 1 {
		t.Fatalf("tab reset to %d", p.tab)
	}

	// CancelEdit
	p.enterCompose()
	if !p.IsEditing() {
		t.Fatal("should be composing")
	}
	p.CancelEdit()
	if p.IsEditing() {
		t.Fatal("CancelEdit should stop composing")
	}

	// nil 安全
	var nilP *MemoryPanel
	nilP.CancelEdit()
	nilP.SelectProjectScope()
	if nilP.IsEditing() {
		t.Fatal("nil IsEditing")
	}
}

func TestMemoryPanelCurrentDocBoundsAndLanguage(t *testing.T) {
	p := newTestMemoryPanel()
	p.SetData([]MemoryDoc{
		{Scope: "memory_summary.md", Content: "only", Exists: true},
	}, nil)

	// tab 越界回落 docs[0]
	p.tab = 99
	if got := p.currentDoc().Content; got != "only" {
		t.Fatalf("oob tab doc = %q", got)
	}
	p.tab = -1
	if got := p.currentDoc().Content; got != "only" {
		t.Fatalf("neg tab doc = %q", got)
	}

	// 语言切换刷新占位与内容
	out, _ := p.Update(LanguageChangeMsg{Language: "en"})
	if mp := out.(*MemoryPanel); mp.language != "en" {
		t.Fatalf("lang = %q", mp.language)
	}

	// SetData 在 scope=1 但 project 变 nil 时回落全局
	p.scope = 1
	p.SetData([]MemoryDoc{{Scope: "memory_summary.md", Content: "g", Exists: true}}, nil)
	if p.scope != 0 {
		t.Fatalf("scope = %d, want fallback to 0", p.scope)
	}
}

func TestMemoryPanelComposeViaA(t *testing.T) {
	p := newTestMemoryPanel()
	p.SetSize(80, 24)

	updated, cmd := p.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !updated.(*MemoryPanel).IsEditing() {
		t.Fatal("'a' should enter compose")
	}
	if cmd == nil {
		t.Fatal("'a' should return editor Init")
	}
	// 编辑态按键走 editor（非 ctrl+s 不退出）
	updated, _ = updated.(*MemoryPanel).Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !updated.(*MemoryPanel).IsEditing() {
		t.Fatal("typing should stay in compose")
	}
}
