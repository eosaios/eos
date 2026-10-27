package cache

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"testing"
)

func TestMultiLevelCacheGetPromotes(t *testing.T) {
	m := NewMultiLevelCache[string, int](map[CacheLevel]LevelConfig{
		CacheLevelL1: {Capacity: 4},
		CacheLevelL2: {Capacity: 8},
		CacheLevelL3: {Capacity: 16},
	})

	m.PutL3("k", 7)
	if v, ok := m.Get("k"); !ok || v != 7 {
		t.Fatalf("Get from L3 = %d,%v; want 7,true", v, ok)
	}
	// L3 命中后应晋升到 L2。
	if !m.levels[CacheLevelL2].Has("k") {
		t.Fatal("L3 hit must promote entry to L2")
	}
	if v, ok := m.Get("k"); !ok || v != 7 {
		t.Fatalf("Get after promotion = %d,%v; want 7,true", v, ok)
	}
	// L2 命中后应晋升到 L1。
	if !m.levels[CacheLevelL1].Has("k") {
		t.Fatal("L2 hit must promote entry to L1")
	}

	stats := m.Stats()
	if stats.L3Hits != 1 || stats.L2Hits != 1 || stats.L1Hits != 0 || stats.Promotes != 2 {
		t.Fatalf("Stats() = %+v; want L3Hits=1 L2Hits=1 Promotes=2", stats)
	}
}

func TestMultiLevelCacheMissAndPutLevels(t *testing.T) {
	m := NewMultiLevelCache[string, int](nil) // DefaultLevelConfig

	if _, ok := m.Get("nope"); ok {
		t.Fatal("Get on empty must miss")
	}
	if m.Has("nope") {
		t.Fatal("Has on empty must be false")
	}

	m.PutL1("a", 1)
	m.PutL2("b", 2)
	m.PutL3("c", 3)
	for key, want := range map[string]int{"a": 1, "b": 2, "c": 3} {
		if v, ok := m.Get(key); !ok || v != want {
			t.Fatalf("Get(%s) = %d,%v; want %d,true", key, v, ok, want)
		}
	}

	// Delete 清除所有级别的同键条目。
	m.PutL1("dup", 9)
	m.PutL2("dup", 9)
	m.PutL3("dup", 9)
	m.Delete("dup")
	if m.Has("dup") {
		t.Fatal("Delete must remove the key from every level")
	}

	m.Clear()
	if m.Has("a") || m.Has("b") || m.Has("c") {
		t.Fatal("Clear must empty all levels")
	}
	if got := m.Stats(); got.Gets != 0 || got.Puts != 0 {
		t.Fatalf("Clear must reset stats; got %+v", got)
	}
}

func TestMultiLevelCacheGetOrElseFillsL1(t *testing.T) {
	m := NewMultiLevelCache[string, int](nil)
	calls := 0
	load := func() (int, error) { calls++; return 5, nil }

	if v, err := m.GetOrElse("k", load); err != nil || v != 5 {
		t.Fatalf("GetOrElse = %d,%v; want 5,nil", v, err)
	}
	if v, err := m.GetOrElse("k", load); err != nil || v != 5 {
		t.Fatalf("GetOrElse second = %d,%v; want 5,nil", v, err)
	}
	if calls != 1 {
		t.Fatalf("load ran %d times; want 1", calls)
	}
	// GetOrElse 的新数据写入 L1。
	if !m.levels[CacheLevelL1].Has("k") {
		t.Fatal("GetOrElse must fill L1 on miss")
	}
}

func TestMultiLevelCacheHitRateAggregation(t *testing.T) {
	m := NewMultiLevelCache[string, int](nil)
	m.PutL1("a", 1)
	m.PutL2("b", 2)

	m.Get("a") // L1 hit
	m.Get("b") // L2 hit
	m.Get("x") // miss

	if got := m.TotalHits(); got != 2 {
		t.Fatalf("TotalHits() = %d; want 2", got)
	}
	if r := m.HitRate(); r != 2.0/3.0 {
		t.Fatalf("HitRate() = %v; want %v", r, 2.0/3.0)
	}
}
