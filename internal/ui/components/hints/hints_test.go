package hints

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestHintsLifecycle(t *testing.T) {
	m := New()
	m.SetSize(80, 10)
	m.SetStyle(lipgloss.NewStyle())

	if m.Visible() {
		t.Fatal("default hidden")
	}
	m.SetHints([]Hint{{Key: "a", Desc: "A", Value: "a"}, {Key: "b", Desc: "B", Value: "b"}})
	m.Show()
	if !m.Visible() {
		t.Fatal("show")
	}
	if m.Height() <= 0 {
		t.Fatal("height")
	}

	m.AddHint("c", "C")
	m.CursorDown()
	if m.Selected() == "" {
		t.Fatal("selected")
	}
	m.CursorUp()
	m.ClearHints()
	if m.Visible() {
		t.Fatal("clear hides?")
	}

	// Update 透传
	m.Show()
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	if out := m.View(); out == "" && m.Visible() {
		t.Fatal("view")
	}
	_ = strings.Contains(m.View(), "a")
}

func TestHintsEmptySelected(t *testing.T) {
	m := New()
	if m.Selected() != "" {
		t.Fatal("empty selected")
	}
	m.CursorDown()
	m.CursorUp()
}
