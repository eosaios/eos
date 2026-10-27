package messages

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// messages.go 余臂批测：工具明细块余下工具族与默认臂三序、wrapLine
// 深臂（ANSI 转义/tab/CJK/空格回折/超宽硬折）、截断族、displayToolName
// 已知映射与空段输入。

import (
	"strings"
	"testing"
)

func TestBuildToolDetailBlocksRemainingTools(t *testing.T) {
	has := func(blocks []toolDetailBlock, label string) bool {
		for _, b := range blocks {
			if b.label == label {
				return true
			}
		}
		return false
	}

	// searchcodebase：Query 明细。
	b := buildToolDetailBlocks("searchcodebase", map[string]any{"information_request": "找登录逻辑"})
	if !has(b, "Query") {
		t.Fatalf("searchcodebase = %+v", b)
	}
	// runcommand：command（\r\n 归一）+ cwd + type + blocking 全子键。
	b = buildToolDetailBlocks("runcommand", map[string]any{
		"command": "make\r\nbuild", "cwd": "/repo", "command_type": "build", "blocking": true,
	})
	for _, label := range []string{"Command", "Cwd", "Type", "Blocking"} {
		if !has(b, label) {
			t.Fatalf("runcommand 缺 %s: %+v", label, b)
		}
	}
	// websearch：Query。
	b = buildToolDetailBlocks("websearch", map[string]any{"query": "eos cli"})
	if !has(b, "Query") {
		t.Fatalf("websearch = %+v", b)
	}
	// deletefile：Paths 列表拼接。
	b = buildToolDetailBlocks("deletefile", map[string]any{"file_paths": []any{"/a.go", "/b.go"}})
	if !has(b, "Paths") {
		t.Fatalf("deletefile = %+v", b)
	}
	// 默认臂三序：command 优先。
	b = buildToolDetailBlocks("custom", map[string]any{"command": "run", "path": "/p", "file_paths": []any{"x"}})
	if !has(b, "Command") || has(b, "Path") {
		t.Fatalf("默认臂 command 优先 = %+v", b)
	}
	b = buildToolDetailBlocks("custom", map[string]any{"file_path": "/p.txt"})
	if !has(b, "Path") {
		t.Fatalf("默认臂 path 别名 = %+v", b)
	}
	b = buildToolDetailBlocks("custom", map[string]any{"paths": []any{"x", "y"}})
	if !has(b, "Paths") {
		t.Fatalf("默认臂 paths = %+v", b)
	}
	// glob 的 In 子键。
	b = buildToolDetailBlocks("glob", map[string]any{"pattern": "*.go", "path": "src"})
	if !has(b, "In") {
		t.Fatalf("glob In = %+v", b)
	}
}

func TestWrapLineDeepArms(t *testing.T) {
	// ANSI 转义 token 零宽参与折行（不折断转义序列）。
	s := "\x1b[31m" + strings.Repeat("word ", 20) + "\x1b[m tail"
	lines := splitAndWrapANSI(s, 24)
	if len(lines) < 2 {
		t.Fatalf("长行未折: %d", len(lines))
	}
	// tab 记 4 宽：aaa(3)+tab(4)=7 超过 4 → 在 tab 处折行。
	lines = splitAndWrapANSI("aaa\tbbb", 4)
	if len(lines) != 2 || lines[0] != "aaa" || lines[1] != "bbb" {
		t.Fatalf("tab 宽度折行 = %q", lines)
	}
	// CJK 双宽参与折行。
	lines = splitAndWrapANSI(strings.Repeat("中", 30), 10)
	if len(lines) < 5 {
		t.Fatalf("CJK 折行 = %d 行", len(lines))
	}
	// 无空格超宽词硬折。
	lines = splitAndWrapANSI(strings.Repeat("x", 50), 10)
	if len(lines) != 5 {
		t.Fatalf("硬折 = %v", lines)
	}
	// 折行尾部空格修剪。
	lines = splitAndWrapANSI("aaaa bbbb cccc dddd", 9)
	for _, ln := range lines {
		if strings.HasSuffix(ln, " ") {
			t.Fatalf("行尾空格未修剪: %q", ln)
		}
	}
	// 空串。
	if got := splitAndWrapANSI("", 10); len(got) != 1 || got[0] != "" {
		t.Fatalf("空串 = %v", got)
	}
}

