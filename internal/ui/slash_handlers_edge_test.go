package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"
)

func TestHandleThemeSlashShowAndSet(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 无参：显示当前主题
	app.handleThemeSlash(nil)

	// 设置主题
	app.handleThemeSlash([]string{"light"})
	if app.state.Theme != "light" {
		t.Fatalf("theme = %q", app.state.Theme)
	}
	// 大小写归一
	app.handleThemeSlash([]string{"DARK"})
	if app.state.Theme != "dark" {
		t.Fatalf("theme = %q", app.state.Theme)
	}
}

func TestHandlePlanStyleCustomAndUsage(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 无参：显示当前
	app.handlePlanStyleSlash(nil)

	// custom 裸参 → 用法提示
	app.handlePlanStyleSlash([]string{"custom"})

	// custom:<text>
	app.handlePlanStyleSlash([]string{"custom", "very", "detailed"})
	// detailed
	app.handlePlanStyleSlash([]string{"detailed"})
	// concise
	app.handlePlanStyleSlash([]string{"concise"})
}

func TestHandlePermissionsUsageOnUnknown(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 未知参数 → usage
	app.handlePermissionsSlash([]string{"nope"})
	// 无参：打印快照
	app.handlePermissionsSlash(nil)
	// 执行模式
	app.handlePermissionsSlash([]string{"auto"})
	// access 非法
	app.handlePermissionsSlash([]string{"access", "nope"})
}

func TestHandlePlanSlashShowsTodos(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// 无参：计划与待办
	app.handlePlanSlash(nil)
	// 带执行模式参数：转发 permissions
	app.handlePlanSlash([]string{"plan"})
}

func TestHandleRenameAndStatsAndShare(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// rename 缺参
	app.handleRenameSlash(nil)
	// rename
	app.handleRenameSlash([]string{"my", "title"})

	// stats
	app.handleStatsSlash()

	// share（会话存在时走 clipboard/落盘分支）
	app.handleShareSlash()
}

func TestHandleExportSlash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// 导出到临时路径（json）
	app.handleExportSlash([]string{"/tmp/eos-test-export.json"})
	// markdown 格式
	app.handleExportSlash([]string{"/tmp/eos-test-export.md", "md"})
}

func TestExecutionModeUsageLocalize(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	usage := app.executionModeUsage()
	if !strings.Contains(usage, "/permissions") {
		t.Fatalf("usage = %q", usage)
	}
	app.state.Language = "en"
	if en := app.executionModeUsage(); en == usage {
		t.Fatal("en usage should differ")
	}
}
