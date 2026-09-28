package setup

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ai"
	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/ui/styles"
	"github.com/eosaios/eos/pkg/coreapi"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
)

// applyTestCatalog 注入临时服务商/模型目录，测试结束清空全局目录，
// 避免污染同包后续用例。
func applyTestCatalog(t *testing.T, state coreapi.ModelCatalogState) {
	t.Helper()
	ai.ApplyCoreModelCatalog(state)
	t.Cleanup(func() {
		ai.ApplyCoreModelCatalog(coreapi.ModelCatalogState{})
	})
}

func testCatalogState() coreapi.ModelCatalogState {
	return coreapi.ModelCatalogState{
		AllowCustomProvider: true,
		AllowCustomModel:    true,
		Providers: []coreapi.ModelProviderOption{
			{
				ID:        "demo",
				Name:      "Demo Provider",
				Website:   "https://demo.example",
				APIKeyEnv: "DEMO_KEY",
				Endpoints: []coreapi.ProviderEndpoint{
					{Plan: "api", Format: "openai_chat", APIBase: "https://demo.example/v1"},
					{Plan: "token", Format: "openai_chat", APIBase: "https://demo.example/token"},
				},
				DefaultModels: []string{"demo-fixed"},
			},
		},
		Presets: []coreapi.ModelPresetOption{
			{
				ID:            "demo-fixed",
				Name:          "Demo Fixed",
				ProviderID:    "demo",
				ModelName:     "demo-fixed-1",
				Plan:          "api",
				Format:        "openai_chat",
				ContextWindow: 128000,
				SupportsVision: true,
				SupportsTools:  true,
				Tags:          []string{"推荐"},
			},
			{
				ID:         "demo-plan",
				Name:       "Demo Plan",
				ProviderID: "demo",
				ModelName:  "plan-b",
				Plan:       "token",
				Format:     "openai_chat",
				PlanModels: []coreapi.PlanModel{
					{ModelID: "plan-a", Label: "A", ContextWindow: 32000},
					{ModelID: "plan-b", Label: "B", ContextWindow: 64000},
				},
			},
		},
	}
}

func newWizard(lang string) *ModelSetupView {
	return NewModelSetupWizard(styles.NewStyles(styles.DefaultDarkTheme()), lang)
}

func TestProviderDisplayName(t *testing.T) {
	custom := &ai.ProviderConfig{ID: "custom", Name: "自定义"}
	if got := providerDisplayName(custom, "zh"); got != "自定义" {
		t.Fatalf("custom zh = %q", got)
	}
	if got := providerDisplayName(custom, "en"); got != "Custom" {
		t.Fatalf("custom en = %q", got)
	}
	other := &ai.ProviderConfig{ID: "demo", Name: "Demo"}
	if got := providerDisplayName(other, "zh"); got != "Demo" {
		t.Fatalf("other = %q", got)
	}
	// 回归：旧实现在 p==nil 时会落到 p.Name 解引用 panic
	if got := providerDisplayName(nil, "zh"); got != "" {
		t.Fatalf("nil provider = %q, want empty", got)
	}
}

