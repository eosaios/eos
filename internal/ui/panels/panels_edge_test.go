package panels

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/ui/styles"

	tea "charm.land/bubbletea/v2"
)

func testStyles() *styles.Styles {
	return styles.NewStyles(styles.DefaultDarkTheme())
}

func TestBasePanelAccessors(t *testing.T) {
	p := NewBasePanel("demo")
	if p.GetName() != "demo" {
		t.Fatalf("name = %q", p.GetName())
	}
	if p.IsActive() {
		t.Fatal("new panel should be inactive")
	}
	p.SetActive(true)
	if !p.IsActive() {
		t.Fatal("SetActive(true) failed")
	}
	p.SetSize(80, 24)
	w, h := p.GetSize()
	if w != 80 || h != 24 {
		t.Fatalf("size = %dx%d", w, h)
	}

	// 有标题：高度扣 1
	out := p.RenderBorder("body", "Title")
	if !strings.Contains(out, "Title") || !strings.Contains(out, "body") {
		t.Fatalf("render = %q", out)
	}
	// 无标题 + 零尺寸兜底
	out = p.RenderBorder("x", "")
	if !strings.Contains(out, "x") {
		t.Fatalf("empty-title render = %q", out)
	}
	// height=0 时 h 变负再夹到 0
	p.SetSize(40, 0)
	out = p.RenderBorder("y", "T")
	if !strings.Contains(out, "y") {
		t.Fatalf("zero-height render = %q", out)
	}
}

// ---------- Cost ----------

func TestCostPanelEmptyAndStats(t *testing.T) {
	p := NewCostPanel(testStyles(), "zh")
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}
	p.SetSize(100, 40)
	view := p.View()
	if !strings.Contains(view, "暂无") && !strings.Contains(view, "统计") {
		// 空表占位行 + 标题
		t.Fatalf("empty view:\n%s", view)
	}

	p.SetStats([]CostStats{
		{Model: "gpt-4o", Rounds: 3, Input: "1k", Reply: "2k", Total: "3k"},
	}, TotalStats{TotalRounds: 3, TotalInput: "1k", TotalReply: "2k", TotalTokens: "3k", TotalDuration: 900})
	view = p.View()
	if !strings.Contains(view, "gpt-4o") {
		t.Fatalf("stats missing model:\n%s", view)
	}
	if !strings.Contains(view, "300ms") {
		t.Fatalf("avg duration missing:\n%s", view)
	}

	if w, h := p.GetSize(); w != 100 || h != 40 {
		t.Fatalf("size = %dx%d", w, h)
	}
}

