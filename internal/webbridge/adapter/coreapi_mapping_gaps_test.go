package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// coreapi_mapping/core 辅助余臂批测：快照全集合映射（会话/消息/任务/
// 代理）、模型目录嵌套映射、指针克隆族、消息双向映射、模式归一矩阵、
// metadata 深克隆值类型。

import (
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestRuntimeSnapshotFromCoreAPICollections(t *testing.T) {
	current := coreapi.SessionSnapshot{ID: "s-cur", WorkspacePath: "/ws"}
	snap := coreapi.StateSnapshot{
		ForegroundWorkspace: "/ws",
		Sessions:            []coreapi.SessionSnapshot{{ID: "s-1"}, {ID: "s-2"}},
		CurrentSession:      &current,
		Messages: []coreapi.SessionMessage{
			{Role: "user", Content: "hi", ImagePaths: []string{"a.png"}, Metadata: map[string]any{"k": "v"}},
		},
		Tasks: []coreapi.TaskSnapshot{{ID: "t-1", Status: "running", CanKill: true}},
		Agents: []coreapi.Agent{
			{ID: "a-1", ParentAgentID: "p", RoleID: "r", Task: " job ", Status: " running "},
		},
	}
	out := runtimeSnapshotFromCoreAPI(snap)

	if out.ForegroundWorkspace != "/ws" || len(out.Sessions) != 2 {
		t.Fatalf("快照头 = %+v", out)
	}
	if out.CurrentSession == nil || out.CurrentSession.ID != "s-cur" {
		t.Fatalf("current = %+v", out.CurrentSession)
	}
	if len(out.Messages) != 1 || out.Messages[0].Metadata["k"] != "v" {
		t.Fatalf("messages = %+v", out.Messages)
	}
	if len(out.Tasks) != 1 || !out.Tasks[0].CanKill {
		t.Fatalf("tasks = %+v", out.Tasks)
	}
	if len(out.Agents) != 1 || out.Agents[0].Status != "running" || out.Agents[0].Task != "job" {
		t.Fatalf("agents = %+v", out.Agents)
	}
}

func TestCostItemsAndBackgroundTasksMapping(t *testing.T) {
	ip := 11
	fp := 1.5
	items := costItemsFromCoreAPI([]coreapi.CostItem{{
		Model: " m ", InputTokens: &ip, ReplyTokens: &ip,
		CachedInputTokens: &ip, TotalTokens: &ip, ContextInputTokens: &ip,
		CostUSD: &fp, UsageKnown: true, CostKnown: true,
	}})
	if len(items) != 1 || items[0].Model != "m" || *items[0].InputTokens != 11 || *items[0].CostUSD != 1.5 {
		t.Fatalf("cost items = %+v", items)
	}
	// nil 指针族。
	if items := costItemsFromCoreAPI([]coreapi.CostItem{{}}); len(items) != 1 || items[0].InputTokens != nil || items[0].CostUSD != nil {
		t.Fatalf("nil 指针 = %+v", items)
	}
	// 后台任务映射（trim）。
	tasks := BackgroundTasksFromCoreAPI([]coreapi.TaskSnapshot{{ID: " t ", Status: " running ", Label: " job "}})
	if len(tasks) != 1 || tasks[0].ID != "t" || tasks[0].Status != "running" || tasks[0].Label != "job" {
		t.Fatalf("background = %+v", tasks)
	}
	// 版本项映射。
	versions := versionItemsFromCoreAPI([]coreapi.VersionItem{{ID: " v ", File: " f "}})
	if len(versions) != 1 || versions[0].ID != "v" {
		t.Fatalf("versions = %+v", versions)
	}
}

func TestClonePtrFamily(t *testing.T) {
	if cloneIntPtr(nil) != nil || cloneFloatPtr(nil) != nil {
		t.Fatal("nil 直通")
	}
	v := 42
	if got := cloneIntPtr(&v); *got != 42 {
		t.Fatal("int 克隆")
	}
	f := 2.5
	if got := cloneFloatPtr(&f); *got != 2.5 {
		t.Fatal("float 克隆")
	}
}

func TestSessionMessagesBidirectional(t *testing.T) {
	msgs := []SessionMessage{{
		Role: "user", Content: "hi", ImagePaths: []string{"a"},
		Metadata: map[string]any{"k": "v"},
	}}
	round := sessionMessagesFromCoreAPI(coreAPISessionMessages(msgs))
	if len(round) != 1 || round[0].Content != "hi" || round[0].Metadata["k"] != "v" {
		t.Fatalf("往返 = %+v", round)
	}
	if msgs[0].Metadata["k"] != "v" {
		t.Fatal("原 metadata 不应被改写")
	}
}

func TestModelCatalogNestedMapping(t *testing.T) {
	catalog := coreapi.ModelCatalogState{
		Providers: []coreapi.ModelProviderOption{{
			ID: " p ", Name: " n ", Website: " w ", APIKeyEnv: " K ",
			Endpoints:     []coreapi.ProviderEndpoint{{Plan: " free ", Format: " openai ", APIBase: " https://api "}},
			DefaultModels: []string{"m1"},
		}},
		Presets: []coreapi.ModelPresetOption{{
			ID: " pre ", Name: " nm ", ProviderID: " p ", ModelName: " m ",
			Tags: []string{"t"}, SupportsReasoningEffort: true,
			ReasoningLevels: []string{"low", "high"}, SupportsVision: true,
			PlanModels: []coreapi.PlanModel{{
				ModelID: " m ", Label: " L ", ContextWindow: 128000,
			}},
		}},
		AllowCustomProvider: true,
	}
	out := modelCatalogFromCoreAPI(catalog)
	if len(out.Providers) != 1 || out.Providers[0].ID != "p" {
		t.Fatalf("providers = %+v", out.Providers)
	}
	if len(out.Providers[0].Endpoints) != 1 || out.Providers[0].Endpoints[0].APIBase != "https://api" {
		t.Fatalf("endpoints = %+v", out.Providers[0].Endpoints)
	}
	if len(out.Presets) != 1 || len(out.Presets[0].PlanModels) != 1 {
		t.Fatalf("presets = %+v", out.Presets)
	}
	if pm := out.Presets[0].PlanModels[0]; pm.ContextWindow != 128000 {
		t.Fatalf("plan model = %+v", pm)
	}
	if p := out.Presets[0]; p.ID != "pre" || p.Name != "nm" || len(p.Tags) != 1 ||
		len(p.ReasoningLevels) != 2 || !p.SupportsVision {
		t.Fatalf("preset = %+v", p)
	}
}

func TestNormalizeModesMatrix(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plan", "plan"}, {" PLAN ", "plan"}, {"计划优先", "plan"}, {"先出计划", "plan"},
		{"auto", "auto"}, {"", "auto"}, {"bogus", "auto"},
	} {
		if got := normalizeExecutionMode(tc.in); got != tc.want {
			t.Fatalf("exec(%q) = %q", tc.in, got)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"read-only", "read-only"}, {"readonly", "read-only"}, {"ro", "read-only"},
		{"danger-full-access", "danger-full-access"}, {"full_access", "danger-full-access"},
		{"full", "danger-full-access"}, {"完全访问", "danger-full-access"},
		{"workspace-write", "workspace-write"}, {"workspace", "workspace-write"}, {"ww", "workspace-write"},
		{"", "workspace-write"}, {"bogus", "workspace-write"},
	} {
		if got := normalizeSandboxMode(tc.in); got != tc.want {
			t.Fatalf("sandbox(%q) = %q", tc.in, got)
		}
	}
}

func TestCloneSessionMessageMetadataValues(t *testing.T) {
	inner := map[string]any{"deep": "v"}
	meta := map[string]any{
		"str": "s", "num": 1.5, "b": true,
		"list": []any{"x"}, "map": inner,
	}
	cloned := cloneSessionMessageMetadata(meta)
	if cloned["str"] != "s" || cloned["num"] != 1.5 || cloned["b"] != true {
		t.Fatalf("标量克隆 = %v", cloned)
	}
	// 列表与嵌套 map 深克隆。
	list, _ := cloned["list"].([]any)
	if len(list) != 1 || list[0] != "x" {
		t.Fatalf("列表克隆 = %#v", cloned["list"])
	}
	m, _ := cloned["map"].(map[string]any)
	if m["deep"] != "v" {
		t.Fatalf("嵌套 map = %#v", cloned["map"])
	}
	// 改克隆不回写原。
	m["deep"] = "changed"
	if inner["deep"] != "v" {
		t.Fatal("深克隆失败：原嵌套被改写")
	}
	if cloneSessionMessageMetadata(nil) != nil {
		t.Fatal("nil 元数据应 nil")
	}
}
