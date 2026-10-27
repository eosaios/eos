package webbridge

import (
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// 档位（reasoningLevels）必须完整透传到前端目录：preset 级（如方舟套餐
// [off, auto]）与套餐模型级标注（如 kimi-k3 思考常开 [low, high, max]）。
func TestBuildModelCatalogStateCarriesReasoningLevels(t *testing.T) {
	catalog := adapter.ModelCatalogState{
		Presets: []adapter.ModelPresetOption{
			{
				ID:                      "ark-agent-plan-openai",
				SupportsReasoningEffort: true,
				ReasoningLevels:         []string{"off", "auto"},
				PlanModels: []adapter.PlanModel{
					{ModelID: "kimi-k3", ReasoningLevels: []string{"low", "high", "max"}},
					{ModelID: "glm-5.3", ReasoningLevels: []string{"off", "low", "high", "max"}},
				},
			},
		},
	}
	out := (&BridgeService{}).buildModelCatalogState(catalog)
	if len(out.Presets) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(out.Presets))
	}
	preset := out.Presets[0]
	if len(preset.ReasoningLevels) != 2 || preset.ReasoningLevels[0] != "off" || preset.ReasoningLevels[1] != "auto" {
		t.Fatalf("preset reasoning levels not carried: %v", preset.ReasoningLevels)
	}
	if got := preset.PlanModels[0].ReasoningLevels; len(got) != 3 || got[0] != "low" || got[2] != "max" {
		t.Fatalf("kimi-k3 reasoning levels not carried: %v", got)
	}
	if got := preset.PlanModels[1].ReasoningLevels; len(got) != 4 || got[0] != "off" || got[3] != "max" {
		t.Fatalf("glm-5.3 reasoning levels not carried: %v", got)
	}
}