func TestModelCapabilityLabelCombinations(t *testing.T) {
	cases := []struct {
		name string
		m    *ai.ModelCatalogEntry
		lang string
		want string
	}{
		{"none-zh", &ai.ModelCatalogEntry{}, "zh", "-"},
		{"all-zh", &ai.ModelCatalogEntry{SupportsVision: true, SupportsReasoningEffort: true, SupportsTools: true}, "zh", "视/理/工"},
		{"all-en", &ai.ModelCatalogEntry{SupportsVision: true, SupportsReasoningEffort: true, SupportsTools: true}, "en", "V/R/T"},
		{"vision-only-en", &ai.ModelCatalogEntry{SupportsVision: true}, "en", "V"},
		{"reasoning-tools-zh", &ai.ModelCatalogEntry{SupportsReasoningEffort: true, SupportsTools: true}, "zh", "理/工"},
	}
	for _, tc := range cases {
		if got := modelCapabilityLabel(tc.m, tc.lang); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDedupeNameAndSetExistingNames(t *testing.T) {
	v := newWizard("zh")
	if got := v.dedupeName("fresh"); got != "fresh" {
		t.Fatalf("no existing = %q", got)
	}

	// SetExistingNames 会 TrimSpace：demo 与 demo-2 都已占用
	v.SetExistingNames([]string{"demo", " demo-2 "})
	if got := v.dedupeName("demo"); got != "demo-3" {
		t.Fatalf("dedupe demo = %q, want demo-3", got)
	}
	if got := v.dedupeName("other"); got != "other" {
		t.Fatalf("dedupe other = %q", got)
	}

	// 连续冲突序号
	v.SetExistingNames([]string{"x", "x-2", "x-3"})
	if got := v.dedupeName("x"); got != "x-4" {
		t.Fatalf("dedupe x = %q, want x-4", got)
	}
}

func TestSetSizeClampsMinWidth(t *testing.T) {
	v := newWizard("zh")
	v.SetSize(10, 10)
	if v.width != 10 {
		t.Fatalf("width = %d", v.width)
	}
	// provider/model table 最小宽度 20
	if w := v.providerTable.Width(); w != 20 {
		t.Fatalf("providerTable width = %d, want 20", w)
	}
	v.SetSize(120, 40)
	if w := v.providerTable.Width(); w != 108 {
		t.Fatalf("providerTable width = %d, want 108", w)
	}
	if w := v.inputs[0].Width(); w != 100 {
		t.Fatalf("input width = %d, want 100", w)
	}
}

func TestCurrentPlanModelAndCycle(t *testing.T) {
	v := newWizard("zh")
	// 无套餐选择
	if v.hasPlanChoice() {
		t.Fatal("hasPlanChoice should be false without PlanModels")
	}
	if v.currentPlanModel() != nil {
		t.Fatal("currentPlanModel should be nil without plan")
	}
	v.cyclePlanModel(1) // no-op

	v.selectedModel = &ai.ModelCatalogEntry{
		PlanModels: []coreapi.PlanModel{
			{ModelID: "a", Label: "A"},
			{ModelID: "b", Label: "B"},
		},
	}
	if !v.hasPlanChoice() {
		t.Fatal("hasPlanChoice should be true with 2 PlanModels")
	}
	// 越界 planIndex
	v.planIndex = 99
	if v.currentPlanModel() != nil {
		t.Fatal("out-of-range planIndex should yield nil")
	}

	v.planIndex = 0
	v.cyclePlanModel(1)
	if got := v.inputs[3].Value(); got != "b" {
		t.Fatalf("after +1 model = %q, want b", got)
	}
	v.cyclePlanModel(1) // wrap to a
	if got := v.inputs[3].Value(); got != "a" {
		t.Fatalf("after wrap model = %q, want a", got)
	}
	v.cyclePlanModel(-1) // wrap back to b
	if got := v.inputs[3].Value(); got != "b" {
		t.Fatalf("after -1 wrap model = %q, want b", got)
	}
}

func TestLoadModelsNilProviderIsNoop(t *testing.T) {
	v := newWizard("zh")
	before := v.modelTable.Rows()
	v.loadModels(nil)
	if len(v.modelTable.Rows()) != len(before) {
		t.Fatal("loadModels(nil) should not change rows")
	}
}

func TestLoadModelsBuildsRowsWithCapabilityAndTags(t *testing.T) {
	applyTestCatalog(t, testCatalogState())
	v := newWizard("zh")
	provider := ai.GetProviderByID("demo")
	if provider == nil {
		t.Fatal("demo provider missing from catalog")
	}
	v.loadModels(provider)
	rows := v.modelTable.Rows()
	// 2 preset + 1 自定义
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3: %+v", len(rows), rows)
	}
	if rows[0][0] != "Demo Fixed" {
		t.Fatalf("row0 name = %q", rows[0][0])
	}
	// 128000 → 128.0K
	if rows[0][1] != "128.0K" {
		t.Fatalf("row0 ctx = %q, want 128.0K", rows[0][1])
	}
	if rows[0][2] != "视/工" {
		t.Fatalf("row0 caps = %q, want 视/工", rows[0][2])
	}
	if rows[0][3] != "推荐" {
		t.Fatalf("row0 tags = %q", rows[0][3])
	}
	// 无上下文显示 "-"
	if rows[1][1] != "-" {
		t.Fatalf("row1 ctx = %q, want -", rows[1][1])
	}
	// 32K 不足 1000 的分支：直接数字；此处 plan-a 是 32000 → 32.0K
	// 自定义行
	if rows[2][0] != "自定义" {
		t.Fatalf("row2 = %+v", rows[2])
	}
}

func TestLoadProvidersAddsCustomRow(t *testing.T) {
	applyTestCatalog(t, testCatalogState())
	v := newWizard("zh")
	v.loadProviders()
	rows := v.providerTable.Rows()
	// demo + custom
	if len(rows) != 2 {
		t.Fatalf("provider rows = %d, want 2: %+v", len(rows), rows)
	}
	if rows[0][0] != "Demo Provider" || rows[0][3] != "★" {
		t.Fatalf("demo row = %+v", rows[0])
	}
	if rows[1][0] != "自定义" {
		t.Fatalf("custom row = %+v", rows[1])
	}
}

func TestUpdateEscWalksBackThroughSteps(t *testing.T) {
	applyTestCatalog(t, testCatalogState())
	v := newWizard("zh")
	v.loadProviders()

	// Provider → esc 取消
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("provider esc should cancel")
	}
	if _, ok := cmd().(SetupCancelMsg); !ok {
		t.Fatalf("provider esc cmd = %T", cmd())
	}

	// 进到 Model 再 esc 回 Provider（先 loadModels 保证表格有行，SetCursor(0) 才能落在 0）
	v.step = ModelSetupStepModel
	v.modelFocused = true
	v.selectedProvider = ai.GetProviderByID("demo")
	v.loadModels(v.selectedProvider)
	v.modelTable.SetCursor(1)
	v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.step != ModelSetupStepProvider || !v.providerFocused {
		t.Fatalf("after model esc: step=%v providerFocused=%v", v.step, v.providerFocused)
	}
	if v.modelTable.Cursor() != 0 {
		t.Fatalf("model cursor = %d, want reset to 0", v.modelTable.Cursor())
	}

	// Config → esc 回 Model
	v.step = ModelSetupStepConfig
	v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.step != ModelSetupStepModel || !v.modelFocused {
		t.Fatalf("after config esc: step=%v modelFocused=%v", v.step, v.modelFocused)
	}

	// Custom 非编辑 → esc 回 Provider
	v.step = ModelSetupStepCustom
	v.editMode = false
	v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.step != ModelSetupStepProvider {
		t.Fatalf("custom esc step = %v", v.step)
	}

	// Custom 编辑模式 → esc 取消
	v.step = ModelSetupStepCustom
	v.editMode = true
	_, cmd = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("edit-mode custom esc should cancel")
	}
	if _, ok := cmd().(SetupCancelMsg); !ok {
		t.Fatalf("edit esc cmd = %T", cmd())
	}
}

