package panels

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/pkg/settings"
	"github.com/eosaios/eos/pkg/coreapi"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func boolPtr(v bool) *bool { return &v }

// ---------- Models plan picker ----------

func TestModelsPlanModelsHelpers(t *testing.T) {
	p := newTestModelsPanel()
	p.SetPlanModels(map[string][]coreapi.PlanModel{
		"plan-preset": {
			{ModelID: "m-a", Label: "A", ContextWindow: 32000, SupportsVision: boolPtr(true), SupportsTools: boolPtr(true)},
			{ModelID: "m-b", Label: "B"},
		},
	})

	if p.planModelsFor(nil) != nil {
		t.Fatal("nil entry should yield nil")
	}
	entry := &config.ModelEntry{Name: "x", PresetID: "plan-preset"}
	got := p.planModelsFor(entry)
	if len(got) != 2 {
		t.Fatalf("planModelsFor = %d", len(got))
	}
	if p.planModelsFor(&config.ModelEntry{PresetID: "missing"}) != nil {
		t.Fatal("missing preset should be nil")
	}

	// 能力标签
	zh := p.planCapabilityLabel(got[0])
	if zh != "视/工" {
		t.Fatalf("caps zh = %q", zh)
	}
	if p.planCapabilityLabel(got[1]) != "-" {
		t.Fatalf("empty caps = %q", p.planCapabilityLabel(got[1]))
	}
	p.language = "en"
	if p.planCapabilityLabel(got[0]) != "V/T" {
		t.Fatalf("caps en = %q", p.planCapabilityLabel(got[0]))
	}

	// derefPlanCap
	if derefPlanCap(nil) || !derefPlanCap(boolPtr(true)) || derefPlanCap(boolPtr(false)) {
		t.Fatal("derefPlanCap")
	}

	// reloadPlanModels：条目不存在
	if p.reloadPlanModels("nope") {
		t.Fatal("missing entry should return false")
	}
}

func TestModelsPlanPickerFlow(t *testing.T) {
	p := newTestModelsPanel()
	p.SetSize(100, 40)
	p.SetModels([]config.ModelEntry{
		{Name: "plan-entry", PresetID: "plan-preset", Model: "m-a"},
		{Name: "plain", Model: "x"},
	}, "plan-entry")
	p.SetPlanModels(map[string][]coreapi.PlanModel{
		"plan-preset": {
			{ModelID: "m-a", Label: "A", ContextWindow: 32000, SupportsVision: boolPtr(true)},
			{ModelID: "m-b", Label: "B", ContextWindow: 0},
		},
	})

	// openPlanPicker：选中条目有 ≥2 套餐模型才进入
	p.openPlanPicker()
	if p.planPickerEntry == nil {
		t.Fatal("should enter plan picker")
	}
	view := p.View()
	if !strings.Contains(view, "plan-entry") || !strings.Contains(view, "m-a") {
		t.Fatalf("plan picker view:\n%s", view)
	}

	// 表格导航（非 enter/esc）
	p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if p.planTable.Cursor() != 1 {
		t.Fatalf("cursor = %d", p.planTable.Cursor())
	}

	// enter 确认
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	sel := cmd().(ModelPlanSelectMsg)
	if sel.EntryName != "plan-entry" || sel.ModelID != "m-b" {
		t.Fatalf("plan select = %+v", sel)
	}
	if p.planPickerEntry != nil {
		t.Fatal("picker should close after enter")
	}

	// esc 取消
	p.openPlanPicker()
	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if p.planPickerEntry != nil {
		t.Fatal("esc should close picker")
	}

	// 单一/无套餐模型不进入
	p.table.SetCursor(1)
	p.openPlanPicker()
	if p.planPickerEntry != nil {
		t.Fatal("plain model should not open picker")
	}

	// 快捷键 m 进入
	p.table.SetCursor(0)
	p.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if p.planPickerEntry == nil {
		t.Fatal("m should open picker")
	}

	// SetPlanModels：条目从列表消失时退出子模式
	p.openPlanPicker()
	p.SetModels([]config.ModelEntry{{Name: "plain", Model: "x"}}, "plain")
	p.SetPlanModels(map[string][]coreapi.PlanModel{})
	if p.planPickerEntry != nil {
		t.Fatal("SetPlanModels should close picker when entry removed from list")
	}

	// 越界 enter 不发消息（需处于 picker 子模式）
	p.SetModels([]config.ModelEntry{
		{Name: "plan-entry", PresetID: "plan-preset", Model: "m-a"},
	}, "plan-entry")
	p.SetPlanModels(map[string][]coreapi.PlanModel{
		"plan-preset": {
			{ModelID: "m-a", Label: "A"},
			{ModelID: "m-b", Label: "B"},
		},
	})
	p.openPlanPicker()
	if p.planPickerEntry == nil {
		t.Fatal("should reopen picker")
	}
	p.planTable.SetRows(nil) // 清空行使 cursor 越界
	if _, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatalf("oob enter = %T", cmd())
	}
}

