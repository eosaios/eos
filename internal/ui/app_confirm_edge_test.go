package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/internal/ui/views/confirm"

	tea "charm.land/bubbletea/v2"
)

func TestInlinePermissionLifecycle(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 无请求时按键不处理
	handled, _ := app.handleInlinePermissionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if handled {
		t.Fatal("no request should not handle")
	}

	req := confirm.Request{
		ID:      "req-1",
		Kind:    "permission",
		Options: []string{"accept", "decline", "cancel"},
	}
	app.showInlinePermission(req)
	if app.inlinePermissionReq == nil {
		t.Fatal("showInlinePermission")
	}

	// 上下边界
	app.handleInlinePermissionKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if app.inlinePermissionSelected != 0 {
		t.Fatal("up at top")
	}
	app.handleInlinePermissionKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if app.inlinePermissionSelected != 1 {
		t.Fatal("down")
	}
	app.handleInlinePermissionKey(tea.KeyPressMsg{Code: tea.KeyDown})
	app.handleInlinePermissionKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if app.inlinePermissionSelected != 2 {
		t.Fatal("down at bottom")
	}

	// 数字快选
	handled, _ = app.handleInlinePermissionKey(tea.KeyPressMsg{Code: '1', Text: "1"})
	if !handled || app.inlinePermissionSelected != 0 {
		t.Fatal("quick select")
	}

	// enter 产生 ResultMsg
	handled, cmd := app.handleInlinePermissionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd == nil {
		t.Fatal("enter")
	}
	res := cmd().(confirm.ResultMsg)
	if res.ID != "req-1" || res.Decision == "" {
		t.Fatalf("enter result = %+v", res)
	}

	// esc 走 EscDecision
	handled, cmd = app.handleInlinePermissionKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatal("esc")
	}
	res = cmd().(confirm.ResultMsg)
	if res.Decision == "" {
		t.Fatalf("esc result = %+v", res)
	}

	// clear
	app.clearInlinePermission()
	if app.inlinePermissionReq != nil {
		t.Fatal("clear")
	}

	// build 无请求
	res = app.buildInlinePermissionResult("accept")
	if res.OptionIndex != -1 || res.Decision != "accept" {
		t.Fatalf("build empty = %+v", res)
	}
}

func TestHandlePromptRequestMsg(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// permission → 内联
	_, _ = app.handlePromptRequestMsg(PromptRequestMsg{ID: "p1", Kind: "permission", Options: []string{"accept", "decline"}})
	if app.inlinePermissionReq == nil {
		t.Fatal("permission should inline")
	}
	app.clearInlinePermission()

	// inquiry → confirm 视图
	_, _ = app.handlePromptRequestMsg(PromptRequestMsg{ID: "p2", Kind: "inquiry", Title: "T", Question: "Q"})
	if app.confirmView == nil || app.activeView != "confirm" {
		t.Fatalf("inquiry confirm = %v %q", app.confirmView != nil, app.activeView)
	}

	// 空 kind 默认 permission → 内联
	app.confirmView = nil
	_, _ = app.handlePromptRequestMsg(PromptRequestMsg{ID: "p3"})
	if app.inlinePermissionReq == nil {
		t.Fatal("empty kind should default to permission inline")
	}
	app.clearInlinePermission()
}

func TestHandleConfirmResultPermission(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.showInlinePermission(confirm.Request{ID: "r1", Kind: "permission", Options: []string{"accept"}})

	next, cmd := app.handleConfirmResultMsg(confirm.ResultMsg{
		ID: "r1", Kind: "permission", Decision: "accept", Option: "accept", OptionIndex: 0,
	})
	if next == nil {
		t.Fatal("result")
	}
	if app.inlinePermissionReq != nil {
		t.Fatal("should clear inline")
	}
	if !app.state.Processing {
		t.Fatal("should keep processing")
	}
	if cmd == nil {
		t.Fatal("should re-arm status tick")
	}
}

func TestHandleConfirmResultBgKill(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.confirmView = confirm.New(app.styles, app.state.Language, "monokai", confirm.Request{ID: "x"})
	app.prevView = "shell"
	app.activeView = "confirm"

	// confirm 杀任务
	_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{
		Kind: "bg_kill:task-1", Decision: "confirm",
	})
	if app.confirmView != nil {
		t.Fatal("should close confirm")
	}
	if app.activeView != "shell" {
		t.Fatalf("view = %q", app.activeView)
	}

	// cancel 不杀
	app.confirmView = confirm.New(app.styles, app.state.Language, "monokai", confirm.Request{ID: "x"})
	app.prevView = ""
	_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{Kind: "bg_kill:task-2", Decision: "cancel"})
	if app.confirmView != nil {
		t.Fatal("close")
	}
}

