package messages

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"
	"time"
)

func TestMessageTypesAndRenders(t *testing.T) {
	s := testStyles()
	w := 80
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	user := &UserMessage{Content: "hello", Timestamp: ts}
	if user.Type() != MsgTypeUser {
		t.Fatalf("user type = %q", user.Type())
	}
	if out := stripANSIForTest(user.Render(s, w)); !strings.Contains(out, "hello") {
		t.Fatalf("user render = %q", out)
	}

	agent := &AgentBubbleMessage{
		Name: "sub", Event: "result", SourceName: "main", AgentID: "id-1",
		Content: "done", Done: true, Tokens: 10, Duration: 2 * time.Second,
		Timestamp: ts, Actions: []BubbleAction{{Kind: "copy", Label: "Copy"}},
	}
	if agent.Type() != MsgTypeAgent {
		t.Fatalf("agent type = %q", agent.Type())
	}
	if out := stripANSIForTest(agent.Render(s, w)); !strings.Contains(out, "done") {
		t.Fatalf("agent render = %q", out)
	}

	tool := &ToolCallMessage{Name: "Bash", Params: map[string]any{"cmd": "ls"}, Status: "success", Result: "ok", Duration: time.Second}
	if tool.Type() != MsgTypeTool {
		t.Fatalf("tool type = %q", tool.Type())
	}
	if out := stripANSIForTest(tool.Render(s, w)); !strings.Contains(out, "ok") {
		t.Fatalf("tool render = %q", out)
	}

	plan := &PlanMessage{
		Title: "T", Description: "D",
		Steps: []PlanStep{
			{Number: 1, Description: "a", Status: "completed"},
			{Number: 2, Description: "b", Status: "running"},
			{Number: 3, Description: "c", Status: "failed"},
			{Number: 4, Description: "d", Status: "pending"},
		},
	}
	if plan.Type() != MsgTypePlan {
		t.Fatal("plan type")
	}
	out := stripANSIForTest(plan.Render(s, w))
	for _, want := range []string{"Plan", "a", "b", "c", "d"} {
		if !strings.Contains(out, want) {
			t.Fatalf("plan missing %q:\n%s", want, out)
		}
	}

	thinking := &ThinkingMessage{
		Content: "line1\n\nlast line", Duration: 1500 * time.Millisecond,
		Steps: []ThinkingStep{{Description: "s1", Status: "done"}, {Description: "s2", Status: "doing"}, {Description: "s3", Status: "pending"}},
	}
	if thinking.Type() != MsgTypeThinking {
		t.Fatal("thinking type")
	}
	// 折叠态显示 last non-empty line
	out = stripANSIForTest(thinking.Render(s, w))
	if !strings.Contains(out, "last line") {
		t.Fatalf("collapsed thinking:\n%s", out)
	}
	thinking.Expanded = true
	thinking.ToggleHint = "Ctrl+E"
	out = stripANSIForTest(thinking.Render(s, w))
	if !strings.Contains(out, "s1") || !strings.Contains(out, "line1") || !strings.Contains(out, "Ctrl+E") {
		t.Fatalf("expanded thinking:\n%s", out)
	}

	for _, level := range []string{"info", "warning", "error", "success", ""} {
		sys := &SystemMessage{Content: "sys", Level: level}
		wantType := MsgTypeSystem
		switch level {
		case "error":
			wantType = MsgTypeError
		case "warning":
			wantType = MsgTypeWarning
		case "info":
			wantType = MsgTypeInfo
		}
		if sys.Type() != wantType {
			t.Fatalf("sys type level=%q = %q", level, sys.Type())
		}
		if out := stripANSIForTest(sys.Render(s, w)); !strings.Contains(out, "sys") {
			t.Fatalf("sys render %q:\n%s", level, out)
		}
	}
	// PreStyled 不折行
	pre := &SystemMessage{Content: "a\nb", Level: "info", PreStyled: true}
	out = stripANSIForTest(pre.Render(s, w))
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Fatalf("prestyled:\n%s", out)
	}
}

func TestSplitAndWrapAndLastNonEmptyLine(t *testing.T) {
	if got := splitAndWrapANSI("a\nb", 0); len(got) != 1 {
		t.Fatalf("maxWidth 0 = %v", got)
	}
	got := splitAndWrapANSI("hello\nworld", 80)
	if len(got) != 2 {
		t.Fatalf("split = %v", got)
	}

	if LastNonEmptyLine("a\n\n  \nb\n\n") != "b" {
		t.Fatalf("LastNonEmptyLine = %q", LastNonEmptyLine("a\n\n  \nb\n\n"))
	}
	if LastNonEmptyLine("   \n\n") != "" {
		t.Fatal("all blank")
	}
	if LastNonEmptyLine("only") != "only" {
		t.Fatal("single")
	}
}

