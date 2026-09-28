package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/config"
)

func TestAllMsgTypesImplementMsg(t *testing.T) {
	var msgs []Msg = []Msg{
		BrowserTakeoverConfirmMsg{},
		BrowserTakeoverStartedMsg{},
		BrowserTakeoverEndedMsg{},
		BrowserActionMsg{},
		BrowserDownloadDoneMsg{},
		BrowserPickSelectedMsg{},
		WindowSizeMsg{},
		TickMsg{},
		KeyMsg{},
		MouseMsg{},
		AIRequestMsg{},
		AIResponseMsg{},
		ItemStartedMsg{},
		ItemDeltaMsg{},
		ItemCompletedMsg{},
		InvokeDoneMsg{},
		PredictionUpdateMsg{},
		ThinkingMsg{},
		ToolCallMsg{},
		ToolResultMsg{},
		AgentTaskMsg{},
		AgentFinalMsg{},
		ModeChangedMsg{},
		PromptRequestMsg{},
		PromptResultMsg{},
		PanelOpenMsg{},
		PanelCloseMsg{},
		PanelActionMsg{},
		SettingsUpdateMsg{},
		ErrorMsg{},
		VersionCheckMsg{},
	}
	for _, m := range msgs {
		m.msgType()
	}
}

func TestWorkspaceStateHelpers(t *testing.T) {
	setTestHome(t)
	path := t.TempDir()
	rememberKnownWorkspace(path, true)
	cfg, _ := config.Load()
	if !workspaceListed(&cfg, path) {
		t.Fatalf("known = %v", cfg.KnownWorkspaces)
	}
	forgetKnownWorkspace(path)
	cfg, _ = config.Load()
	if workspaceListed(&cfg, path) {
		t.Fatalf("forgot = %v", cfg.KnownWorkspaces)
	}

	// saveWorkspaceConfig 失败不 panic
	saveWorkspaceConfig(config.Config{}, filepath.Join(t.TempDir(), "no", "such", "deep", "x.json"))
	_ = os.TempDir()
	_ = time.Now()
}

func workspaceListed(cfg *config.Config, path string) bool {
	for _, w := range cfg.KnownWorkspaces {
		if w == path {
			return true
		}
	}
	return false
}

func TestSelectionHelpers(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	a := selectionCoord{line: 2, col: 5}
	b := selectionCoord{line: 1, col: 3}
	s, e := normalizeSelection(a, b)
	if s.line > e.line {
		t.Fatalf("normalize = %+v %+v", s, e)
	}
	if clampInt(5, 0, 3) != 3 || clampInt(-1, 0, 3) != 0 || clampInt(2, 0, 3) != 2 {
		t.Fatal("clampInt")
	}

	app.clearSelection()
	_ = app.selDistance(selectionCoord{}, selectionCoord{line: 1})
	app.applySelectionHighlight()
	app.copySelectedText()
}
