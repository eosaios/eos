package lazy

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"testing"
	"time"
)

func TestLazyMapGetOrCreateAndPreload(t *testing.T) {
	calls := 0
	lm := NewLazyMap(func(k string) (string, error) {
		calls++
		return "v-" + k, nil
	})

	l := lm.GetOrCreate("a")
	if l == nil {
		t.Fatal("getorcreate")
	}
	if l2 := lm.GetOrCreate("a"); l2 != l {
		t.Fatal("same key same instance")
	}

	errs := lm.Preload("b", "c")
	if len(errs) != 2 || errs[0] != nil || errs[1] != nil {
		t.Fatalf("preload errs = %v", errs)
	}
	if calls < 2 {
		t.Fatalf("calls = %d", calls)
	}

	// Range
	n := 0
	lm.Range(func(k string, v *Lazy[string]) bool {
		n++
		return true
	})
	if n < 3 {
		t.Fatalf("range = %d", n)
	}

	// Evict：刚加载的不应驱逐
	if evicted := lm.Evict(time.Hour); evicted != 0 {
		t.Fatalf("evict fresh = %d", evicted)
	}
	// maxIdle=0 会驱除所有已加载（Evict 用严格 > 比较 idle，快平台上
	// 加载与驱逐可能落在同一时钟读数——先确保时钟推进）
	time.Sleep(2 * time.Millisecond)
	if evicted := lm.Evict(0); evicted == 0 {
		t.Fatal("evict idle")
	}
}

func TestLazyMapLoadError(t *testing.T) {
	boom := errors.New("boom")
	lm := NewLazyMap(func(k string) (int, error) {
		if k == "bad" {
			return 0, boom
		}
		return 1, nil
	})
	errs := lm.Preload("bad")
	if len(errs) != 1 || errs[0] == nil {
		t.Fatalf("preload err = %v", errs)
	}
}

func TestLazyStateAndReset(t *testing.T) {
	l := New(func() (string, error) { return "v", nil })
	if l.IsLoading() || l.IsFailed() || l.IsLoaded() {
		t.Fatal("initial state")
	}
	if l.State() != LoadStateNotLoaded {
		t.Fatalf("state = %v", l.State())
	}
	if _, ok := l.Peek(); ok {
		t.Fatal("peek empty")
	}
	if l.ValueOrZero() != "" {
		t.Fatal("zero value")
	}

	v, err := l.Get()
	if err != nil || v != "v" {
		t.Fatal(err)
	}
	if !l.IsLoaded() {
		t.Fatal("loaded")
	}
	if got, ok := l.Peek(); !ok || got != "v" {
		t.Fatalf("peek = %v %v", got, ok)
	}
	if l.ValueOrZero() != "v" {
		t.Fatal("value or zero")
	}
	if l.MustGet() != "v" {
		t.Fatal("mustget")
	}
	_ = l.LoadedAt()
	_ = l.LoadDuration()
	_ = l.GetResult()

	// Reset 丢弃缓存
	l.Reset()
	if l.IsLoaded() {
		t.Fatal("reset")
	}
	// SetLoader / callbacks
	l.SetLoader(func() (string, error) { return "w", nil })
	l.SetOnLoad(func(string) {})
	l.SetOnError(func(error) {})
	if v, err := l.Get(); err != nil || v != "w" {
		t.Fatalf("reloaded = %v %v", v, err)
	}

	// Failed state
	bad := New(func() (string, error) { return "", errors.New("x") })
	_, _ = bad.Get()
	if !bad.IsFailed() {
		t.Fatal("failed")
	}
	_ = bad.ValueOrZero()
}

func TestLazyMapMore(t *testing.T) {
	lm := NewLazyMap(func(k string) (int, error) { return len(k), nil })
	if lm.Has("a") {
		t.Fatal("empty has")
	}
	v, err := lm.Get("a")
	if err != nil || v != 1 {
		t.Fatalf("get = %v %v", v, err)
	}
	if !lm.Has("a") || !lm.IsLoaded("a") {
		t.Fatal("has/loaded")
	}
	lm.Delete("a")
	if lm.Has("a") {
		t.Fatal("deleted")
	}
	lm.GetOrCreate("b")
	if _, err := lm.Get("b"); err != nil {
		t.Fatal(err)
	}
	if lm.Size() != 1 {
		t.Fatalf("size = %d", lm.Size())
	}
	if len(lm.Keys()) != 1 {
		t.Fatal("keys")
	}
	lm.Clear()
	if lm.Size() != 0 {
		t.Fatal("clear")
	}
}
