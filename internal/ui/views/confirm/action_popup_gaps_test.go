package confirm

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// ActionPopup 余臂批测：Update 全键位（esc/up/down 边界/enter 越界/
// 数字快选命中与越界/无关键）、View 标题与空 label 回落、SetSize。

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newPopup() *ActionPopup {
	return NewActionPopup(testStyles(), "zh", ActionRequest{
		Title: "选择操作",
		Actions: []ActionItem{
			{Kind: "open", Label: "打开"},
			{Kind: "reveal", Label: "定位"},
			{Kind: "copy", Label: ""},
		},
	})
}

func TestActionPopupUpdateKeys(t *testing.T) {
	m := newPopup()

	// 无关消息直通。
	if got, cmd := m.Update(nil); got != m || cmd != nil {
		t.Fatal("nil 消息应直通")
	}

	// esc → cancel 命令。
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc 应产命令")
	}
	if msg := cmd(); msg.(ActionResultMsg).Kind != "cancel" {
		t.Fatal("esc 应 cancel")
	}

	// down 到底不再进。
	for i := 0; i < 5; i++ {
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.selected != 2 {
		t.Fatalf("down 底界 = %d", m.selected)
	}
	// up 到顶不再退。
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	for i := 0; i < 5; i++ {
		m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if m.selected != 0 {
		t.Fatalf("up 顶界 = %d", m.selected)
	}

	// 数字快选：命中与越界（5 超出 3 项不动）。
	m.Update(tea.KeyPressMsg{Code: '2'})
	if m.selected != 1 {
		t.Fatalf("快选 = %d", m.selected)
	}
	m.Update(tea.KeyPressMsg{Code: '9'})
	if m.selected != 1 {
		t.Fatalf("越界快选不应动 = %d", m.selected)
	}
	m.Update(tea.KeyPressMsg{Code: '0'})
	if m.selected != 1 {
		t.Fatalf("0 不在快选域 = %d", m.selected)
	}

	// enter：选中项 → kind 命令。
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter 应产命令")
	}
	res := cmd().(ActionResultMsg)
	if res.Kind != m.req.Actions[m.selected].Kind || res.Index != m.req.Index {
		t.Fatalf("enter 结果 = %+v（选中 %d）", res, m.selected)
	}

	// enter 越界守卫：selected 拉出界。
	m2 := newPopup()
	m2.selected = 99
	_, cmd = m2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("越界 enter 应无命令")
	}

	// 无关字符键。
	m3 := newPopup()
	m3.Update(tea.KeyPressMsg{Code: 'x'})
}

func TestActionPopupViewBranches(t *testing.T) {
	m := newPopup()
	m.SetSize(60, 20)
	out := m.View()
	if !strings.Contains(out, "选择操作") {
		t.Fatalf("标题:\n%s", out)
	}
	if !strings.Contains(out, "1. 打开") || !strings.Contains(out, "2. 定位") {
		t.Fatalf("选项:\n%s", out)
	}
	// 空 label 回落 kind。
	if !strings.Contains(out, "3. copy") {
		t.Fatalf("空 label 回落:\n%s", out)
	}

	// 空 title → i18n 默认。
	m2 := NewActionPopup(testStyles(), "zh", ActionRequest{
		Actions: []ActionItem{{Kind: "a"}},
	})
	if out2 := m2.View(); strings.TrimSpace(out2) == "" {
		t.Fatal("空标题应有默认渲染")
	}
	if cmd := m2.Init(); cmd != nil {
		t.Fatal("Init 恒 nil")
	}
}