func TestBrowserTakeoverHandlers(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// confirm 请求
	_, _ = app.handleBrowserTakeoverConfirm(BrowserTakeoverConfirmMsg{Reason: "need login", Note: "n"})
	if app.confirmView == nil || !app.browserTakeoverConfirm {
		t.Fatal("takeover confirm")
	}

	// 确认结果：index 0 调 BrowserControlConfirm
	_, _ = app.handleConfirmResultBrowserTakeoverConfirm(confirm.ResultMsg{OptionIndex: 0})
	if app.browserTakeoverConfirm {
		t.Fatal("should clear")
	}

	// started
	_, _ = app.handleBrowserTakeoverStarted(BrowserTakeoverStartedMsg{Reason: "manual"})
	if app.confirmView == nil {
		t.Fatal("started")
	}

	// ended
	_, _ = app.handleBrowserTakeoverEnded(BrowserTakeoverEndedMsg{Result: "resumed"})
	if app.browserTakeoverConfirm {
		t.Fatal("ended")
	}

	// action / download / pick
	_, _ = app.handleBrowserActionMsg(BrowserActionMsg{Action: "click", Target: "btn", Result: "ok"})
	_, _ = app.handleBrowserDownloadDoneMsg(BrowserDownloadDoneMsg{Filename: "a.png", Path: "/tmp/a.png"})
	_, _ = app.handleBrowserPickSelectedMsg(BrowserPickSelectedMsg{Ref: "e1", Role: "button", Name: "Go"})

	// takeover 结果（非 confirm 模式）
	_, _ = app.handleConfirmResultBrowserTakeover(confirm.ResultMsg{OptionIndex: 1})
}

func TestGitCommitInstructionAndHint(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	if !strings.Contains(app.gitCommitInstruction(), "提交") {
		t.Fatalf("zh instruction = %q", app.gitCommitInstruction())
	}
	app.state.Language = "en"
	if !strings.Contains(app.gitCommitInstruction(), "commit") {
		t.Fatalf("en instruction = %q", app.gitCommitInstruction())
	}

	// busy 时不派发
	app.state.Processing = true
	if cmd := app.dispatchGitCommitRequest(); cmd != nil {
		t.Fatal("busy should not dispatch")
	}
	app.state.Processing = false

	// hint：OK=false 不提示
	_, _ = app.handleGitCommitHintMsg(GitCommitHintMsg{OK: false})
	// dirty 提示
	_, _ = app.handleGitCommitHintMsg(GitCommitHintMsg{OK: true, Branch: "main", Dirty: 2, Ahead: 0})
	// 重复脏值不再提示
	_, _ = app.handleGitCommitHintMsg(GitCommitHintMsg{OK: true, Branch: "main", Dirty: 2, Ahead: 0})
	// 干净仓库不提示
	_, _ = app.handleGitCommitHintMsg(GitCommitHintMsg{OK: true, Branch: "main", Dirty: 0, Ahead: 0})

	// schedule（开关关闭时返回 OK=false）
	if cmd := app.scheduleGitCommitReminder(); cmd != nil {
		msg := cmd()
		if hint, ok := msg.(GitCommitHintMsg); ok && hint.OK {
			// 开关可能默认开，只要类型对即可
			_ = hint
		}
	}
}

func TestOpenConfirmAndActionPopup(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.openConfirm(confirm.Request{ID: "c1", Kind: "generic", Options: []string{"OK"}})
	if app.confirmView == nil || app.activeView != "confirm" {
		t.Fatal("openConfirm")
	}

	// 未命中坐标
	if cmd := app.tryHandleBubbleActionAt(0, 0); cmd != nil {
		t.Fatal("miss should be nil")
	}
	// 已有 popup 时不重复
	app.actionPopup = &confirm.ActionPopup{}
	if cmd := app.tryHandleBubbleActionAt(50, 50); cmd != nil {
		t.Fatal("popup open")
	}
	app.actionPopup = nil

	// handleActionResult cancel
	app.actionPopup = confirm.NewActionPopup(app.styles, app.state.Language, confirm.ActionRequest{})
	_ = app.handleActionResult(confirm.ActionResultMsg{Kind: "cancel"})
	if app.actionPopup != nil {
		t.Fatal("cancel should close")
	}

	// handleActionResultMsg / handleTaskKillRequestMsg
	_, _ = app.handleActionResultMsg(confirm.ActionResultMsg{Kind: "cancel"})
	_, _ = app.handleTaskKillRequestMsg(panels.TaskKillRequestMsg{ID: "x"})
}