func TestUpdateEnterSelectsBuiltinProviderAndModel(t *testing.T) {
	applyTestCatalog(t, testCatalogState())
	v := newWizard("zh")
	v.loadProviders()
	// 选第一个内置服务商
	v.providerTable.SetCursor(0)
	v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.step != ModelSetupStepModel {
		t.Fatalf("step = %v, want Model", v.step)
	}
	if v.selectedProvider == nil || v.selectedProvider.ID != "demo" {
		t.Fatalf("selectedProvider = %+v", v.selectedProvider)
	}
	if v.customProvider {
		t.Fatal("customProvider should be false")
	}

	// 选第一个内置模型（固定 preset）
	v.modelTable.SetCursor(0)
	v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.step != ModelSetupStepConfig {
		t.Fatalf("step = %v, want Config", v.step)
	}
	if v.selectedModel == nil || v.selectedModel.ID != "demo-fixed" {
		t.Fatalf("selectedModel = %+v", v.selectedModel)
	}
	if !v.modelReadOnly || !v.apiBaseReadOnly {
		t.Fatalf("readonly flags: model=%v apiBase=%v", v.modelReadOnly, v.apiBaseReadOnly)
	}
	if got := v.inputs[0].Value(); got != "demo-fixed" {
		t.Fatalf("display name = %q, want preset ID", got)
	}
	if got := v.inputs[1].Value(); got != "https://demo.example/v1" {
		t.Fatalf("api base = %q", got)
	}
	if got := v.inputs[3].Value(); got != "demo-fixed-1" {
		t.Fatalf("model = %q", got)
	}
}

