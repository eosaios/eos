package settings

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeIntoScalarsAndSlices(t *testing.T) {
	dst := Settings{Language: "zh", MaxInjectKB: 10, Workspaces: []string{"a"}}
	src := Settings{
		Language:        "en",
		Theme:           "dark",
		MaxInjectKB:     20,
		AutoContext:     true,
		Trusted:         true,
		TrustedAt:       "now",
		MaxTurnTokens:   100,
		MaxSessionTokens: 200,
		PlanPromptStyle: "concise",
		PlanBubbleColor: "#fff",
		WatchMode:       "fs",
		WatchDebounceMs: 5,
		PollIntervalSec: 1,
		ActiveWorkspace: "/ws",
		DesktopNotifications: boolPtr(true),
		GitCommitReminder:     boolPtr(false),
		Workspaces:            []string{"a", "b"},
	}
	mergeInto(&dst, src)

	if dst.Language != "en" || dst.Theme != "dark" || dst.MaxInjectKB != 20 {
		t.Fatalf("scalars = %+v", dst)
	}
	if !dst.AutoContext || !dst.Trusted || dst.TrustedAt != "now" {
		t.Fatalf("flags = %+v", dst)
	}
	if dst.MaxTurnTokens != 100 || dst.MaxSessionTokens != 200 {
		t.Fatalf("tokens = %+v", dst)
	}
	if len(dst.Workspaces) != 2 {
		t.Fatalf("workspaces = %v", dst.Workspaces)
	}
	// 零值不覆盖
	dst2 := Settings{Language: "zh"}
	mergeInto(&dst2, Settings{})
	if dst2.Language != "zh" {
		t.Fatalf("zero = %+v", dst2)
	}
}

func TestMergeIntoAutoRulesAndPermissions(t *testing.T) {
	dst := Settings{
		AutoRules: []AutoRule{{Pattern: "p1"}},
		Permissions: &Permissions{
			AllowedTools: []string{"a"},
			Rules:        []PermissionRule{{Pattern: "r1", Decision: "allow"}},
		},
	}
	src := Settings{
		AutoRules: []AutoRule{{Pattern: "p1"}, {Pattern: "p2"}},
		Permissions: &Permissions{
			AllowedTools: []string{"b"},
			DeniedTools:  []string{"c"},
			Rules: []PermissionRule{
				{Pattern: "r1", Decision: "deny"},
				{Pattern: "r2", Decision: "allow"},
			},
		},
	}
	mergeInto(&dst, src)
	if len(dst.AutoRules) != 2 {
		t.Fatalf("rules = %v", dst.AutoRules)
	}
	if len(dst.Permissions.AllowedTools) != 1 || dst.Permissions.AllowedTools[0] != "b" {
		t.Fatalf("allowed = %v", dst.Permissions.AllowedTools)
	}
	if len(dst.Permissions.DeniedTools) != 1 {
		t.Fatal("denied")
	}
	if len(dst.Permissions.Rules) != 2 {
		t.Fatalf("perm rules = %v", dst.Permissions.Rules)
	}
	if dst.Permissions.Rules[0].Decision != "deny" {
		t.Fatalf("override = %+v", dst.Permissions.Rules[0])
	}

	// dst.Permissions 为 nil 时创建
	dst2 := Settings{}
	mergeInto(&dst2, Settings{Permissions: &Permissions{AllowedTools: []string{"x"}}})
	if dst2.Permissions == nil {
		t.Fatal("nil permissions")
	}
}

func TestMergePermissionRules(t *testing.T) {
	base := []PermissionRule{{Pattern: "a", Decision: "allow"}, {Pattern: "b", Decision: "allow"}}
	overlay := []PermissionRule{{Pattern: "b", Decision: "deny"}, {Pattern: "c", Decision: "allow"}}
	got := mergePermissionRules(base, overlay)
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	if got[1].Decision != "deny" {
		t.Fatalf("override = %+v", got[1])
	}
	if got[2].Pattern != "c" {
		t.Fatalf("append = %+v", got[2])
	}
	// 空 base
	if len(mergePermissionRules(nil, overlay)) != 2 {
		t.Fatal("nil base")
	}
	// 空 overlay
	if len(mergePermissionRules(base, nil)) != 2 {
		t.Fatal("nil overlay")
	}
}

func boolPtr(v bool) *bool { return &v }

func TestLoadLayerAndSaveErrors(t *testing.T) {
	// loadLayer 缺失文件 → 零值
	got := loadLayer(filepath.Join(t.TempDir(), "nope.json"))
	if got.Language != "" {
		t.Fatalf("missing = %+v", got)
	}
	// loadLayer 坏 JSON → 零值
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadLayer(bad); got.Language != "" {
		t.Fatalf("bad = %+v", got)
	}

	// Save 写失败（目录不可写）
	m := NewManager(filepath.Join(t.TempDir(), "no", "deep", "x.json"))
	// MkdirAll 会创建，成功
	if err := m.Save(&Settings{Language: "en"}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMergedLayerErrors(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "user.json")
	if err := os.WriteFile(user, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadMerged(user, filepath.Join(root, "proj"))
	if s == nil {
		t.Fatal("merged")
	}
}
