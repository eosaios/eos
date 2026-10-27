package styles

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestThemes(t *testing.T) {
	if DefaultDarkTheme() == nil || DefaultLightTheme() == nil || HighContrastTheme() == nil {
		t.Fatal("nil theme")
	}
	if GetTheme("dark") == nil {
		t.Fatal("dark")
	}
	if GetTheme("light") == nil {
		t.Fatal("light")
	}
	if GetTheme("high-contrast") == nil {
		t.Fatal("high-contrast")
	}
	// 未知回落
	if GetTheme("nope") == nil {
		t.Fatal("fallback")
	}
}

func TestNewStyles(t *testing.T) {
	s := NewStyles(DefaultDarkTheme())
	if s == nil {
		t.Fatal("nil styles")
	}
	_ = s.TextMuted.Render("x")
	_ = s.TextInfo.Render("x")
	_ = s.TextSuccess.Render("x")
	_ = s.TextError.Render("x")
	_ = s.MsgInfo.Render("x")
	_ = s.MsgError.Render("x")
	_ = s.MsgWarning.Render("x")
	_ = s.ContentPanel.Render("x")
	_ = s.StreamAIPrefix.Render("x")
	_ = s.StreamMeta.Render("x")
	_ = s.MsgPlanHeader.Render("x")
	_ = s.MsgPlanStep.Render("x")
	_ = s.MsgThinkingHeader.Render("x")
	_ = s.MsgAgentHeader.Render("x")

	s2 := NewStyles(DefaultLightTheme())
	_ = s2.Text.Render("x")
	s3 := NewStyles(HighContrastTheme())
	_ = s3.Text.Render("x")
}