func TestUpdateEnterSelectsPlanModel(t *testing.T) {
	applyTestCatalog(t, testCatalogState())
	v := newWizard("zh")
	v.loadProviders()
	v.providerTable.SetCursor(0)
	v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	v.modelTable.SetCursor(1) // demo-plan
	v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if v.step != ModelSetupStepConfig {
		t.Fatalf("step = %v", v.step)
	}
	if v.modelReadOnly {
		t.Fatal("plan model should not be readOnly")
	}
	// 默认选中 ModelName=plan-b
	if got := v.inputs[3].Value(); got != "plan-b" {
		t.Fatalf("model = %q, want plan-b (default from ModelName)", got)
	}
	if v.planIndex != 1 {
		t.Fatalf("planIndex = %d, want 1", v.planIndex)
	}
	if got := v.inputs[1].Value(); got != "https://demo.example/token" {
		t.Fatalf("api base = %q, want token endpoint", got)
	}
}

func TestUpdateEnterSelectsCustomProviderAndCustomModel(t *testing.T) {
	applyTestCatalog(t, testCatalogState())
	v := newWizard("zh")
	v.loadProviders()
	// 最后一行为「自定义」
	v.providerTable.SetCursor(len(ai.GetAllProviders()))
	v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.step != ModelSetupStepCustom || !v.customProvider {
		t.Fatalf("step=%v customProvider=%v", v.step, v.customProvider)
	}
	if v.focusIndex != 0 {
		t.Fatalf("focusIndex = %d, want 0", v.focusIndex)
	}

	// 自定义模型路径
	v = newWizard("zh")
	v.loadProviders()
	v.providerTable.SetCursor(0)
	v.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // → model step
	v.modelTable.SetCursor(len(v.models))         // 自定义行
	v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.step != ModelSetupStepConfig || !v.customModel {
		t.Fatalf("step=%v customModel=%v", v.step, v.customModel)
	}
	if !v.apiBaseReadOnly || v.modelReadOnly {
		t.Fatalf("flags apiBaseRO=%v modelRO=%v", v.apiBaseReadOnly, v.modelReadOnly)
	}
	if got := v.inputs[1].Value(); got != "https://demo.example/v1" && got != "" {
		// DefaultAPIBase 由 endpoints 推导
		t.Fatalf("api base = %q", got)
	}
}

func TestUpdateEnterOnModelWithNilEntryIsIgnored(t *testing.T) {
	v := newWizard("zh")
	v.step = ModelSetupStepModel
	v.models = []*ai.ModelCatalogEntry{nil}
	v.modelFocused = true
	v.modelTable.SetRows([]table.Row{{"nil"}})
	// 直接构造 cursor=0 命中 nil model
	v.modelTable.SetCursor(0)
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("nil model enter should not produce cmd")
	}
	if v.step != ModelSetupStepModel {
		t.Fatalf("step changed to %v", v.step)
	}
}

func TestUpdateEnterOnConfigCallsHandleSave(t *testing.T) {
	v := newWizard("zh")
	v.step = ModelSetupStepConfig
	v.customModel = true
	v.modelReadOnly = false
	v.selectedProvider = &ai.ProviderConfig{ID: "demo"}
	v.inputs[0].SetValue("my-model")
	v.inputs[1].SetValue("https://x.example/v1")
	v.inputs[2].SetValue("sk-1")
	v.inputs[3].SetValue("m1")
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("config enter should save")
	}
	msg := cmd().(ModelFormCompleteMsg)
	if msg.Config.Name != "my-model" || msg.Config.Model != "m1" {
		t.Fatalf("saved = %+v", msg.Config)
	}
}