func TestModelsPanelMoreAccessors(t *testing.T) {
	p := newTestModelsPanel()
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}
	p.SetSize(80, 30)
	p.SetModels([]config.ModelEntry{{Name: "a", Model: "m"}}, "b")
	p.SetCurrentModel("a")
	p.Refresh()
	if sel := p.GetSelectedModel(); sel == nil || sel.Name != "a" {
		t.Fatalf("selected = %+v", sel)
	}
	// 越界：空列表时选中为 nil
	p.SetModels(nil, "")
	if p.GetSelectedModel() != nil {
		t.Fatal("empty models selection should be nil")
	}
	p.actionIndex = 99
	if p.GetCurrentAction() != "" {
		t.Fatal("oob action")
	}

	// 语言切换
	out, _ := p.Update(LanguageChangeMsg{Language: "en"})
	if mp := out.(*ModelsPanel); mp.language != "en" {
		t.Fatalf("lang = %q", mp.language)
	}
}

// ---------- Settings edit / actions ----------

func newTestSettingsPanel(t *testing.T) *SettingsPanel {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".eos", "settings.json")
	p := NewSettingsPanel(testStyles(), settings.NewManager(path), "zh")
	p.SetSize(100, 40)
	p.LoadSettings()
	return p
}

func TestSettingsPanelSettersAndInit(t *testing.T) {
	p := newTestSettingsPanel(t)
	if cmd := p.Init(); cmd != nil {
		t.Fatal("Init should be nil")
	}
	s := defaultPanelSettings()
	s.Language = "en"
	p.SetSettings(s)
	p.SetSettings(nil) // nil 不覆盖
	p.SetGlobalPredictionEnabled(true)
	p.SetMemoryInjectionEnabled(true)
	if !p.globalPredictionEnabled || !p.memoryInjectionEnabled {
		t.Fatal("setters failed")
	}
	if p.settings.Language != "en" {
		t.Fatalf("settings lang = %q", p.settings.Language)
	}
	if got := p.GetCurrentAction(); got != "Edit" {
		t.Fatalf("action = %q", got)
	}
	key, val := p.GetSelectedSetting()
	if key != "AutoContext" || val != "true" {
		t.Fatalf("selected = %q/%q", key, val)
	}
}

