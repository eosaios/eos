package ai

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "strings"

func ContextWindowTokens(model string) int {
	m := strings.ToLower(strings.TrimSpace(model))
	if v, ok := getContextWindowOverride(m); ok && v > 0 {
		return v
	}
	if entry := findCatalogEntryByKey(m); entry != nil {
		if v := GetCatalogContextWindow(entry); v > 0 {
			return v
		}
	}
	switch {
	case strings.Contains(m, "gpt-5.5"):
		return 1050000
	case strings.Contains(m, "gpt-5-codex"):
		return 400000
	case strings.Contains(m, "gpt-4o"):
		return 128000
	case strings.Contains(m, "gpt-4"):
		return 128000
	case strings.Contains(m, "gpt-3.5"):
		return 16000
	case strings.Contains(m, "deepseek-v4"):
		return 1000000
	case strings.Contains(m, "kimi-k2.5"), strings.Contains(m, "kimi-k2-5"), strings.Contains(m, "kimi-for-coding"):
		return 256000
	case strings.HasPrefix(m, "qwen3.6-plus"), strings.HasPrefix(m, "qwen3.5-plus"):
		return 1000000
	case strings.HasPrefix(m, "qwen3.6-max-preview"), strings.HasPrefix(m, "qwen3-max"):
		return 262144
	case strings.HasPrefix(m, "mimo-v2.6"):
		return 1000000
	case strings.HasPrefix(m, "gemini-3.1-pro-preview"), strings.HasPrefix(m, "gemini-3-flash-preview"), strings.HasPrefix(m, "gemini-3.1-flash-lite-preview"):
		return 1048576
	case strings.Contains(m, "claude-opus-4-7"), strings.Contains(m, "claude-opus-4-6"), strings.Contains(m, "claude-sonnet-4-6"):
		return 1000000
	case strings.Contains(m, "claude-haiku-4-5"):
		return 200000
	case strings.Contains(m, "qwen"):
		return 32768
	case strings.Contains(m, "claude-3"):
		return 200000
	case m == "":
		return 128000
	default:
		return 128000
	}
}
