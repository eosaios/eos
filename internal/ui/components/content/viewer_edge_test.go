package content

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

func TestNewAndSizeStyle(t *testing.T) {
	m := New(80, 10)
	if m.width != 80 || m.height != 10 {
		t.Fatalf("size = %dx%d", m.width, m.height)
	}
	m.SetSize(100, 20)
	if m.width != 100 || m.Height() != 20 {
		t.Fatalf("after SetSize = %dx%d", m.width, m.Height())
	}
	m.SetStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")))
	if out := m.View(); out == "" {
		t.Fatal("styled view empty")
	}
}

func TestAppendLineClearContent(t *testing.T) {
	m := New(80, 5)
	if cmd := m.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}

	m.Append("hello")
	if m.Content() != "hello" {
		t.Fatalf("append = %q", m.Content())
	}
	m.AppendLine("world")
	if !strings.Contains(m.Content(), "world\n") {
		t.Fatalf("appendLine = %q", m.Content())
	}
	if m.LineCount() != 2 {
		t.Fatalf("lines = %d", m.LineCount())
	}

	m.Clear()
	if m.Content() != "" || m.LineCount() != 1 {
		t.Fatalf("clear = %q lines=%d", m.Content(), m.LineCount())
	}

	m.SetContent("a\nb\nc")
	if m.Content() != "a\nb\nc" {
		t.Fatalf("setContent = %q", m.Content())
	}
	if !m.AtBottom() {
		t.Fatal("SetContent should goto bottom")
	}
}

func TestScrollNavigation(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("x", 10)
	}
	m := New(80, 5)
	m.SetContent(strings.Join(lines, "\n"))

	if !m.AtBottom() {
		t.Fatal("start at bottom")
	}
	m.GotoTop()
	if m.YOffset() != 0 {
		t.Fatalf("top offset = %d", m.YOffset())
	}
	if m.AtBottom() {
		t.Fatal("top is not bottom")
	}

	m.LineDown()
	if m.YOffset() == 0 {
		t.Fatal("LineDown")
	}
	m.LineUp()
	if m.YOffset() != 0 {
		t.Fatalf("LineUp = %d", m.YOffset())
	}

	m.HalfViewDown()
	if m.YOffset() == 0 {
		t.Fatal("HalfViewDown")
	}
	m.HalfViewUp()
	if m.YOffset() != 0 {
		t.Fatalf("HalfViewUp = %d", m.YOffset())
	}

	m.GotoBottom()
	if !m.AtBottom() {
		t.Fatal("GotoBottom")
	}
	if p := m.ScrollPercent(); p < 0 || p > 1 {
		t.Fatalf("ScrollPercent = %v", p)
	}

	// Update 透传
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_ = cmd
}

func TestSetContentPreserveOffsetClamp(t *testing.T) {
	m := New(80, 3)
	m.SetContent(strings.Join([]string{"1", "2", "3", "4", "5", "6", "7", "8"}, "\n"))
	m.GotoTop()
	m.LineDown()
	m.LineDown()
	old := m.YOffset()
	if old == 0 {
		t.Fatal("need non-zero offset")
	}

	// 内容变短：offset 被夹到 maxOffset
	m.SetContentPreserveOffset("a\nb")
	if m.YOffset() > m.LineCount()-m.Height() && m.LineCount() > m.Height() {
		t.Fatalf("offset not clamped: %d", m.YOffset())
	}

	// 空内容
	m.SetContentPreserveOffset("")
	if m.Content() != "" {
		t.Fatalf("empty = %q", m.Content())
	}

	// 负 offset 夹到 0（通过 SetContent 后手动）
	m.SetContent("x\ny\nz")
	m.SetContentPreserveOffset("p")
	if m.YOffset() < 0 {
		t.Fatalf("negative offset = %d", m.YOffset())
	}
}
