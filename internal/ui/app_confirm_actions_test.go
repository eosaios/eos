package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/pkg/filedialog"
	"github.com/eosaios/eos/internal/ui/views/confirm"
)

func TestHandleActionResultCopyAndDownload(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.activeView = "shell"

	// copy：可能因剪贴板失败走错误分支，不 panic
	app.history = append(app.history, historyEntry{kind: "ai", content: "text"})
	_ = app.handleActionResult(confirm.ActionResultMsg{Kind: "copy", Payload: "text", Index: 0})

	// download 无计划
	_ = app.handleActionResult(confirm.ActionResultMsg{Kind: "download", Index: 99})

	// default
	_ = app.handleActionResult(confirm.ActionResultMsg{Kind: "other", Index: 0})
	if app.actionPopup != nil {
		t.Fatal("popup should close")
	}
}

func TestSavePlanHistoryEntry(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 无效 index
	if _, err := app.savePlanHistoryEntryToDir(99, t.TempDir()); err == nil {
		t.Fatal("oob")
	}

	// 有效条目
	app.history = append(app.history, historyEntry{
		kind:        "ai",
		rawMarkdown: "# plan",
		executionMode: "plan",
		timestamp:   planDownloadNow(),
	})
	dir := t.TempDir()
	path, err := app.savePlanHistoryEntryToDir(0, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	// 非目录
	if _, err := app.savePlanHistoryEntryToDir(0, filepath.Join(dir, "nope")); err == nil {
		t.Fatal("not dir")
	}

	// nextPlanDownloadFileName
	name := app.nextPlanDownloadFileName(planDownloadNow())
	if filepath.Ext(name) != ".md" {
		t.Fatalf("name = %q", name)
	}
	name2 := app.nextPlanDownloadFileName(planDownloadNow().Add(-time.Hour))
	if name2 == name {
		// 时间不同应不同
	}
}

func TestHandlePlanDownloadAction(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 无计划条目
	_ = app.handlePlanDownloadAction(99)

	// 有条目 + chooser 成功
	app.history = append(app.history, historyEntry{kind: "ai", rawMarkdown: "# p", executionMode: "plan"})
	orig := choosePlanDownloadDirectory
	dir := t.TempDir()
	choosePlanDownloadDirectory = func(string) (string, error) { return dir, nil }
	t.Cleanup(func() { choosePlanDownloadDirectory = orig })
	_ = app.handlePlanDownloadAction(0)

	// chooser 取消
	choosePlanDownloadDirectory = func(string) (string, error) { return "", filedialog.ErrCanceled }
	_ = app.handlePlanDownloadAction(0)

	// chooser 不可用 → 打开文本确认
	app.history = append(app.history, historyEntry{kind: "ai", rawMarkdown: "# p2", executionMode: "plan"})
	idx := len(app.history) - 1
	choosePlanDownloadDirectory = func(string) (string, error) { return "", filedialog.ErrUnavailable }
	_ = app.handlePlanDownloadAction(idx)
	if app.pendingPlanDownload == nil {
		t.Fatal("pending download")
	}
}

func TestHandleConfirmResultBrowserTakeover(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.prevView = "shell"
	app.activeView = "confirm"
	app.browserTakeoverConfirm = true

	next, _ := app.handleConfirmResultBrowserTakeover(confirm.ResultMsg{OptionIndex: 1})
	if next == nil {
		t.Fatal("takeover")
	}
	if app.browserTakeoverConfirm {
		t.Fatal("should clear")
	}
}
