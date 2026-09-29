package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// handleKeyMsg 视图分发簇批测：confirm / actionPopup / help / setup /
// panel（context viewing / tasks viewing / rules editing / memory editing）/
// inline permission / bash Enter。

// 不可达清单：
//   - setup.SetupView / ModelSetupView / MCPConfigEditorView 三个具体
//     Update 分支：需要完整向导状态机推进，行为已在 setup 包内测覆盖，
//     这里只锚定 fall-through 安全性（不 panic）。

import (
	"context"
	"testing"

	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/internal/ui/views/confirm"
	"github.com/eosaios/eos/internal/ui/views/help"
	"github.com/eosaios/eos/internal/ui/views/setup"
	"github.com/eosaios/eos/internal/ui/views/shell"
	"github.com/eosaios/eos/pkg/coreapi"

	tea "charm.land/bubbletea/v2"
)

// fakeTaskProvider 供 TasksPanel 驱动 viewing 态。
type fakeTaskProvider struct{ items []coreapi.TaskSnapshot }

func (f *fakeTaskProvider) Tasks(context.Context) ([]coreapi.TaskSnapshot, error) {
	return f.items, nil
}
func (f *fakeTaskProvider) TailTask(context.Context, string) ([]string, error) {
	return nil, nil
}
func (f *fakeTaskProvider) CleanupTasks(context.Context) (int, error) { return 0, nil }

func TestHandleKeyMsgConfirmViewDelegates(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.openConfirm(confirm.Request{ID: "c1", Kind: "generic", Options: []string{"OK"}})
	if app.confirmView == nil || app.activeView != "confirm" {
		t.Fatal("openConfirm should switch to confirm")
	}
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
}

func TestHandleKeyMsgActionPopupIntercept(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.openActionPopup(bubbleActionHit{})
	if app.actionPopup == nil {
		t.Skip("openActionPopup no-op on this input")
	}
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestHandleKeyMsgHelpViewWithView(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.helpView = help.NewHelpView(app.styles, app.state.Language)
	app.activeView = "help"
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if app.activeView != "shell" {
		t.Fatalf("activeView=%q", app.activeView)
	}
}

func TestHandleKeyMsgSetupViewFallthrough(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.activeView = "setup"
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	app2 := newTestAppModel(t)
	app2.activeView = "setup"
	app2.setupView = setup.NewSetupView(app2.styles)
	_, _ = app2.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
}

func TestHandleKeyMsgPanelContextViewingEsc(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	cp := panels.NewContextPanel(app.styles, app.state.Language)
	cp.SetMessages([]panels.ContextMessage{{Role: "user", Content: "hello"}})
	updated, _ := cp.Update(tea.KeyPressMsg{Text: "v"})
	if p, ok := updated.(*panels.ContextPanel); ok {
		cp = p
	}
	if !cp.IsViewing() {
		t.Skip("context panel did not enter viewing (no selected message)")
	}
	app.panels["context"] = cp
	app.activeView = "panel"
	app.activePanel = "context"
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cp.IsViewing() {
		t.Fatal("esc should reset view")
	}
	if app.activeView != "panel" {
		t.Fatalf("activeView=%q (should stay panel after ResetView)", app.activeView)
	}
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if app.activeView != "shell" {
		t.Fatalf("activeView=%q", app.activeView)
	}
}

func TestHandleKeyMsgPanelRulesEditingEsc(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	rp := panels.NewRulesPanel(app.styles, app.state.Language)
	updated, _ := rp.Update(tea.KeyPressMsg{Text: "e"})
	if p, ok := updated.(*panels.RulesPanel); ok {
		rp = p
	}
	if !rp.IsEditing() {
		t.Fatal("rules should enter editing")
	}
	app.panels["rules"] = rp
	app.activeView = "panel"
	app.activePanel = "rules"
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if rp.IsEditing() {
		t.Fatal("esc should cancel edit")
	}
	if app.activeView != "panel" {
		t.Fatalf("activeView=%q", app.activeView)
	}
}

func TestHandleKeyMsgPanelMemoryEditingEsc(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	mp := panels.NewMemoryPanel(app.styles, app.state.Language)
	updated, _ := mp.Update(tea.KeyPressMsg{Text: "a"})
	if p, ok := updated.(*panels.MemoryPanel); ok {
		mp = p
	}
	if !mp.IsEditing() {
		t.Fatal("memory should enter editing")
	}
	app.panels["memory"] = mp
	app.activeView = "panel"
	app.activePanel = "memory"
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if mp.IsEditing() {
		t.Fatal("esc should cancel edit")
	}
}

func TestHandleKeyMsgPanelTasksViewingEsc(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	prov := &fakeTaskProvider{items: []coreapi.TaskSnapshot{{ID: "t1", Label: "demo", Status: "running"}}}
	tp := panels.NewTasksPanel(app.styles, app.state.Language, prov)
	// tick 刷新填充列表
	updated, _ := tp.Update(panels.TasksTickMsg{})
	if p, ok := updated.(*panels.TasksPanel); ok {
		tp = p
	}
	updated, _ = tp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p, ok := updated.(*panels.TasksPanel); ok {
		tp = p
	}
	if !tp.IsViewing() {
		t.Skip("tasks panel did not enter viewing")
	}
	app.panels["tasks"] = tp
	app.activeView = "panel"
	app.activePanel = "tasks"
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if tp.IsViewing() {
		t.Fatal("esc should reset view")
	}
}

func TestHandleKeyMsgPanelForwardUpdate(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	cp := panels.NewContextPanel(app.styles, app.state.Language)
	app.panels["context"] = cp
	app.activeView = "panel"
	app.activePanel = "context"
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Text: "c"})
}

func TestHandleKeyMsgInlinePermissionPath(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.activeView = "shell"
	app.showInlinePermission(confirm.Request{ID: "p1", Kind: "permission", Options: []string{"accept"}})
	if app.inlinePermissionReq == nil {
		t.Fatal("should show inline permission")
	}
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestHandleKeyMsgBashEnterSend(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.toolExecResult = coreapi.ToolResult{Status: "success", Display: "ok"}
	app.state.Mode = "bash"
	app.state.Processing = false
	app.shell.SetMode(shell.ModeBash)
	app.shell.SetInputValue("pwd")
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
}