func TestSettingsPanelHandleActions(t *testing.T) {
	p := newTestSettingsPanel(t)

	// Save
	p.actionIndex = 1
	_, cmd := p.handleAction()
	save := cmd().(SettingsSaveMsg)
	if save.Settings == nil || save.GlobalPredictionEnabled == nil {
		t.Fatalf("save = %+v", save)
	}

	// Reset
	p.actionIndex = 2
	_, cmd = p.handleAction()
	if _, ok := cmd().(SettingsResetMsg); !ok {
		t.Fatalf("reset = %T", cmd())
	}

	// Refresh（无 cmd）
	p.actionIndex = 3
	if _, cmd = p.handleAction(); cmd != nil {
		t.Fatalf("refresh cmd = %T", cmd())
	}

	// 快捷键 s/r
	_, cmd = p.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if _, ok := cmd().(SettingsSaveMsg); !ok {
		t.Fatalf("s = %T", cmd())
	}
	_, cmd = p.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if _, ok := cmd().(SettingsResetMsg); !ok {
		t.Fatalf("r = %T", cmd())
	}
	// f5 刷新
	p.Update(tea.KeyPressMsg{Code: tea.KeyF5})

	// 左右环绕
	p.actionIndex = 0
	p.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if p.GetCurrentAction() != "Refresh" {
		t.Fatalf("left wrap = %q", p.GetCurrentAction())
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if p.GetCurrentAction() != "Edit" {
		t.Fatalf("right wrap = %q", p.GetCurrentAction())
	}
}

func TestSettingsPanelChoiceEditMode(t *testing.T) {
	p := newTestSettingsPanel(t)

	// Language 行（index 5）是选择式
	p.table.SetCursor(5)
	key, _ := p.GetSelectedSetting()
	if key != "Language" {
		t.Fatalf("key = %q", key)
	}
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd != nil {
		t.Fatalf("choice edit should not blink: %T", cmd)
	}
	if !p.editMode || p.editChoices == nil {
		t.Fatal("should enter choice edit")
	}
	if p.editKeyLabel() == "" {
		t.Fatal("editKeyLabel empty")
	}
	view := p.View()
	if !strings.Contains(view, "settings.edit") && !strings.Contains(view, "编辑") {
		t.Fatalf("edit view:\n%s", view)
	}

	// 右移到 en 并保存
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := cmd().(SettingsSaveMsg); !ok {
		t.Fatalf("enter save = %T", cmd())
	}
	if p.settings.Language != "en" {
		t.Fatalf("lang = %q", p.settings.Language)
	}
	if p.editMode {
		t.Fatal("should exit edit")
	}
}

func TestSettingsPanelTextEditModeAndSaveValues(t *testing.T) {
	p := newTestSettingsPanel(t)

	// MaxInjectKB 文本编辑
	p.table.SetCursor(4)
	key, _ := p.GetSelectedSetting()
	if key != "MaxInjectKB" {
		t.Fatalf("key = %q", key)
	}
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Fatal("text edit should return blink")
	}
	if !p.editMode || p.editChoices != nil {
		t.Fatal("text edit mode")
	}
	p.editInput.SetValue("64")
	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := cmd().(SettingsSaveMsg); !ok {
		t.Fatalf("save = %T", cmd())
	}
	if p.settings.MaxInjectKB != 64 {
		t.Fatalf("MaxInjectKB = %d", p.settings.MaxInjectKB)
	}

	// esc 取消
	p.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if p.editMode {
		t.Fatal("esc should exit")
	}

	// saveEditValue 各字段
	p.settings = defaultPanelSettings()
	tn := false
	p.settings.DesktopNotifications = &tn
	p.settings.GitCommitReminder = &tn

	cases := []struct {
		key   string
		value string
		check func()
	}{
		{"AutoContext", "false", func() {
			if p.settings.AutoContext {
				t.Fatal("AutoContext")
			}
		}},
		{"DesktopNotifications", "true", func() {
			if p.settings.DesktopNotifications == nil || !*p.settings.DesktopNotifications {
				t.Fatal("DesktopNotifications")
			}
		}},
		{"GitCommitReminder", "1", func() {
			if p.settings.GitCommitReminder == nil || !*p.settings.GitCommitReminder {
				t.Fatal("GitCommitReminder")
			}
		}},
		{"Language", "en", func() {
			if p.settings.Language != "en" {
				t.Fatal("Language")
			}
		}},
		{"Theme", "light", func() {
			if p.settings.Theme != "light" {
				t.Fatal("Theme")
			}
		}},
		{"PlanPromptStyle", "detailed", func() {
			if p.settings.PlanPromptStyle != "detailed" {
				t.Fatal("PlanPromptStyle")
			}
		}},
		{"DiffTheme", " monokai ", func() {
			if p.diffTheme != "monokai" {
				t.Fatalf("DiffTheme = %q", p.diffTheme)
			}
		}},
		{"MemoryInjection(Global)", "true", func() {
			if !p.memoryInjectionEnabled {
				t.Fatal("MemoryInjection")
			}
		}},
		{"NextMessagePrediction(Global)", "false", func() {
			if p.globalPredictionEnabled {
				t.Fatal("NextMessagePrediction")
			}
		}},
		{"PlanBubbleColor", "#ff0000", func() {
			if p.settings.PlanBubbleColor != "#ff0000" {
				t.Fatal("PlanBubbleColor")
			}
		}},
		{"WatchDebounceMs", "abc", func() {
			// 非法数字不改值
		}},
		{"PollIntervalSec", "9", func() {
			if p.settings.PollIntervalSec != 9 {
				t.Fatalf("PollIntervalSec = %d", p.settings.PollIntervalSec)
			}
		}},
	}
	for _, tc := range cases {
		p.editKey = tc.key
		p.editChoices = nil
		p.editInput = textinput.New()
		p.editInput.SetValue(tc.value)
		p.saveEditValue()
		tc.check()
	}

	// settings 为 nil 时 saveEditValue 直接返回
	p.settings = nil
	p.editKey = "Language"
	p.editInput.SetValue("en")
	p.saveEditValue() // 不应 panic

	// 未知 editKey 标签回落原 key
	p.editKey = "UnknownKey"
	if p.editKeyLabel() != "UnknownKey" {
		t.Fatalf("label = %q", p.editKeyLabel())
	}

	// choiceLabelKey
	if choiceLabelKey("high-contrast") != "high_contrast" {
		t.Fatal("high-contrast key")
	}
	if choiceLabelKey("dark") != "dark" {
		t.Fatal("dark key")
	}
}

