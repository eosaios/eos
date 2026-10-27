package panels

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"

	tea "charm.land/bubbletea/v2"
)

type fakeTaskProvider struct {
	tasks      []coreapi.TaskSnapshot
	tasksErr   error
	tailLines  []string
	tailErr    error
	cleanupN   int
	cleanupErr error
}

func (f *fakeTaskProvider) Tasks(context.Context) ([]coreapi.TaskSnapshot, error) {
	return f.tasks, f.tasksErr
}
func (f *fakeTaskProvider) TailTask(context.Context, string) ([]string, error) {
	return f.tailLines, f.tailErr
}
func (f *fakeTaskProvider) CleanupTasks(context.Context) (int, error) {
	return f.cleanupN, f.cleanupErr
}

func sampleTasks() []coreapi.TaskSnapshot {
	return []coreapi.TaskSnapshot{
		{
			ID:        "t1",
			Kind:      "shell_task",
			Status:    "running",
			StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			Label:     "npm test",
			Summary:   "running tests",
		},
		{
			ID:        "t2",
			Kind:      "agent",
			Status:    "done",
			StartedAt: time.Date(2026, 1, 2, 4, 0, 0, 0, time.UTC),
			Summary:   "agent summary",
			Metadata:  map[string]any{"k": "v"},
		},
	}
}

func TestTasksPanelListAndRefresh(t *testing.T) {
	fp := &fakeTaskProvider{tasks: sampleTasks()}
	p := NewTasksPanel(testStyles(), "zh", fp)
	if cmd := p.Init(); cmd == nil {
		t.Fatal("Init should return tick cmd")
	}
	p.SetSize(100, 30)

	view := p.View()
	if !strings.Contains(view, "t1") || !strings.Contains(view, "npm test") {
		t.Fatalf("list view:\n%s", view)
	}

	// r 刷新
	p.tasks = nil
	p.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if len(p.tasks) != 2 {
		t.Fatalf("after r tasks = %d", len(p.tasks))
	}

	// tick 也刷新
	p.tasks = nil
	out, cmd := p.Update(TasksTickMsg{})
	if _, ok := cmd().(TasksTickMsg); !ok {
		t.Fatalf("tick cmd = %T", cmd())
	}
	if out.(*TasksPanel).tasks == nil {
		t.Fatal("tick should refresh")
	}

	// 语言切换
	out, _ = p.Update(LanguageChangeMsg{Language: "en"})
	if tp := out.(*TasksPanel); tp.language != "en" {
		t.Fatalf("lang = %q", tp.language)
	}

	// provider 错误清空列表
	fp.tasksErr = errors.New("boom")
	p.refresh()
	if p.tasks != nil {
		t.Fatalf("err refresh tasks = %v", p.tasks)
	}
}

func TestTasksPanelDetailView(t *testing.T) {
	fp := &fakeTaskProvider{tasks: sampleTasks(), tailLines: []string{"line1", "  ", "line3"}}
	p := NewTasksPanel(testStyles(), "zh", fp)
	p.SetSize(100, 30)

	// enter 打开 shell_task 详情（走 TailTask）
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !p.IsViewing() {
		t.Fatal("enter should open view")
	}
	view := p.View()
	if !strings.Contains(view, "t1") || !strings.Contains(view, "line1") {
		t.Fatalf("detail view:\n%s", view)
	}
	// 空行被替换成空格
	if !strings.Contains(strings.Join(p.viewLines, "|"), " ") {
		t.Fatalf("blank line not normalized: %v", p.viewLines)
	}

	// k 在 viewing 中 kill 当前
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if msg := cmd().(TaskKillRequestMsg); msg.ID != "t1" {
		t.Fatalf("kill = %+v", msg)
	}

	// tick 在 viewing 中刷新详情
	_, _ = p.Update(TasksTickMsg{})

	// 重置
	p.ResetView()
	if p.IsViewing() || p.viewID != "" {
		t.Fatal("ResetView failed")
	}
}

