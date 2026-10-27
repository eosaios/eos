package content

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"
)

func TestSetContentPreserveOffset_AutoBottomWhenAtBottom(t *testing.T) {
	m := New(80, 3)
	m.SetContent(strings.Join([]string{"1", "2", "3", "4", "5"}, "\n"))
	if !m.AtBottom() {
		t.Fatalf("expected at bottom after SetContent")
	}
	m.SetContentPreserveOffset(strings.Join([]string{"1", "2", "3", "4", "5", "6"}, "\n"))
	if !m.AtBottom() {
		t.Fatalf("expected stay at bottom after SetContentPreserveOffset when at bottom")
	}
}

func TestSetContentPreserveOffset_NoAutoBottomWhenScrolledUp(t *testing.T) {
	m := New(80, 3)
	m.SetContent(strings.Join([]string{"1", "2", "3", "4", "5"}, "\n"))
	if !m.AtBottom() {
		t.Fatalf("expected at bottom after SetContent")
	}
	m.LineUp()
	if m.AtBottom() {
		t.Fatalf("expected not at bottom after LineUp")
	}
	old := m.YOffset()
	m.SetContentPreserveOffset(strings.Join([]string{"1", "2", "3", "4", "5", "6"}, "\n"))
	if m.AtBottom() {
		t.Fatalf("expected not auto-scroll to bottom when user scrolled up")
	}
	if m.YOffset() != old {
		t.Fatalf("expected offset preserved, got %d want %d", m.YOffset(), old)
	}
}
