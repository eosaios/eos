package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"testing"

	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/internal/ui/views/setup"

	tea "charm.land/bubbletea/v2"
)

func TestOpenActionPopupAndMouse(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// openActionPopup
	app.openActionPopup(bubbleActionHit{
		y: 1, lines: 2, idx: 0,
		actions: []string{"copy", "download"},
		text:    "payload",
	})
	if app.actionPopup == nil {
		t.Fatal("popup")
	}

	// handleMouseMsg 需要真实 MouseMsg，此处只验证 openActionPopup
	// （MouseMsg 为接口，nil 会在 handleContentSelection 类型断言处 panic）
}

func TestUpdateDispatchVariousMsgs(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	msgs := []tea.Msg{
		ctxUsageTickMsg{},
		panels.LanguageChangeMsg{Language: "en"},
		GitSummaryMsg{Branch: "main", Dirty: 1},
		panels.TaskToastMsg{Text: "hi"},
		panels.LSPRefreshMsg{},
		panels.RulesRefreshMsg{},
		panels.CostRefreshMsg{},
		panels.ContextCompactMsg{},
		panels.ContextClearMsg{},
		panels.MemoryRefreshMsg{},
		panels.ModelRefreshMsg{},
		panels.MCPSaveMsg{},
		panels.VersionsDeleteAllMsg{},
		panels.ModelSelectMsg{Name: "x"},
		panels.ModelPlanSelectMsg{EntryName: "e", ModelID: "m"},
		panels.ModelDeleteMsg{Name: "x"},
		panels.ModelSyncMsg{},
		panels.ModelRefreshMsg{},
		panels.ModelAddMsg{},
		panels.MCPToggleMsg{Name: "x"},
		panels.MCPAddMsg{},
		panels.MCPAddBrowserMsg{},
		panels.MCPEditMsg{Name: "x"},
		panels.MCPDeleteMsg{Name: "x"},
		MCPReloadDoneMsg{},
		LSPReloadDoneMsg{},
		WorkspaceReloadDoneMsg{},
		setup.ModelFormCompleteMsg{},
	}
	for _, msg := range msgs {
		if next, _ := app.Update(msg); next == nil {
			t.Fatalf("nil update for %T", msg)
		}
	}
}
