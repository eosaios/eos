package lazy

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLoadStateString(t *testing.T) {
	cases := map[LoadState]string{
		LoadStateNotLoaded: "not_loaded",
		LoadStateLoading:   "loading",
		LoadStateLoaded:    "loaded",
		LoadStateFailed:    "failed",
		LoadState(99):      "unknown",
	}
	for in, want := range cases {
		if got := in.String(); got != want {
			t.Fatalf("String(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestLazyGetSuccessAndCache(t *testing.T) {
	calls := 0
	l := New(func() (string, error) {
		calls++
		return "value", nil
	})
	if l.IsLoaded() {
		t.Fatal("not loaded yet")
	}
	v, err := l.Get()
	if err != nil || v != "value" {
		t.Fatalf("Get = %q %v", v, err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
	// 缓存命中
	if _, err := l.Get(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("cached calls = %d", calls)
	}
	if err := l.Load(); err != nil {
		t.Fatal(err)
	}
}

func TestLazyGetErrorAndCallbacks(t *testing.T) {
	boom := errors.New("boom")
	var gotErr error
	var gotVal string
	l := NewWithCallbacks(
		func() (string, error) { return "", boom },
		func(v string) { gotVal = v },
		func(err error) { gotErr = err },
	)
	_, err := l.Get()
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if gotErr == nil {
		t.Fatal("onError")
	}
	if gotVal != "" {
		t.Fatal("onLoad should not fire")
	}
	if l.IsLoaded() {
		t.Fatal("failed is not loaded")
	}

	// 成功回调
	l2 := NewWithCallbacks(
		func() (string, error) { return "ok", nil },
		func(v string) { gotVal = v },
		func(err error) { gotErr = err },
	)
	if _, err := l2.Get(); err != nil {
		t.Fatal(err)
	}
	if gotVal != "ok" {
		t.Fatalf("onLoad = %q", gotVal)
	}
}

func TestLazyGetWithContext(t *testing.T) {
	l := New(func() (string, error) { return "x", nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := l.GetWithContext(ctx)
	if err != nil || v != "x" {
		t.Fatalf("ctx get = %q %v", v, err)
	}

	// 已加载后直接返回
	if v, err := l.GetWithContext(ctx); err != nil || v != "x" {
		t.Fatalf("cached ctx = %q %v", v, err)
	}

	// 取消
	block := make(chan struct{})
	l2 := New(func() (string, error) {
		<-block
		return "late", nil
	})
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel2()
	}()
	_, err = l2.GetWithContext(ctx2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err = %v", err)
	}
	close(block)
}
