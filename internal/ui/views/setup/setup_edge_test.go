package setup

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/styles"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func newSetupView() *SetupView {
	return NewSetupView(styles.NewStyles(styles.DefaultDarkTheme()))
}

func TestNewSetupViewInitialState(t *testing.T) {
	v := newSetupView()
	if v.step != StepProvider {
		t.Fatalf("step = %v, want StepProvider", v.step)
	}
	if len(v.providers) != 4 {
		t.Fatalf("providers = %v, want 4 entries", v.providers)
	}
	if len(v.inputs) != 4 {
		t.Fatalf("inputs = %d, want 4", len(v.inputs))
	}
	// API Key 输入必须密码回显
	if v.inputs[2].EchoMode != textinput.EchoPassword {
		t.Fatalf("inputs[2].EchoMode = %v, want EchoPassword", v.inputs[2].EchoMode)
	}
	if v.focusIndex != 0 {
		t.Fatalf("focusIndex = %d, want 0", v.focusIndex)
	}
	if !v.inputs[0].Focused() {
		t.Fatal("inputs[0] should be focused after constructor")
	}
	if got := v.inputs[0].Placeholder; !strings.Contains(got, "anthropic") {
		t.Fatalf("placeholder = %q, want contains anthropic", got)
	}
}

func TestSetupViewSetSizeAndInit(t *testing.T) {
	v := newSetupView()
	v.SetSize(100, 30)
	if v.width != 100 || v.height != 30 {
		t.Fatalf("size = %dx%d", v.width, v.height)
	}
	for i, in := range v.inputs {
		if in.Width() != 80 {
			t.Fatalf("inputs[%d].Width = %d, want 80", i, in.Width())
		}
	}
	if cmd := v.Init(); cmd == nil {
		t.Fatal("Init should return blink cmd")
	}
}

func TestSetupViewUpdateTabCyclesProviders(t *testing.T) {
	v := newSetupView()
	v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if v.focusIndex != 1 {
		t.Fatalf("focusIndex = %d, want 1", v.focusIndex)
	}
	if got := v.inputs[0].Value(); got != "openai" {
		t.Fatalf("provider value = %q, want openai", got)
	}
	// 继续 tab 到末尾后回绕
	v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if v.focusIndex != 0 {
		t.Fatalf("focusIndex after wrap = %d, want 0", v.focusIndex)
	}
	if got := v.inputs[0].Value(); got != "anthropic" {
		t.Fatalf("provider value after wrap = %q, want anthropic", got)
	}
}

func TestSetupViewEscCancels(t *testing.T) {
	v := newSetupView()
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should return cancel cmd")
	}
	if _, ok := cmd().(SetupCancelMsg); !ok {
		t.Fatalf("esc cmd() = %T, want SetupCancelMsg", cmd())
	}
}

func TestSetupViewHandleEnterWalksAllSteps(t *testing.T) {
	v := newSetupView()
	// StepProvider：空值回落到第一个服务商
	_, cmd := v.handleEnter()
	if cmd != nil {
		t.Fatal("provider enter should not complete")
	}
	if v.config.Provider != "anthropic" {
		t.Fatalf("Provider = %q, want anthropic", v.config.Provider)
	}
	if v.step != StepAPIBase {
		t.Fatalf("step = %v, want StepAPIBase", v.step)
	}

	// StepAPIBase
	v.inputs[1].SetValue("https://api.example.com")
	_, _ = v.handleEnter()
	if v.config.APIBase != "https://api.example.com" {
		t.Fatalf("APIBase = %q", v.config.APIBase)
	}
	if v.step != StepAPIKey {
		t.Fatalf("step = %v, want StepAPIKey", v.step)
	}

	// StepAPIKey
	v.inputs[2].SetValue("sk-test")
	_, _ = v.handleEnter()
	if v.config.APIKey != "sk-test" {
		t.Fatalf("APIKey = %q", v.config.APIKey)
	}
	if v.step != StepModel {
		t.Fatalf("step = %v, want StepModel", v.step)
	}

	// StepModel → complete
	v.inputs[3].SetValue("gpt-4o")
	_, cmd = v.handleEnter()
	if cmd == nil {
		t.Fatal("model enter should return complete cmd")
	}
	msg, ok := cmd().(SetupCompleteMsg)
	if !ok {
		t.Fatalf("complete cmd() = %T, want SetupCompleteMsg", cmd())
	}
	if msg.Config.Name != "" || msg.Config.Model != "gpt-4o" || msg.Config.Provider != "anthropic" {
		t.Fatalf("complete config = %+v", msg.Config)
	}
	if v.step != StepComplete {
		t.Fatalf("step = %v, want StepComplete", v.step)
	}
}

