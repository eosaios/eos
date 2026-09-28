package ai

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestContextWindowTokensHeuristics(t *testing.T) {
	cases := map[string]int{
		"gpt-5.5":           1050000,
		"gpt-5-codex":       400000,
		"gpt-4o":            128000,
		"gpt-3.5-turbo":     16000,
		"deepseek-v4":       1000000,
		"kimi-k2.5":         256000,
		"qwen3.6-plus":      1000000,
		"qwen3-max":         262144,
		"mimo-v2.6":         1000000,
		"unknown-model-xyz": 128000, // 默认回落
		"":                  128000, // 空名也走默认
	}
	for model, want := range cases {
		if got := ContextWindowTokens(model); got != want {
			t.Fatalf("ContextWindowTokens(%q) = %d, want %d", model, got, want)
		}
	}
}

func TestCatalogContextWindowAndFind(t *testing.T) {
	if GetCatalogContextWindow(nil) != 0 {
		t.Fatal("nil entry")
	}
	entry := &ModelCatalogEntry{ID: "x", ModelName: "m", ContextWindow: 42}
	if got := GetCatalogContextWindow(entry); got != 42 {
		t.Fatalf("cw = %d", got)
	}
	if findCatalogEntryByKey("") != nil {
		t.Fatal("empty key")
	}
	if findCatalogEntryByKey("nope-not-in-catalog") != nil {
		t.Fatal("missing")
	}
}

func TestProviderGetters(t *testing.T) {
	if GetProvider(ProviderType("nope")) != nil {
		t.Fatal("missing provider")
	}
	if GetAPIBase(ProviderType("nope"), APITypeStandard, "") != "" {
		t.Fatal("no provider base")
	}
	// customBase 优先
	if GetAPIBase(ProviderType("nope"), APITypeStandard, "https://x") != "https://x" {
		t.Fatal("custom base")
	}
	if GetModelsByProvider(ProviderType("nope")) != nil {
		t.Fatal("no models")
	}
}

func TestFirstNonEmptyAndAPIType(t *testing.T) {
	if firstNonEmpty("", "x") != "x" {
		t.Fatal("firstNonEmpty")
	}
	if firstNonEmpty("  a  ", "b") != "a" {
		t.Fatal("primary")
	}
	if firstNonEmpty("", "  ") != "" {
		t.Fatal("empty")
	}
	if apiTypeFromPlanFormat("code", "openai_chat") != APITypeCodePlan {
		t.Fatal("code plan")
	}
	if apiTypeFromPlanFormat("token", "openai_chat") != APITypeTokenPlan {
		t.Fatal("token plan")
	}
	if apiTypeFromPlanFormat("", "openai_chat") != APITypeStandard {
		t.Fatal("standard")
	}
	if apiTypeFromPlanFormat("", "anthropic") != APITypeClaude {
		t.Fatal("claude")
	}
	if apiTypeFromPlanFormat("token", "anthropic") != APITypeTokenPlanClaude {
		t.Fatal("token claude")
	}
}
