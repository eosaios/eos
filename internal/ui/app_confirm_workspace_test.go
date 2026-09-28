package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"os"
	"testing"

	"github.com/eosaios/eos/internal/ui/views/confirm"
)

func TestHandleConfirmResultWorkspaceTrust(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// switchWorkspaceTrusted 会 os.Chdir 进工作区（产品预期：TUI 进程
	// 跟随工作区）；Windows 下进程 cwd 占用目录句柄会让 TempDir 清理
	// 失败，测试结束前恢复 cwd。
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// cancel / 空 path → Quit
	next, cmd := app.handleConfirmResultWorkspaceTrust(confirm.ResultMsg{Decision: "cancel"})
	if next == nil {
		t.Fatal("cancel")
	}
	_ = cmd

	app.trustPendingPath = "/tmp/ws"
	app.trustPendingAction = "switch"
	// OptionIndex != 0 → Quit
	_, _ = app.handleConfirmResultWorkspaceTrust(confirm.ResultMsg{OptionIndex: 1})

	// 确认信任 + switch
	app.trustPendingPath = t.TempDir()
	app.trustPendingAction = "switch"
	app.prevView = "shell"
	app.activeView = "confirm"
	if next, _ := app.handleConfirmResultWorkspaceTrust(confirm.ResultMsg{OptionIndex: 0}); next == nil {
		t.Fatal("trust")
	}

	// 确认信任 + init
	app.trustPendingPath = t.TempDir()
	app.trustPendingAction = "init"
	app.prevView = ""
	app.activeView = "confirm"
	if next, _ := app.handleConfirmResultWorkspaceTrust(confirm.ResultMsg{OptionIndex: 0}); next == nil {
		t.Fatal("init")
	}
}

func TestHandleConfirmResultWorkspaceAdd(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 非 confirm → 关闭
	app.prevView = "shell"
	app.activeView = "confirm"
	if next, _ := app.handleConfirmResultWorkspaceAdd(confirm.ResultMsg{Decision: "cancel"}); next == nil {
		t.Fatal("cancel")
	}

	// confirm + 非法路径
	app.confirmView = confirm.New(app.styles, app.state.Language, "monokai", confirm.Request{ID: "x"})
	if next, _ := app.handleConfirmResultWorkspaceAdd(confirm.ResultMsg{Decision: "confirm", Text: "  "}); next == nil {
		t.Fatal("empty path")
	}

	// confirm + 不存在目录
	if next, _ := app.handleConfirmResultWorkspaceAdd(confirm.ResultMsg{
		Decision: "confirm", Text: t.TempDir() + "/nope",
	}); next == nil {
		t.Fatal("missing dir")
	}

	// confirm + 合法目录
	if next, _ := app.handleConfirmResultWorkspaceAdd(confirm.ResultMsg{
		Decision: "confirm", Text: t.TempDir(),
	}); next == nil {
		t.Fatal("add ws")
	}
}

func TestHandleConfirmResultPlanDownloadPath(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.prevView = "shell"
	app.activeView = "confirm"

	// 非 confirm
	if next, _ := app.handleConfirmResultPlanDownloadPath(confirm.ResultMsg{Decision: "cancel"}); next == nil {
		t.Fatal("cancel")
	}
	// confirm 无历史
	app.confirmView = confirm.New(app.styles, app.state.Language, "monokai", confirm.Request{ID: "x"})
	if next, _ := app.handleConfirmResultPlanDownloadPath(confirm.ResultMsg{Decision: "confirm", Text: t.TempDir()}); next == nil {
		t.Fatal("download")
	}
}

func TestSetActiveCancelAndMarkInflight(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.setActiveCancel(func() {})
	app.markInflightToolsCanceled("canceled")
	app.cancelActiveRequest()
	// 二次 cancel 返回 false
	if app.cancelActiveRequest() {
		t.Fatal("second cancel")
	}
}
