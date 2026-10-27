// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

package ui

// slash_verifiers + 版本面板批测：
//   - /init-verifiers：带参数用法、空目录无特征、Web/CLI/API 三类特征
//     （tempdir 布点）、非目录错误臂
//   - verifiers 纯函数：标签中英文、类型排序（含 default 99）、建议
//     文案中英文、existingRelativePaths 去重与空白跳过
//   - 版本面板：refreshVersionsPanel 聚合排序/加载失败、handleVersionsLoad
//     过滤、回滚/删除/删文件/清空的成功与失败臂
//
// 不可达清单：
//   - /init-verifiers 的 root==="" 臂：currentWorkspaceRoot 在 Getwd 成功时
//     恒非空（同 slash_runtime 书面豁免）。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestInitVerifiersSlashArms(t *testing.T) {
	setTestHome(t)

	t.Run("usage with args", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handleInitVerifiersSlash([]string{"extra"})
		if got := lastSystemText(app); !strings.Contains(got, "用法") {
			t.Fatalf("want usage, got %q", got)
		}
	})

	t.Run("empty workspace no markers", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.foregroundWS = t.TempDir()
		app.handleInitVerifiersSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "未检测到") {
			t.Fatalf("want no-markers message, got %q", got)
		}
	})

	t.Run("web cli api markers", func(t *testing.T) {
		root := t.TempDir()
		for _, rel := range []string{
			"frontend/package.json",
			"internal/ui",
			"openapi.yaml",
		} {
			full := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		app, eng := newTestAppModelWithEngine(t)
		eng.foregroundWS = root
		app.handleInitVerifiersSlash(nil)
		got := lastSystemText(app)
		for _, want := range []string{"Web", "CLI", "API", "Playwright", "Tmux", "HTTP", "frontend/package.json"} {
			if !strings.Contains(got, want) {
				t.Fatalf("verifiers output missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("not a directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "plain.txt")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		app, eng := newTestAppModelWithEngine(t)
		eng.foregroundWS = file
		app.handleInitVerifiersSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "检测验证工具失败") {
			t.Fatalf("want detect error, got %q", got)
		}
	})

	t.Run("english output", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		app, eng := newTestAppModelWithEngine(t)
		eng.foregroundWS = root
		app.state.Language = "en"
		app.handleInitVerifiersSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "Best for terminal interactions") {
			t.Fatalf("want EN suggestion, got %q", got)
		}
	})
}

func TestVerifierPureHelpers(t *testing.T) {
	if got := verifierProjectTypeLabel(verifierProjectWeb, "en"); got != "Web" {
		t.Fatalf("web en = %q", got)
	}
	if got := verifierProjectTypeLabel(verifierProjectCLI, ""); got != "CLI" {
		t.Fatalf("cli zh = %q", got)
	}
	if got := verifierProjectTypeLabel(verifierProjectAPI, "EN"); got != "API" {
		t.Fatalf("api EN = %q", got)
	}
	if got := verifierProjectTypeLabel(verifierProjectType("bogus"), "en"); got != "BOGUS" {
		t.Fatalf("default label = %q", got)
	}
	if got := verifierProjectTypeOrder(verifierProjectType("bogus")); got != 99 {
		t.Fatalf("default order = %d", got)
	}
	if verifierProjectTypeOrder(verifierProjectWeb) >= verifierProjectTypeOrder(verifierProjectCLI) {
		t.Fatal("web should sort before cli")
	}

	suggestion := verifierToolSuggestion{
		Tool: "Playwright", ReasonZH: "中文理由", ReasonEN: "english reason",
	}
	if got := localizeSuggestion(suggestion, "en"); got != "english reason" {
		t.Fatalf("en suggestion = %q", got)
	}
	if got := localizeSuggestion(suggestion, ""); got != "中文理由" {
		t.Fatalf("zh suggestion = %q", got)
	}
	// 重复类型只保留一条建议
	dup := verifierToolSuggestions([]verifierProjectDetection{
		{Type: verifierProjectWeb}, {Type: verifierProjectWeb},
	})
	if len(dup) != 1 {
		t.Fatalf("dup suggestions = %d", len(dup))
	}

	// existingRelativePaths：空白跳过、命中返回
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := existingRelativePaths(root, "  ", "main.go", "main.go", "missing.go")
	if len(got) != 1 || got[0] != "main.go" {
		t.Fatalf("existingRelativePaths = %v", got)
	}
}

func TestVersionsPanelAggregationAndArms(t *testing.T) {
	setTestHome(t)
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	list := []coreapi.VersionItem{
		{ID: "v1", File: "src/a.go", CreatedAt: base},
		{ID: "v2", File: "src/a.go", CreatedAt: base.Add(time.Hour)},
		{ID: "v3", File: "src/b.go", CreatedAt: base.Add(2 * time.Hour)},
		{ID: "v4", File: "   ", CreatedAt: base}, // 空文件名跳过
	}

	t.Run("refresh aggregates per file", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.versionList = list
		app.refreshVersionsPanel()
		if got := lastSystemText(app); strings.Contains(got, "Failed") {
			t.Fatalf("unexpected error: %q", got)
		}
	})

	t.Run("refresh load error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.versionListErr = errors.New("boom list")
		app.refreshVersionsPanel()
		if got := lastSystemText(app); !strings.Contains(got, "Failed to load versions") {
			t.Fatalf("want load error, got %q", got)
		}
	})

	t.Run("load file versions filter", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.versionList = list
		app.handleVersionsLoad("src/a.go")
		if got := lastSystemText(app); strings.Contains(got, "Failed") {
			t.Fatalf("unexpected error: %q", got)
		}
		// 空路径早退
		app.handleVersionsLoad("  ")
	})

	t.Run("load versions error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.versionListErr = errors.New("boom list")
		app.handleVersionsLoad("src/a.go")
		if got := lastSystemText(app); !strings.Contains(got, "Failed to load versions") {
			t.Fatalf("want load error, got %q", got)
		}
	})

	t.Run("rollback error arm and early return", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.rollbackErr = errors.New("boom rollback")
		app.handleVersionsRollback("src/a.go", "v2")
		if got := lastSystemText(app); !strings.Contains(got, "版本回滚失败") {
			t.Fatalf("want rollback error, got %q", got)
		}
		app.handleVersionsRollback("src/a.go", "  ") // 空版本 ID 早退
		app.handleVersionsRollback("  ", "v2")       // 空路径早退
	})

	t.Run("delete version error arm", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.versionDelErr = errors.New("boom delete")
		app.handleVersionsDelete("src/a.go", "v2")
		if got := lastSystemText(app); !strings.Contains(got, "删除版本失败") {
			t.Fatalf("want delete error, got %q", got)
		}
		app.handleVersionsDelete("src/a.go", " ") // 早退
	})

	t.Run("delete file versions error arm", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.deleteFileErr = errors.New("boom file")
		app.handleVersionsDeleteFile("src/a.go")
		if got := lastSystemText(app); !strings.Contains(got, "删除文件版本失败") {
			t.Fatalf("want delete-file error, got %q", got)
		}
		app.handleVersionsDeleteFile(" ") // 早退
	})

	t.Run("clear versions error arm", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.clearErr = errors.New("boom clear")
		app.handleVersionsDeleteAll()
		if got := lastSystemText(app); !strings.Contains(got, "清空版本历史失败") {
			t.Fatalf("want clear error, got %q", got)
		}
	})
}
