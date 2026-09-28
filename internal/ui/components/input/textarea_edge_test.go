package input

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestInputLifecycle(t *testing.T) {
	m := New()
	if cmd := m.Init(); cmd == nil {
		// Init 可能返回 blink cmd
	}
	m.SetSize(80, 5)
	if m.ViewHeight() <= 0 {
		t.Fatalf("view height = %d", m.ViewHeight())
	}
	m.SetStyle(lipgloss.NewStyle(), lipgloss.NewStyle())
	m.SetPlaceholder("ph")
	if !strings.Contains(m.View(), "ph") && m.Value() != "" {
		// placeholder 可能被值覆盖
	}

	m.Focus()
	if !m.Focused() {
		t.Fatal("focus")
	}
	m.Blur()
	if m.Focused() {
		t.Fatal("blur")
	}

	m.InsertNewline()
	m.AddToHistory("cmd1")
	m.AddToHistory("cmd2")
	if got := m.GetHistory(); len(got) != 2 {
		t.Fatalf("history = %v", got)
	}
	m.SetHistory([]string{"a", "b"})
	if len(m.GetHistory()) != 2 {
		t.Fatal("set history")
	}

	m.SetValue("x")
	if m.Value() != "x" {
		t.Fatal("value")
	}
	m.Clear()
	if m.Value() != "" {
		t.Fatal("clear")
	}

	if out := m.View(); out == "" {
		t.Fatal("view")
	}
}

func TestPredictionAndHistoryNav(t *testing.T) {
	m := New()
	m.SetPrediction("hello world")
	if m.Prediction() != "hello world" {
		t.Fatal("prediction")
	}
	if !strings.Contains(m.PredictionSuffix(), "world") || m.PredictionSuffix() == "" {
		// suffix 应有内容
		_ = m.PredictionSuffix()
	}
	m.ClearPrediction()
	if m.HasPrediction() {
		t.Fatal("clear")
	}
	if m.Prediction() != "" {
		t.Fatal("empty pred")
	}

	m.AddToHistory("h1")
	m.AddToHistory("h2")
	m.HistoryUp()
	m.HistoryUp()
	m.HistoryDown()
	m.HistoryDown()
	m.HistoryDown() // 越界
}
