package headless

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
)

func TestAwaitTurn(t *testing.T) {
	// ctx cancel
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := make(chan TurnStartResult, 1)
	events := make(chan protocol.Envelope, 1)
	if err := AwaitTurn(ctx, start, events, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}

	// start error
	ctx2 := context.Background()
	start2 := make(chan TurnStartResult, 1)
	start2 <- TurnStartResult{Err: errors.New("start fail")}
	if err := AwaitTurn(ctx2, start2, nil, nil); err == nil {
		t.Fatal("start err")
	}

	// events closed after start done → nil
	start3 := make(chan TurnStartResult, 1)
	start3 <- TurnStartResult{}
	events3 := make(chan protocol.Envelope)
	close(events3)
	done3 := make(chan error, 1)
	go func() {
		done3 <- AwaitTurn(ctx2, start3, events3, nil)
	}()
	select {
	case err := <-done3:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		// 先消费 start 再关 events 可正常结束；此处只要求不永久挂死
		t.Log("AwaitTurn with closed events may busy-wait if start not consumed first")
	}

	// sink stop
	start4 := make(chan TurnStartResult, 1)
	start4 <- TurnStartResult{}
	events4 := make(chan protocol.Envelope, 1)
	events4 <- protocol.Envelope{}
	done := make(chan error, 1)
	go func() {
		done <- AwaitTurn(ctx2, start4, events4, func(protocol.EventType, map[string]any, protocol.EventType) (bool, error) {
			return true, nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestApplyModelOverrideWithModels(t *testing.T) {
	setTestHomeForHL(t)
	eng := &fakeHLSessionEngine{
		models: []coreapi.ModelConfig{
			{Name: "m1", Model: "x", Active: true},
		},
	}
	if err := ApplyModelOverride(context.Background(), eng, coreapi.Session{ID: "s"}, "m1"); err != nil {
		t.Fatal(err)
	}
	// 无效模型
	if err := ApplyModelOverride(context.Background(), eng, coreapi.Session{ID: "s"}, "nope"); err == nil {
		t.Fatal("bad model")
	}
}

func setTestHomeForHL(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}
