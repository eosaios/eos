package help

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func newHelp() *HelpView {
	return NewHelpView(styles.NewStyles(styles.DefaultDarkTheme()), "zh")
}

func TestHelpViewLifecycle(t *testing.T) {
	h := newHelp()
	if cmd := h.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}

	h.SetSize(100, 40)
	if h.width != 100 || h.height != 40 {
		t.Fatalf("size = %dx%d", h.width, h.height)
	}
	if h.vp.Width() <= 0 || h.vp.Height() <= 0 {
		t.Fatalf("viewport = %dx%d", h.vp.Width(), h.vp.Height())
	}

	// 零尺寸 relayout 兜底
	h2 := newHelp()
	h2.relayout()
	if h2.vp.Width() < 10 {
		t.Fatalf("fallback width = %d", h2.vp.Width())
	}

	// 极小尺寸夹到下限
	h.SetSize(4, 4)
	if h.vp.Width() < 10 || h.vp.Height() < 5 {
		t.Fatalf("clamped = %dx%d", h.vp.Width(), h.vp.Height())
	}

	h.SetLanguage("en")
	if h.language != "en" || !h.resetTop {
		t.Fatal("SetLanguage")
	}
	h.ResetScroll()
	if !h.resetTop {
		t.Fatal("ResetScroll")
	}

	// View 默认尺寸 + 内容缓存
	view := h.View()
	if !strings.Contains(stripANSIHelpTest(view), "F2") {
		t.Fatalf("view:\n%s", view)
	}
	// 二次 View 走内容缓存路径
	view2 := h.View()
	if view2 == "" {
		t.Fatal("cached view empty")
	}

	// 语言切换消息
	out, _ := h.Update(LanguageChangeMsg{Language: "zh"})
	if out.language != "zh" {
		t.Fatalf("lang msg = %q", out.language)
	}

	// 按键滚动
	h.SetSize(100, 20)
	_, _ = h.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
}

func TestRenderAlignedRow(t *testing.T) {
	h := newHelp()
	// 短 label 同行
	out := stripANSIHelpTest(h.renderAlignedRow("F2", "toggle", 12, lipgloss.NewStyle(), lipgloss.NewStyle()))
	if strings.Contains(out, "\n") {
		t.Fatalf("short label should be one line: %q", out)
	}
	if !strings.HasPrefix(out, "F2") {
		t.Fatalf("prefix = %q", out)
	}

	// 长 label 换行
	out = stripANSIHelpTest(h.renderAlignedRow("very-long-key-name", "desc", 12, lipgloss.NewStyle(), lipgloss.NewStyle()))
	if !strings.Contains(out, "\n") {
		t.Fatalf("long label should wrap: %q", out)
	}

	// trim 空白
	out = stripANSIHelpTest(h.renderAlignedRow("  key  ", "  desc  ", 12, lipgloss.NewStyle(), lipgloss.NewStyle()))
	if !strings.HasPrefix(out, "key") {
		t.Fatalf("trimmed = %q", out)
	}

	// renderKey / renderCmd
	if out := stripANSIHelpTest(h.renderKey("Tab", "next")); !strings.Contains(out, "Tab") {
		t.Fatalf("renderKey = %q", out)
	}
	if out := stripANSIHelpTest(h.renderCmd("/help", "show help")); !strings.Contains(out, "/help") {
		t.Fatalf("renderCmd = %q", out)
	}
}

func TestRenderContentEN(t *testing.T) {
	h := newHelp()
	out := stripANSIHelpTest(h.renderContent("en"))
	if !strings.Contains(out, "F2") {
		t.Fatalf("en content:\n%s", out)
	}
}