func TestTruncateFamilyArms(t *testing.T) {
	// 未超限原样返回。
	if got := truncateInline("short", 10); got != "short" {
		t.Fatalf("inline 未超限 = %q", got)
	}
	// 超限：省略号 + 余量。
	got := truncateInline(strings.Repeat("a", 30), 10)
	if !strings.Contains(got, "...(+20 chars)") {
		t.Fatalf("inline 超限 = %q", got)
	}
	// 块截断：多行提示。
	got2 := truncateBlockValue(strings.Repeat("b", 40), 15)
	if !strings.Contains(got2, "[truncated: showing first 15 of 40 chars]") {
		t.Fatalf("block 超限 = %q", got2)
	}
	if got2 := truncateBlockValue("short", 10); got2 != "short" {
		t.Fatalf("block 未超限 = %q", got2)
	}
	// rune 截断边界（CJK 按 rune 不按字节）。
	clipped, truncated, total := truncateRunes("中文内容截断测试", 4)
	if !truncated || total != 8 || clipped != "中文内容" {
		t.Fatalf("runes = %q,%v,%d", clipped, truncated, total)
	}
	// limit<=0 不截断。
	if _, truncated, _ = truncateRunes("any", 0); truncated {
		t.Fatal("limit 0 不应截断")
	}
}

func TestDisplayToolNameArms(t *testing.T) {
	cases := []struct{ in, want string }{
		{"fs", "FS"},
		{"mcp", "MCP"},
		{"time_now", "TimeNow"},
		{"runcommand", "RunCommand"},
		{"web-search", "WebSearch"}, // 连字符归一
		{"todo_write", "TodoWrite"},
	}
	for _, tc := range cases {
		if got := displayToolName(tc.in); got != tc.want {
			t.Fatalf("displayToolName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// 全空段输入：builder 空 → 原样返回。
	if got := displayToolName("__"); got != "__" {
		t.Fatalf("空段输入 = %q", got)
	}
}

func TestGetIntAndListArms(t *testing.T) {
	// read+limit：int/int64/float64 三型与非法型。
	for _, v := range []any{5, int64(5), 5.0, "5"} {
		blocks := buildToolDetailBlocks("read", map[string]any{
			"file_path": "/a", "offset": 1, "limit": v,
		})
		_ = blocks
	}
	// 字符串列表：[]string 空/1/2/3+/超 max、[]any 混型/空串跳过/空列表。
	for _, tc := range []struct {
		name   string
		params map[string]any
	}{
		{"deletefile", map[string]any{"file_paths": []string{}}},
		{"deletefile", map[string]any{"file_paths": []string{"a"}}},
		{"deletefile", map[string]any{"file_paths": []string{"a", "b"}}},
		{"deletefile", map[string]any{"file_paths": []string{"a", "b", "c"}}},
		{"deletefile", map[string]any{"file_paths": []string{"a", "b", "c", "d"}}},
		{"deletefile", map[string]any{"file_paths": []any{}}},
		{"deletefile", map[string]any{"file_paths": []any{"a", "b", "c", "d", "e"}}},
		{"deletefile", map[string]any{"file_paths": []any{"a", 42, "", "  ", "b"}}},
		{"deletefile", map[string]any{"file_paths": 42}},
	} {
		_ = buildToolDetailBlocks(tc.name, tc.params)
	}
	// read 无 limit（offset 单独）与双数值。
	_ = buildToolDetailBlocks("read", map[string]any{"file_path": "/a", "offset": 3})
	_ = buildToolDetailBlocks("read", map[string]any{"file_path": "/a", "offset": 3, "limit": 7})
	// 未知键值不进明细（getInt/getStr 未命中臂）。
	_ = buildToolDetailBlocks("read", map[string]any{"file_path": "/a", "offset": nil})
}