func TestSetupViewHandleEnterUsesExplicitProvider(t *testing.T) {
	v := newSetupView()
	v.inputs[0].SetValue("openrouter")
	_, _ = v.handleEnter()
	if v.config.Provider != "openrouter" {
		t.Fatalf("Provider = %q, want openrouter", v.config.Provider)
	}
}

func TestSetupViewUpdateEnterRoutesToHandleEnter(t *testing.T) {
	v := newSetupView()
	v.inputs[0].SetValue("ark")
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("provider enter via Update should not complete")
	}
	if v.step != StepAPIBase {
		t.Fatalf("step = %v, want StepAPIBase", v.step)
	}
}

// 非快捷键消息仍会转发给当前输入框（可输入文本）。
func TestSetupViewUpdateForwardsTypingToInput(t *testing.T) {
	v := newSetupView()
	_, _ = v.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := v.inputs[0].Value(); !strings.Contains(got, "x") {
		t.Fatalf("input value = %q, want contains x", got)
	}
}

func TestSetupViewViewRendersEveryStep(t *testing.T) {
	v := newSetupView()
	// 默认尺寸兜底
	view := v.View()
	if !strings.Contains(view, "Welcome to EOS") {
		t.Fatalf("missing title:\n%s", view)
	}
	if !strings.Contains(view, "1. Provider") || !strings.Contains(view, "anthropic") {
		t.Fatalf("provider step content missing:\n%s", view)
	}

	v.step = StepAPIBase
	view = v.View()
	if !strings.Contains(view, "API Base") {
		t.Fatalf("api base step content missing:\n%s", view)
	}

	v.step = StepAPIKey
	view = v.View()
	if !strings.Contains(view, "API Key") {
		t.Fatalf("api key step content missing:\n%s", view)
	}

	v.step = StepModel
	v.config.Provider = "openai"
	view = v.View()
	if !strings.Contains(view, "gpt-4o") {
		t.Fatalf("model suggestions missing:\n%s", view)
	}

	// 未知服务商不渲染建议列表
	v.config.Provider = "unknown-vendor"
	view = v.View()
	if strings.Contains(view, "Suggested models") {
		t.Fatalf("unknown provider should not show suggestions:\n%s", view)
	}

	v.step = StepComplete
	view = v.View()
	if !strings.Contains(view, "Setup complete") {
		t.Fatalf("complete step content missing:\n%s", view)
	}
}

func TestSetupViewRenderProgressStylesPastCurrentFuture(t *testing.T) {
	v := newSetupView()
	v.step = StepAPIKey // 0/1 已完成，2 当前，3 未来
	progress := v.renderProgress()
	// 步骤标签是 1-based：Provider/API Base/API Key/Model
	if !strings.Contains(progress, "1. Provider") || !strings.Contains(progress, "4. Model") {
		t.Fatalf("progress = %q", progress)
	}
	// 已完成用 success 色（绿），当前 Bold+info，未来 muted——断言四段顺序
	parts := strings.Split(progress, "→")
	if len(parts) != 4 {
		t.Fatalf("progress parts = %d, want 4: %q", len(parts), progress)
	}
}

func TestSetupViewRenderProvidersHighlightsFocus(t *testing.T) {
	v := newSetupView()
	v.focusIndex = 2
	out := v.renderProviders()
	if !strings.Contains(out, "ark") || !strings.Contains(out, "openrouter") {
		t.Fatalf("providers render = %q", out)
	}
	// 聚焦项带背景色序列；至少确认四个服务商名都在
	for _, name := range v.providers {
		if !strings.Contains(out, name) {
			t.Fatalf("missing provider %q in %q", name, out)
		}
	}
}
