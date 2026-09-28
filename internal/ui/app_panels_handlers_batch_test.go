package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-LCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// internal/ui/app_panels_handlers_batch_test.go — 批测：app_panels.go 消息
// 处理器族（0% 尾巴）：rules/memory/context/cost/settings/language/tasks/
// lsp 各 Msg 入口的成功/失败/空值臂。失败臂经 testEngine 注入口注入
// （见 app_model_test_engine_test.go，零值保持原成功行为）。

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eosaios/eos/internal/i18n"
	"github.com/eosaios/eos/internal/pkg/settings"
	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/pkg/coreapi"
)

// newTestAppModelWithEngine 复刻 newTestAppModel 但保留 engine 指针供注入。
func newTestAppModelWithEngine(t *testing.T) (*AppModel, *testEngine) {
	t.Helper()
	engine := newTestEngine()
	engine.models = []coreapi.ModelConfig{{
		Name:    "default-model",
		APIBase: "https://example.com/v1",
		Model:   "demo-model",
	}}
	engine.activeModel = "default-model"
	return NewAppModelFromCoreEngine(engine), engine
}

// lastSystem 返回最后一条系统消息（kind=system）的 level 与内容。
func lastSystem(t *testing.T, m *AppModel) (level, content string) {
	t.Helper()
	for i := len(m.history) - 1; i >= 0; i-- {
		if m.history[i].kind == "system" {
			return m.history[i].level, m.history[i].content
		}
	}
	t.Fatal("没有系统消息")
	return "", ""
}

func TestRulesSaveHandlerBranches(t *testing.T) {
	setTestHome(t)
	app, engine := newTestAppModelWithEngine(t)

	// 默认 scope（空/空白 → project）
	next, _ := app.handleRulesSaveMsg(panels.RulesSaveMsg{Content: "# rules"})
	if next == nil {
		t.Fatal("model nil")
	}
	if lvl, txt := lastSystem(t, app); lvl != "success" || txt != i18n.T("rules.saved_project", app.state.Language) {
		t.Fatalf("project toast = %q/%q", lvl, txt)
	}
	// 显式 global（大小写与空白归一）
	app.handleRulesSaveMsg(panels.RulesSaveMsg{Scope: "  Global ", Content: "x"})
	if lvl, txt := lastSystem(t, app); lvl != "success" || txt != i18n.T("rules.saved_global", app.state.Language) {
		t.Fatalf("global toast = %q/%q", lvl, txt)
	}
	// 保存失败 → error toast
	engine.saveRulesErr = errors.New("disk full")
	app.handleRulesSaveMsg(panels.RulesSaveMsg{Content: "x"})
	if lvl, txt := lastSystem(t, app); lvl != "error" || !strings.Contains(txt, "disk full") {
		t.Fatalf("失败 toast = %q/%q", lvl, txt)
	}
}

func TestMemorySaveHandlerBranches(t *testing.T) {
	setTestHome(t)
	app, engine := newTestAppModelWithEngine(t)

	// 空内容 → warning，不落盘
	app.handleMemorySaveMsg(panels.MemorySaveMsg{Content: "   "})
	if lvl, _ := lastSystem(t, app); lvl != "warning" {
		t.Fatalf("空内容 = %q", lvl)
	}
	// 正常保存 → success
	app.handleMemorySaveMsg(panels.MemorySaveMsg{Content: "笔记", Scope: "project"})
	if lvl, _ := lastSystem(t, app); lvl != "success" {
		t.Fatalf("保存 = %q", lvl)
	}
	// 失败 → error
	engine.saveMemoryErr = errors.New("quota")
	app.handleMemorySaveMsg(panels.MemorySaveMsg{Content: "笔记"})
	if lvl, txt := lastSystem(t, app); lvl != "error" || !strings.Contains(txt, "quota") {
		t.Fatalf("失败 toast = %q/%q", lvl, txt)
	}
	// 直调 handleMemorySave（nil 防御）
	var nilApp *AppModel
	nilApp.handleMemorySave(panels.MemorySaveMsg{Content: "x"})
}

func TestContextHandlersBranches(t *testing.T) {
	setTestHome(t)
	app, engine := newTestAppModelWithEngine(t)

	// compact：空回执 → 通用文案
	app.handleContextCompactMsg(panels.ContextCompactMsg{})
	if lvl, txt := lastSystem(t, app); lvl != "success" || txt != i18n.T("context.compacted", app.state.Language) {
		t.Fatalf("compact 空 = %q/%q", lvl, txt)
	}
	// compact：自定义回执
	engine.compactMessage = "压缩了 42%"
	app.handleContextCompactMsg(panels.ContextCompactMsg{})
	if lvl, txt := lastSystem(t, app); lvl != "success" || txt != "压缩了 42%" {
		t.Fatalf("compact 回执 = %q/%q", lvl, txt)
	}
	// compact 失败
	engine.compactErr = errors.New("busy")
	app.handleContextCompactMsg(panels.ContextCompactMsg{})
	if lvl, txt := lastSystem(t, app); lvl != "error" || !strings.Contains(txt, "busy") {
		t.Fatalf("compact 失败 = %q/%q", lvl, txt)
	}

	// clear：成功 → 旧消息清空，只剩本条成功提示
	app.history = append(app.history, historyEntry{kind: "user", content: "x"})
	app.handleContextClearMsg(panels.ContextClearMsg{})
	if len(app.history) != 1 || app.history[0].kind != "system" {
		t.Fatalf("clear 后 history = %+v", app.history)
	}
	if lvl, _ := lastSystem(t, app); lvl != "success" {
		t.Fatalf("clear = %q", lvl)
	}
	// clear 失败
	engine.clearCtxErr = errors.New("denied")
	app.handleContextClearMsg(panels.ContextClearMsg{})
	if lvl, txt := lastSystem(t, app); lvl != "error" || !strings.Contains(txt, "denied") {
		t.Fatalf("clear 失败 = %q/%q", lvl, txt)
	}

	// export：成功 → toast 带路径
	app.handleContextExportMsg(panels.ContextExportMsg{})
	lvl, txt := lastSystem(t, app)
	if lvl != "success" || !strings.Contains(txt, "context-") {
		t.Fatalf("export = %q/%q", lvl, txt)
	}
	// export 失败
	engine.exportCtxErr = errors.New("readonly")
	app.handleContextExportMsg(panels.ContextExportMsg{})
	if lvl, txt := lastSystem(t, app); lvl != "error" || !strings.Contains(txt, "readonly") {
		t.Fatalf("export 失败 = %q/%q", lvl, txt)
	}
}

