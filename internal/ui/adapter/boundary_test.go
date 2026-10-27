package adapter

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// eachImport 遍历当前目录下非测试 Go 源文件的 import 列表。
// parser.ParseDir 自 Go 1.25 起废弃（不按 build tags 归属文件），
// 这里按 os.ReadDir + parser.ParseFile 逐文件解析 ImportsOnly。
func eachImport(t *testing.T, fn func(filePath, importPath string)) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	entries, err := os.ReadDir(wd)
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parser.ParseFile(%q) error = %v", name, err)
		}
		filePath := filepath.Join(wd, name)
		for _, imp := range file.Imports {
			fn(filePath, strings.Trim(imp.Path.Value, `"`))
		}
	}
}

func TestImportBoundary(t *testing.T) {
	forbidden := map[string]string{
		"internal/tools/git":  "adapter 禁止直接依赖 internal/tools/git，请改用 coreapi.GitService (a.engine.Git())",
		"internal/tools/file": "adapter 禁止直接依赖 internal/tools/fileops 生产代码，请改用 coreapi 版本方法",
	}

	forbiddenPrefixes := []struct {
		prefix string
		msg    string
	}{
		{"github.com/eosaios/eos/internal/tools", "adapter 禁止直接依赖 internal/tools，请改用 coreapi.Engine"},
	}

	eachImport(t, func(filePath, path string) {
		if msg, ok := forbidden[path]; ok {
			t.Errorf("%s: forbidden import %q — %s", filePath, path, msg)
		}

		for _, fp := range forbiddenPrefixes {
			if strings.HasPrefix(path, fp.prefix) {
				if _, isExact := forbidden[path]; isExact {
					continue
				}
				t.Errorf("%s: forbidden import %q — %s", filePath, path, fp.msg)
			}
		}
	})
}

func TestAllowedImports(t *testing.T) {
	allowed := map[string]bool{
		"github.com/eosaios/eos/internal/ai":                true,
		"github.com/eosaios/eos/internal/config":            true,
		"github.com/eosaios/eos/internal/pkg/settings":      true,
		"github.com/eosaios/eos/pkg/coreapi":                true,
		"github.com/eosaios/eos/pkg/coreapi/jsonrpc":        true,
		"github.com/eosaios/eos/pkg/coreapi/sidecar":        true,
		"github.com/eosaios/eos/pkg/coreapi/sidecar/client": true,
		"github.com/eosaios/eos/pkg/protocol":               true,
		"github.com/eosaios/eos/pkg/protocol/jsonrpc":       true,
	}

	eachImport(t, func(filePath, path string) {
		if strings.HasPrefix(path, "github.com/eosaios/eos/internal/") ||
			strings.HasPrefix(path, "github.com/eosaios/eos/pkg/") {
			if !allowed[path] {
				t.Errorf("%s: unexpected internal/pkg import %q not in allowlist — if legitimate, update TestAllowedImports", filePath, path)
			}
		}
	})
}

func TestCoreFallbackContraction(t *testing.T) {
	migratedChecks := []struct {
		methodName    string
		primaryToken  string
		fallbackToken string
		primaryBefore string
		msg           string
	}{
		{
			methodName: "GetModelInfo", primaryToken: "a.engine.Models().List(",
			fallbackToken: "a.runtime.ListModelDescriptors()", primaryBefore: "a.runtime.ListModelDescriptors()",
			msg: "GetModelInfo 主路径应通过 a.engine.Models().List(...)，禁止再 a.runtime.ListModelDescriptors() 直连",
		},
		{
			methodName: "ResolveAPIConfig", primaryToken: "a.engine.Models().List(",
			fallbackToken: "a.runtime.ListModelDescriptors()", primaryBefore: "a.runtime.ListModelDescriptors()",
			msg: "ResolveAPIConfig 主路径应通过 a.engine.Models().List(...)，禁止再 a.runtime.ListModelDescriptors() 直连",
		},
		{
			methodName: "CurrentContextUsage", primaryToken: "a.engine.Context().Stats(",
			fallbackToken: "a.core.GetContext().GetCurrentUsage()", primaryBefore: "a.core.GetContext().GetCurrentUsage()",
			msg: "CurrentContextUsage 不应再直接调用 a.core.GetContext().GetCurrentUsage()",
		},
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}

	raw, err := os.ReadFile(wd + string(os.PathSeparator) + "core_client.go")
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	content := string(raw)

	for _, mc := range migratedChecks {
		primaryIdx := strings.Index(content, mc.primaryToken)
		if primaryIdx < 0 {
			t.Errorf("%s: 未找到主路径 token %q — 迁移可能被意外移除", mc.methodName, mc.primaryToken)
			continue
		}
		if mc.primaryBefore != "" {
			fallbackIdx := strings.Index(content, mc.fallbackToken)
			if fallbackIdx >= 0 && fallbackIdx < primaryIdx {
				t.Errorf("%s: fallback token %q 出现在主路径 token %q 之前 — %s", mc.methodName, mc.fallbackToken, mc.primaryToken, mc.msg)
			}
		} else {
			re, err := regexp.Compile(regexp.QuoteMeta(mc.fallbackToken))
			if err != nil {
				t.Fatalf("invalid pattern %q: %v", mc.fallbackToken, err)
			}
			if locs := re.FindAllStringIndex(content, -1); len(locs) > 0 {
				for _, loc := range locs {
					line := strings.Count(content[:loc[0]], "\n") + 1
					t.Errorf("runtime.go:%d: %s — %s", line, mc.fallbackToken, mc.msg)
				}
			}
		}
	}
}