func TestDisplayToolName(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		"  ":                  "",
		"Bash":                "Bash",
		"fs_read":             "FSRead",
		"mcp.call":            "Call",
		"package.tool_name":   "ToolName",
		"time_now":            "TimeNow",
		"websearch":           "WebSearch",
		"read_file":           "ReadFile",
		"fs.write_file":       "WriteFile",
		"http_request":        "HTTPRequest",
		"get_json_data":       "GetJSONData",
		"run-command":         "RunCommand",
		"SearchCodebase":      "SearchCodebase",
		"a":                   "A",
		"_leading":            "Leading",
	}
	for in, want := range cases {
		if got := displayToolName(in); got != want {
			t.Fatalf("displayToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAgentRenderHelpers(t *testing.T) {
	s := testStyles()
	if out := stripANSIForTest(renderAgentEventDot(s, "failed", false, false)); !strings.Contains(out, "●") {
		t.Fatal("failed dot")
	}
	renderAgentEventDot(s, "cancelled", false, false)
	renderAgentEventDot(s, "dispatch", false, false)
	renderAgentEventDot(s, "started", false, false)
	renderAgentEventDot(s, "progress", false, false)
	renderAgentEventDot(s, "update", false, false)
	renderAgentEventDot(s, "result", true, false)
	renderAgentEventDot(s, "result", true, true)
	renderAgentEventDot(s, "", false, true)

	if out := stripANSIForTest(renderAgentEventLabel(s, "")); !strings.Contains(out, "[result]") {
		t.Fatalf("empty label = %q", out)
	}
	if out := stripANSIForTest(renderAgentEventLabel(s, "DISPATCH")); !strings.Contains(out, "[dispatch]") {
		t.Fatalf("label = %q", out)
	}

	if out := stripANSIForTest(renderAgentRoute(s, "a", "b")); !strings.Contains(out, "a -> b") {
		t.Fatalf("route = %q", out)
	}
	if out := stripANSIForTest(renderAgentRoute(s, "", "b")); !strings.Contains(out, "b") {
		t.Fatalf("route name only = %q", out)
	}
	if out := stripANSIForTest(renderAgentRoute(s, "a", "")); !strings.Contains(out, "a") {
		t.Fatalf("route source only = %q", out)
	}

	if out := renderAgentID(s, ""); out != "" {
		t.Fatalf("empty id = %q", out)
	}
	if out := stripANSIForTest(renderAgentID(s, "short-id")); !strings.Contains(out, "short-id") {
		t.Fatalf("id = %q", out)
	}

	if shortID("abcdefghijklmnopqrstuvwxyz0123456789") == "abcdefghijklmnopqrstuvwxyz0123456789" {
		t.Fatal("shortID should truncate long")
	}
	if shortID("tiny") != "tiny" {
		t.Fatal("shortID short")
	}
}

func TestTruncateAndFormatHelpers(t *testing.T) {
	clipped0, trunc0, total0 := truncateRunes("abc", 0)
	if clipped0 != "abc" || trunc0 || total0 != 3 {
		t.Fatalf("limit 0 = %q %v %d", clipped0, trunc0, total0)
	}
	if _, truncated, _ := truncateRunes("abc", 10); truncated {
		t.Fatal("under limit")
	}
	clipped, truncated, total := truncateRunes("abcdef", 3)
	if !truncated || clipped != "abc" || total != 6 {
		t.Fatalf("truncateRunes = %q %v %d", clipped, truncated, total)
	}

	if got := truncateInline("short", 10); got != "short" {
		t.Fatalf("inline short = %q", got)
	}
	if got := truncateInline("abcdefghij", 4); !strings.Contains(got, "...(+6 chars)") {
		t.Fatalf("inline = %q", got)
	}

	if got := truncateBlockValue("short", 10); got != "short" {
		t.Fatalf("block short = %q", got)
	}
	if got := truncateBlockValue(strings.Repeat("x", 10), 4); !strings.Contains(got, "showing first 4 of 10") {
		t.Fatalf("block = %q", got)
	}

	if firstNonEmptyString("", "  ", "x") != "x" {
		t.Fatal("firstNonEmpty")
	}
	if firstNonEmptyString("", " ") != "" {
		t.Fatal("all empty")
	}

	if formatDuration(500 * time.Millisecond) != "500ms" {
		t.Fatalf("ms = %q", formatDuration(500*time.Millisecond))
	}
	if formatDuration(1500 * time.Millisecond) != "1.5s" {
		t.Fatalf("s = %q", formatDuration(1500*time.Millisecond))
	}

	s := testStyles()
	if renderProgressBar(50, 2, s) != "" {
		t.Fatal("narrow bar")
	}
	bar := stripANSIForTest(renderProgressBar(50, 10, s))
	if !strings.HasPrefix(bar, "[") || !strings.HasSuffix(bar, "]") {
		t.Fatalf("bar = %q", bar)
	}
	if renderProgressBar(-10, 10, s) == "" || renderProgressBar(200, 10, s) == "" {
		t.Fatal("clamped bar")
	}
}

func TestWrapLineAndToolResultEdges(t *testing.T) {
	// wrapLine：空/maxWidth<=0/短行/长行折行
	if got := wrapLine("", 10); len(got) != 1 {
		t.Fatalf("empty = %v", got)
	}
	if got := wrapLine("x", 0); len(got) != 1 {
		t.Fatalf("zero width = %v", got)
	}
	if got := wrapLine("short", 80); len(got) != 1 {
		t.Fatalf("short = %v", got)
	}
	long := strings.Repeat("word ", 30)
	if got := wrapLine(long, 20); len(got) < 2 {
		t.Fatalf("long wrap = %v", got)
	}

	// RenderToolResult 失败态
	r := NewRenderer(testStyles(), 100)
	out := stripANSIForTest(r.RenderToolResult("Bash", "boom", false, 0))
	if !strings.Contains(out, "boom") {
		t.Fatalf("fail result = %q", out)
	}
}

func TestWrapAndBubbleWidth(t *testing.T) {
	if got := wrapText("x", 0); len(got) != 1 {
		t.Fatalf("wrap 0 = %v", got)
	}
	if got := wrapText("a\n\nb", 80); len(got) != 3 {
		t.Fatalf("blank line = %v", got)
	}

	if bubbleWidth(10) != 10 {
		t.Fatalf("tiny = %d", bubbleWidth(10))
	}
	if bubbleWidth(80) != 80 {
		t.Fatalf("mid = %d", bubbleWidth(80))
	}
	if bubbleWidth(120) != 118 {
		t.Fatalf("120 = %d", bubbleWidth(120))
	}
	if bubbleWidth(200) != 144 {
		t.Fatalf("wide = %d", bubbleWidth(200))
	}
}

func TestStreamHelpersAndDispatchType(t *testing.T) {
	s := testStyles()

	dispatch := &AgentDispatchMessage{AgentName: "sub", Event: "dispatch", Task: "do"}
	if dispatch.Type() != MsgTypeAgent {
		t.Fatalf("dispatch type = %q", dispatch.Type())
	}
	if out := stripANSIForTest(dispatch.Render(s, 80)); !strings.Contains(out, "do") {
		t.Fatalf("dispatch render = %q", out)
	}

	// stream 基础
	if got := prefixLines([]string{"a", "b"}, "> ", "  "); len(got) != 2 {
		t.Fatalf("prefixLines = %v", got)
	}
	if got := normalizeBlankLines([]string{"a", "", "", "b"}); len(got) < 2 {
		t.Fatalf("normalize = %v", got)
	}

	// renderAIStream 预格式化与普通
	if out := renderAIStream(s, "plain text", false, 80); out == "" {
		t.Fatal("ai stream empty")
	}
	if out := renderAIStream(s, "styled\nlines", true, 80); out == "" {
		t.Fatal("ai prestyled empty")
	}

	// renderMetaLine
	if renderMetaLine(s, 0, 0) != "" {
		t.Fatal("empty meta")
	}
	if out := stripANSIForTest(renderMetaLine(s, 10, time.Second)); !strings.Contains(out, "10 tokens") {
		t.Fatalf("meta = %q", out)
	}
	if out := stripANSIForTest(renderMetaLine(s, 0, 2*time.Second)); !strings.Contains(out, "2.0s") {
		t.Fatalf("meta dur = %q", out)
	}

	if streamContentWidth(50) <= 0 {
		t.Fatal("stream width")
	}
	if streamContentWidth(20) <= 0 {
		t.Fatal("narrow stream width")
	}

	// toolInvocationSummary
	_ = toolInvocationSummary("read", map[string]any{"path": "/x"})
	_ = toolInvocationSummary("Bash", map[string]any{"command": "ls"})
}

func TestBuildToolDetailBlocks(t *testing.T) {
	blocks := buildToolDetailBlocks("read", map[string]any{"path": "/a/b", "offset": 1})
	if len(blocks) == 0 {
		t.Fatal("read should produce blocks")
	}
	if blocks := buildToolDetailBlocks("Bash", map[string]any{"command": "ls -la"}); len(blocks) == 0 {
		t.Fatal("bash blocks")
	}
	// write / edit / search / fetch / todo 各自的明细键
	for _, tc := range []struct {
		name   string
		params map[string]any
	}{
		{"write", map[string]any{"path": "p", "content": strings.Repeat("c", 50)}},
		{"edit", map[string]any{"path": "p", "old_text": "a", "new_text": "b"}},
		{"glob", map[string]any{"pattern": "*.go"}},
		{"grep", map[string]any{"pattern": "foo", "path": "src"}},
		{"webfetch", map[string]any{"url": "https://example.com"}},
		{"todo", map[string]any{"todos": []any{map[string]any{"content": "t", "status": "pending"}}}},
		{"unknown_tool", map[string]any{"k": "v"}},
	} {
		_ = buildToolDetailBlocks(tc.name, tc.params)
	}
}

func TestRendererSettersAndRenders(t *testing.T) {
	s := testStyles()
	r := NewRenderer(s, 100)
	r.SetWidth(120)
	r.SetChromaTheme("monokai")
	r.SetAgentNameMap(map[string]string{"sub1": "Verifier"})
	r.SetAgentLabels("主智能体", "子智能体")
	r.SetMainAgentName("Lead")
	r.EnableMarkdown(false)

	if r.displayAgentName("sub1") != "Verifier" {
		t.Fatalf("name map = %q", r.displayAgentName("sub1"))
	}
	if r.displayAgentName("other") != "other" {
		t.Fatalf("name raw = %q", r.displayAgentName("other"))
	}
	if r.displayAgentName("") != "" {
		t.Fatal("empty name")
	}
	if got := r.maybeRenderMarkdown("plain", true); got != "plain" {
		t.Fatalf("md disabled = %q", got)
	}
	r.EnableMarkdown(true)
	if got := r.maybeRenderMarkdown("# t", true); got == "" {
		t.Fatal("md enabled empty")
	}
	if got := r.maybeRenderMarkdown("# t", false); got == "" {
		t.Fatal("md streaming empty")
	}

	ts := time.Now()
	for _, out := range []string{
		r.RenderUserInputAt("hi", ts),
		r.RenderUserInput("hi"),
		r.RenderAIResponseAt("resp", 10, time.Second, true, ts),
		r.RenderAIResponseAtWithCopy("resp", 0, 0, false, ts, "Copy"),
		r.RenderAIResponseAtWithActions("resp", 0, 0, true, ts, []BubbleAction{{Kind: "copy", Label: "Copy"}}),
		r.RenderAIResponse("resp", 1, time.Millisecond, false),
		r.RenderToolCall("Bash", map[string]any{"command": "ls"}),
		r.RenderToolEvent("Bash", map[string]any{"command": "ls"}, "success", "out", time.Second),
		r.RenderToolResult("Bash", "out", true, time.Second),
		r.RenderAgentTask("sub", "task", "goal", 50, 1, 2, "running", time.Second, []string{"r1"}),
		r.RenderAgentTaskAt("sub", "id", "src", "sid", "dispatch", "task", ts),
		r.RenderAgentFinal("sub", "final"),
		r.RenderAgentFinalAt("sub", "id", "src", "sid", "result", "final", ts),
		r.RenderAgentFinalAtWithCopy("sub", "final", ts, "Copy"),
		r.RenderAgentFinalAtWithActions("sub", "id", "src", "sid", "result", "final", ts, []BubbleAction{{Kind: "download", Label: "Save"}}),
		r.RenderPlan("title", "desc", []PlanStep{{Number: 1, Description: "s", Status: "pending"}}),
		r.RenderThinking("think", time.Second, true, []ThinkingStep{{Description: "s", Status: "done"}}),
		r.RenderThinkingWithHint("think", time.Second, false, nil, "Ctrl+E"),
		r.RenderSystem("sys", "info"),
		r.RenderSystemPreStyled("pre", "warning"),
	} {
		if out == "" {
			t.Fatal("empty render output")
		}
	}
}
