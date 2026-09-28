package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/panels"
)

func TestVersionPathHelpers(t *testing.T) {
	if versionFileMatches("", "a") || versionFileMatches("a", "") {
		t.Fatal("empty")
	}
	if !versionFileMatches("a/b.txt", "a/b.txt") {
		t.Fatal("equal")
	}
	if !versionFileMatches("A/B.TXT", "a/b.txt") {
		t.Fatal("case")
	}
	// 绝对 vs 相对后缀
	abs := filepath.ToSlash(filepath.Join("/ws", "src", "a.go"))
	if !versionFileMatches(abs, "src/a.go") {
		t.Fatalf("abs vs rel: %q", abs)
	}
	if !versionFileMatches("src/a.go", abs) {
		t.Fatal("rel vs abs")
	}
	if versionFileMatches("other/a.go", "src/a.go") {
		t.Fatal("mismatch")
	}

	if normalizeVersionPath("") != "" {
		t.Fatal("empty norm")
	}
	if normalizeVersionPath("./a/b") != "a/b" {
		t.Fatalf("dot slash = %q", normalizeVersionPath("./a/b"))
	}
	if normalizeVersionPath("  x/y  ") != "x/y" {
		t.Fatalf("trim = %q", normalizeVersionPath("  x/y  "))
	}

	if !isAbsVersionPath("/a/b") {
		t.Fatal("unix abs")
	}
	if isAbsVersionPath("rel/a") {
		t.Fatal("rel")
	}

	if versionSummarySize("") != 0 {
		t.Fatal("empty size")
	}
	if versionSummarySize("no size") != 0 {
		t.Fatal("no size")
	}
	if versionSummarySize("size=123") != 123 {
		t.Fatalf("size = %d", versionSummarySize("size=123"))
	}
	if versionSummarySize("edit, size=99, other") != 99 {
		t.Fatalf("comma size = %d", versionSummarySize("edit, size=99, other"))
	}
}

func TestResolveWorkspaceInputPath(t *testing.T) {
	// 空
	if _, err := resolveWorkspaceInputPath("  ", "zh"); err == nil {
		t.Fatal("empty")
	}

	// 相对路径 → 绝对
	got, err := resolveWorkspaceInputPath("src", "zh")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("not abs = %q", got)
	}

	// ~ 展开
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err = resolveWorkspaceInputPath("~/proj", "zh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, filepath.Clean(home)) {
		t.Fatalf("home expand = %q want prefix %q", got, home)
	}

	got, err = resolveWorkspaceInputPath("~", "zh")
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("tilde root")
	}
}

func TestWorkspaceAndVersionsMsgWrappers(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	if next, _ := app.handleWorkspaceSelectMsg(panels.WorkspaceSelectMsg{Path: "/tmp"}); next == nil {
		t.Fatal("select")
	}
	if next, _ := app.handleWorkspaceDeleteMsg(panels.WorkspaceDeleteMsg{Path: "/tmp"}); next == nil {
		t.Fatal("delete")
	}
	if next, _ := app.handleWorkspaceAddMsg(panels.WorkspaceAddMsg{}); next == nil {
		t.Fatal("add")
	}
	if next, _ := app.handleWorkspaceReloadDoneMsg(WorkspaceReloadDoneMsg{}); next == nil {
		t.Fatal("reload done")
	}
	if next, _ := app.handleMCPReloadDoneMsg(MCPReloadDoneMsg{}); next == nil {
		t.Fatal("mcp reload")
	}
	if next, _ := app.handleLSPReloadDoneMsg(LSPReloadDoneMsg{}); next == nil {
		t.Fatal("lsp reload")
	}

	// versions wrappers
	if next, _ := app.handleVersionsLoadMsg(panels.VersionsLoadMsg{FilePath: "a"}); next == nil {
		t.Fatal("load")
	}
	if next, _ := app.handleVersionsRollbackMsg(panels.VersionsRollbackMsg{FilePath: "a", Timestamp: "t"}); next == nil {
		t.Fatal("rollback")
	}
	if next, _ := app.handleVersionsDeleteMsg(panels.VersionsDeleteMsg{FilePath: "a", Timestamp: "t"}); next == nil {
		t.Fatal("delete")
	}
	if next, _ := app.handleVersionsDeleteFileMsg(panels.VersionsDeleteFileMsg{FilePath: "a"}); next == nil {
		t.Fatal("delete file")
	}
	if next, _ := app.handleVersionsDeleteAllMsg(panels.VersionsDeleteAllMsg{}); next == nil {
		t.Fatal("delete all")
	}

	// 纯 handler 不 panic
	app.handleWorkspaceRemove("/tmp/nope")
	app.handleVersionsLoad("a.txt")
	app.handleVersionsRollback("a.txt", "t")
	app.handleVersionsDelete("a.txt", "t")
	app.handleVersionsDeleteFile("a.txt")
	app.handleVersionsDeleteAll()
	app.refreshVersionsPanel()
	app.refreshWorkspacePanel()

	// 信任检查
	_ = app.isWorkspaceTrusted("/tmp")
	app.openWorkspaceAddConfirm()
	app.openWorkspaceTrustConfirm("/tmp")
	if cmd := app.handleWorkspaceUse("/tmp/nope"); cmd != nil {
		// 可能产生 reload cmd
		_ = cmd
	}
}
