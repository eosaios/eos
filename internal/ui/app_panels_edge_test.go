package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/pkg/coreapi"
)

func TestParseContextPreviewLine(t *testing.T) {
	role, content := parseContextPreviewLine("user: hello world")
	if role != "user" || content != "hello world" {
		t.Fatalf("got %q/%q", role, content)
	}
	// 无冒号 → message
	role, content = parseContextPreviewLine("  plain  ")
	if role != "message" || content != "plain" {
		t.Fatalf("no colon = %q/%q", role, content)
	}
	// 空 role
	role, content = parseContextPreviewLine(": body")
	if role != "message" || content != "body" {
		t.Fatalf("empty role = %q/%q", role, content)
	}
	// 多冒号：只切第一个
	role, content = parseContextPreviewLine("sys: a:b")
	if role != "sys" || content != "a:b" {
		t.Fatalf("multi colon = %q/%q", role, content)
	}
}

func TestEstimateDisplayTokens(t *testing.T) {
	if estimateDisplayTokens("") != 0 || estimateDisplayTokens("   ") != 0 {
		t.Fatal("empty")
	}
	if estimateDisplayTokens("a") != 1 {
		t.Fatal("single rune")
	}
	// 约 4 字符/token（按 rune）
	if got := estimateDisplayTokens("abcd"); got != 1 {
		t.Fatalf("4 chars = %d", got)
	}
	if got := estimateDisplayTokens("abcde"); got != 2 {
		t.Fatalf("5 chars = %d", got)
	}
	if got := estimateDisplayTokens("中文中文"); got != 1 {
		t.Fatalf("cjk = %d", got)
	}
}

func TestRulesAndMemorySnapshotDocument(t *testing.T) {
	snap := coreapi.RulesSnapshot{
		Documents: []coreapi.RuleDocument{
			{Scope: "project", Content: "p", Path: "/p"},
			{Scope: "Global", Content: "g", Path: "/g"},
		},
	}
	doc := rulesSnapshotDocument(snap, "PROJECT")
	if doc.Content != "p" {
		t.Fatalf("project = %+v", doc)
	}
	doc = rulesSnapshotDocument(snap, "global")
	if doc.Content != "g" {
		t.Fatalf("global = %+v", doc)
	}
	// 缺失 → 仅含 scope
	doc = rulesSnapshotDocument(snap, "missing")
	if doc.Content != "" || doc.Scope != "missing" {
		t.Fatalf("missing = %+v", doc)
	}

	msnap := coreapi.MemorySnapshot{
		Documents: []coreapi.MemoryDocument{
			{Scope: "memory_summary.md", Content: "s", Exists: true},
			{Scope: "MEMORY.md", Content: "h", Exists: true},
		},
	}
	d := memorySnapshotDocument(msnap, "MEMORY.MD")
	if d.Content != "h" {
		t.Fatalf("handbook = %+v", d)
	}
	// 按 scopes 顺序取首个命中
	d = memorySnapshotDocument(msnap, "nope", "memory_summary.md")
	if d.Content != "s" {
		t.Fatalf("fallback scope = %+v", d)
	}
	// 全缺失
	d = memorySnapshotDocument(msnap, "zzz")
	if d.Scope != "zzz" || d.Content != "" {
		t.Fatalf("zero = %+v", d)
	}
	// 无 scopes
	d = memorySnapshotDocument(msnap)
	if d.Scope != "" {
		t.Fatalf("empty scopes = %+v", d)
	}

	// panelMemoryDoc 投影
	pd := panelMemoryDoc(coreapi.MemoryDocument{Scope: "x", Path: "p", Content: "c", Exists: true})
	if pd.Scope != "x" || pd.Path != "p" || pd.Content != "c" || !pd.Exists {
		t.Fatalf("panel doc = %+v", pd)
	}
}

func TestPanelProjectMemoryAndScopes(t *testing.T) {
	if panelProjectMemory(coreapi.MemorySnapshot{}) != nil {
		t.Fatal("empty projects")
	}

	scopes := memoryDocScopesList()
	if scopes[0] != "memory_summary.md" || scopes[1] != "MEMORY.md" {
		t.Fatalf("scopes = %v", scopes)
	}

	pm := panelProjectMemory(coreapi.MemorySnapshot{
		Projects: []coreapi.ProjectMemorySummary{
			{
				Key:  "k",
				Root: "/r",
				Name: "n",
				Documents: []coreapi.MemoryDocument{
					{Scope: "memory_summary.md", Content: "s"},
					{Scope: "MEMORY.md", Content: "h"},
				},
			},
		},
	})
	if pm == nil || pm.Key != "k" || pm.Name != "n" {
		t.Fatalf("project = %+v", pm)
	}
	if pm.Docs[0].Content != "s" || pm.Docs[1].Content != "h" {
		t.Fatalf("docs = %+v", pm.Docs)
	}

	// memorySnapshotDocumentOf 缺失回落 scope
	d := memorySnapshotDocumentOf(nil, "MEMORY.md")
	if d.Scope != "memory.md" {
		// ToLower 后
		if !strings.EqualFold(d.Scope, "MEMORY.md") {
			t.Fatalf("of = %+v", d)
		}
	}
}

