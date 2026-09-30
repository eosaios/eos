// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

package ui

// app_confirm 第二轮补缺：handleConfirmResultMsg 的 kind 分发线（既有测试
// 直调子 handler）、审批回包/杀任务/信任工作区/添加工作区错误臂、
// prevView 回落 else 臂、planDownloadEntry 空 markdown、保存目录解析失败
// 与写盘失败、内联权限 UI 守卫与空选项回退、气泡动作命中开弹窗。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/views/confirm"
	tea "charm.land/bubbletea/v2"
)

func TestUpdateInlinePermissionUIGuards(t *testing.T) {
	setTestHome(t)
	var nilModel *AppModel
	nilModel.updateInlinePermissionUI() // nil 接收者：不 panic
	app := newTestAppModel(t)
	app.shell = nil
	app.updateInlinePermissionUI() // nil shell：不 panic
}

func TestBuildInlinePermissionResultEmptyOptionDecline(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.inlinePermissionReq = &confirm.Request{
		Options: []string{""}, // 选中项本身为空 → decision 兜底 decline
	}
	msg := app.buildInlinePermissionResult("")
	if msg.Decision != "decline" {
		t.Fatalf("decision = %q, want decline", msg.Decision)
	}
}

func TestHandleInlinePermissionKeyUpAndDefault(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.inlinePermissionReq = &confirm.Request{Options: []string{"allow_once", "deny"}}
	app.inlinePermissionSelected = 1
	handled, _ := app.handleInlinePermissionKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if !handled {
		t.Fatal("up should be handled")
	}
	if app.inlinePermissionSelected != 0 {
		t.Fatalf("selected = %d, want 0", app.inlinePermissionSelected)
	}
	// 非导航键：回落 false
	handled, _ = app.handleInlinePermissionKey(tea.KeyPressMsg{Text: "x"})
	if handled {
		t.Fatal("plain key should fall through")
	}
}

func TestTryHandleBubbleActionAtOpensPopup(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.actionHits = []bubbleActionHit{{y: 0, lines: 1, idx: 0, actions: []string{"copy"}, text: "hi"}}
	ox, oy := app.shell.ContentOrigin()
	// x/y 在内容区内且命中第 0 行
	if app.actionPopup != nil {
		t.Fatal("popup should start closed")
	}
	app.tryHandleBubbleActionAt(ox, oy)
	if app.actionPopup == nil {
		t.Fatal("hit should open action popup")
	}
}

func TestPlanDownloadEntryEmptyMarkdownAndSaveErrors(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// 空 rawMarkdown → planDownloadEntry false → handlePlanDownloadAction 走 unavailable
	app.history = append(app.history, historyEntry{kind: "ai", rawMarkdown: "   ", executionMode: "plan"})
	_ = app.handlePlanDownloadAction(0)
	if got := lastSystemText(app); !strings.Contains(got, "计划") {
		t.Fatalf("want unavailable warning, got %q", got)
	}

	// 保存目录解析失败（空目录字符串）
	app.history = append(app.history, historyEntry{kind: "ai", rawMarkdown: "# p", executionMode: "plan"})
	idx := len(app.history) - 1
	if _, err := app.savePlanHistoryEntryToDir(idx, "  "); err == nil {
		t.Fatal("blank dir should fail resolve")
	}
	// 目录不存在 → not_directory
	if _, err := app.savePlanHistoryEntryToDir(idx, filepath.Join(t.TempDir(), "ghost")); err == nil {
		t.Fatal("missing dir should fail stat")
	}
	// 写盘失败（注入 writePlanDownloadFile）
	dir := t.TempDir()
	origWrite := writePlanDownloadFile
	writePlanDownloadFile = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
	t.Cleanup(func() { writePlanDownloadFile = origWrite })
	if _, err := app.savePlanHistoryEntryToDir(idx, dir); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("want write error, got %v", err)
	}
}