func TestCostPanelActions(t *testing.T) {
	p := NewCostPanel(testStyles(), "zh")

	// 左右环绕
	if p.GetCurrentAction() != "Clear" {
		t.Fatalf("initial action = %q", p.GetCurrentAction())
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyLeft}) // wrap to Refresh
	if p.GetCurrentAction() != "Refresh" {
		t.Fatalf("after left wrap = %q", p.GetCurrentAction())
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // wrap to Clear
	if p.GetCurrentAction() != "Clear" {
		t.Fatalf("after right wrap = %q", p.GetCurrentAction())
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if p.GetCurrentAction() != "Export" {
		t.Fatalf("after right = %q", p.GetCurrentAction())
	}

	// enter 执行当前操作
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := cmd().(CostExportMsg); !ok {
		t.Fatalf("enter on Export = %T", cmd())
	}

	// 快捷键
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if _, ok := cmd().(CostClearMsg); !ok {
		t.Fatalf("c = %T", cmd())
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if _, ok := cmd().(CostExportMsg); !ok {
		t.Fatalf("e = %T", cmd())
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if _, ok := cmd().(CostRefreshMsg); !ok {
		t.Fatalf("r = %T", cmd())
	}

	// 语言切换刷新列
	p2, _ := p.Update(LanguageChangeMsg{Language: "en"})
	if cp, ok := p2.(*CostPanel); !ok || cp.language != "en" {
		t.Fatalf("lang switch = %v", p2)
	}

	// GetCurrentAction 越界
	p.actionIndex = 99
	if got := p.GetCurrentAction(); got != "" {
		t.Fatalf("oob action = %q", got)
	}
}

// ---------- Workspace ----------

func TestWorkspaceItemTitles(t *testing.T) {
	named := WorkspaceItem{workspace: Workspace{Name: "proj", Path: "/ws/proj"}, active: true}
	if named.Title() != "proj" {
		t.Fatalf("title = %q", named.Title())
	}
	if named.Description() != "* /ws/proj" {
		t.Fatalf("desc = %q", named.Description())
	}
	unnamed := WorkspaceItem{workspace: Workspace{Path: "/ws/plain"}}
	if unnamed.Title() != "plain" {
		t.Fatalf("unnamed title = %q", unnamed.Title())
	}
	if unnamed.Description() != "/ws/plain" {
		t.Fatalf("unnamed desc = %q", unnamed.Description())
	}
	if !strings.Contains(unnamed.FilterValue(), "plain") {
		t.Fatalf("filter = %q", unnamed.FilterValue())
	}
}

func TestWorkspacePanelSelectAddDelete(t *testing.T) {
	p := NewWorkspacePanel(testStyles(), "zh")
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}
	p.SetWorkspaces([]Workspace{
		{Name: "a", Path: "/ws/a"},
		{Name: "b", Path: "/ws/b"},
	}, "/ws/b")
	p.SetSize(80, 30)

	// enter/u 选中当前项（默认第一项 a）
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg := cmd().(WorkspaceSelectMsg)
	if msg.Path != "/ws/a" {
		t.Fatalf("select = %q", msg.Path)
	}

	_, cmd = p.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if _, ok := cmd().(WorkspaceSelectMsg); !ok {
		t.Fatalf("u = %T", cmd())
	}

	_, cmd = p.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if _, ok := cmd().(WorkspaceAddMsg); !ok {
		t.Fatalf("a = %T", cmd())
	}

	_, cmd = p.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	del := cmd().(WorkspaceDeleteMsg)
	if del.Path != "/ws/a" {
		t.Fatalf("delete = %q", del.Path)
	}

	// 空列表：enter/d 不产生选择消息
	empty := NewWorkspacePanel(testStyles(), "zh")
	_, cmd = empty.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("empty enter cmd = %T", cmd())
	}
	_, cmd = empty.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if cmd != nil {
		t.Fatalf("empty d cmd = %T", cmd())
	}

	// 语言切换
	out, _ := p.Update(LanguageChangeMsg{Language: "en"})
	if wp := out.(*WorkspacePanel); wp.language != "en" {
		t.Fatalf("lang = %q", wp.language)
	}

	if view := p.View(); !strings.Contains(view, "Workspace Panel") {
		t.Fatalf("view missing border title:\n%s", view)
	}
}

// ---------- Versions ----------

func TestFileAndVersionItemLabels(t *testing.T) {
	f := FileItem{Path: "a.txt", Count: 3, Last: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	if f.Title() != "a.txt (3)" {
		t.Fatalf("file title = %q", f.Title())
	}
	if f.Description() != "2026-01-02 03:04:05" {
		t.Fatalf("file desc = %q", f.Description())
	}
	empty := FileItem{Path: "b"}
	if empty.Title() != "b" {
		t.Fatal("zero count should omit count")
	}
	if empty.Description() != "" {
		t.Fatal("zero last should be empty desc")
	}
	if f.FilterValue() != "a.txt" {
		t.Fatalf("filter = %q", f.FilterValue())
	}

	v := VersionItem{Timestamp: "ts", FilePath: "a.txt", Size: 12}
	if v.Title() != "ts" || v.Description() != "12 bytes" {
		t.Fatalf("version = %q/%q", v.Title(), v.Description())
	}
	if (VersionItem{Timestamp: "t", FilePath: "f"}).Description() != "f" {
		t.Fatal("zero size should fall back to path")
	}
	if v.FilterValue() != "ts" {
		t.Fatalf("v filter = %q", v.FilterValue())
	}
}

func TestVersionsPanelModesAndKeys(t *testing.T) {
	p := NewVersionsPanel(testStyles())
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}
	p.SetLanguage("  ")
	if p.language != "zh" {
		t.Fatalf("blank lang = %q", p.language)
	}
	p.SetLanguage("en")
	if p.language != "en" {
		t.Fatalf("lang = %q", p.language)
	}

	p.SetFiles([]FileItem{{Path: "a.txt", Count: 1}})
	p.SetSize(80, 30)

	// files 模式 enter → VersionsLoadMsg
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	load := cmd().(VersionsLoadMsg)
	if load.FilePath != "a.txt" {
		t.Fatalf("load = %q", load.FilePath)
	}

	// files 模式 x → 删除该文件全部版本
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if _, ok := cmd().(VersionsDeleteFileMsg); !ok {
		t.Fatalf("x in files = %T", cmd())
	}

	// 切到 versions 模式
	p.SetVersions("a.txt", []VersionItem{{Timestamp: "t1", Size: 5}})
	if p.mode != "versions" {
		t.Fatalf("mode = %q", p.mode)
	}

	// versions 模式 r → 回滚（FilePath 由 SetVersions 注入）
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	rb := cmd().(VersionsRollbackMsg)
	if rb.FilePath != "a.txt" || rb.Timestamp != "t1" {
		t.Fatalf("rollback = %+v", rb)
	}

	// versions 模式 x → 删除该版本
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if _, ok := cmd().(VersionsDeleteMsg); !ok {
		t.Fatalf("x in versions = %T", cmd())
	}

	// ctrl+x → 删除全部
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	if _, ok := cmd().(VersionsDeleteAllMsg); !ok {
		t.Fatalf("ctrl+x = %T", cmd())
	}

	// esc 回 files
	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if p.mode != "files" {
		t.Fatalf("after esc mode = %q", p.mode)
	}
	// files 模式 esc 不再切模式
	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if p.mode != "files" {
		t.Fatalf("files esc mode = %q", p.mode)
	}

	// 语言切换
	p.Update(LanguageChangeMsg{Language: "en"})
	if p.language != "en" {
		t.Fatalf("lang = %q", p.language)
	}

	// Reset 回 files
	p.Reset()
	if p.mode != "files" || len(p.versions.Items()) != 0 {
		t.Fatalf("reset mode=%q versions=%d", p.mode, len(p.versions.Items()))
	}

	// View 两模式
	p.SetFiles([]FileItem{{Path: "a.txt"}})
	if view := p.View(); !strings.Contains(view, "a.txt") {
		t.Fatalf("files view:\n%s", view)
	}
	p.SetVersions("a.txt", []VersionItem{{Timestamp: "t1"}})
	if view := p.View(); !strings.Contains(view, "t1") {
		t.Fatalf("versions view:\n%s", view)
	}
}
