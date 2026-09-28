package modes

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestNormalizeExecutionMode(t *testing.T) {
	cases := map[string]string{
		"":             "auto",
		"plan":         "plan",
		"PLAN":         "plan",
		"plan_first":   "plan",
		"计划优先":       "plan",
		"auto":         "auto",
		"auto-mode":    "auto",
		"自动":          "auto",
		"unknown-xyz":  "auto",
	}
	for in, want := range cases {
		if got := NormalizeExecutionMode(in); got != want {
			t.Fatalf("exec(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAccessMode(t *testing.T) {
	cases := map[string]string{
		"":                  "workspace-write",
		"read-only":         "read-only",
		"readonly":          "read-only",
		"只读":               "read-only",
		"workspace-write":   "workspace-write",
		"workspace":         "workspace-write",
		"danger-full-access": "danger-full-access",
		"full":              "danger-full-access",
		"完全访问":            "danger-full-access",
		"nope":              "workspace-write",
	}
	for in, want := range cases {
		if got := NormalizeAccessMode(in); got != want {
			t.Fatalf("access(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeApprovalMode(t *testing.T) {
	cases := map[string]string{
		"":            "on-request",
		"on-request":  "on-request",
		"untrusted":   "untrusted",
		"never":       "never",
		"从不审批":       "never",
		"nope":        "on-request",
	}
	for in, want := range cases {
		if got := NormalizeApprovalMode(in); got != want {
			t.Fatalf("approval(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeSandboxModeAndResolveAccess(t *testing.T) {
	if got := NormalizeSandboxMode(""); got != "workspace-write" {
		t.Fatalf("empty sandbox = %q", got)
	}
	if got := NormalizeSandboxMode("read-only"); got != "read-only" {
		t.Fatalf("sandbox = %q", got)
	}

	if got := ResolveAccessMode(ExecSession{AccessMode: "read-only"}); got != "read-only" {
		t.Fatalf("access = %q", got)
	}
	if got := ResolveAccessMode(ExecSession{SandboxMode: "danger-full-access"}); got != "danger-full-access" {
		t.Fatalf("sandbox = %q", got)
	}
	if got := ResolveAccessMode(ExecSession{}); got != "workspace-write" {
		t.Fatalf("default = %q", got)
	}
}

func TestSupportedDescriptors(t *testing.T) {
	if len(SupportedExecutionModes()) < 2 {
		t.Fatal("exec modes")
	}
	if len(SupportedAccessModes()) < 3 {
		t.Fatal("access modes")
	}
	if len(SupportedApprovalModes()) < 3 {
		t.Fatal("approval modes")
	}
	// 别名拷贝独立
	a := SupportedExecutionModes()
	a[0].Aliases = append(a[0].Aliases, "mutated")
	b := SupportedExecutionModes()
	for _, x := range b[0].Aliases {
		if x == "mutated" {
			t.Fatal("alias slice shared")
		}
	}
}
