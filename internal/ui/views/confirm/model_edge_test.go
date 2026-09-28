package confirm

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newTestModel(kind string, opts []string, allowText bool) *Model {
	return New(testStyles(), "zh", "monokai", Request{
		ID: "req-1",
		Kind: kind,
		Title: "确认操作",
		Question: "执行该命令？",
		Options: opts,
		Diff: "+ added\n- removed",
		DiffPath: "/ws/a.txt",
		AllowText: allowText,
		TextHint: "输入补充",
	})
}

func TestConfirmModelLifecycle(t *testing.T) {
	m := newTestModel("permission", []string{"accept", "decline"}, false)
	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init = %v, want nil", cmd)
	}
	m.SetSize(100, 40)
	if view := m.View(); view == "" {
		t.Error("View should not be empty")
	}

	// esc：EscDecision 推断 decision 并带出选项。
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	msg := cmd()
	res, ok := msg.(ResultMsg)
	if !ok {
		t.Fatalf("esc produced %T", msg)
	}
	if res.ID != "req-1" || res.Decision == "" {
		t.Errorf("esc result = %+v", res)
	}

	// up/down 导航边界。
	m2 := newTestModel("generic", []string{"a", "b", "c"}, false)
	_, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyUp})   // 顶部不再上移
	_, cmd = m2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res := cmd().(ResultMsg); res.Option != "a" || res.Decision != "a" {
		t.Errorf("enter at top = %+v", res)
	}
	_, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // 底部不再下移
	_, cmd = m2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res := cmd().(ResultMsg); res.Option != "c" || res.OptionIndex != 2 {
		t.Errorf("enter at bottom = %+v", res)
	}

	// 数字快选：'3' 选中第三项。
	_, _ = m2.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	_, cmd = m2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res := cmd().(ResultMsg); res.Option != "c" {
		t.Errorf("quick select = %+v", res)
	}
	// 越界数字被忽略。
	_, _ = m2.Update(tea.KeyPressMsg{Code: '9', Text: "9"})
	_, cmd = m2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res := cmd().(ResultMsg); res.OptionIndex == 8 {
		t.Errorf("out-of-range quick select applied: %+v", res)
	}

	// 非按钮字母不改选择。
	_, cmd = m2.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if cmd != nil {
		res := cmd()
		t.Logf("letter key msg = %#v", res)
	}

	// 空选项 + 非 permission：enter 决策回落 confirm。
	m3 := New(testStyles(), "zh", "monokai", Request{Kind: "generic"})
	_, cmd = m3.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res := cmd().(ResultMsg); res.Decision != "confirm" {
		t.Errorf("empty options decision = %q", res.Decision)
	}
}

func TestConfirmModelTextInput(t *testing.T) {
	m := newTestModel("generic", []string{"ok"}, true)
	// tab 聚焦文本框，输入字符进入 input。
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	_, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	_, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if got := strings.TrimSpace(m.input.Value()); got != "hi" {
		t.Errorf("input value = %q", got)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res := cmd().(ResultMsg); res.Text != "hi" {
		t.Errorf("enter text = %q", res.Text)
	}
	// 再 tab 回选项焦点。
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focusTxt {
		t.Error("tab should toggle focus back to options")
	}
	// AllowText=false 时 tab 不切换。
	m2 := newTestModel("generic", []string{"ok"}, false)
	_, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m2.focusTxt {
		t.Error("tab should be ignored when text not allowed")
	}
}

func TestConfirmModelViewBranches(t *testing.T) {
	// permission 空标题 → status.header；含 diff 路径与选项标签本地化。
	m := New(testStyles(), "zh", "monokai", Request{
		Kind:    "permission",
		Options: []string{"accept", "acceptForSession", "decline", "cancel"},
		Diff:    strings.Repeat("+ line\n", 1200), // 触发 5000 截断
		DiffPath: "/ws/x.txt",
	})
	view := m.View()
	if !strings.Contains(view, "/ws/x.txt") || !strings.Contains(view, "...") {
		t.Errorf("permission view missing diff path or truncation marker")
	}
	for _, marker := range []string{"1. ", "2. ", "3. ", "4. "} {
		if !strings.Contains(view, marker) {
			t.Errorf("permission options missing %q", marker)
		}
	}
	// 非 permission 空标题 → op.confirm；无 diff。
	m2 := New(testStyles(), "zh", "", Request{Kind: "generic", Question: "q", Options: []string{"yes"}})
	if v := m2.View(); !strings.Contains(v, "1. yes") {
		t.Errorf("generic view options = %q", v)
	}
	// AllowText 渲染提示与输入框。
	m3 := newTestModel("generic", []string{"ok"}, true)
	if v := m3.View(); !strings.Contains(v, "输入补充") {
		t.Errorf("text hint missing: %q", v)
	}
}

func TestStrconvItoa(t *testing.T) {
	cases := map[int]string{0: "0", 1: "1", 42: "42", -7: "-7", 987654: "987654"}
	for in, want := range cases {
		if got := strconvItoa(in); got != want {
			t.Errorf("strconvItoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestItoa(t *testing.T) {
	if got := itoa(12); got != "12" {
		t.Errorf("itoa = %q", got)
	}
}

func TestRenderInlinePermission(t *testing.T) {
	// nil styles 安静返回空。
	if got := RenderInlinePermission(nil, "zh", "", Request{}, 0, 80); got != "" {
		t.Errorf("nil styles = %q", got)
	}
	req := Request{
		Title:    "标题",
		Question: "问题",
		Options:  []string{"accept", "decline"},
		Diff:     strings.Repeat("+ x\n", 20),
		DiffPath: "/ws/b.txt",
	}
	view := RenderInlinePermission(testStyles(), "zh", "monokai", req, 1, 80)
	if !strings.Contains(view, "标题") || !strings.Contains(view, "问题") || !strings.Contains(view, "/ws/b.txt") {
		t.Errorf("inline permission missing sections:\n%s", view)
	}
	// 空标题回落默认文案；窄宽度自适应。
	view = RenderInlinePermission(testStyles(), "zh", "", Request{Options: []string{"accept"}}, 0, 10)
	if view == "" {
		t.Error("narrow width should still render")
	}
}

func TestPermissionOptionLabel(t *testing.T) {
	if got := permissionOptionLabel("zh", "accept", 0); !strings.HasPrefix(got, "1. ") {
		t.Errorf("accept label = %q", got)
	}
	if got := permissionOptionLabel("zh", "unknown", 2); !strings.HasPrefix(got, "3. unknown") {
		t.Errorf("default label = %q", got)
	}
}

func TestTruncateInlinePermissionDiff(t *testing.T) {
	if got := truncateInlinePermissionDiff("   "); got != "" {
		t.Errorf("blank diff = %q", got)
	}
	short := "a\nb"
	if got := truncateInlinePermissionDiff(short); got != short {
		t.Errorf("short diff = %q", got)
	}
	long := strings.Repeat("line\n", 10)
	if got := truncateInlinePermissionDiff(long); !strings.Contains(got, "...") || strings.Count(got, "line") != 8 {
		t.Errorf("long diff truncation = %q", got)
	}
	wide := strings.Repeat("字", 900)
	if got := truncateInlinePermissionDiff(wide); !strings.HasSuffix(got, "...") {
		t.Errorf("wide diff truncation length = %d", len(got))
	}
}