func TestOverlayCenter(t *testing.T) {
	out := overlayCenter(40, 10, "bg", "POPUP")
	if !strings.Contains(out, "POPUP") {
		t.Fatalf("overlay = %q", out)
	}
}

func TestAggregateCostItemsAndOptionalInt(t *testing.T) {
	n := func(v int) *int { return &v }
	items := []coreapi.CostItem{
		{Model: "beta", InputTokens: n(10), ReplyTokens: n(5), TotalTokens: n(15)},
		{Model: "beta", InputTokens: n(1), ReplyTokens: n(2), TotalTokens: n(3)},
		{Model: "  ", InputTokens: n(7), ReplyTokens: nil, TotalTokens: n(7)},
		{Model: "alpha", InputTokens: nil, ReplyTokens: n(9), TotalTokens: n(9)},
	}
	out := aggregateCostItemsByModel(items)
	// 聚合键是原样模型名（空名 → unknown）；按小写排序
	if len(out) != 3 {
		t.Fatalf("agg = %+v", out)
	}
	if out[0].Model != "alpha" || out[1].Model != "beta" || out[2].Model != "unknown" {
		t.Fatalf("order = %+v", out)
	}
	if out[1].Rounds != 2 || *out[1].Input != 11 || *out[1].Reply != 7 || *out[1].Total != 18 {
		t.Fatalf("beta agg = %+v", out[1])
	}
	if out[2].Rounds != 1 || *out[2].Input != 7 || out[2].Reply != nil {
		t.Fatalf("unknown agg = %+v", out[2])
	}
	if out[0].Input != nil {
		t.Fatalf("alpha input should stay nil: %+v", out[0])
	}

	// addOptionalInt
	if addOptionalInt(nil, nil) != nil {
		t.Fatal("nil+nil")
	}
	if addOptionalInt(n(1), nil) == nil || *addOptionalInt(n(1), nil) != 1 {
		t.Fatal("keep total")
	}
	if got := addOptionalInt(nil, n(2)); got == nil || *got != 2 {
		t.Fatal("start from value")
	}
	if got := addOptionalInt(n(1), n(2)); got == nil || *got != 3 {
		t.Fatal("sum")
	}
}

func TestOptionalIntLabel(t *testing.T) {
	setTestHome(t)
	m := newTestAppModel(t)
	if got := m.optionalIntLabel(nil); got != "未知" {
		t.Fatalf("nil label = %q", got)
	}
	v := 42
	if got := m.optionalIntLabel(&v); got != "42" {
		t.Fatalf("val = %q", got)
	}
	m.state.Language = "en"
	if got := m.optionalIntLabel(nil); got != "unknown" {
		t.Fatalf("en = %q", got)
	}
}

func TestHandleGitSummaryMsgAndThrottle(t *testing.T) {
	setTestHome(t)
	m := newTestAppModel(t)

	next, _ := m.handleGitSummaryMsg(GitSummaryMsg{Branch: "main", Dirty: 2, Ahead: 1})
	if next == nil {
		t.Fatal("handleGitSummaryMsg")
	}

	// 首次调度
	cmd := m.maybeRefreshGitSummary()
	if cmd == nil {
		t.Fatal("first refresh should schedule")
	}
	// 节流窗口内不重复
	if cmd := m.maybeRefreshGitSummary(); cmd != nil {
		t.Fatal("throttled")
	}
	// 工作区变化立即刷新
	m.gitSummaryRoot = "other"
	if cmd := m.maybeRefreshGitSummary(); cmd == nil {
		t.Fatal("workspace change should refresh")
	}
}

func TestMemoryDocScopesListOrder(t *testing.T) {
	if memoryDocScopesList() != [2]string{"memory_summary.md", "MEMORY.md"} {
		t.Fatalf("scopes = %v", memoryDocScopesList())
	}
	_ = panels.MemoryDoc{}
}
