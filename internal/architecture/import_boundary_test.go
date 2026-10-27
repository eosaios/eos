// Package architecture 守护 EOS 架构边界，防止旧 Go 内核（已退役删除）的耦合回退。
//
// 引擎已收敛为 Rust-only：UI/TUI/CLI 只能通过 pkg/coreapi/sidecar/client + coreapi.Engine
// 访问运行时。本文件守住三条边界，防止被删除的 internal/bridge、internal/runtime、
// internal/tools、pkg/core 被重新引入：
//
//  1. TestUIDirectRuntimeCouplingDoesNotSpread — internal/ui 不得导入已退役包。
//  2. TestNewCorePackagesDoNotImportLegacyRuntime — 新核心包不得导入 internal/* 或 pkg/core。
//  3. TestCLIHeadlessNoBridgeImport / TestCLIProductionPathsForbidLegacyGoCore — CLI
//     不得直接耦合 legacy runtime。
package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const modulePath = "github.com/eosaios/eos"

// forbiddenUIImports 列出已退役删除的包路径。守卫仍保留：防止任何人重新引入
// internal/bridge / internal/runtime / internal/tools / pkg/core（旧 Go/Eino 内核）。
var forbiddenUIImports = []string{
	"github.com/eosaios/eos/internal/bridge",
	"github.com/eosaios/eos/internal/runtime",
	"github.com/eosaios/eos/internal/tools",
	"github.com/eosaios/eos/pkg/core",
}

func TestUIDirectRuntimeCouplingDoesNotSpread(t *testing.T) {
	root := moduleRoot(t)
	uiRoot := filepath.Join(root, "internal", "ui")
	violations := map[string][]string{}

	err := filepath.WalkDir(uiRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := modulePath + "/" + filepath.ToSlash(rel)
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if isForbiddenUIImport(importPath) {
				violations[pkg] = append(violations[pkg], importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ui imports: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("new UI direct runtime coupling detected: %#v", violations)
	}
}

func TestNewCorePackagesDoNotImportLegacyRuntime(t *testing.T) {
	root := moduleRoot(t)
	packageRoots := []string{
		filepath.Join(root, "pkg", "coreapi"),
		filepath.Join(root, "pkg", "protocol", "jsonrpc"),
		filepath.Join(root, "pkg", "sandbox"),
	}
	violations := map[string][]string{}
	for _, packageRoot := range packageRoots {
		err := filepath.WalkDir(packageRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			pkg, err := packagePath(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				if isForbiddenNewCoreImport(importPath) {
					violations[pkg] = append(violations[pkg], importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s imports: %v", packageRoot, err)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("new core package imported legacy runtime/UI: %#v", violations)
	}
}

func isForbiddenUIImport(importPath string) bool {
	for _, forbidden := range forbiddenUIImports {
		if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
			return true
		}
	}
	return false
}

var forbiddenCLIImports = []string{
	"github.com/eosaios/eos/internal/bridge",
	"github.com/eosaios/eos/internal/tools",
	"github.com/eosaios/eos/internal/session",
}

// cliForbiddenProductionImports 是所有 CLI 命令必须遵守的禁列。
// 这些包（旧 Go/Eino 内核）已被删除，守卫保留以防止重新引入。
var cliForbiddenProductionImports = []string{
	"github.com/eosaios/eos/pkg/core",
	"github.com/eosaios/eos/internal/bridge",
	"github.com/eosaios/eos/internal/runtime",
	"github.com/eosaios/eos/internal/tools",
}

func TestCLIHeadlessNoBridgeImport(t *testing.T) {
	root := moduleRoot(t)
	cliRoot := filepath.Join(root, "internal", "cli")
	violations := map[string][]string{}

	err := filepath.WalkDir(cliRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := modulePath + "/" + filepath.ToSlash(rel)
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if isForbiddenCLIImport(importPath) {
				violations[pkg] = append(violations[pkg], importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk cli imports: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("CLI headless direct runtime coupling detected; use pkg/core.Runtime or JSON-RPC client instead:\n%#v", violations)
	}
}

func isForbiddenCLIImport(importPath string) bool {
	for _, forbidden := range forbiddenCLIImports {
		if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
			return true
		}
	}
	return false
}

// TestCLIProductionPathsForbidLegacyGoCore 守护所有 CLI 入口不直接 import 已退役的
// 旧 Go 内核（pkg/core / internal/bridge / internal/runtime / internal/tools）。
// 这些包已删除；守卫保留以防止被重新接回 CLI 路径。
func TestCLIProductionPathsForbidLegacyGoCore(t *testing.T) {
	root := moduleRoot(t)
	cliRoot := filepath.Join(root, "internal", "cli")
	violations := map[string][]string{}

	err := filepath.WalkDir(cliRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		basename := filepath.Base(path)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		pkg, err := packagePath(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forbidden := range cliForbiddenProductionImports {
				if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
					violations[pkg+"/"+basename] = append(violations[pkg+"/"+basename], importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk cli imports: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("CLI path imports legacy Go core (pkg/core / bridge / runtime / tools); use pkg/coreapi/sidecar + engineprovider instead:\n%#v", violations)
	}
}

func isForbiddenNewCoreImport(importPath string) bool {
	if strings.HasPrefix(importPath, modulePath+"/internal/") {
		return true
	}
	return importPath == modulePath+"/pkg/core" || strings.HasPrefix(importPath, modulePath+"/pkg/core/")
}

func packagePath(root, dir string) (string, error) {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return modulePath, nil
	}
	return modulePath + "/" + rel, nil
}

// toolAPIImplLegacyFiles, toolAPIImplForbiddenImports and
// TestToolAPIImplDependencyBoundary guarded the internal/toolapi/impl
// dependency boundary. That package was removed together with the rest of
// the Go gateway layer, so the guard is no longer applicable.

// TestDeletedPackagesStayDeleted asserts that Go packages superseded by the
// Rust kernel are not re-introduced. Each listed directory was a full Go
// reimplementation of functionality the Rust core already provides
// (context indexing, memory store, skill loader, agent orchestrator, etc).
// Re-creating them would re-introduce duplicate business logic in the shell
// layer, violating the "shell does not do business adjudication" rule.
func TestDeletedPackagesStayDeleted(t *testing.T) {
	root := moduleRoot(t)
	deletedPackages := []string{
		"internal/context",
		"internal/memory",
		"internal/store",
		"internal/skills",
		"pkg/agentcore",
	}
	for _, pkg := range deletedPackages {
		dir := filepath.Join(root, filepath.FromSlash(pkg))
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			t.Errorf("deleted package %s has been re-created; this functionality now lives in the Rust kernel", pkg)
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
