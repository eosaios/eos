package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"testing"

	"github.com/eosaios/eos/internal/pkg/settings"
	"github.com/eosaios/eos/internal/ui/panels"
)

func TestRefreshPanelFunctions(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 各 refresh 不 panic
	app.refreshContextPanel()
	app.refreshMemoryPanel()
	app.refreshLSPPanel()
	app.refreshRulesPanel()
	app.refreshSettingsPanel()
	app.refreshCostPanel()
	app.updateContextUsageUI()
	app.updateBGTaskCountUI()

	// handlePanelMsg
	if cmd := app.handlePanelMsg(nil); cmd != nil {
		t.Fatal("nil cmd")
	}

	// rules/memory save
	app.handleRulesSave(panels.RulesSaveMsg{Scope: "project", Content: "x"})
	app.handleRulesSave(panels.RulesSaveMsg{Scope: "global", Content: "y"})
	app.handleMemorySave(panels.MemorySaveMsg{Content: "note", Scope: "project"})

	// 有历史时 updateContextUsageUI 显示
	app.history = append(app.history, historyEntry{kind: "user", content: "x"})
	app.updateContextUsageUI()
}

func TestHandleSettingsSaveAndCost(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	s := &settings.Settings{Language: "en", Theme: "light"}
	app.handleSettingsSave(s, boolPtr(true), boolPtr(true), "monokai")
	app.handleSettingsSave(nil, nil, nil, "")
}

func boolPtr(v bool) *bool { return &v }
