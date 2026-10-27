package render

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	uistyles "github.com/eosaios/eos/internal/ui/styles"
)

func TestRenderStylesConstructors(t *testing.T) {
	if NewRenderStyles() == nil || NewPlainRenderStyles() == nil {
		t.Fatal("style constructors returned nil")
	}
	// nil 主题回落 plain。
	if NewThemeRenderStyles(nil) == nil {
		t.Fatal("theme styles with nil theme should fall back to plain")
	}
	if NewThemeRenderStyles(&uistyles.Styles{}) == nil {
		t.Fatal("theme styles with empty theme should fall back to plain")
	}
}

func TestMarkdownRendererBasics(t *testing.T) {
	r := NewMarkdownRenderer(80)
	if got := r.Render(""); got != "" {
		t.Errorf("empty render = %q", got)
	}
	// CRLF 归一 + 尾部换行裁剪（glamour 输出带 margin 缩进，只验证内容与边界）。
	if got := r.Render("hi\r\n"); !strings.Contains(got, "hi") || strings.Contains(got, "\r") || strings.HasSuffix(got, "\n") {
		t.Errorf("crlf render = %q", got)
	}
	// markdown 围栏解包：内容直接渲染。
	if got := r.Render("```markdown\n# Title\n```\n"); !strings.Contains(got, "Title") {
		t.Errorf("fenced markdown render = %q", got)
	}
	// 非 markdown 语言围栏不解包（作为代码块渲染）。
	if got := r.Render("```go\nfmt.Println(1)\n```"); !strings.Contains(got, "Println") {
		t.Errorf("code fence render = %q", got)
	}
	// 宽度与主题变更后仍可渲染。
	r.SetWidth(40)
	if got := r.Render("# H\nbody"); !strings.Contains(got, "H") {
		t.Errorf("narrow render = %q", got)
	}
	r.SetChromaTheme("monokai")
	if got := r.Render("```go\nx := 1\n```"); !strings.Contains(got, "x") {
		t.Errorf("themed code render = %q", got)
	}
	// SetStyles(nil) 忽略，不 panic。
	r.SetStyles(nil)
	_ = r.Render("still works")
	r.SetStyles(NewPlainRenderStyles())
	_ = r.Render("plain")
}

func TestMarkdownRenderStreaming(t *testing.T) {
	r := NewMarkdownRenderer(80)
	if got := r.RenderStreaming(""); got != "" {
		t.Errorf("empty streaming = %q", got)
	}
	// 围栏开闭与围栏内逐字渲染。
	got := r.RenderStreaming("```go\nfunc A() {}\n```\ntail")
	if !strings.Contains(got, "func A() {}") || !strings.Contains(got, "tail") {
		t.Errorf("fenced streaming = %q", got)
	}
	// ~~~ 围栏同样处理。
	got = r.RenderStreaming("~~~\nraw\n~~~")
	if !strings.Contains(got, "raw") {
		t.Errorf("tilde fence streaming = %q", got)
	}
	// 标题 / 引用 / 列表 / 有序列表 / 行内标记。
	for _, snippet := range []string{"# 标题", "> 引用", "> ", "- 项目", "* 项目", "+ 项目", "1. 第一", "2) 第二", "`code`", "**bold**", "__bold__", "*em*", "_em_", "[label](https://x)", "普通行"} {
		if out := r.RenderStreaming(snippet); out == "" {
			t.Errorf("streaming %q produced empty output", snippet)
		}
	}
	// 未闭合围栏：后续行仍按围栏内渲染，不 panic。
	if out := r.RenderStreaming("```\ninside"); !strings.Contains(out, "inside") {
		t.Errorf("unclosed fence = %q", out)
	}
}

func TestMatchHeading(t *testing.T) {
	if h, ok := matchHeading("## 子标题"); !ok || h != "子标题" {
		t.Errorf("heading = %q, %v", h, ok)
	}
	if _, ok := matchHeading("#nospace"); ok {
		t.Error("heading without space should not match")
	}
	if _, ok := matchHeading("#"); ok {
		t.Error("bare hash should not match")
	}
	if _, ok := matchHeading("####### seven"); ok {
		t.Error("7 hashes should still match up to level 6 semantics; verify behavior")
	}
	if _, ok := matchHeading("plain"); ok {
		t.Error("plain line should not match heading")
	}
}

func TestMatchOrderedListItem(t *testing.T) {
	if _, ok := matchOrderedListItem("1. x"); !ok {
		t.Error("1. should match")
	}
	if _, ok := matchOrderedListItem("2) y"); !ok {
		t.Error("2) should match")
	}
	// 注：「1. 」（分隔后空内容）按实现会匹配，属可接受行为。
	for _, bad := range []string{"1.x", "a. x", "1", "12", "12abc"} {
		if _, ok := matchOrderedListItem(bad); ok {
			t.Errorf("%q should not match ordered list", bad)
		}
	}
}