func TestTasksPanelNonShellDetailAndKill(t *testing.T) {
	fp := &fakeTaskProvider{tasks: sampleTasks()}
	p := NewTasksPanel(testStyles(), "zh", fp)
	p.SetSize(100, 30)

	// 选中第二行（agent）
	p.table.SetCursor(1)
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !p.IsViewing() {
		t.Fatal("should open")
	}
	view := p.View()
	if !strings.Contains(view, "agent summary") || !strings.Contains(view, "k: v") {
		t.Fatalf("agent detail:\n%s", view)
	}

	// 非 shell_task 无 details
	p.openView("missing")
	if !strings.Contains(strings.Join(p.viewLines, "\n"), "(no details)") {
		// findTask 失败时 viewTask 为空 → no details
		t.Fatalf("no details: %v", p.viewLines)
	}

	// 非 viewing 时 k kill 选中项
	p.ResetView()
	p.table.SetCursor(0)
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if msg := cmd().(TaskKillRequestMsg); msg.ID != "t1" {
		t.Fatalf("kill = %+v", msg)
	}

	// 空列表 k 不产生消息
	p.tasks = nil
	if _, cmd = p.Update(tea.KeyPressMsg{Code: 'k', Text: "k"}); cmd != nil {
		t.Fatalf("empty k = %T", cmd())
	}
}

func TestTasksPanelCleanupAndTailError(t *testing.T) {
	fp := &fakeTaskProvider{tasks: sampleTasks(), cleanupN: 2}
	p := NewTasksPanel(testStyles(), "zh", fp)
	p.SetSize(100, 30)

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	toast := cmd().(TaskToastMsg)
	if !strings.Contains(toast.Text, "2") {
		t.Fatalf("toast = %q", toast.Text)
	}

	// cleanup=0 无 toast
	fp.cleanupN = 0
	if _, cmd = p.Update(tea.KeyPressMsg{Code: 'c', Text: "c"}); cmd != nil {
		t.Fatalf("zero cleanup cmd = %T", cmd())
	}

	// TailTask 错误
	fp.tailErr = errors.New("tail fail")
	p.openView("t1")
	if !strings.Contains(strings.Join(p.viewLines, "\n"), "tail fail") {
		t.Fatalf("tail err lines = %v", p.viewLines)
	}

	// shell_task 且 provider 为 nil
	p2 := NewTasksPanel(testStyles(), "zh", nil)
	p2.tasks = sampleTasks()
	p2.openView("t1")
	if !strings.Contains(strings.Join(p2.viewLines, "\n"), "(no task provider)") {
		t.Fatalf("nil provider lines = %v", p2.viewLines)
	}

	// viewID 空时 refreshView 直接返回
	p2.viewID = ""
	p2.refreshView()
}

func TestTasksPanelLongCommandTruncates(t *testing.T) {
	long := strings.Repeat("x", 80)
	fp := &fakeTaskProvider{tasks: []coreapi.TaskSnapshot{{
		ID: "long", Kind: "shell_task", Status: "run",
		StartedAt: time.Now(), Label: long,
	}}}
	p := NewTasksPanel(testStyles(), "zh", fp)
	p.SetSize(80, 24)
	rows := p.table.Rows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if !strings.HasSuffix(rows[0][4], "…") {
		t.Fatalf("cmd cell = %q, want ellipsis", rows[0][4])
	}
}

// ---------- Context ----------

func TestContextPanelMessagesAndStats(t *testing.T) {
	p := NewContextPanel(testStyles(), "zh")
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}
	p.SetSize(100, 40)

	// 空消息
	if p.GetSelectedMessage() != nil {
		t.Fatal("empty selection should be nil")
	}
	view := p.View()
	if view == "" {
		t.Fatal("empty view")
	}

	// 有消息 + 统计
	long := strings.Repeat("字", 50)
	p.SetMessages([]ContextMessage{
		{Role: "user", Content: "hello\nworld", Tokens: 10},
		{Role: "assistant", Content: long, Tokens: 20},
		{Role: "user", Content: "", Tokens: 1},
	})
	p.SetStats("gpt-4o", 8000, 1000, 2000)

	view = p.View()
	for _, want := range []string{"user", "assistant", "gpt-4o", "8000"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if sel := p.GetSelectedMessage(); sel == nil || sel.Role != "user" {
		t.Fatalf("selected = %+v", sel)
	}

	// 预览截断 + 换行折叠已发生（updateTable 内）
	rows := p.table.Rows()
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	if strings.Contains(rows[0][1], "\n") {
		t.Fatalf("preview contains newline: %q", rows[0][1])
	}

	// 窄宽下 preview 列最小 18
	p.SetSize(20, 20)
	p.updateTableColumns()
	if p.previewWidth < 18 {
		t.Fatalf("previewWidth = %d", p.previewWidth)
	}
}

