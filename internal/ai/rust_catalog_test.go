package ai

import (
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

// ep is a shorthand for creating ProviderEndpoint literals in tests.
func ep(plan, format, base string) coreapi.ProviderEndpoint {
	return coreapi.ProviderEndpoint{Plan: plan, Format: format, APIBase: base}
}

func TestApplyCoreModelCatalogReplacesProviderAndPresetLookups(t *testing.T) {
	t.Cleanup(func() {
		globalRegistry = NewProviderRegistry()
		globalCatalog = NewModelCatalog()
	})

	ApplyCoreModelCatalog(coreapi.ModelCatalogState{
		Providers: []coreapi.ModelProviderOption{{
			ID:            "example",
			Name:          "Example",
			Endpoints:     []coreapi.ProviderEndpoint{ep("api", "openai_chat", "https://example.invalid/v1")},
			DefaultModels: []string{"example-vision"},
		}},
		Presets: []coreapi.ModelPresetOption{{
			ID:             "example-vision",
			Name:           "Example Vision",
			ProviderID:     "example",
			ModelName:      "example-vision",
			Plan:           "api",
			Format:         "openai_chat",
			ContextWindow:  12345,
			SupportsVision: true,
			SupportsTools:  true,
		}},
		AllowCustomProvider: true,
		AllowCustomModel:    true,
	})

	providers := GetAllProviders()
	if len(providers) != 1 || providers[0].ID != "example" {
		t.Fatalf("providers=%+v, want runtime catalog provider only", providers)
	}
	if got := GetProviderByID("example"); got == nil || got.DefaultAPIBase != "https://example.invalid/v1" {
		t.Fatalf("GetProviderByID(example)=%+v", got)
	}
	entry := GetModelEntry("example-vision")
	if entry == nil {
		t.Fatal("GetModelEntry(example-vision)=nil")
	}
	if entry.Provider != ProviderType("example") || entry.ContextWindow != 12345 {
		t.Fatalf("entry=%+v", entry)
	}
	if !SupportsVisionFromCatalog("example-vision") {
		t.Fatal("SupportsVisionFromCatalog(example-vision)=false, want true")
	}
	if !SupportsToolsFromCatalog("example-vision") {
		t.Fatal("SupportsToolsFromCatalog(example-vision)=false, want true")
	}
}

func TestApplyCoreModelCatalogEmptySnapshotStaysEmpty(t *testing.T) {
	t.Cleanup(func() {
		globalRegistry = NewProviderRegistry()
		globalCatalog = NewModelCatalog()
	})

	ApplyCoreModelCatalog(coreapi.ModelCatalogState{})

	if providers := GetAllProviders(); len(providers) != 0 {
		t.Fatalf("providers=%+v, want empty catalog", providers)
	}
	if models := GetAllModels(); len(models) != 0 {
		t.Fatalf("models=%+v, want empty catalog", models)
	}
	if SupportsVisionFromCatalog("example-vision") {
		t.Fatal("SupportsVisionFromCatalog(example-vision)=true, want false for empty snapshot")
	}
	if SupportsToolsFromCatalog("example-vision") {
		t.Fatal("SupportsToolsFromCatalog(example-vision)=true, want false for empty snapshot")
	}
	if AllowCustomProviderFromCatalog() {
		t.Fatal("AllowCustomProviderFromCatalog()=true, want false for empty snapshot")
	}
	if AllowCustomModelFromCatalog() {
		t.Fatal("AllowCustomModelFromCatalog()=true, want false for empty snapshot")
	}
}

// TestMiniMaxTokenPlanCatalog 对齐内核 MiniMax 目录：Token Plan 复用 coding
// plan 的两个端点，preset 按 (plan, format) 查端点必须拿到正确 base，
// 且套餐 PlanModels（M3/M2.7）透传不丢。
func TestMiniMaxTokenPlanCatalog(t *testing.T) {
	t.Cleanup(func() {
		globalRegistry = NewProviderRegistry()
		globalCatalog = NewModelCatalog()
	})

	ApplyCoreModelCatalog(coreapi.ModelCatalogState{
		Providers: []coreapi.ModelProviderOption{{
			ID:        "minimax",
			Name:      "MiniMax",
			APIKeyEnv: "MINIMAX_API_KEY",
			Endpoints: []coreapi.ProviderEndpoint{
				ep("api", "openai_chat", "https://api.minimaxi.com/v1"),
				ep("coding", "openai_chat", "https://api.minimaxi.com/v1"),
				ep("coding", "anthropic", "https://api.minimaxi.com/anthropic/v1"),
			},
		}},
		Presets: []coreapi.ModelPresetOption{
			{
				ID:         "minimax-token-plan-openai",
				Name:       "MiniMax Token Plan (OpenAI)",
				ProviderID: "minimax",
				ModelName:  "MiniMax-M3",
				Plan:       "coding",
				Format:     "openai_chat",
				PlanModels: []coreapi.PlanModel{
					{ModelID: "MiniMax-M3", Label: "MiniMax M3"},
					{ModelID: "MiniMax-M2.7", Label: "MiniMax M2.7"},
				},
			},
			{
				ID:         "minimax-token-plan-claude",
				Name:       "MiniMax Token Plan (Claude)",
				ProviderID: "minimax",
				ModelName:  "MiniMax-M3",
				Plan:       "coding",
				Format:     "anthropic",
			},
		},
	})

	provider := GetProviderByID("minimax")
	if provider == nil {
		t.Fatal("GetProviderByID(minimax)=nil")
	}

	openai := GetModelEntry("minimax-token-plan-openai")
	if openai == nil {
		t.Fatal("GetModelEntry(minimax-token-plan-openai)=nil")
	}
	if got := provider.ResolveAPIBase(openai.Plan, openai.PlanFormat); got != "https://api.minimaxi.com/v1" {
		t.Fatalf("token-plan-openai base=%q, want https://api.minimaxi.com/v1", got)
	}
	if len(openai.PlanModels) != 2 || openai.PlanModels[0].ModelID != "MiniMax-M3" || openai.PlanModels[1].ModelID != "MiniMax-M2.7" {
		t.Fatalf("token-plan-openai plan models=%+v, want M3 + M2.7", openai.PlanModels)
	}

	claude := GetModelEntry("minimax-token-plan-claude")
	if claude == nil {
		t.Fatal("GetModelEntry(minimax-token-plan-claude)=nil")
	}
	if got := provider.ResolveAPIBase(claude.Plan, claude.PlanFormat); got != "https://api.minimaxi.com/anthropic/v1" {
		t.Fatalf("token-plan-claude base=%q, want https://api.minimaxi.com/anthropic/v1", got)
	}
}

// TestResolveAPIBaseUnknownPlanFormat 查不到的 (plan, format) 返回空串，
// 由调用方回落，不静默给错端点。
func TestResolveAPIBaseUnknownPlanFormat(t *testing.T) {
	cfg := &ProviderConfig{
		Endpoints: []coreapi.ProviderEndpoint{ep("api", "openai_chat", "https://example.invalid/v1")},
	}
	if got := cfg.ResolveAPIBase("coding", "anthropic"); got != "" {
		t.Fatalf("ResolveAPIBase(coding, anthropic)=%q, want empty", got)
	}
	if got := cfg.ResolveAPIBase("", ""); got != "" {
		t.Fatalf("ResolveAPIBase(empty)=%q, want empty", got)
	}
}
