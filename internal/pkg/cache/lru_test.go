package cache

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLRUCachePutGetRoundTrip(t *testing.T) {
	c := NewLRUCache[string, int](4)
	c.Put("a", 1)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %d,%v; want 1,true", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("Get(missing) should miss")
	}
	if _, ok := c.Peek("missing"); ok {
		t.Fatal("Peek(missing) should miss")
	}
}

// 容量满时驱逐最久未使用的键。
func TestLRUCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewLRUCache[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // 驱逐 a

	if c.Contains("a") {
		t.Fatal("a should be evicted")
	}
	if got := c.Keys(); len(got) != 2 || got[0] != "c" || got[1] != "b" {
		t.Fatalf("Keys() = %v; want [c b] (MRU 在前)", got)
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("evicted key must not resolve")
	}
}

// Get 会把条目移到队首，改变后续驱逐顺序。
func TestLRUCacheGetRefreshesRecency(t *testing.T) {
	c := NewLRUCache[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("Get(a) should hit")
	}
	c.Put("c", 3) // 驱逐 b（a 刚被访问过）

	if c.Contains("b") {
		t.Fatal("b should be evicted after a was refreshed")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("a should survive; got %d,%v", v, ok)
	}
}

// Peek 只读不改变 LRU 顺序。
func TestLRUCachePeekKeepsRecency(t *testing.T) {
	c := NewLRUCache[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	if v, ok := c.Peek("a"); !ok || v != 1 {
		t.Fatalf("Peek(a) = %d,%v; want 1,true", v, ok)
	}
	c.Put("c", 3) // a 未被 Peek 提升，仍是被驱逐对象

	if c.Contains("a") {
		t.Fatal("Peek must not refresh recency; a should be evicted")
	}
}

func TestLRUCacheTTLExpiry(t *testing.T) {
	c := NewLRUCacheWithTTL[string, int](4, 30*time.Millisecond)
	c.Put("a", 1)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("fresh entry should hit")
	}
	time.Sleep(50 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Fatal("expired entry must miss on Get")
	}
	if c.Has("a") {
		t.Fatal("expired entry must not be reported by Has")
	}

	c.Put("b", 2)
	time.Sleep(50 * time.Millisecond)
	if n := c.CleanExpired(); n != 1 {
		t.Fatalf("CleanExpired() = %d; want 1", n)
	}
	if c.Len() != 0 {
		t.Fatalf("Len() = %d after CleanExpired; want 0", c.Len())
	}
}

func TestLRUCachePutWithExpiration(t *testing.T) {
	c := NewLRUCache[string, int](4) // 无全局 TTL
	c.PutWithExpiration("a", 1, 20*time.Millisecond)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("fresh per-entry expiration should hit")
	}
	time.Sleep(40 * time.Millisecond)
	// 条目仍在 map 中（未触发惰性删除），但 Has/Peek 必须按过期判定。
	if c.Has("a") {
		t.Fatal("expired per-entry entry must not be visible")
	}
	if _, ok := c.Peek("a"); ok {
		t.Fatal("Peek must respect per-entry expiration")
	}
}

func TestLRUCacheStatsAndHitRate(t *testing.T) {
	c := NewLRUCache[string, int](4)
	c.Put("a", 1)
	c.Get("a") // hit
	c.Get("x") // miss
	c.Get("y") // miss

	want := CacheStats{Hits: 1, Misses: 2, Evictions: 0, Puts: 1, Gets: 3}
	if got := c.Stats(); got != want {
		t.Fatalf("Stats() = %+v; want %+v", got, want)
	}
	if r := c.HitRate(); r != 1.0/3.0 {
		t.Fatalf("HitRate() = %v; want %v", r, 1.0/3.0)
	}
	c.ResetStats()
	if got := c.Stats(); got != (CacheStats{}) {
		t.Fatalf("Stats() after reset = %+v; want zero", got)
	}
}

func TestLRUCacheEvictCallbackAndResize(t *testing.T) {
	var evicted []string
	c := NewLRUCache[string, int](4)
	c.SetOnEvict(func(k string, _ int) { evicted = append(evicted, k) })

	for i := 0; i < 4; i++ {
		c.Put(fmt.Sprintf("k%d", i), i)
	}
	c.Resize(2) // 驱逐 k0 k1
	if got := c.Size(); got != 2 {
		t.Fatalf("Size() after Resize = %d; want 2", got)
	}
	if len(evicted) != 2 || evicted[0] != "k0" || evicted[1] != "k1" {
		t.Fatalf("evicted = %v; want [k0 k1]", evicted)
	}

	c.Delete("k2")
	if c.Delete("k2") {
		t.Fatal("second Delete of same key must return false")
	}
	c.Clear()
	if !c.IsEmpty() {
		t.Fatal("Clear() must empty the cache")
	}
}

func TestLRUCacheGetOrElse(t *testing.T) {
	c := NewLRUCache[string, int](4)
	loaded := 0
	load := func() (int, error) { loaded++; return 42, nil }

	if v, err := c.GetOrElse("k", load); err != nil || v != 42 {
		t.Fatalf("GetOrElse miss-load = %d,%v; want 42,nil", v, err)
	}
	if v, err := c.GetOrElse("k", load); err != nil || v != 42 {
		t.Fatalf("GetOrElse hit = %d,%v; want 42,nil", v, err)
	}
	if loaded != 1 {
		t.Fatalf("load executed %d times; want 1 (second call must hit cache)", loaded)
	}

	boom := fmt.Errorf("boom")
	if _, err := c.GetOrElse("err", func() (int, error) { return 0, boom }); err != boom {
		t.Fatalf("GetOrElse error passthrough = %v; want boom", err)
	}
	if c.Has("err") {
		t.Fatal("failed load must not populate cache")
	}
}

// 并发读写冒烟：数百 goroutine 混合 Put/Get/Delete 不死锁不 panic。
func TestLRUCacheConcurrentAccess(t *testing.T) {
	c := NewLRUCache[int, int](64)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				key := (g*200 + i) % 100
				c.Put(key, key)
				c.Get(key)
				if i%10 == 0 {
					c.Delete(key)
				}
			}
		}(g)
	}
	wg.Wait()
	if c.Size() > c.Capacity() {
		t.Fatalf("Size() = %d exceeds capacity %d", c.Size(), c.Capacity())
	}
}
