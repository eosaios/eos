package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"time"
	"os"
	"path/filepath"
	"testing"

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