func TestSettingsPanelViewEditModeBranches(t *testing.T) {
	p := newTestSettingsPanel(t)
	// 文本编辑视图
	p.table.SetCursor(4)
	p.enterEditMode()
	view := p.viewEditMode()
	if !strings.Contains(view, "MaxInjectKB") && !strings.Contains(view, "上下文") {
		t.Fatalf("text edit view:\n%s", view)
	}

	// 选择编辑视图（Theme）
	p.exitEditMode()
	p.table.SetCursor(6)
	key, _ := p.GetSelectedSetting()
	if key != "Theme" {
		t.Fatalf("key = %q", key)
	}
	p.enterEditMode()
	view = p.viewEditMode()
	if !strings.Contains(view, "dark") {
		t.Fatalf("choice edit view:\n%s", view)
	}

	// 空选中不进编辑
	p.exitEditMode()
	p.rowKeys = nil
	p.table.SetRows(nil)
	if _, cmd := p.enterEditMode(); cmd != nil {
		t.Fatal("oob enterEdit should be nil cmd")
	}
	if p.editMode {
		t.Fatal("oob should not enter edit")
	}

	// 列表视图
	p.exitEditMode()
	p.LoadSettings()
	view = p.View()
	if !strings.Contains(view, "AutoContext") && !strings.Contains(view, "自动") {
		t.Fatalf("list view:\n%s", view)
	}

	// 语言切换
	out, _ := p.Update(LanguageChangeMsg{Language: "en"})
	if sp := out.(*SettingsPanel); sp.language != "en" {
		t.Fatalf("lang = %q", sp.language)
	}
}