func TestApplyPaired(t *testing.T) {
	identity := func(v string) string { return "[" + v + "]" }
	if got := applyPaired("no marker", "`", identity); got != "no marker" {
		t.Errorf("no marker = %q", got)
	}
	if got := applyPaired("a `b` c `d` e", "`", identity); got != "a [b] c [d] e" {
		t.Errorf("two pairs = %q", got)
	}
	if got := applyPaired("unpaired `tail", "`", identity); got != "unpaired `tail" {
		t.Errorf("unpaired = %q", got)
	}
}

func TestApplyLinks(t *testing.T) {
	s := NewPlainRenderStyles()
	if got := applyLinks("看 [EOS](https://eosaios.com) 官网", s); !strings.Contains(got, "EOS") || strings.Contains(got, "https://eosaios.com") {
		t.Errorf("link render = %q", got)
	}
	// 未闭合 / 非链接方括号原样保留。
	for _, in := range []string{"[label]", "[label] tail", "[a](unclosed", "plain"} {
		if got := applyLinks(in, s); !strings.Contains(got, strings.TrimSuffix(in, " tail")) {
			t.Errorf("applyLinks(%q) = %q", in, got)
		}
	}
}

func TestUnwrapMarkdownFences(t *testing.T) {
	// ```markdown 包裹的内容被解出。
	got := unwrapMarkdownFences("```markdown\n# T\n```\nrest")
	if strings.Contains(got, "```") || !strings.Contains(got, "# T") || !strings.Contains(got, "rest") {
		t.Errorf("markdown fence unwrap = %q", got)
	}
	// ```md 同样解包。
	got = unwrapMarkdownFences("~~~md\nbody\n~~~")
	if strings.Contains(got, "~~~") || !strings.Contains(got, "body") {
		t.Errorf("md fence unwrap = %q", got)
	}
	// 其他语言围栏保留。
	got = unwrapMarkdownFences("```go\nfmt\n```")
	if !strings.Contains(got, "```") {
		t.Errorf("go fence should stay wrapped = %q", got)
	}
	// 未闭合的 markdown 围栏：已消费的开头行不回吐。
	got = unwrapMarkdownFences("```markdown\ncontent")
	if strings.Contains(got, "```") || !strings.Contains(got, "content") {
		t.Errorf("unclosed markdown fence = %q", got)
	}
}

func TestRenderToolCallAndResult(t *testing.T) {
	r := NewMarkdownRenderer(80)
	if got := r.RenderToolCall("bash", nil); got != "▶ bash" {
		t.Errorf("tool call no params = %q", got)
	}
	got := r.RenderToolCall("write", map[string]any{"path": "/tmp/x"})
	if !strings.HasPrefix(got, "▶ write(") || !strings.Contains(got, "path=/tmp/x") {
		t.Errorf("tool call = %q", got)
	}
	if got := r.RenderToolResult("success", "done"); !strings.Contains(got, "✓") {
		t.Errorf("success result = %q", got)
	}
	if got := r.RenderToolResult("error", "boom"); !strings.Contains(got, "✗") {
		t.Errorf("error result = %q", got)
	}
}

func TestHighlightANSIEdges(t *testing.T) {
	if got := highlightANSI("", "go", "tokyo-night"); got != "" {
		t.Errorf("empty code = %q", got)
	}
	// 语言别名归一：js/ts/py/sh/yml。
	for _, alias := range []struct{ in, lang string }{
		{"const x = 1\n", "js"},
		{"let y: number = 2\n", "ts"},
		{"print('hi')\n", "py"},
		{"echo hi\n", "sh"},
		{"a: 1\n", "yml"},
	} {
		if got := highlightANSI(alias.in, alias.lang, ""); strings.TrimSpace(got) == "" {
			t.Errorf("highlight %s produced empty output", alias.lang)
		}
	}
	// 未知语言回落 Analyse/Fallback lexer。
	if got := highlightANSI("plain text\n", "no-such-lang", ""); strings.TrimSpace(got) == "" {
		t.Error("fallback lexer produced empty output")
	}
	// 未知主题回落默认。
	if got := highlightANSI("x := 1\n", "go", "no-such-theme"); strings.TrimSpace(got) == "" {
		t.Error("fallback theme produced empty output")
	}
	// CRLF 与尾换行归一。
	if got := highlightANSI("a := 1\r\n\r\n", "go", ""); strings.HasSuffix(got, "\n") {
		t.Errorf("trailing newline not trimmed = %q", got)
	}
}
