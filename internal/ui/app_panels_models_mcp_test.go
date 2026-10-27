package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"testing"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/internal/ui/views/setup"
	"github.com/eosaios/eos/pkg/coreapi"
)

func TestHandleModelSelectAndDelete(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.handleModelSelect(panels.ModelSelectMsg{Name: "default-model"})
	app.handleModelSelect(panels.ModelSelectMsg{Name: ""}) // 失败分支

	app.handleModelDelete(panels.ModelDeleteMsg{Name: "default-model"})
	app.handleModelDelete(panels.ModelDeleteMsg{Name: "missing"})

	app.handleModelSyncEnv()
}

func TestHandleModelPlanSelect(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.handleModelPlanSelect(panels.ModelPlanSelectMsg{EntryName: "e1", ModelID: "m1"})
	// 空 entry → 仍走一遍
	app.handleModelPlanSelect(panels.ModelPlanSelectMsg{})
}

func TestHandleModelFormCompleteModes(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 编辑模式
	app.handleModelFormComplete(setup.ModelFormCompleteMsg{
		EditMode: true,
		Config:   setup.SetupConfig{Name: "edited", APIBase: "https://x", Model: "m"},
	})
	if app.activeView != "shell" {
		t.Fatalf("view = %q", app.activeView)
	}

	// preset 模式
	app.handleModelFormComplete(setup.ModelFormCompleteMsg{
		Config: setup.SetupConfig{Name: "p1", Provider: "demo", PresetID: "preset-1", Model: "m"},
	})

	// custom provider
	app.handleModelFormComplete(setup.ModelFormCompleteMsg{
		Config: setup.SetupConfig{Name: "c1", Provider: "custom", Model: "m"},
	})

	// custom model + 空名
	app.handleModelFormComplete(setup.ModelFormCompleteMsg{
		Config: setup.SetupConfig{Provider: "demo", Model: "m"},
	})
}

func TestModelMsgWrappers(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	if next, _ := app.handleModelSelectMsg(panels.ModelSelectMsg{Name: "x"}); next == nil {
		t.Fatal("select msg")
	}
	if next, _ := app.handleModelPlanSelectMsg(panels.ModelPlanSelectMsg{EntryName: "a", ModelID: "b"}); next == nil {
		t.Fatal("plan select msg")
	}
	if next, _ := app.handleModelAddMsg(panels.ModelAddMsg{}); next == nil {
		t.Fatal("add msg")
	}
	if next, _ := app.handleModelEditMsg(panels.ModelEditMsg{Name: "default-model"}); next == nil {
		t.Fatal("edit msg")
	}
	if next, _ := app.handleModelDeleteMsg(panels.ModelDeleteMsg{Name: "x"}); next == nil {
		t.Fatal("delete msg")
	}
	if next, _ := app.handleModelSyncMsg(panels.ModelSyncMsg{}); next == nil {
		t.Fatal("sync msg")
	}
	if next, _ := app.handleModelRefreshMsg(panels.ModelRefreshMsg{}); next == nil {
		t.Fatal("refresh msg")
	}
	if next, _ := app.handleModelFormCompleteMsg(setup.ModelFormCompleteMsg{
		Config: setup.SetupConfig{Name: "z", Provider: "demo", Model: "m"},
	}); next == nil {
		t.Fatal("form complete msg")
	}

	// 未找到模型
	app.handleModelEdit(panels.ModelEditMsg{Name: "nope"})
}

func TestMCPToggleAndCRUD(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// toggle：列表为空 → 失败
	cmd := app.handleMCPToggle(panels.MCPToggleMsg{Name: "fs"})
	if cmd != nil {
		t.Fatal("empty list toggle")
	}

	app.handleMCPAdd()
	if app.activeView != "setup" || app.setupView == nil {
		t.Fatal("add opens editor")
	}

	app.handleMCPAddBrowser()
	if app.activeView != "setup" {
		t.Fatal("add browser")
	}

	app.handleMCPEdit(panels.MCPEditMsg{Name: "missing"})
	app.handleMCPDelete(panels.MCPDeleteMsg{Name: "missing"})
	app.handleMCPSave()
}

func TestMCPMsgWrappers(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	if next, _ := app.handleMCPToggleMsg(panels.MCPToggleMsg{Name: "x"}); next == nil {
		t.Fatal("toggle msg")
	}
	if next, _ := app.handleMCPAddMsg(panels.MCPAddMsg{}); next == nil {
		t.Fatal("add msg")
	}
	if next, _ := app.handleMCPAddBrowserMsg(panels.MCPAddBrowserMsg{}); next == nil {
		t.Fatal("add browser msg")
	}
	if next, _ := app.handleMCPEditMsg(panels.MCPEditMsg{Name: "x"}); next == nil {
		t.Fatal("edit msg")
	}
	if next, _ := app.handleMCPDeleteMsg(panels.MCPDeleteMsg{Name: "x"}); next == nil {
		t.Fatal("delete msg")
	}
	if next, _ := app.handleMCPSaveMsg(panels.MCPSaveMsg{}); next == nil {
		t.Fatal("save msg")
	}

	// config cancel
	app.handleMCPConfigCancelMsg(setup.MCPConfigCancelMsg{})
	// config submit（非法 JSON）
	app.handleMCPConfigSubmitMsg(setup.MCPConfigSubmitMsg{Text: "{bad"})
	// config submit（数组空）
	app.handleMCPConfigSubmitMsg(setup.MCPConfigSubmitMsg{Text: "[]", Edit: false})
	// config submit（对象）
	app.handleMCPConfigSubmitMsg(setup.MCPConfigSubmitMsg{
		Text: `{"name":"fs","type":"stdio","enabled":true}`,
		Edit: true, OriginalName: "fs",
	})
}

func TestRefreshModelsPanel(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.refreshModelsPanel() // 不 panic 即可

	_ = coreapi.ModelConfig{}
	_ = config.ModelEntry{}
}