func TestConfirmResultDispatchKinds(t *testing.T) {
	setTestHome(t)

	t.Run("permission respond error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.approvalRespondErr = errors.New("respond boom")
		_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{ID: "p1", Kind: "permission", Decision: "accept"})
		if got := lastSystemText(app); !strings.Contains(got, "respond boom") {
			t.Fatalf("want respond error surfaced, got %q", got)
		}
	})

	t.Run("generic kind respond error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.approvalRespondErr = errors.New("generic boom")
		// 非 inquiry 的泛化 kind 走 Approvals().Respond（inquiry 走 Inquiries）
		_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{ID: "p2", Kind: "other_kind", Decision: "accept"})
		if got := lastSystemText(app); !strings.Contains(got, "generic boom") {
			t.Fatalf("want generic respond error, got %q", got)
		}
	})

	t.Run("bg_kill confirm error and prev view", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.killTaskErr = errors.New("kill boom")
		app.prevView = "panel"
		_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{Kind: "bg_kill:t1", Decision: "confirm"})
		if got := lastSystemText(app); !strings.Contains(got, "kill boom") {
			t.Fatalf("want kill error, got %q", got)
		}
		if app.activeView != "panel" {
			t.Fatalf("activeView = %q, want prevView panel", app.activeView)
		}
	})

	t.Run("bg_kill confirm success", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{Kind: "bg_kill:t2", Decision: "confirm"})
		if got := lastSystemText(app); !strings.Contains(got, "t2") {
			t.Fatalf("want stopped toast naming task, got %q", got)
		}
	})

	t.Run("workspace_add reject restores view", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.prevView = ""
		_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{Kind: "workspace_add", Decision: "cancel"})
		if app.activeView != "shell" {
			t.Fatalf("activeView = %q, want shell", app.activeView)
		}
	})

	t.Run("workspace_add confirm add error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.workspaceAddErr = errors.New("add boom")
		_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{Kind: "workspace_add", Decision: "confirm", Text: t.TempDir()})
		if got := lastSystemText(app); !strings.Contains(got, "add boom") {
			t.Fatalf("want add error, got %q", got)
		}
	})

	t.Run("plan_download_path without pending request", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{Kind: "plan_download_path", Decision: "confirm"})
		if app.activeView != "shell" {
			t.Fatalf("activeView = %q, want shell", app.activeView)
		}
	})

	t.Run("workspace_trust cancel quits", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.trustPendingPath = "c:/some/ws"
		_, cmd := app.handleConfirmResultMsg(confirm.ResultMsg{Kind: "workspace_trust", Decision: "cancel"})
		if cmd == nil {
			t.Fatal("cancel should return quit cmd")
		}
	})
}

func TestWorkspaceTrustErrorArms(t *testing.T) {
	setTestHome(t)

	t.Run("local trust store failure", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		// 本地信任存储失败：工作区内 .eos 预置为普通文件 → 落盘失败
		//（信任路径是 <workspace>/.eos/workspace-trust.json，不走 HOME）
		ws := t.TempDir()
		if err := os.WriteFile(filepath.Join(ws, ".eos"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		app.trustPendingPath = ws
		app.trustPendingAction = ""
		_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{Kind: "workspace_trust", Decision: "confirm", OptionIndex: 0})
		if got := lastSystemText(app); !strings.Contains(got, "失败") && !strings.Contains(got, "failed") {
			t.Fatalf("want trust failure surfaced, got %q", got)
		}
	})

	t.Run("trust sync warning", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.trustWorkspaceErr = errors.New("sync boom")
		app.trustPendingPath = t.TempDir()
		app.trustPendingAction = ""
		_, _ = app.handleConfirmResultMsg(confirm.ResultMsg{Kind: "workspace_trust", Decision: "confirm", OptionIndex: 0})
		found := false
		for _, h := range app.history {
			if strings.Contains(h.content, "sync boom") {
				found = true
			}
		}
		if !found {
			t.Fatalf("want trust sync warning, last = %q", lastSystemText(app))
		}
		if app.activeView != "shell" {
			t.Fatalf("activeView = %q, want shell", app.activeView)
		}
	})
}
