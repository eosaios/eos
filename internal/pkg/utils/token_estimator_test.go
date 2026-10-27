package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestEstimateTokensWeighted_BasicBuckets(t *testing.T) {
	if got := EstimateTokensWeighted("", "abcd"); got != 1 {
		t.Fatalf("unexpected tokens for english: %d", got)
	}
	if got := EstimateTokensWeighted("", "中文中文"); got != 6 {
		t.Fatalf("unexpected tokens for cjk: %d", got)
	}
	if got := EstimateTokensWeighted("", "中文abcd"); got != 4 {
		t.Fatalf("unexpected tokens for mixed: %d", got)
	}
}

func TestTokenEstimateCache_RoundTrip(t *testing.T) {
	cache := NewTokenEstimateCache(2)
	key := TokenEstimateKey("gpt-4o", "hello")
	if _, ok := cache.Get(key); ok {
		t.Fatalf("expected miss")
	}
	cache.Put(key, 123)
	if v, ok := cache.Get(key); !ok || v != 123 {
		t.Fatalf("unexpected cache value: %v %v", v, ok)
	}
}
