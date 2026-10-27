package ai

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// 批十四批测：GetCatalogContextWindow 全回退链 / GetAPIBase 四类型回退。

import (
	"testing"
)

func TestGetCatalogContextWindowFallbackChain(t *testing.T) {
	if got := GetCatalogContextWindow(nil); got != 0 {
		t.Fatalf("nil entry = %d", got)
	}

	// ModelName 命中 override（sync.Map 同包注入，测试后清理）。
	entry := &ModelCatalogEntry{ID: "e1", ModelName: "m-override", ContextWindow: 64}
	contextWindowOverrides.Store("m-override", 128)
	defer contextWindowOverrides.Delete("m-override")
	if got := GetCatalogContextWindow(entry); got != 128 {
		t.Fatalf("modelName override = %d, want 128", got)
	}
	contextWindowOverrides.Delete("m-override")

	// ID 命中 override。
	contextWindowOverrides.Store("e1", 256)
	defer contextWindowOverrides.Delete("e1")
	if got := GetCatalogContextWindow(entry); got != 256 {
		t.Fatalf("id override = %d, want 256", got)
	}
	contextWindowOverrides.Delete("e1")

	// 自带 ContextWindow。
	if got := GetCatalogContextWindow(entry); got != 64 {
		t.Fatalf("entry window = %d, want 64", got)
	}

	// 兄弟 preset 同底层模型继承（globalCatalog 换入后恢复）。
	oldEntries := globalCatalog.GetAll()
	globalCatalog.replaceAll([]*ModelCatalogEntry{
		{ID: "sibling", ModelName: "shared-model", ContextWindow: 200},
	})
	defer globalCatalog.replaceAll(oldEntries)
	if got := GetCatalogContextWindow(&ModelCatalogEntry{ModelName: "shared-model"}); got != 200 {
		t.Fatalf("sibling inheritance = %d, want 200", got)
	}
	if got := GetCatalogContextWindow(&ModelCatalogEntry{ID: "sibling"}); got != 200 {
		t.Fatalf("sibling by id = %d, want 200", got)
	}
	if got := GetCatalogContextWindow(&ModelCatalogEntry{ModelName: "unknown"}); got != 0 {
		t.Fatalf("no match = %d", got)
	}
}

func TestGetAPIBaseFallbackMatrix(t *testing.T) {
	// customBase 一票优先。
	if got := GetAPIBase(ProviderCustom, APITypeCodePlan, "https://custom"); got != "https://custom" {
		t.Fatalf("customBase = %q", got)
	}
	// 未注册服务商。
	if got := GetAPIBase(ProviderType("nope"), APITypeCodePlan, ""); got != "" {
		t.Fatalf("unknown provider = %q", got)
	}

	// 注册临时服务商覆盖全回退矩阵，结束后恢复原注册表。
	const probe ProviderType = "__probe__"
	old := globalRegistry.GetAll()
	globalRegistry.replaceAll([]*ProviderConfig{{
		ID:                     "probe",
		Name:                   "probe",
		Type:                   probe,
		DefaultAPIBase:         "https://default",
		CodePlanAPIBase:        "https://codeplan",
		ClaudeAPIBase:          "https://claude",
		TokenPlanAPIBase:       "https://tokenplan",
		TokenPlanClaudeAPIBase: "https://tokenclaude",
	}})
	defer globalRegistry.replaceAll(old)

	cases := []struct {
		apiType APIType
		want    string
	}{
		{APITypeCodePlan, "https://codeplan"},
		{APITypeClaude, "https://claude"},
		{APITypeTokenPlan, "https://tokenplan"},
		{APITypeTokenPlanClaude, "https://tokenclaude"},
	}
	for _, tc := range cases {
		if got := GetAPIBase(probe, tc.apiType, ""); got != tc.want {
			t.Fatalf("GetAPIBase(%v) = %q, want %q", tc.apiType, got, tc.want)
		}
	}

	// 各类型缺专用配置时逐级回退到 Default。
	globalRegistry.replaceAll([]*ProviderConfig{{
		ID: "probe", Name: "probe", Type: probe,
		DefaultAPIBase:   "https://default",
		CodePlanAPIBase:  "https://codeplan",
		TokenPlanAPIBase: "https://tokenplan",
	}})
	if got := GetAPIBase(probe, APITypeClaude, ""); got != "https://codeplan" {
		t.Fatalf("claude→codeplan fallback = %q", got)
	}
	if got := GetAPIBase(probe, APITypeTokenPlanClaude, ""); got != "https://tokenplan" {
		t.Fatalf("tokenclaude→tokenplan fallback = %q", got)
	}
	globalRegistry.replaceAll([]*ProviderConfig{{
		ID: "probe", Name: "probe", Type: probe, DefaultAPIBase: "https://default",
	}})
	for _, tc := range cases {
		if got := GetAPIBase(probe, tc.apiType, ""); got != "https://default" {
			t.Fatalf("bare provider %v = %q, want default", tc.apiType, got)
		}
	}
}