func TestHandleSaveRejectsInvalidPlanModel(t *testing.T) {
	v := newPlanConfigWizard()
	v.selectedProvider = &ai.ProviderConfig{ID: "test-plan", Name: "Test Plan"}
	v.inputs[3].SetValue("not-in-plan")
	cmd := v.handleSave()
	if cmd != nil {
		t.Fatal("invalid plan model should return nil cmd")
	}
	if v.errMsg == "" {
		t.Fatal("errMsg should be set")
	}
	if !strings.Contains(v.errMsg, "model-a") || !strings.Contains(v.errMsg, "model-b") {
		t.Fatalf("errMsg = %q, want whitelist IDs", v.errMsg)
	}

	// 修正后可保存
	v.inputs[3].SetValue("model-b")
	cmd = v.handleSave()
	if cmd == nil {
		t.Fatal("valid plan model should save")
	}
	if v.errMsg != "" {
		t.Fatalf("errMsg should clear, got %q", v.errMsg)
	}
}

func mustSave(t *testing.T, v *ModelSetupView) ModelFormCompleteMsg {
	t.Helper()
	cmd := v.handleSave()
	if cmd == nil {
		t.Fatal("handleSave() = nil")
	}
	msg, ok := cmd().(ModelFormCompleteMsg)
	if !ok {
		t.Fatalf("handleSave() = %T", cmd())
	}
	return msg
}

func TestHandleSaveDisplayNameFallbacks(t *testing.T) {
	// customProvider 空显示名 → model-时间戳
	v := newWizard("zh")
	v.step = ModelSetupStepCustom
	v.customProvider = true
	v.inputs[1].SetValue("https://c.example")
	v.inputs[3].SetValue("cm")
	msg := mustSave(t, v)
	if !strings.HasPrefix(msg.Config.Name, "model-") {
		t.Fatalf("custom provider name = %q", msg.Config.Name)
	}
	if msg.Config.Provider != "custom" {
		t.Fatalf("provider = %q", msg.Config.Provider)
	}
	if msg.Config.PresetID != "" {
		t.Fatalf("presetID = %q, want empty", msg.Config.PresetID)
	}

	// customModel 空显示名 → 用模型名
	v = newWizard("zh")
	v.step = ModelSetupStepConfig
	v.customModel = true
	v.selectedProvider = &ai.ProviderConfig{ID: "demo"}
	v.inputs[3].SetValue("my-model")
	msg = mustSave(t, v)
	if msg.Config.Name != "my-model" {
		t.Fatalf("custom model name = %q", msg.Config.Name)
	}
	if msg.Config.Provider != "demo" || msg.Config.PresetID != "" {
		t.Fatalf("custom model config = %+v", msg.Config)
	}

	// customModel 且模型名也空 → custom-model-时间戳
	v.inputs[3].SetValue("")
	msg = mustSave(t, v)
	if !strings.HasPrefix(msg.Config.Name, "custom-model-") {
		t.Fatalf("empty custom model name = %q", msg.Config.Name)
	}

	// 固定 preset 空显示名 → selectedModel.Name
	v = newWizard("zh")
	v.step = ModelSetupStepConfig
	v.selectedProvider = &ai.ProviderConfig{ID: "demo"}
	v.selectedModel = &ai.ModelCatalogEntry{ID: "p1", Name: "Preset One", ModelName: "m1"}
	v.inputs[3].SetValue("m1")
	msg = mustSave(t, v)
	if msg.Config.Name != "Preset One" {
		t.Fatalf("preset name = %q", msg.Config.Name)
	}
	if msg.Config.PresetID != "p1" {
		t.Fatalf("presetID = %q", msg.Config.PresetID)
	}

	// 无 selectedModel 空显示名 → model-时间戳
	v.selectedModel = nil
	v.customModel = false
	msg = mustSave(t, v)
	if !strings.HasPrefix(msg.Config.Name, "model-") {
		t.Fatalf("fallback name = %q", msg.Config.Name)
	}
}

func TestHandleSaveEditModeForcesOriginalName(t *testing.T) {
	v := newWizard("zh")
	v.LoadForEdit(config.ModelEntry{
		Name:    "orig",
		APIBase: "https://e.example",
		Model:   "m",
	})
	v.inputs[0].SetValue("changed")
	cmd := v.handleSave()
	if cmd == nil {
		t.Fatal("edit save should produce cmd")
	}
	msg := cmd().(ModelFormCompleteMsg)
	if msg.Config.Name != "orig" || !msg.EditMode {
		t.Fatalf("edit save = %+v", msg.Config)
	}
}