func TestContextPanelActionsAndViewDetail(t *testing.T) {
	p := NewContextPanel(testStyles(), "zh")
	p.SetSize(100, 40)
	p.SetMessages([]ContextMessage{
		{Role: "user", Content: "hello", Tokens: 5},
		{Role: "assistant", Content: "   \n", Tokens: 2},
	})

	// 左右环绕
	p.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if p.GetCurrentAction() != "View" {
		t.Fatalf("left wrap = %q", p.GetCurrentAction())
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if p.GetCurrentAction() != "Compact" {
		t.Fatalf("right wrap = %q", p.GetCurrentAction())
	}

	// enter Compact
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := cmd().(ContextCompactMsg); !ok {
		t.Fatalf("compact = %T", cmd())
	}

	// 快捷键
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if _, ok := cmd().(ContextCompactMsg); !ok {
		t.Fatalf("c = %T", cmd())
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if _, ok := cmd().(ContextClearMsg); !ok {
		t.Fatalf("x = %T", cmd())
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if _, ok := cmd().(ContextExportMsg); !ok {
		t.Fatalf("e = %T", cmd())
	}

	// v 查看详情（空内容 → (empty)）
	p.table.SetCursor(1)
	p.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if !p.IsViewing() {
		t.Fatal("v should open detail")
	}
	if !strings.Contains(p.detail.View(), "(empty)") {
		t.Fatalf("detail empty content:\n%s", p.detail.View())
	}
	detail := p.View()
	if !strings.Contains(detail, "Token") {
		t.Fatalf("detail meta missing:\n%s", detail)
	}

	// esc 关闭
	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if p.IsViewing() {
		t.Fatal("esc should close")
	}

	// enter 切到 View 动作
	for range 3 {
		p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	if p.GetCurrentAction() != "View" {
		t.Fatalf("action = %q", p.GetCurrentAction())
	}
	p.table.SetCursor(0)
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !p.IsViewing() {
		t.Fatal("enter View should open")
	}
	// 选中为空时 viewDetail 自复位
	p.messages = nil
	p.table.SetRows(nil)
	view := p.View()
	if p.IsViewing() {
		// viewDetail 会把 viewing 置回 false 并重渲染
		_ = view
	}
	if p.IsViewing() {
		t.Fatal("nil selected should reset viewing")
	}

	// ResetView / 语言切换
	p.ResetView()
	out, _ := p.Update(LanguageChangeMsg{Language: "en"})
	if cp := out.(*ContextPanel); cp.language != "en" {
		t.Fatalf("lang = %q", cp.language)
	}

	// 越界 action
	p.actionIndex = 99
	if p.GetCurrentAction() != "" {
		t.Fatal("oob action")
	}
}

func TestTruncateForWidthAndFixedWidthLine(t *testing.T) {
	if got := truncateForWidth("abc", 0); got != "" {
		t.Fatalf("width 0 = %q", got)
	}
	if got := truncateForWidth("hello", 10); got != "hello" {
		t.Fatalf("short = %q", got)
	}
	if got := truncateForWidth("hello", 1); got != "…" {
		t.Fatalf("width 1 = %q", got)
	}
	// 压缩空白
	if got := truncateForWidth("  a\n\tb  ", 20); got != "a b" {
		t.Fatalf("normalize = %q", got)
	}
	// 中文按宽度截断
	got := truncateForWidth("中文测试内容很多", 4)
	if !strings.HasSuffix(got, "…") || runewidthLen(got) > 4 {
		t.Fatalf("cjk truncate = %q", got)
	}
	// 宽度 1 的中英文都返回 …
	if truncateForWidth("中", 1) != "…" {
		t.Fatal("cjk width 1")
	}

	if line := renderFixedWidthLine("x", 0); line != "x" {
		t.Fatalf("fixed 0 = %q", line)
	}
	if line := renderFixedWidthLine("x", 10); !strings.Contains(line, "x") {
		t.Fatalf("fixed 10 = %q", line)
	}
}

func runewidthLen(s string) int {
	n := 0
	for _, r := range s {
		if r == '…' {
			n++
			continue
		}
		if r > 0x7f {
			n += 2
		} else {
			n++
		}
	}
	return n
}
