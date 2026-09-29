package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// app_confirm / app_messages / app_panels 缺口批测：计划下载错误臂 /
// bubble 命中边界 / diff 主题 / agent delta 接受判定 / AI 响应分支 /
// 面板刷新错误臂 / ctxUsage tick。

// 不可达清单：无。

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eosaios/eos/internal/ui/panels"
)

func withStubPlanDir(t *testing.T, dir string, err error) {
	t.Helper()
	orig := choosePlanDownloadDirectory
	choosePlanDownloadDirectory = func(string) (string, error) { return dir, err }
	t.Cleanup(func() { choosePlanDownloadDirectory = orig })
}

func planHistoryApp(t *testing.T) (*AppModel, int) {
	t.Helper()
	app := newTestAppModel(t)
	app.history = append(app.history, historyEntry{kind: "ai", rawMarkdown: "# plan body", executionMode: "plan"})
	return app, len(app.history) - 1
}

func TestHandlePlanDownloadErrorAndNonPlan(t *testing.T) {
	setTestHome(t)

	// 非 plan 条目 → unavailable 提示
	app := newTestAppModel(t)
	app.history = append(app.history, historyEntry{kind: "user", content: "hi"})
	runCmd(app.handlePlanDownloadAction(0))

	// chooser 其它错误 → failed 提示
	app2, idx := planHistoryApp(t)
	withStubPlanDir(t, "", errors.New("disk full"))
	runCmd(app2.handlePlanDownloadAction(idx))

	// 保存成功：chooser 返回可写目录
	app3, idx3 := planHistoryApp(t)
	dir := t.TempDir()
	withStubPlanDir(t, dir, nil)
	runCmd(app3.handlePlanDownloadAction(idx3))

	// 保存失败：目标是只读/文件路径
	app4, idx4 := planHistoryApp(t)
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	withStubPlanDir(t, blocker, nil)
	runCmd(app4.handlePlanDownloadAction(idx4))
}

func TestTryHandleBubbleActionAtBounds(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 已有 popup → nil
	app.openActionPopup(bubbleActionHit{y: 0, lines: 1, actions: []string{"copy"}})
	if app.actionPopup == nil {
		t.Skip("popup not opened")
	}
	if cmd := app.tryHandleBubbleActionAt(0, 0); cmd != nil {
		t.Fatal("popup open should nil")
	}
	app.actionPopup = nil

	ox, oy := app.shell.ContentOrigin()
	// 坐标在 origin 左/上
	if cmd := app.tryHandleBubbleActionAt(ox-1, oy); cmd != nil {
		t.Fatal("left of origin")
	}
	if cmd := app.tryHandleBubbleActionAt(ox, oy-1); cmd != nil {
		t.Fatal("above origin")
	}
	// y 超出内容高
	if cmd := app.tryHandleBubbleActionAt(ox, oy+app.shell.ContentHeight()+5); cmd != nil {
		t.Fatal("below content")
	}
	// 无 actionHits
	app.actionHits = nil
	if cmd := app.tryHandleBubbleActionAt(ox, oy); cmd != nil {
		t.Fatal("no hits")
	}
	// hit 不命中当前行
	app.actionHits = []bubbleActionHit{{y: 9999, lines: 1, actions: []string{"copy"}}}
	if cmd := app.tryHandleBubbleActionAt(ox, oy); cmd != nil {
		t.Fatal("miss hit")
	}
}

func TestDiffHighlightThemeArms(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.diffTheme = ""
	if app.diffHighlightTheme() == "" {
		t.Fatal("empty should fallback default")
	}
	app.diffTheme = "  monokai  "
	if app.diffHighlightTheme() != "monokai" {
		t.Fatalf("theme=%q", app.diffHighlightTheme())
	}
}

func TestAcceptsAgentTextDeltaArms(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	if app.acceptsAgentTextDelta("x") {
		t.Fatal("not processing")
	}
	app.state.Processing = true
	if app.acceptsAgentTextDelta("x") {
		t.Fatal("no active item")
	}
	app.activeItemID = "item-1"
	if !app.acceptsAgentTextDelta("") {
		t.Fatal("empty itemID should accept")
	}
	if !app.acceptsAgentTextDelta("item-1") {
		t.Fatal("match should accept")
	}
	if app.acceptsAgentTextDelta("other") {
		t.Fatal("mismatch should reject")
	}
}

func TestHandleAIResponseBranches(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// delta 且不在处理中 → 早退
	_, _ = app.handleAIResponseMsg(AIResponseMsg{Type: "delta", Content: "x"})

	// delegated + delta → 早退
	app.state.Processing = true
	app.delegatedThisRound = true
	_, _ = app.handleAIResponseMsg(AIResponseMsg{Type: "delta", Content: "x"})

	// final
	app.delegatedThisRound = false
	_, _ = app.handleAIResponseMsg(AIResponseMsg{Type: "final", Content: "done", RID: "r1"})

	// error
	_, _ = app.handleAIResponseMsg(AIResponseMsg{Type: "error", Content: "boom", RID: "r2"})

	// 未知 type
	_, _ = app.handleAIResponseMsg(AIResponseMsg{Type: "weird", Content: "z"})
}

func TestRefreshContextPanelErrorArms(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	// ContextPreview 失败臂
	eng.loadMessagesErr = errors.New("preview down")
	app.refreshContextPanel()

	// 面板缺失
	app2, _ := newTestAppModelWithEngine(t)
	app2.panels = map[string]panels.Panel{}
	app2.refreshContextPanel()
}

func TestUpdateContextUsageAndBGTaskArms(t *testing.T) {
	setTestHome(t)
	// nil 安全
	empty := &AppModel{}
	empty.updateContextUsageUI()
	empty.updateBGTaskCountUI()

	app := newTestAppModel(t)
	app.history = append(app.history, historyEntry{kind: "user", content: "x"})
	app.updateContextUsageUI()
	app.updateBGTaskCountUI()

	// ctxUsageTick 返回 tick cmd
	if app.ctxUsageTick() == nil {
		t.Fatal("tick cmd")
	}
}

func TestHandleTaskKillRequestArms(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// 空 id 早退
	_, _ = app.handleTaskKillRequestMsg(panels.TaskKillRequestMsg{ID: "  "})

	// 有 id → 打开 bg_kill 确认框
	app2 := newTestAppModel(t)
	_, _ = app2.handleTaskKillRequestMsg(panels.TaskKillRequestMsg{ID: "t1"})
	if app2.confirmView == nil || app2.activeView != "confirm" {
		t.Fatalf("kill confirm: view=%q", app2.activeView)
	}
}
