package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"testing"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/pkg/coreapi"
)

func TestSelectModelForCurrentContextScopes(t *testing.T) {
	// 无会话、有前台工作区 → workspace
	eng := &fakeEngine{
		state: coreapi.StateSnapshot{ForegroundWorkspace: "/ws"},
	}
	a := NewCoreClientAdapterFromEngine(eng)
	defer func() { _ = a.Close() }()
	scope, err := a.SelectModelForCurrentContext(context.Background(), "m1")
	if err != nil || scope != "workspace" {
		t.Fatalf("workspace scope = %q %v", scope, err)
	}

	// 无会话、无工作区 → global
	a3 := NewCoreClientAdapterFromEngine(&fakeEngine{})
	defer func() { _ = a3.Close() }()
	scope, err = a3.SelectModelForCurrentContext(nil, "m2") // ctx nil 回落 Background
	if err != nil || scope != "global" {
		t.Fatalf("global scope = %q %v", scope, err)
	}

	// 空名
	if _, err := a.SelectModelForCurrentContext(context.Background(), "  "); err == nil {
		t.Fatal("empty name")
	}

	// SelectWorkspaceModel 无工作区 → no-op nil
	if err := a3.SelectWorkspaceModel(context.Background(), "m"); err != nil {
		t.Fatalf("no ws = %v", err)
	}
	// 有工作区
	if err := a.SelectWorkspaceModel(context.Background(), "m"); err != nil {
		t.Fatal(err)
	}
	if err := a.SelectWorkspaceModel(context.Background(), " "); err == nil {
		t.Fatal("empty name")
	}
}

func TestSwitchPlanModelValidation(t *testing.T) {
	a := NewCoreClientAdapterFromEngine(&fakeEngine{})
	defer func() { _ = a.Close() }()

	if err := a.SwitchPlanModel(context.Background(), "", ""); err == nil {
		t.Fatal("empty args")
	}

	eng := &fakeEngine{
		models: []coreapi.ModelConfig{{Name: "plain-model", Model: "x"}},
	}
	a2 := NewCoreClientAdapterFromEngine(eng)
	defer func() { _ = a2.Close() }()
	if err := a2.SwitchPlanModel(context.Background(), "missing", "pm"); err == nil {
		t.Fatal("missing entry")
	}
	if err := a2.SwitchPlanModel(context.Background(), "plain-model", "pm"); err == nil {
		t.Fatal("no preset")
	}

	// 有 preset → 走 SaveModel 成功
	eng2 := &fakeEngine{
		models: []coreapi.ModelConfig{{Name: "plan-e", Model: "x", PresetID: "p1", ProviderID: "demo"}},
	}
	a3 := NewCoreClientAdapterFromEngine(eng2)
	defer func() { _ = a3.Close() }()
	if err := a3.SwitchPlanModel(context.Background(), "plan-e", "pm-2"); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertSaveAndCatalog(t *testing.T) {
	eng := &fakeEngine{}
	a := NewCoreClientAdapterFromEngine(eng)
	defer func() { _ = a.Close() }()

	if err := a.UpsertModelEntry(context.Background(), config.ModelEntry{Name: "n", APIKey: "sk-real"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveModel(context.Background(), coreapi.ModelSaveRequest{Name: "n", Mode: "custom_model"}); err != nil {
		t.Fatal(err)
	}
	if a.ModelCatalogSnapshot(context.Background()) == nil {
		// fake Catalog 返回零值 state，指针非 nil
	}

	var nilA *CoreClientAdapter
	if err := nilA.UpsertModelEntry(context.Background(), config.ModelEntry{}); err == nil {
		t.Fatal("nil upsert")
	}
	if err := nilA.SaveModel(context.Background(), coreapi.ModelSaveRequest{}); err == nil {
		t.Fatal("nil save")
	}
	if nilA.ModelCatalogSnapshot(context.Background()) != nil {
		t.Fatal("nil catalog")
	}
}
