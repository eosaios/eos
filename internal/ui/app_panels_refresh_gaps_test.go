package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// app_panels 刷新函数缺口批测：refreshLSPPanel / refreshContextPanel /
// refreshRulesPanel / refreshMemoryPanel / refreshCostPanel /
// maybeRefreshGitSummary 的面板缺失 / adapter nil / API 错误 / 成功臂。

// 不可达清单：无。

import (
	"errors"
	"testing"

	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/pkg/coreapi"
)

func TestRefreshLSPPanelArms(t *testing.T) {
	setTestHome(t)

	// 面板缺失
	app, _ := newTestAppModelWithEngine(t)
	app.panels = map[string]panels.Panel{}
	app.refreshLSPPanel()

	// adapter nil
	app2, _ := newTestAppModelWithEngine(t)
	app2.panels["lsp"] = panels.NewLSPPanel(app2.styles, app2.state.Language)
	app2.adapter = nil
	app2.refreshLSPPanel()

	// LSPServers 错误
	app3, eng := newTestAppModelWithEngine(t)
	app3.panels["lsp"] = panels.NewLSPPanel(app3.styles, app3.state.Language)
	eng.lspServersErr = errors.New("lsp down")
	app3.refreshLSPPanel()

	// 成功：含 running 与 not_found
	app4, eng4 := newTestAppModelWithEngine(t)
	app4.panels["lsp"] = panels.NewLSPPanel(app4.styles, app4.state.Language)
	eng4.lspServers = []coreapi.LSPServer{
		{Language: "go", Command: "gopls", Status: "running"},
		{Language: "py", Command: "pylsp", Status: "not_found"},
	}
	app4.refreshLSPPanel()
}

func TestRefreshContextPanelArms(t *testing.T) {
	setTestHome(t)

	// 面板缺失
	app, _ := newTestAppModelWithEngine(t)
	app.panels = map[string]panels.Panel{}
	app.refreshContextPanel()

	// adapter nil
	app2, _ := newTestAppModelWithEngine(t)
	app2.panels["context"] = panels.NewContextPanel(app2.styles, app2.state.Language)
	app2.adapter = nil
	app2.refreshContextPanel()

	// Preview 错误
	app3, eng := newTestAppModelWithEngine(t)
	app3.panels["context"] = panels.NewContextPanel(app3.styles, app3.state.Language)
	eng.contextPreviewErr = errors.New("preview down")
	app3.refreshContextPanel()

	// Stats 错误
	app4, eng4 := newTestAppModelWithEngine(t)
	app4.panels["context"] = panels.NewContextPanel(app4.styles, app4.state.Language)
	eng4.contextPreview = []string{"user: hello", "assistant: hi"}
	eng4.contextStatsErr = errors.New("stats down")
	app4.refreshContextPanel()

	// 成功：含空行跳过
	app5, eng5 := newTestAppModelWithEngine(t)
	app5.panels["context"] = panels.NewContextPanel(app5.styles, app5.state.Language)
	eng5.contextPreview = []string{"user: hello", "   ", "assistant: hi"}
	eng5.contextStats = coreapi.ContextStats{Estimated: 42}
	app5.refreshContextPanel()
}

func TestRefreshRulesPanelArms(t *testing.T) {
	setTestHome(t)

	// 面板缺失
	app, _ := newTestAppModelWithEngine(t)
	app.panels = map[string]panels.Panel{}
	app.refreshRulesPanel()

	// adapter nil
	app2, _ := newTestAppModelWithEngine(t)
	app2.panels["rules"] = panels.NewRulesPanel(app2.styles, app2.state.Language)
	app2.adapter = nil
	app2.refreshRulesPanel()

	// RulesSnapshot 错误
	app3, eng := newTestAppModelWithEngine(t)
	app3.panels["rules"] = panels.NewRulesPanel(app3.styles, app3.state.Language)
	eng.rulesSnapshotErr = errors.New("rules down")
	app3.refreshRulesPanel()

	// 成功
	app4, eng4 := newTestAppModelWithEngine(t)
	app4.panels["rules"] = panels.NewRulesPanel(app4.styles, app4.state.Language)
	eng4.rulesSnapshot = coreapi.RulesSnapshot{
		ActiveRoot: "project",
		Documents: []coreapi.RuleDocument{
			{Scope: "project", Path: "AGENTS.md", Content: "# rules", Exists: true},
			{Scope: "global", Path: "global.md", Content: "", Exists: false},
		},
	}
	app4.refreshRulesPanel()
}

