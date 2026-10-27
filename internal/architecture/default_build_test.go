package architecture

// 本文件守护 release / default build 不会重新引入已退役的 Go app-server / Go tool-host。
//
// 这些 Go 二进制（cmd/eos-tool-host、eos app-server、eos serve、eos daemon）连同
// 旧 Go/Eino 内核已整体删除。守卫保留为防回归：任何 production 代码若试图 spawn
// "eos-tool-host" 或 import cmd/eos-tool-host 包，立即失败。

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// goSidecarStringBlacklist 列出已退役 Go sidecar 二进制 / env 字面量。
// 守卫保留：防止 default build 偷偷把它们重新接回 production 路径。
var goSidecarStringBlacklist = []string{
	"eos-tool-host",
	"EOS_TOOL_HOST",
	"EOS_TOOL_HOST_FAKE",
}

// TestDefaultBuildDoesNotReferenceGoAppServerOrToolHost 扫描 production 代码路径
// （main.go / internal/cli / internal/ui）确认 default build 不会 reference
// 已退役的 Go app-server 或 Go tool-host 二进制。
func TestDefaultBuildDoesNotReferenceGoAppServerOrToolHost(t *testing.T) {
	root := moduleRoot(t)
	roots := []string{
		filepath.Join(root, "main.go"),
		filepath.Join(root, "internal", "cli"),
		filepath.Join(root, "internal", "ui"),
	}
	var hits []string

	for _, target := range roots {
		info, err := os.Stat(target)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if hasForbiddenString(t, target) {
				rel, _ := filepath.Rel(root, target)
				hits = append(hits, filepath.ToSlash(rel))
			}
			continue
		}
		err = filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if hasForbiddenString(t, path) {
				rel, _ := filepath.Rel(root, path)
				hits = append(hits, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", target, err)
		}
	}

	if len(hits) > 0 {
		sort.Strings(hits)
		t.Fatalf("default build references retired Go app-server or Go tool-host; remove these references:\n%s",
			strings.Join(hits, "\n"))
	}
}

// hasForbiddenString 检查某个 Go 文件是否 import 或字面量引用已退役 Go sidecar。
func hasForbiddenString(t *testing.T, path string) bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return false
	}
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if importPath == "github.com/eosaios/eos/cmd/eos-tool-host" ||
			strings.HasPrefix(importPath, "github.com/eosaios/eos/cmd/eos-tool-host/") {
			return true
		}
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(src)
	for _, needle := range goSidecarStringBlacklist {
		if idx := strings.Index(text, needle); idx >= 0 {
			if isInsideComment(text, idx) {
				continue
			}
			return true
		}
	}
	return false
}

// isInsideComment 粗略判断 text[idx] 是否在 // 或 /* */ 注释中。
func isInsideComment(text string, idx int) bool {
	lineStart := strings.LastIndex(text[:idx], "\n") + 1
	if lineStart < 0 {
		lineStart = 0
	}
	line := text[lineStart:idx]
	if slash := strings.Index(line, "//"); slash >= 0 {
		return true
	}
	lastOpen := strings.LastIndex(text[:idx], "/*")
	if lastOpen < 0 {
		return false
	}
	lastClose := strings.LastIndex(text[:idx], "*/")
	return lastClose <= lastOpen
}

// _ = runtime.Caller
var _ = runtime.Caller
