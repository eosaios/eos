package panels

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// tasks/models/context 面板余臂批测：Tasks 的 k/c/tick/mouse/viewing
// 分发族与 selectedID 边界，Models 的左右切换环绕与 Use/Model/Add 动作、
// 能力徽标 zh/en 形态。

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eosaios/eos/internal/config"
)

func TestTasksPanelUpdateArms(t *testing.T) {
	fp := &fakeTaskProvider{tasks: sampleTasks(), cleanupN: 3}
	p := NewTasksPanel(testStyles(), "zh", fp)
	p.SetSize(100, 30)

	// tick：刷新 + 重排 tick。
	p2, cmd := p.Update(TasksTickMsg{})
	if cmd == nil {
		t.Fatal("tick 应续排")
	}
	_ = p2.(*TasksPanel)

	// viewing 中的 tick/r/k/mouse 分发。
	p.openView("t1")
	if !p.viewing {
		t.Fatal("openView 未生效")
	}
	p.Update(TasksTickMsg{})
	p.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	p3, cmd := p.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if cmd == nil {
		t.Fatal("viewing kill 应产命令")
	}
	if msg := cmd(); msg.(TaskKillRequestMsg).ID != "t1" {
		t.Fatalf("kill id = %v", msg)
	}
	_ = p3
	p.Update(tea.MouseMsg(tea.MouseClickMsg{}))

	// 语言切换。
	p.Update(LanguageChangeMsg{Language: "en"})

	// 非 viewing 的 k。
	p.viewing = false
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if cmd == nil {
		t.Fatal("列表 kill 应产命令")
	}

	// c 清理成功（n>0 产 toast）。
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatal("清理有产物应产 toast")
	}

	// c 清理失败/零产物：无命令。
	fp2 := &fakeTaskProvider{tasks: sampleTasks(), cleanupErr: errBoom()}
	p2c := NewTasksPanel(testStyles(), "zh", fp2)
	p2c.SetSize(100, 30)
	if _, cmd := p2c.Update(tea.KeyPressMsg{Code: 'c', Text: "c"}); cmd != nil {
		t.Fatal("清理零产物应无命令")
	}

	// enter 开视图（选中行）。
	p3c := NewTasksPanel(testStyles(), "zh", &fakeTaskProvider{tasks: sampleTasks()})
	p3c.SetSize(100, 30)
	p3c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !p3c.viewing {
		t.Fatal("enter 应开视图")
	}

	// selectedID 越界（游标超表）。
	p4 := NewTasksPanel(testStyles(), "zh", &fakeTaskProvider{})
	p4.SetSize(100, 30)
	p4.refresh()
	if got := p4.selectedID(); got != "" {
		t.Fatalf("空表 selectedID = %q", got)
	}
	// viewing 中的普通键走 viewport。
	p4.viewing = true
	p4.Update(tea.KeyPressMsg{Code: 'x'})
	// 非键消息 + 非 viewing 走 table。
	p4.viewing = false
	p4.Update(tea.MouseMsg(tea.MouseClickMsg{}))
}

type boomError struct{}

func (boomError) Error() string { return "boom" }

func errBoom() error { return boomError{} }

func TestModelsPanelActionSwitchArms(t *testing.T) {
	p := newTestModelsPanel()
	p.SetSize(100, 30)

	// 左右环绕切换。
	p.actionIndex = 0
	p.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if p.actionIndex != len(p.actionOps)-1 {
		t.Fatalf("左环绕 = %d", p.actionIndex)
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if p.actionIndex != 0 {
		t.Fatalf("右环绕 = %d", p.actionIndex)
	}

	// 表格聚焦时左右先走 table 分支（无聚焦标记时直落 switch——已覆盖）。
	// 动作序列：Use（有模型时产 SelectMsg）→ Model（开选择器）→ Add（产 AddMsg）。
	p.SetModels([]config.ModelEntry{{Name: "gpt-x", APIBase: "https://api", Model: "m", APIKey: "k"}}, "gpt-x")
	if got := p.GetCurrentAction(); got != "Use" {
		t.Fatalf("初始 action = %q", got)
	}
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Use 有选中模型应产命令")
	}
	// 切到 Model。
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got := p.GetCurrentAction(); got != "Model" {
		t.Fatalf("第二 action = %q", got)
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	// 切到 Add。
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got := p.GetCurrentAction(); got != "Add" {
		t.Fatalf("第三 action = %q", got)
	}
	if _, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Fatal("Add 应产命令")
	}
}

func TestModelsPanelCapabilityBadgesBilingual(t *testing.T) {
	zh := newTestModelsPanel()
	// 渲染 zh 能力徽标（视/理/工）路径。
	zh.SetSize(100, 30)
	_ = zh.View()

	en := NewModelsPanel(testStyles(), "en")
	en.SetSize(100, 30)
	_ = en.View()
}
