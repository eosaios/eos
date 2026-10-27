package ai

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func resolveFixtures() ([]coreapi.ModelConfig, *coreapi.ModelCatalogState) {
	entries := []coreapi.ModelConfig{
		{Name: "minimax-token-plan-openai", Model: "MiniMax-M2.7", APIBase: "https://api.minimaxi.com/v1", ProviderID: "minimax", PresetID: "minimax-token-plan-openai", Active: true},
		{Name: "deepseek", Model: "deepseek-chat", ProviderID: "deepseek", PresetID: "deepseek-chat"},
	}
	catalog := &coreapi.ModelCatalogState{
		Providers: []coreapi.ModelProviderOption{
			{ID: "minimax", Name: "MiniMax", Endpoints: []coreapi.ProviderEndpoint{
				{Plan: "api", Format: "openai_chat", APIBase: "https://api.minimaxi.com/v1"},
				{Plan: "coding", Format: "openai_chat", APIBase: "https://api.minimaxi.com/v1"},
				{Plan: "coding", Format: "anthropic", APIBase: "https://api.minimaxi.com/anthropic"},
			}},
		},
		Presets: []coreapi.ModelPresetOption{
			{ID: "minimax-token-plan-openai", Name: "MiniMax Token Plan (OpenAI)", ProviderID: "minimax", ModelName: "MiniMax-M3", Plan: "coding", Format: "openai_chat", PlanModels: []coreapi.PlanModel{
				{ModelID: "MiniMax-M3", Label: "MiniMax M3", ContextWindow: 1000000},
				{ModelID: "MiniMax-M2.7", Label: "MiniMax M2.7", ContextWindow: 204800},
			}},
			{ID: "minimax-m3", Name: "MiniMax M3", ProviderID: "minimax", ModelName: "MiniMax-M3", Plan: "api", Format: "openai_chat"},
		},
	}
	return entries, catalog
}

func TestResolveModelInput(t *testing.T) {
	entries, catalog := resolveFixtures()

	cases := []struct {
		name             string
		input            string
		wantEntry        string
		wantPlanModel    string
		wantPlanSwitch   bool
		wantErrSubstring string
	}{
		// 条目名精确 / 归一化
		{name: "entry exact", input: "minimax-token-plan-openai", wantEntry: "minimax-token-plan-openai", wantPlanModel: "MiniMax-M2.7"},
		{name: "entry case-insensitive", input: "DeepSeek", wantEntry: "deepseek", wantPlanModel: "deepseek-chat"},
		// 桌面端写进会话的 label（带空格）——本次事故的直接形态
		{name: "plan label with space", input: "MiniMax M3", wantEntry: "minimax-token-plan-openai", wantPlanModel: "MiniMax-M3", wantPlanSwitch: true},
		{name: "plan model id hyphen", input: "minimax-m3", wantEntry: "minimax-token-plan-openai", wantPlanModel: "MiniMax-M3", wantPlanSwitch: true},
		{name: "plan model id exact already-used", input: "MiniMax-M2.7", wantEntry: "minimax-token-plan-openai", wantPlanModel: "MiniMax-M2.7"},
		// preset 显示名
		{name: "preset display name", input: "MiniMax Token Plan (OpenAI)", wantEntry: "minimax-token-plan-openai", wantPlanModel: "MiniMax-M2.7"},
		// 失败路径
		{name: "unknown", input: "gpt-99", wantErrSubstring: "unknown model"},
		{name: "empty", input: "  ", wantErrSubstring: "required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ResolveModelInput(tc.input, entries, catalog)
			if tc.wantErrSubstring != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got resolution %+v", tc.wantErrSubstring, res)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstring) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrSubstring)
				}
				if strings.TrimSpace(tc.input) != "" && !strings.Contains(err.Error(), "minimax-token-plan-openai") {
					t.Fatalf("error should embed available models hint: %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.EntryName != tc.wantEntry {
				t.Fatalf("entry = %q, want %q", res.EntryName, tc.wantEntry)
			}
			if res.PlanModelID != tc.wantPlanModel {
				t.Fatalf("plan model = %q, want %q", res.PlanModelID, tc.wantPlanModel)
			}
			if res.NeedsPlanSwitch != tc.wantPlanSwitch {
				t.Fatalf("plan switch = %v, want %v", res.NeedsPlanSwitch, tc.wantPlanSwitch)
			}
			if res.PresetID == "" || res.ProviderID == "" {
				t.Fatalf("resolution should carry preset/provider linkage: %+v", res)
			}
		})
	}
}

// 旧版条目缺 preset_id/provider_id：按 api_base 与 provider 端点兜底关联。
func TestResolveModelInputLegacyEntryWithoutPresetID(t *testing.T) {
	entries := []coreapi.ModelConfig{
		// ~/.eos.json 里旧版创建的真实形态：无 provider_id / preset_id。
		{Name: "minimax-token-plan-openai", Model: "MiniMax-M2.7", APIBase: "https://api.minimaxi.com/v1", Source: "user", Active: true},
	}
	_, catalog := resolveFixtures()

	res, err := ResolveModelInput("MiniMax M3", entries, catalog)
	if err != nil {
		t.Fatalf("legacy entry should resolve via endpoint-base fallback: %v", err)
	}
	if res.EntryName != "minimax-token-plan-openai" {
		t.Fatalf("entry = %q", res.EntryName)
	}
	if res.PlanModelID != "MiniMax-M3" || !res.NeedsPlanSwitch {
		t.Fatalf("want plan switch to MiniMax-M3, got %+v", res)
	}
	if res.PresetID != "minimax-token-plan-openai" || res.ProviderID != "minimax" {
		t.Fatalf("linkage should be backfilled from preset: %+v", res)
	}
}

func TestResolveModelInputWithoutCatalog(t *testing.T) {
	entries, _ := resolveFixtures()
	// catalog=nil：套餐 label 级匹配跳过，但条目名/模型 ID 仍可用
	if _, err := ResolveModelInput("minimax-token-plan-openai", entries, nil); err != nil {
		t.Fatalf("entry name should resolve without catalog: %v", err)
	}
	if _, err := ResolveModelInput("MiniMax M3", entries, nil); err == nil {
		t.Fatal("plan label should NOT resolve without catalog")
	}
}

func TestNormalizeModelKey(t *testing.T) {
	cases := map[string]string{
		"MiniMax M3":     "minimaxm3",
		"minimax-m3":     "minimaxm3",
		"MiniMax_M3":     "minimaxm3",
		"MiniMax.M3":     "minimaxm3",
		" MiniMax · M3 ": "minimaxm3",
		"DeepSeek":       "deepseek",
	}
	for in, want := range cases {
		if got := NormalizeModelKey(in); got != want {
			t.Fatalf("NormalizeModelKey(%q) = %q, want %q", in, got, want)
		}
	}
}
