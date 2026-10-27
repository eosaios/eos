package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"errors"
	"testing"

	"github.com/eosaios/eos/internal/ui/adapter"
	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/internal/ui/views/confirm"
	"github.com/eosaios/eos/internal/ui/views/setup"

	tea "charm.land/bubbletea/v2"
)

func TestUpdateDispatchFull(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	msgs := []tea.Msg{
		ctxUsageTickMsg{},
		GitSummaryMsg{Branch: "main", Dirty: 1, Ahead: 1},
		GitCommitHintMsg{OK: true, Branch: "main", Dirty: 1},
		tea.WindowSizeMsg{Width: 120, Height: 40},
		adapter.RuntimeEvent{Type: "item.started", Data: map[string]any{"item": map[string]any{"kind": "agent_message", "id": "i1"}}},
		predictionDebounceMsg{Seq: 0, Draft: ""},
		PredictionUpdateMsg{Seq: 0, Draft: ""},
		AIResponseMsg{Type: "final", Content: "hi"},
		ItemStartedMsg{ItemID: "i1", ItemType: "agent_message"},
		ItemDeltaMsg{ItemID: "i1", Delta: "x"},
		ItemCompletedMsg{ItemID: "i1", Text: "done"},
		InvokeDoneMsg{Content: "c"},
		ThinkingMsg{Content: "t", Done: true},
		ToolCallMsg{ID: "t1", Name: "Bash"},
		ToolResultMsg{ID: "t1", Status: "success", Output: "ok"},
		ModeChangedMsg{Mode: "plan"},
		AgentTaskMsg{AgentName: "s", Task: "t", Event: "dispatch"},
		AgentFinalMsg{AgentName: "s", Content: "c", Event: "result"},
		ErrorMsg{Err: errors.New("e")},
		clearCopiedMsg{idx: 0},
		BrowserTakeoverConfirmMsg{Reason: "r"},
		BrowserTakeoverStartedMsg{Reason: "r"},
		BrowserTakeoverEndedMsg{Result: "ok"},
		BrowserActionMsg{Action: "click"},
		BrowserDownloadDoneMsg{Filename: "f", Path: "/p"},
		BrowserPickSelectedMsg{Ref: "e1"},
		PromptRequestMsg{ID: "p", Kind: "inquiry"},
		confirm.ResultMsg{ID: "x", Kind: "generic", Decision: "ok"},
		confirm.ActionResultMsg{Kind: "cancel"},
		setup.SetupCompleteMsg{},
		setup.SetupCancelMsg{},
		setup.ModelFormCompleteMsg{},
		setup.MCPConfigCancelMsg{},
		setup.MCPConfigSubmitMsg{Text: "{}"},
		panels.LanguageChangeMsg{Language: "en"},
		panels.TaskToastMsg{Text: "t"},
		panels.LSPRefreshMsg{},
		panels.RulesRefreshMsg{},
		panels.CostRefreshMsg{},
		panels.ContextCompactMsg{},
		panels.ContextClearMsg{},
		panels.MemoryRefreshMsg{},
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
		panels.MCPSaveMsg{},
		panels.VersionsLoadMsg{FilePath: "a"},
		panels.VersionsRollbackMsg{FilePath: "a", Timestamp: "t"},
		panels.VersionsDeleteMsg{FilePath: "a", Timestamp: "t"},
		panels.VersionsDeleteFileMsg{FilePath: "a"},
		panels.VersionsDeleteAllMsg{},
		panels.WorkspaceSelectMsg{Path: "/tmp"},
		panels.WorkspaceDeleteMsg{Path: "/tmp"},
		panels.WorkspaceAddMsg{},
		MCPReloadDoneMsg{},
		LSPReloadDoneMsg{},
		WorkspaceReloadDoneMsg{},
	}
	for _, msg := range msgs {
		if next, _ := app.Update(msg); next == nil {
			t.Fatalf("nil update for %T", msg)
		}
	}
}
