package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"testing"

	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/internal/ui/views/confirm"
	"github.com/eosaios/eos/internal/ui/views/setup"
	"github.com/eosaios/eos/pkg/coreapi"
)

func TestHandleMCPToggleWithServers(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.mcpList = []coreapi.MCPServer{{Name: "fs", Enabled: true}}

	cmd := app.handleMCPToggle(panels.MCPToggleMsg{Name: "fs"})
	if cmd == nil {
		t.Fatal("toggle should return reload cmd")
	}
	msg := cmd()
	if _, ok := msg.(MCPReloadDoneMsg); !ok {
		t.Fatalf("msg = %T", msg)
	}

	// 未找到
	if cmd := app.handleMCPToggle(panels.MCPToggleMsg{Name: "nope"}); cmd != nil {
		t.Fatal("missing server")
	}
}

func TestHandleMCPEditWithEntry(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.mcpList = []coreapi.MCPServer{{Name: "fs", Type: "stdio", Enabled: true}}

	app.handleMCPEdit(panels.MCPEditMsg{Name: "fs"})
	if app.activeView != "setup" {
		t.Fatalf("view = %q", app.activeView)
	}

	// 未找到
	app.handleMCPEdit(panels.MCPEditMsg{Name: "nope"})
}

func TestHandleConfirmResultPlanDownloadPathFull(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 无 pending
	app.prevView = "shell"
	app.activeView = "confirm"
	if next, _ := app.handleConfirmResultPlanDownloadPath(confirm.ResultMsg{Decision: "confirm", Text: "/tmp"}); next == nil {
		t.Fatal("nil")
	}

	// 有 pending + confirm + 合法目录
	app.history = append(app.history, historyEntry{kind: "ai", rawMarkdown: "# p", executionMode: "plan"})
	app.pendingPlanDownload = &planDownloadRequest{HistoryIndex: 0}
	dir := t.TempDir()
	if next, _ := app.handleConfirmResultPlanDownloadPath(confirm.ResultMsg{Decision: "confirm", Text: dir}); next == nil {
		t.Fatal("confirm")
	}

	// 有 pending + confirm + 非法路径
	app.pendingPlanDownload = &planDownloadRequest{HistoryIndex: 0}
	if next, _ := app.handleConfirmResultPlanDownloadPath(confirm.ResultMsg{Decision: "confirm", Text: "/nope"}); next == nil {
		t.Fatal("bad path")
	}
}

func TestTryHandleBubbleActionAt(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 已有 popup
	app.actionPopup = confirm.NewActionPopup(app.styles, app.state.Language, confirm.ActionRequest{})
	if cmd := app.tryHandleBubbleActionAt(50, 50); cmd != nil {
		t.Fatal("popup open")
	}
	app.actionPopup = nil

	// 坐标越界
	if cmd := app.tryHandleBubbleActionAt(0, 0); cmd != nil {
		t.Fatal("oob")
	}
}

func TestNewAppModelFromCoreClient(t *testing.T) {
	setTestHome(t)
	eng := newTestEngine()
	// 通过 engine 构造（CoreClient 路径）
	app := NewAppModelFromCoreEngine(eng)
	if app == nil {
		t.Fatal("nil app")
	}
}

func TestHandleMCPDeleteAndRefresh(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.mcpList = []coreapi.MCPServer{{Name: "fs", Type: "stdio", Enabled: true}}

	// delete 命中
	cmd := app.handleMCPDelete(panels.MCPDeleteMsg{Name: "fs"})
	if cmd == nil {
		t.Fatal("delete cmd")
	}
	if _, ok := cmd().(MCPReloadDoneMsg); !ok {
		t.Fatal("reload msg")
	}

	// refreshMCPPanel
	app.refreshMCPPanel()
	app.handleMCPSave()

	// refreshLSPPanel
	app.refreshLSPPanel()
}

func TestHandleMCPConfigSubmitFormats(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 空文本
	app.handleMCPConfigSubmitMsg(setup.MCPConfigSubmitMsg{Text: "  "})

	// 旧版 JSON 标签格式
	app.handleMCPConfigSubmitMsg(setup.MCPConfigSubmitMsg{
		Text: `{"mcpServers":{"fs":{"command":"npx","args":["-y","fs"]}}}`,
	})

	// 新版数组
	app.handleMCPConfigSubmitMsg(setup.MCPConfigSubmitMsg{
		Text: `[{"name":"a","type":"stdio","command":"x","enabled":true}]`,
	})

	// 单对象
	app.handleMCPConfigSubmitMsg(setup.MCPConfigSubmitMsg{
		Text: `{"name":"b","type":"sse","url":"https://x"}`,
		Edit: true, OriginalName: "b",
	})
}

func TestHandleAIResponseAndDiffTheme(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	next, _ := app.handleAIResponseMsg(AIResponseMsg{Type: "final", Content: "answer", RID: "r"})
	if next == nil {
		t.Fatal("ai response")
	}
	if app.diffHighlightTheme() == "" {
		// 可能为空
		_ = app.diffHighlightTheme()
	}
}
