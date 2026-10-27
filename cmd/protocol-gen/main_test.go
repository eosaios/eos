package main

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// 生成器单元批测：命名转换/类型解析矩阵 + 三产物落盘断言 + main 的
// 好/坏 schema 两条退出路径（go run 子进程驱动）。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGen 构建一次生成器二进制并带参运行，返回进程退出态（构建失败返回 nil）。
func runGen(t *testing.T, schema, out string) *os.ProcessState {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "protocol-gen")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Logf("build: %v\n%s", err, out)
		return nil
	}
	cmd := exec.Command(bin, "-schema", schema, "-out", out)
	_ = cmd.Run()
	return cmd.ProcessState
}

func TestSnakeToPascalMatrix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"session_list", "SessionList"},
		{"api_key", "APIKey"}, // method 缩写
		{"url", "URL"},        // 全缩写
		{"mcp", "MCP"},
		{"id", "ID"},
		{"json", "JSON"},
		{"usd", "USD"},
		{"single", "Single"},
		{"a_b_c", "ABC"},
	}
	for _, tc := range cases {
		if got := snakeToPascal(tc.in, methodAbbreviations); got != tc.want {
			t.Fatalf("snakeToPascal(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// 字段缩写表独立：kb 只在字段语境缩写。
	if got := snakeToPascal("kb_limit", fieldAbbreviations); got != "KBLimit" {
		t.Fatalf("field abbr = %q", got)
	}
	if got := snakeToPascal("mcp_servers", fieldAbbreviations); got != "McpServers" {
		t.Fatalf("mcp 非字段缩写 = %q", got)
	}
}

func TestMethodNameToConst(t *testing.T) {
	cases := []struct{ in, want string }{
		{"session/list", "MethodSessionList"},
		{"session/api_key", "MethodSessionAPIKey"},
		{"a/b/c", "MethodABC"},
		{"workspace/default", "MethodWorkspaceDefault"},
	}
	for _, tc := range cases {
		if got := methodNameToConst(tc.in); got != tc.want {
			t.Fatalf("methodNameToConst(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFieldNameToGo(t *testing.T) {
	if got := fieldNameToGo("url_path"); got != "URLPath" {
		t.Fatalf("url_path = %q", got)
	}
	if got := fieldNameToGo("usd"); got != "USD" {
		t.Fatalf("usd = %q", got)
	}
	if got := fieldNameToGo("created_at"); got != "CreatedAt" {
		t.Fatalf("created_at = %q", got)
	}
}

func TestParseGoTypeMatrix(t *testing.T) {
	// 切片：去括号 + omitempty 跟随 force。
	got, omitempty := parseGoType("[string]", false)
	if got != "[]string" || omitempty {
		t.Fatalf("[string] = %q,%v", got, omitempty)
	}
	got, omitempty = parseGoType("[Turn]", true)
	if got != "[]Turn" || !omitempty {
		t.Fatalf("[Turn] = %q,%v", got, omitempty)
	}
	// 指针基元：解引用 + 恒 omitempty。
	for _, prim := range []string{"bool", "int64", "float64", "string", "uint64", "int", "float32", "int32"} {
		got, omitempty = parseGoType("*"+prim, false)
		if got != prim || !omitempty {
			t.Fatalf("*%s = %q,%v", prim, got, omitempty)
		}
	}
	// 指针结构体：保留指针 + omitempty 跟随。
	got, omitempty = parseGoType("*Session", false)
	if got != "*Session" || omitempty {
		t.Fatalf("*Session = %q,%v", got, omitempty)
	}
	// 普通类型透传。
	got, omitempty = parseGoType("json.RawMessage", true)
	if got != "json.RawMessage" || !omitempty {
		t.Fatalf("raw = %q,%v", got, omitempty)
	}
}

func testSchema() Schema {
	return Schema{
		Version: 1,
		Methods: []Method{
			{Name: "session/list", Group: "session"},
			{Name: "session/api_key", Group: "session"},
			{Name: "workspace/default", Group: "workspace"},
		},
		Types: []TypeDef{
			{Name: "Session", Fields: []Field{
				{JSONName: "id", GoType: "string", O: 0},
				{JSONName: "url_path", GoType: "string", O: 1},
				{JSONName: "payload", GoType: "json.RawMessage", O: 0},
				{JSONName: "count", GoType: "*int64", O: 0},
				{JSONName: "tags", GoType: "[string]", O: 0},
			}},
		},
	}
}

func TestGenerateMethodsOutput(t *testing.T) {
	out := t.TempDir()
	if err := generateMethods(testSchema(), out); err != nil {
		t.Fatalf("generateMethods: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "methods_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	for _, want := range []string{
		`MethodSessionList = "session/list"`,
		`MethodSessionAPIKey = "session/api_key"`,
		`MethodWorkspaceDefault = "workspace/default"`,
		"func CoreMethods() []string {",
		"func MethodGroups() map[string][]string {",
		`"session": {`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("methods_gen 缺 %q:\n%s", want, content)
		}
	}
}

func TestGenerateTypesOutput(t *testing.T) {
	out := t.TempDir()
	if err := generateTypes(testSchema(), out); err != nil {
		t.Fatalf("generateTypes: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "types_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	// json.RawMessage 触发 import。
	if !strings.Contains(content, `import "encoding/json"`) {
		t.Fatalf("types_gen 缺 json import:\n%s", content)
	}
	for _, want := range []string{
		"ID string `json:\"id\"`",
		"URLPath string `json:\"url_path,omitempty\"`",
		"Payload json.RawMessage `json:\"payload\"`",
		"Count int64 `json:\"count,omitempty\"`",
		"Tags []string `json:\"tags\"`",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("types_gen 缺 %q:\n%s", want, content)
		}
	}

	// 无 RawMessage：不落 import。
	bare := Schema{Types: []TypeDef{{Name: "Empty", Fields: []Field{{JSONName: "x", GoType: "string"}}}}}
	if err := generateTypes(bare, out); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(out, "types_gen.go"))
	if strings.Contains(string(raw), "encoding/json") {
		t.Fatal("无 RawMessage 不应 import json")
	}
}

func TestGenerateTestOutput(t *testing.T) {
	out := t.TempDir()
	if err := generateTest(testSchema(), out); err != nil {
		t.Fatalf("generateTest: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "schema_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "func TestGeneratedMethodsCoverAllCoreMethods(t *testing.T)") {
		t.Fatalf("schema_test 形状不符:\n%s", raw)
	}
}

func TestMainBinaryPaths(t *testing.T) {
	if _, err := os.Stat("../../go.mod"); err != nil {
		t.Skip("非仓库环境")
	}
	out := t.TempDir()

	// 坏 schema 路径：stderr + exit 1。
	res := runGen(t, "/definitely/missing/schema.json", out)
	if res == nil || res.Success() {
		t.Fatal("坏 schema 应非零退出")
	}

	// 好 schema：三产物落盘 + 零退出。
	schemaPath := filepath.Join(out, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(`{
		"version": 1,
		"methods": [{"name": "session/list", "group": "session"}],
		"types": [{"name": "S", "fields": [{"json": "id", "go": "string"}]}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	genOut := filepath.Join(out, "generated")
	res = runGen(t, schemaPath, genOut)
	if res == nil || !res.Success() {
		t.Fatal("好 schema 应零退出")
	}
	for _, f := range []string{"methods_gen.go", "types_gen.go", "schema_test.go"} {
		if _, err := os.Stat(filepath.Join(genOut, f)); err != nil {
			t.Fatalf("缺产物 %s: %v", f, err)
		}
	}
}

func TestRunStagesInProcess(t *testing.T) {
	out := t.TempDir()
	schemaPath := filepath.Join(out, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(`{"version":1,"methods":[],"types":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// 坏 schema 路径 / 坏 JSON / 成功链三段。
	if err := run("/definitely/missing.json", out); err == nil || !strings.Contains(err.Error(), "read schema") {
		t.Fatalf("read 阶段 = %v", err)
	}
	badJSON := filepath.Join(out, "bad.json")
	if err := os.WriteFile(badJSON, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(badJSON, out); err == nil || !strings.Contains(err.Error(), "parse schema") {
		t.Fatalf("parse 阶段 = %v", err)
	}
	genOut := filepath.Join(out, "nested", "generated")
	if err := run(schemaPath, genOut); err != nil {
		t.Fatalf("成功链: %v", err)
	}
	for _, f := range []string{"methods_gen.go", "types_gen.go", "schema_test.go"} {
		if _, err := os.Stat(filepath.Join(genOut, f)); err != nil {
			t.Fatalf("缺产物 %s: %v", f, err)
		}
	}
}
