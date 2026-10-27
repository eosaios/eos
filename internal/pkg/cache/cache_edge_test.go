package cache

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"testing"
	"time"
)

func TestCacheLevelString(t *testing.T) {
	cases := map[CacheLevel]string{
		CacheLevelL1:  "L1",
		CacheLevelL2:  "L2",
		CacheLevelL3:  "L3",
		CacheLevel(9): "unknown",
	}
	for in, want := range cases {
		if got := in.String(); got != want {
			t.Fatalf("String(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestLRUValuesRangeTTL(t *testing.T) {
	c := NewLRUCache[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	vals := c.Values()
	if len(vals) != 2 {
		t.Fatalf("values = %v", vals)
	}

	n := 0
	c.Range(func(k string, v int) bool {
		n++
		return true
	})
	if n != 2 {
		t.Fatalf("range = %d", n)
	}
	// 早停
	n = 0
	c.Range(func(k string, v int) bool {
		n++
		return false
	})
	if n != 1 {
		t.Fatalf("early stop = %d", n)
	}

	// TTL
	c2 := NewLRUCacheWithTTL[string, int](5, 20*time.Millisecond)
	c2.Put("x", 1)
	if !c2.IsFull() {
		// 容量 5 只有 1 项
	}
	c2.Put("y", 2)
	c2.Put("z", 3)
	if c2.IsFull() {
		t.Fatal("not full yet")
	}
	time.Sleep(30 * time.Millisecond)
	if cleaned := c2.CleanExpired(); cleaned < 3 {
		t.Fatalf("cleaned = %d", cleaned)
	}

	// PutWithExpiration
	c3 := NewLRUCache[string, int](5)
	c3.PutWithExpiration("e", 1, 10*time.Millisecond)
	if _, ok := c3.Get("e"); !ok {
		t.Fatal("fresh")
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok := c3.Get("e"); ok {
		t.Fatal("expired")
	}

	// Access time / TTL
	c4 := NewLRUCache[string, int](5)
	c4.Put("k", 1)
	if _, ok := c4.GetAccessTime("k"); !ok {
		t.Fatal("access time")
	}
	if _, ok := c4.GetAccessTime("missing"); ok {
		t.Fatal("missing access")
	}
	c4.SetTTL(time.Second)
	if c4.GetTTL() != time.Second {
		t.Fatal("ttl")
	}

	// UpdateCapacity / IsFull
	c5 := NewLRUCache[string, int](2)
	c5.Put("a", 1)
	c5.Put("b", 2)
	if !c5.IsFull() {
		t.Fatal("full")
	}
	c5.UpdateCapacity(5)
	if c5.Capacity() != 5 {
		t.Fatalf("cap = %d", c5.Capacity())
	}

	// HitRate 空
	empty := NewLRUCache[string, int](1)
	if empty.HitRate() != 0 {
		t.Fatal("empty hit rate")
	}
}

func TestMultiLevelCacheAccessors(t *testing.T) {
	m := NewMultiLevelCache[string, int](nil)
	m.Put("a", 1, CacheLevelL1)
	m.Put("b", 2, CacheLevelL2)

	if m.Size(CacheLevelL1) < 0 {
		t.Fatal("size")
	}
	if m.TotalSize() < 2 {
		t.Fatal("total size")
	}
	if m.LevelStats(CacheLevelL1).Puts < 0 {
		t.Fatal("level stats")
	}
	_ = m.LevelHitRate(CacheLevelL1)
	m.ResetStats()
	if m.CleanExpired() < 0 {
		t.Fatal("clean")
	}
	m.ResizeLevel(CacheLevelL1, 50)
	if m.GetLevel(CacheLevelL1) == nil {
		t.Fatal("get level")
	}
	m.SetConfig(CacheLevelL1, LevelConfig{Capacity: 10, TTL: time.Minute})
	cfg, ok := m.GetConfig(CacheLevelL1)
	if !ok || cfg.Capacity != 10 {
		t.Fatalf("config = %+v %v", cfg, ok)
	}

	// WarmUp / Promote（fromLevel < toLevel 才晋升）
	m.WarmUp(CacheLevelL1, map[string]int{"w": 100})
	if !m.Promote("w", CacheLevelL1, CacheLevelL2) {
		t.Fatal("promote")
	}
	if m.Promote("w", CacheLevelL2, CacheLevelL1) {
		t.Fatal("demote should fail")
	}

	// Has / GetOrElse
	if !m.Has("a") {
		t.Fatal("has")
	}
	v, err := m.GetOrElse("zzz", func() (int, error) { return 9, nil })
	if err != nil || v != 9 {
		t.Fatalf("getorelse = %v %v", v, err)
	}
	// Keys
	if len(m.Keys(CacheLevelL1)) == 0 {
		t.Fatal("keys")
	}
}