func TestRefreshMemoryPanelArms(t *testing.T) {
	setTestHome(t)

	// 面板缺失
	app, _ := newTestAppModelWithEngine(t)
	app.panels = map[string]panels.Panel{}
	app.refreshMemoryPanel()

	// adapter nil
	app2, _ := newTestAppModelWithEngine(t)
	app2.panels["memory"] = panels.NewMemoryPanel(app2.styles, app2.state.Language)
	app2.adapter = nil
	app2.refreshMemoryPanel()

	// Snapshot 错误
	app3, eng := newTestAppModelWithEngine(t)
	app3.panels["memory"] = panels.NewMemoryPanel(app3.styles, app3.state.Language)
	eng.memorySnapshotErr = errors.New("mem down")
	app3.refreshMemoryPanel()

	// 成功
	app4, eng4 := newTestAppModelWithEngine(t)
	app4.panels["memory"] = panels.NewMemoryPanel(app4.styles, app4.state.Language)
	eng4.memorySnapshot = coreapi.MemorySnapshot{
		Documents: []coreapi.MemoryDocument{
			{Scope: "project", Path: "mem.md", Content: "note", Exists: true},
			{Scope: "global", Path: "g.md", Content: "", Exists: false},
		},
	}
	app4.refreshMemoryPanel()
}

func TestRefreshCostPanelArms(t *testing.T) {
	setTestHome(t)

	// 面板缺失
	app, _ := newTestAppModelWithEngine(t)
	app.panels = map[string]panels.Panel{}
	app.refreshCostPanel()

	// CostItems 错误
	app2, eng := newTestAppModelWithEngine(t)
	app2.panels["cost"] = panels.NewCostPanel(app2.styles, app2.state.Language)
	eng.costItemsErr = errors.New("cost down")
	app2.refreshCostPanel()

	// UsageSummary 错误
	app3, eng3 := newTestAppModelWithEngine(t)
	app3.panels["cost"] = panels.NewCostPanel(app3.styles, app3.state.Language)
	eng3.costItems = []coreapi.CostItem{{Model: "m1"}}
	eng3.usageSummaryErr = errors.New("usage down")
	app3.refreshCostPanel()

	// 成功
	app4, eng4 := newTestAppModelWithEngine(t)
	app4.panels["cost"] = panels.NewCostPanel(app4.styles, app4.state.Language)
	eng4.costItems = []coreapi.CostItem{{Model: "m1"}}
	input, reply, total := 100, 50, 150
	eng4.usageSummary = coreapi.UsageSummary{Rounds: 3, InputTokens: &input, ReplyTokens: &reply, TotalTokens: &total}
	app4.refreshCostPanel()
}

func TestMaybeRefreshGitSummaryArms(t *testing.T) {
	setTestHome(t)
	// nil 安全
	var nilApp *AppModel
	nilApp.maybeRefreshGitSummary()

	app, eng := newTestAppModelWithEngine(t)
	// gitStatusErr 触发失败臂
	eng.gitStatusErr = errors.New("git down")
	app.maybeRefreshGitSummary()

	// 成功臂
	app2, eng2 := newTestAppModelWithEngine(t)
	eng2.gitStatus = []coreapi.GitChange{{Path: "a.go", State: "M"}}
	app2.maybeRefreshGitSummary()
}
