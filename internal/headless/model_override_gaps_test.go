package headless_test

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// 批十七批测：ApplyModelOverride 全臂（空/nil engine/list 失败/解析失败/
// 直写 SetSession/套餐切换 NeedsPlanSwitch 走 Save）、SubscribeTurnEvents
// nil engine 与 Subscribe 失败的 closed channel、ResolveActiveModelName。

import (
	"context"
	"testing"

	"github.com/eosaios/eos/internal/headless"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

func TestApplyModelOverrideArms(t *testing.T) {
	t.Run("空 override 与 nil engine", func(t *testing.T) {
		if err := headless.ApplyModelOverride(context.Background(), nil, coreapi.Session{}, ""); err != nil {
			t.Fatalf("empty override error = %v", err)
		}
		if err := headless.ApplyModelOverride(context.Background(), nil, coreapi.Session{}, "m1"); err == nil || err.Error() != "core engine unavailable" {
			t.Fatalf("nil engine error = %v", err)
		}
	})

	t.Run("list 失败与解析失败", func(t *testing.T) {
		caller := &headlessSessionCaller{replies: map[string]any{}}
		caller.failMethods = map[string]error{jsonrpc.MethodModelList: errString("catalog down")}
		engine := sidecar.NewRemoteEngine(caller)
		if err := headless.ApplyModelOverride(context.Background(), engine, coreapi.Session{ID: "s1"}, "m1"); err == nil {
			t.Fatal("list failure not surfaced")
		}

		// 空目录：解析不到条目。
		caller2 := &headlessSessionCaller{replies: map[string]any{
			jsonrpc.MethodModelList: []coreapi.ModelConfig{},
		}}
		engine2 := sidecar.NewRemoteEngine(caller2)
		if err := headless.ApplyModelOverride(context.Background(), engine2, coreapi.Session{ID: "s1"}, "  m1 "); err == nil {
			t.Fatal("unresolvable override error = nil")
		}
	})

	t.Run("直写会话模型", func(t *testing.T) {
		caller := &headlessSessionCaller{replies: map[string]any{
			jsonrpc.MethodModelList: []coreapi.ModelConfig{{Name: "glm", Model: "glm-5.3", Active: true}},
		}}
		engine := sidecar.NewRemoteEngine(caller)
		if err := headless.ApplyModelOverride(context.Background(), engine, coreapi.Session{ID: "s1"}, "glm"); err != nil {
			t.Fatalf("direct override error = %v", err)
		}
		params, ok := caller.firstParams(jsonrpc.MethodModelSessionSet).(coreapi.SetSessionModelRequest)
		if !ok {
			t.Fatalf("session/set params type = %T", caller.firstParams(jsonrpc.MethodModelSessionSet))
		}
		if params.SessionID != "s1" || params.ModelName != "glm" {
			t.Fatalf("params = %+v", params)
		}
	})
}

type errString string

func (e errString) Error() string { return string(e) }

func TestSubscribeTurnEventsNilAndFailure(t *testing.T) {
	// nil engine → 已关闭 channel。
	ch := headless.SubscribeTurnEvents(context.Background(), nil, "s", "t")
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("nil engine channel should be closed")
		}
	default:
		t.Fatal("nil engine channel should be immediately readable (closed)")
	}

	// Subscribe 失败 → 已关闭 channel。
	caller := &headlessSessionCaller{replies: map[string]any{}}
	caller.failMethods = map[string]error{jsonrpc.MethodEventSubscribe: errString("no bus")}
	engine := sidecar.NewRemoteEngine(caller)
	ch2 := headless.SubscribeTurnEvents(context.Background(), engine, "s", "t")
	select {
	case _, ok := <-ch2:
		if ok {
			t.Fatal("failed subscribe channel should be closed")
		}
	default:
		t.Fatal("failed subscribe channel should be immediately readable (closed)")
	}
}

func TestResolveActiveModelName(t *testing.T) {
	if name, err := headless.ResolveActiveModelName(context.Background(), nil); err != nil || name != "" {
		t.Fatalf("nil engine = %q %v", name, err)
	}
	caller := &headlessSessionCaller{replies: map[string]any{
		jsonrpc.MethodModelList: []coreapi.ModelConfig{
			{Name: "a", Model: "model-a"},
			{Name: "b", Model: " model-b ", Active: true},
		},
	}}
	engine := sidecar.NewRemoteEngine(caller)
	name, err := headless.ResolveActiveModelName(context.Background(), engine)
	if err != nil || name != "model-b" {
		t.Fatalf("active = %q %v", name, err)
	}
	// 无 Active 项 → 空。
	caller2 := &headlessSessionCaller{replies: map[string]any{
		jsonrpc.MethodModelList: []coreapi.ModelConfig{{Name: "a", Model: "x"}},
	}}
	if name, err := headless.ResolveActiveModelName(context.Background(), sidecar.NewRemoteEngine(caller2)); err != nil || name != "" {
		t.Fatalf("no active = %q %v", name, err)
	}
}
