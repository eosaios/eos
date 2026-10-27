package slash

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestNormalizeCommandResolvesAliases(t *testing.T) {
	tests := map[string]string{
		"/models":   "/model",
		"/ctx":      "/context",
		"/sessions": "/session",
		"/worktree": "/workspace",
		"/settings": "/config",
	}

	for input, want := range tests {
		if got := NormalizeCommand(input); got != want {
			t.Fatalf("NormalizeCommand(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestGetSuggestionsMatchesAliasesWithoutDuplicates(t *testing.T) {
	items := GetSuggestions("/set")
	if len(items) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(items))
	}
	if items[0].Name != "/config" {
		t.Fatalf("expected /config suggestion, got %q", items[0].Name)
	}
}

func TestGroupedVisibleCommandsPreservesExpectedGroups(t *testing.T) {
	groups := GroupedVisibleCommands("zh")
	if len(groups) < 4 {
		t.Fatalf("expected at least 4 groups, got %d", len(groups))
	}
	if groups[0].Group != GroupGeneral {
		t.Fatalf("expected first group to be general, got %q", groups[0].Group)
	}
	if groups[1].Group != GroupProject {
		t.Fatalf("expected second group to be project, got %q", groups[1].Group)
	}
}

func TestHiddenCommandsResolveButStayOutOfVisibleSurfaces(t *testing.T) {
	if NormalizeCommand("/reload-plugins") != "/reload-plugins" {
		t.Fatalf("expected /reload-plugins to remain callable")
	}
	if NormalizeCommand("/doctor") != "/doctor" {
		t.Fatalf("expected /doctor to remain callable")
	}
	for _, cmd := range VisibleCommands() {
		switch cmd.Name {
		case "/reload-plugins", "/doctor", "/stats":
			t.Fatalf("expected %s to be hidden from visible commands", cmd.Name)
		}
	}
	for _, cmd := range GetSuggestions("/reload") {
		if cmd.Name == "/reload-plugins" {
			t.Fatalf("expected /reload-plugins to be hidden from suggestions")
		}
	}
}

func TestVisibleCommandsIncludesPlanStyle(t *testing.T) {
	if NormalizeCommand("/plan-style") != "/plan-style" {
		t.Fatalf("expected /plan-style to be a visible slash command")
	}
}

func TestVisibleCommandsIncludesInitVerifiers(t *testing.T) {
	if NormalizeCommand("/init-verifiers") != "/init-verifiers" {
		t.Fatalf("expected /init-verifiers to be a visible slash command")
	}
}

func TestVisibleCommandsIncludesVerify(t *testing.T) {
	if NormalizeCommand("/verify") != "/verify" {
		t.Fatalf("expected /verify to be a visible slash command")
	}
}