func TestViewRendersAllBranches(t *testing.T) {
	applyTestCatalog(t, testCatalogState())

	// Provider 步
	v := newWizard("zh")
	v.SetSize(100, 40)
	view := v.View()
	if !strings.Contains(view, "选择服务商") {
		t.Fatalf("provider title missing:\n%s", view)
	}

	// Model 步（无 provider 时错误文案）
	v.step = ModelSetupStepModel
	v.selectedProvider = nil
	view = v.View()
	if !strings.Contains(view, "Error: Provider not selected") {
		t.Fatalf("missing provider error:\n%s", view)
	}

	// Model 步（有 provider）
	v.selectedProvider = ai.GetProviderByID("demo")
	v.loadModels(v.selectedProvider)
	view = v.View()
	if !strings.Contains(view, "Demo Provider") {
		t.Fatalf("model step missing provider name:\n%s", view)
	}

	// Config：固定 preset 只读模型
	v.step = ModelSetupStepConfig
	v.selectedModel = &ai.ModelCatalogEntry{ID: "p", Name: "P", ModelName: "m"}
	v.modelReadOnly = true
	v.inputs[3].SetValue("m")
	view = v.View()
	if !strings.Contains(view, "fixed") {
		t.Fatalf("fixed model label missing:\n%s", view)
	}

	// Config：套餐选择器
	v.modelReadOnly = false
	v.selectedModel = &ai.ModelCatalogEntry{
		ID: "plan",
		PlanModels: []coreapi.PlanModel{
			{ModelID: "a", Label: "A", ContextWindow: 32000},
			{ModelID: "b", Label: "B", ContextWindow: 0},
		},
	}
	v.planIndex = 0
	view = v.View()
	if !strings.Contains(view, "套餐内模型") {
		t.Fatalf("plan picker missing:\n%s", view)
	}
	if !strings.Contains(view, "32K") {
		t.Fatalf("plan context missing:\n%s", view)
	}

	// Config：单一 PlanModel 走 fixed 展示
	v.selectedModel = &ai.ModelCatalogEntry{
		ID:         "single",
		PlanModels: []coreapi.PlanModel{{ModelID: "only", Label: "Only"}},
	}
	v.modelReadOnly = false
	v.inputs[3].SetValue("only")
	view = v.View()
	if !strings.Contains(view, "fixed") {
		t.Fatalf("single plan model should show fixed:\n%s", view)
	}

	// Config：errMsg
	v.errMsg = "bad model"
	view = v.View()
	if !strings.Contains(view, "bad model") {
		t.Fatalf("errMsg missing:\n%s", view)
	}

	// customModel 分支
	v.errMsg = ""
	v.customModel = true
	v.selectedModel = nil
	v.inputs[1].SetValue("https://fixed.example")
	view = v.View()
	if !strings.Contains(view, "fixed.example") {
		t.Fatalf("custom model api base missing:\n%s", view)
	}

	// customProvider 非编辑
	v.customModel = false
	v.customProvider = true
	v.editMode = false
	view = v.View()
	if !strings.Contains(view, "显示名称") {
		t.Fatalf("custom provider fields missing:\n%s", view)
	}

	// customProvider 编辑（LoadForEdit 会把 step 置为 Custom）
	v.step = ModelSetupStepCustom
	v.editMode = true
	v.editOriginalName = "keep-me"
	view = v.View()
	if !strings.Contains(view, "keep-me") {
		t.Fatalf("edit original name missing:\n%s", view)
	}
	if !strings.Contains(view, "编辑模型") {
		t.Fatalf("edit title missing:\n%s", view)
	}

	// default title 分支（未知 step）
	v.step = ModelSetupStep(99)
	view = v.View()
	if view == "" {
		t.Fatal("default step view empty")
	}
}

func TestInitReturnsBlink(t *testing.T) {
	v := newWizard("zh")
	if cmd := v.Init(); cmd == nil {
		t.Fatal("Init should return blink")
	}
}
