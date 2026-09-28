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