func TestCostSettingsLanguageTaskHandlers(t *testing.T) {
	setTestHome(t)
	app, _ := newTestAppModelWithEngine(t)

	// cost：clear / export(nyi) / refresh
	if next, _ := app.handleCostClearMsg(panels.CostClearMsg{}); next == nil {
		t.Fatal("cost clear")
	}
	if lvl, _ := lastSystem(t, app); lvl != "success" {
		t.Fatalf("cost clear = %q", lvl)
	}
	app.handleCostExportMsg(panels.CostExportMsg{})
	if lvl, _ := lastSystem(t, app); lvl != "info" {
		t.Fatalf("cost export = %q", lvl)
	}
	if next, _ := app.handleCostRefreshMsg(panels.CostRefreshMsg{}); next == nil {
		t.Fatal("cost refresh")
	}

	// settings：保存（复用既有参数形状）与重置
	disabled := false
	memoryOff := false
	if next, _ := app.handleSettingsSaveMsg(panels.SettingsSaveMsg{
		Settings:                &settings.Settings{Language: "zh"},
		GlobalPredictionEnabled: &disabled,
		MemoryInjectionEnabled:  &memoryOff,
	}); next == nil {
		t.Fatal("settings save")
	}
	app.handleSettingsResetMsg(panels.SettingsResetMsg{})
	if lvl, _ := lastSystem(t, app); lvl != "warning" {
		t.Fatalf("settings reset = %q", lvl)
	}

	// 语言切换：state 与 shell 同步（panel 广播无 panic）
	if next, _ := app.handleLanguageChangeMsg(panels.LanguageChangeMsg{Language: "en"}); next == nil {
		t.Fatal("language")
	}
	if app.state.Language != "en" {
		t.Fatalf("language = %q", app.state.Language)
	}

	// tasks：tick（panel 视图 + 其他视图）与 toast
	app.activeView = "panel"
	app.activePanel = "tasks"
	if next, _ := app.handleTasksTickMsg(panels.TasksTickMsg{}); next == nil {
		t.Fatal("tasks tick(panel)")
	}
	app.activeView = "shell"
	if next, _ := app.handleTasksTickMsg(panels.TasksTickMsg{}); next == nil {
		t.Fatal("tasks tick(shell)")
	}
	app.handleTaskToastMsg(panels.TaskToastMsg{Text: "后台任务完成"})
	if _, txt := lastSystem(t, app); txt != "后台任务完成" {
		t.Fatalf("task toast = %q", txt)
	}

	// LSP refresh：返回的 cmd 在 engine 模式下带错误（无 core client 进程）
	next, cmd := app.handleLSPRefreshMsg(panels.LSPRefreshMsg{})
	if next == nil || cmd == nil {
		t.Fatal("lsp refresh")
	}
	msg := cmd()
	done, ok := msg.(LSPReloadDoneMsg)
	if !ok {
		t.Fatalf("msg = %T", msg)
	}
	if done.Err == nil {
		t.Fatal("engine 模式 Reload 应报 core client 不可用")
	}

	// rules / memory refresh 包装
	if next, _ := app.handleRulesRefreshMsg(panels.RulesRefreshMsg{}); next == nil {
		t.Fatal("rules refresh")
	}
	if next, _ := app.handleMemoryRefreshMsg(panels.MemoryRefreshMsg{}); next == nil {
		t.Fatal("memory refresh")
	}
}

func TestCtxUsageTickAndPanelMsgPassthrough(t *testing.T) {
	setTestHome(t)
	app, _ := newTestAppModelWithEngine(t)

	// tick：context 面板 / memory 面板 / 非面板三分支，均回排下一拍
	for _, tc := range []struct {
		activeView  string
		activePanel string
	}{
		{"panel", "context"},
		{"panel", "memory"},
		{"shell", ""},
	} {
		app.activeView = tc.activeView
		app.activePanel = tc.activePanel
		next, cmd := app.handleCtxUsageTickMsg(ctxUsageTickMsg{})
		if next == nil || cmd == nil {
			t.Fatalf("tick %v: next=%v cmd=%v", tc, next, cmd)
		}
	}
	// tick 生产器（不 invoke：tea.Tick 会真实睡眠）
	if app.ctxUsageTick() == nil {
		t.Fatal("ctxUsageTick cmd nil")
	}

	// handlePanelMsg：nil 透传 nil；cmd 原样透传
	if app.handlePanelMsg(nil) != nil {
		t.Fatal("nil 应透传 nil")
	}
	sentinel := func() tea.Msg { return nil }
	if got := app.handlePanelMsg(sentinel); got == nil {
		t.Fatal("cmd 应透传")
	}
}
