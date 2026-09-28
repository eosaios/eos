package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	uiadapter "github.com/eosaios/eos/internal/ui/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
)

// ---------- slash_runtime 纯函数 ----------

func TestModeInputValidators(t *testing.T) {
	if isSupportedExecutionModeInput("") || isSupportedExecutionModeInput("  ") {
		t.Fatal("empty execution mode should be rejected")
	}
	if !isSupportedExecutionModeInput("plan") || !isSupportedExecutionModeInput("自动") {
		t.Fatal("known execution modes should pass")
	}
	if isSupportedExecutionModeInput("nope") {
		t.Fatal("unknown execution mode should be rejected")
	}

	if isSupportedAccessModeInput("") {
		t.Fatal("empty access mode")
	}
	if !isSupportedAccessModeInput("read-only") || !isSupportedAccessModeInput("工作区写入") {
		t.Fatal("known access modes")
	}
	if isSupportedAccessModeInput("nope") {
		t.Fatal("unknown access mode")
	}

	if isSupportedApprovalModeInput("") {
		t.Fatal("empty approval mode")
	}
	if !isSupportedApprovalModeInput("on-request") {
		t.Fatal("on-request should pass")
	}
	if isSupportedApprovalModeInput("nope") {
		t.Fatal("unknown approval mode")
	}
}

func TestNormalizePlanPromptStyle(t *testing.T) {
	cases := map[string]string{
		"":           "concise",
		"  ":         "concise",
		"CONCISE":    "concise",
		"detailed":   "detailed",
		"custom:x":   "custom:x",
		"custom:":    "concise",
		"custom:  ":  "concise",
		"whatever":   "custom:whatever",
		"My Style":   "custom:My Style",
	}
	for in, want := range cases {
		if got := normalizePlanPromptStyle(in); got != want {
			t.Fatalf("normalizePlanPromptStyle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncateBlock(t *testing.T) {
	// 按字节截断
	got := truncateBlock("abcdefghij", 0, 4)
	if got != "abcd\n[truncated]" {
		t.Fatalf("byte truncate = %q", got)
	}
	// 按行截断
	got = truncateBlock("a\nb\nc\nd", 2, 0)
	if got != "a\nb\n[truncated]" {
		t.Fatalf("line truncate = %q", got)
	}
	// 不截断
	if got := truncateBlock("x\ny", 10, 100); got != "x\ny" {
		t.Fatalf("no truncate = %q", got)
	}
	// CRLF/CR 归一 + TrimSpace
	if got := truncateBlock("  a\r\nb\r ", 0, 0); got != "a\nb" {
		t.Fatalf("normalize = %q", got)
	}
}

func TestBlankFallbackAndOptionalText(t *testing.T) {
	if blankFallback("  ", "fb") != "fb" {
		t.Fatal("blank fallback")
	}
	if blankFallback(" x ", "fb") != "x" {
		t.Fatal("blank trim")
	}
	if optionalIntText(nil, "-") != "-" {
		t.Fatal("nil int")
	}
	n := 42
	if optionalIntText(&n, "-") != "42" {
		t.Fatal("int value")
	}
	if optionalFloatText(nil, "-") != "-" {
		t.Fatal("nil float")
	}
	f := 1.5
	if optionalFloatText(&f, "-") != "$1.500000" {
		t.Fatalf("float value = %q", optionalFloatText(&f, "-"))
	}
}

func TestSessionMetadataHelpers(t *testing.T) {
	if sessionMessageTurnID(nil) != "" {
		t.Fatal("nil turn_id")
	}
	if sessionMessageTurnID(map[string]any{"turn_id": 3}) != "" {
		t.Fatal("non-string turn_id")
	}
	if sessionMessageTurnID(map[string]any{"turn_id": "  t1 "}) != "t1" {
		t.Fatal("turn_id trim")
	}

	if sessionMessageKind(nil) != "" {
		t.Fatal("nil kind")
	}
	if sessionMessageKind(map[string]any{"kind": 1}) != "" {
		t.Fatal("non-string kind")
	}
	if sessionMessageKind(map[string]any{"kind": " Reasoning "}) != "reasoning" {
		t.Fatalf("kind = %q", sessionMessageKind(map[string]any{"kind": " Reasoning "}))
	}

	if sessionMessageToolName(nil) != "" {
		t.Fatal("nil tool name")
	}
	if sessionMessageToolName(map[string]any{"tool_call": "x"}) != "" {
		t.Fatal("non-map tool_call")
	}
	if sessionMessageToolName(map[string]any{"tool_call": map[string]any{"name": " Bash "}}) != "Bash" {
		t.Fatal("tool name trim")
	}

	if mapString(nil, "k") != "" {
		t.Fatal("nil mapString")
	}
	if mapString(map[string]any{"k": 1}, "k") != "" {
		t.Fatal("non-string mapString")
	}
	if mapString(map[string]any{"k": " v "}, "k") != "v" {
		t.Fatal("mapString trim")
	}

	// sessionTimestampLabel：saved_at 优先，其次 UpdatedAt
	meta := coreapi.Session{Metadata: map[string]any{"saved_at": "2026-01-02"}}
	if sessionTimestampLabel(meta) != "2026-01-02" {
		t.Fatal("saved_at")
	}
	meta = coreapi.Session{UpdatedAt: time.Date(2026, 3, 4, 5, 6, 0, 0, time.UTC)}
	if sessionTimestampLabel(meta) != "2026-03-04 05:06" {
		t.Fatalf("updated_at = %q", sessionTimestampLabel(meta))
	}
	if sessionTimestampLabel(coreapi.Session{}) != "" {
		t.Fatal("empty timestamp")
	}

	// sessionLabelFromMeta：title → preview → summary
	if sessionLabelFromMeta(coreapi.Session{Metadata: map[string]any{"title": "T", "preview": "P"}}) != "T" {
		t.Fatal("title first")
	}
	if sessionLabelFromMeta(coreapi.Session{Metadata: map[string]any{"preview": " P "}}) != "P" {
		t.Fatal("preview")
	}
	if sessionLabelFromMeta(coreapi.Session{Metadata: map[string]any{"summary": "S"}}) != "S" {
		t.Fatal("summary")
	}
	if sessionLabelFromMeta(coreapi.Session{}) != "" {
		t.Fatal("empty label")
	}

	// sessionRoundsFromMeta / mapInt 多数值类型
	if sessionRoundsFromMeta(coreapi.Session{Metadata: map[string]any{"rounds": 7}}) != 7 {
		t.Fatal("int rounds")
	}
	if sessionRoundsFromMeta(coreapi.Session{Metadata: map[string]any{"rounds": int64(8)}}) != 8 {
		t.Fatal("int64 rounds")
	}
	if sessionRoundsFromMeta(coreapi.Session{Metadata: map[string]any{"rounds": 9.0}}) != 9 {
		t.Fatal("float64 rounds")
	}
	if sessionRoundsFromMeta(coreapi.Session{Metadata: map[string]any{"rounds": "x"}}) != 0 {
		t.Fatal("bad rounds")
	}
}

// ---------- app_confirm / selection 纯函数 ----------

func TestBubbleActionHitHasAction(t *testing.T) {
	h := bubbleActionHit{actions: []string{"copy", " Download "}}
	if !h.hasAction("copy") || !h.hasAction("COPY") || !h.hasAction("download") {
		t.Fatalf("hasAction failed: %v", h.actions)
	}
	if h.hasAction("delete") {
		t.Fatal("missing action")
	}
	if (bubbleActionHit{}).hasAction("copy") {
		t.Fatal("empty actions")
	}
}

func TestSanitizePlanFileNameSegment(t *testing.T) {
	if sanitizePlanFileNameSegment("  ") != "" {
		t.Fatal("blank")
	}
	if got := sanitizePlanFileNameSegment("abc-123_x.y"); got != "abc-123_x.y" {
		t.Fatalf("safe chars = %q", got)
	}
	// 非法字符折叠成单个 '-'
	if got := sanitizePlanFileNameSegment("a/b\\c:d"); got != "a-b-c-d" {
		t.Fatalf("illegal chars = %q", got)
	}
	// 连续非法字符只产生一个 dash
	if got := sanitizePlanFileNameSegment("a///b"); got != "a-b" {
		t.Fatalf("collapse = %q", got)
	}
	// 首尾 dash 去掉
	if got := sanitizePlanFileNameSegment("/abc/"); got != "abc" {
		t.Fatalf("trim dash = %q", got)
	}
}

func TestUniqueAvailablePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	// 不存在时原样返回
	if got := uniqueAvailablePath(path); got != path {
		t.Fatalf("fresh = %q", got)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 已存在 → plan-2.md
	got := uniqueAvailablePath(path)
	if filepath.Base(got) != "plan-2.md" {
		t.Fatalf("first collision = %q", got)
	}
	if err := os.WriteFile(got, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 再撞 → plan-3.md
	if got := uniqueAvailablePath(path); filepath.Base(got) != "plan-3.md" {
		t.Fatalf("second collision = %q", got)
	}
}

func TestClampInt(t *testing.T) {
	if clampInt(5, 0, 10) != 5 {
		t.Fatal("inside")
	}
	if clampInt(-1, 0, 10) != 0 {
		t.Fatal("below")
	}
	if clampInt(99, 0, 10) != 10 {
		t.Fatal("above")
	}
}

func TestActionLabel(t *testing.T) {
	app := &AppModel{state: AppState{Language: "zh"}}
	if got := app.actionLabel("copy"); got != "复制" {
		t.Fatalf("copy label = %q", got)
	}
	if got := app.actionLabel("download"); got != "下载" {
		t.Fatalf("download label = %q", got)
	}
	if got := app.actionLabel("unknown"); got != "unknown" {
		t.Fatalf("unknown label = %q", got)
	}
	// 大小写/空白不敏感
	if got := app.actionLabel(" COPY "); got != "复制" {
		t.Fatalf("COPY label = %q", got)
	}
	app.state.Language = "en"
	if got := app.actionLabel("copy"); got != "Copy" {
		t.Fatalf("en copy = %q", got)
	}
}

// ---------- ConvertEvent 剩余分支 ----------

func TestParseToolParamsAndFirstNonEmpty(t *testing.T) {
	if got := parseToolParams(""); len(got) != 0 {
		t.Fatalf("empty params = %v", got)
	}
	if got := parseToolParams("not-json"); len(got) != 0 {
		t.Fatalf("bad json = %v", got)
	}
	got := parseToolParams(`{"a":1}`)
	if got["a"] == nil {
		t.Fatalf("params = %v", got)
	}
	if firstNonEmpty("", "  ", "x", "y") != "x" {
		t.Fatal("firstNonEmpty")
	}
	if firstNonEmpty("", " ") != "" {
		t.Fatal("all empty")
	}
}

func TestEventHelpers(t *testing.T) {
	e := uiadapter.RuntimeEvent{
		RID:  "rid-1",
		Data: map[string]any{"a": "  va  ", "b": 1, "list": []any{" x ", "", 2, "y"}, "flag": true, "slist": []string{"p"}},
	}
	if eventString(e.Data, "missing", "a") != "va" {
		t.Fatal("eventString multi-key")
	}
	if eventString(nil, "a") != "" {
		t.Fatal("nil data")
	}
	if eventID(e, "missing") != "rid-1" {
		t.Fatalf("eventID fallback = %q", eventID(e, "missing"))
	}
	if eventID(e, "a") != "va" {
		t.Fatal("eventID key")
	}
	if eventText(e, "missing") != "" && e.Content == "" {
		// Content 为空时回落 ""
	}
	e.Content = " body "
	if eventText(e, "missing") != "body" {
		t.Fatalf("eventText content = %q", eventText(e, "missing"))
	}

	if payloadString(e, "missing") != "" || payloadString(uiadapter.RuntimeEvent{}, "a") != "" {
		t.Fatal("payloadString empty")
	}
	if payloadString(e, "a") != "  va  " {
		t.Fatal("payloadString raw")
	}

	if got := eventStringSlice(e.Data, "list"); len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Fatalf("string slice = %v", got)
	}
	if got := eventStringSlice(e.Data, "slist"); len(got) != 1 || got[0] != "p" {
		t.Fatalf("[]string = %v", got)
	}
	if got := eventStringSlice(e.Data, "missing"); got != nil {
		t.Fatalf("missing slice = %v", got)
	}
	if got := eventStringSlice(nil, "list"); got != nil {
		t.Fatal("nil slice")
	}
	if got := eventStringSlice(e.Data, "a"); got != nil {
		t.Fatalf("non-slice = %v", got)
	}

	if !eventBool(e.Data, "flag") {
		t.Fatal("eventBool true")
	}
	if eventBool(e.Data, "missing") || eventBool(nil, "flag") {
		t.Fatal("eventBool false")
	}

	if got := eventParams(e); got != nil {
		t.Fatalf("no args should be nil, got %v", got)
	}
	if got := eventParams(uiadapter.RuntimeEvent{Data: map[string]any{"arguments": `{"k":"v"}`}}); got["k"] != "v" {
		t.Fatalf("json args = %v", got)
	}
	if got := eventParams(uiadapter.RuntimeEvent{Data: map[string]any{"input": map[string]any{"k": 1}}}); got["k"] == nil {
		t.Fatalf("map input = %v", got)
	}
	if got := eventParams(uiadapter.RuntimeEvent{Data: map[string]any{"arguments": "not-json"}}); got != nil {
		t.Fatalf("bad args = %v", got)
	}
	if got := eventParams(uiadapter.RuntimeEvent{Data: map[string]any{"arguments": ""}}); got != nil {
		t.Fatalf("empty args = %v", got)
	}
}

func TestAgentEventKind(t *testing.T) {
	cases := map[string]string{
		"agent.task":                "dispatch",
		string(protocol.EventTypeAgentStarted):   "dispatch",
		string(protocol.EventTypeAgentProgress):  "update",
		"agent.final":               "result",
		string(protocol.EventTypeAgentDone):      "result",
		string(protocol.EventTypeAgentFailed):    "failed",
		string(protocol.EventTypeAgentCancelled): "cancelled",
		"unknown":                   "",
	}
	for in, want := range cases {
		if got := agentEventKind(in); got != want {
			t.Fatalf("agentEventKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatPickChip(t *testing.T) {
	msg := BrowserPickSelectedMsg{Ref: "e12", Role: "button", Name: "登录"}
	if got := msg.FormatPickChip(); got != `e12 · button "登录"` {
		t.Fatalf("chip = %q", got)
	}
	// 无 Role 直接返回 ref
	msg = BrowserPickSelectedMsg{Ref: "e1"}
	if got := msg.FormatPickChip(); got != "e1" {
		t.Fatalf("no role = %q", got)
	}
	// Ref 空回落 Selector / element
	msg = BrowserPickSelectedMsg{Selector: "div.x", Role: "generic"}
	if got := msg.FormatPickChip(); got != "div.x · generic" {
		t.Fatalf("selector fallback = %q", got)
	}
	msg = BrowserPickSelectedMsg{Role: "img"}
	if got := msg.FormatPickChip(); got != "element · img" {
		t.Fatalf("element fallback = %q", got)
	}
	// 无 Name 不带引号
	msg = BrowserPickSelectedMsg{Ref: "e", Role: "link"}
	if got := msg.FormatPickChip(); got != "e · link" {
		t.Fatalf("no name = %q", got)
	}
}

func TestConvertEventRemainingBranches(t *testing.T) {
	// ItemDelta
	msg := ConvertEvent(uiadapter.RuntimeEvent{
		Type: string(protocol.EventTypeItemDelta),
		RID:  "r1",
		Data: map[string]any{"item_id": "i1", "delta_type": "text", "delta": "hello"},
	})
	delta, ok := msg.(ItemDeltaMsg)
	if !ok || delta.ItemID != "i1" || delta.Delta != "hello" || delta.RID != "r1" {
		t.Fatalf("delta = %+v", msg)
	}

	// RequestDone
	msg = ConvertEvent(uiadapter.RuntimeEvent{
		Type: string(protocol.EventTypeRequestDone),
		Data: map[string]any{"text": "done"},
	})
	if done, ok := msg.(InvokeDoneMsg); !ok || done.Content != "done" {
		t.Fatalf("request done = %+v", msg)
	}

	// Thinking（task.started 等）
	msg = ConvertEvent(uiadapter.RuntimeEvent{
		Type: string(protocol.EventTypeTaskStarted),
		RID:  "r2",
		Data: map[string]any{"label": "working"},
	})
	thinking, ok := msg.(ThinkingMsg)
	if !ok || thinking.Content != "working" || thinking.Done {
		t.Fatalf("thinking = %+v", msg)
	}

	// agent.task 分派
	msg = ConvertEvent(uiadapter.RuntimeEvent{
		Type: "agent.task",
		RID:  "child",
		Data: map[string]any{
			"task": "do it", "goal": "g",
			"source_agent_name": "parent", "source_agent_id": "p1",
		},
	})
	task, ok := msg.(AgentTaskMsg)
	if !ok || task.AgentName != "child" || task.Event != "dispatch" || task.SourceAgentName != "parent" {
		t.Fatalf("agent.task = %+v", msg)
	}

	// ModeChanged
	msg = ConvertEvent(uiadapter.RuntimeEvent{
		Type: string(protocol.EventTypeModeChanged),
		Data: map[string]any{"new_mode": "plan", "old_mode": "auto"},
	})
	mc, ok := msg.(ModeChangedMsg)
	if !ok || mc.Mode != "plan" || mc.PreviousMode != "auto" {
		t.Fatalf("mode = %+v", msg)
	}

	// error
	msg = ConvertEvent(uiadapter.RuntimeEvent{
		Type: "error",
		RID:  "r3",
		Data: map[string]any{"error": "boom"},
	})
	errMsg, ok := msg.(AIResponseMsg)
	if !ok || errMsg.Type != "error" || errMsg.Content != "boom" {
		t.Fatalf("error = %+v", msg)
	}

	// 未知类型 → nil
	if ConvertEvent(uiadapter.RuntimeEvent{Type: "nope"}) != nil {
		t.Fatal("unknown type should be nil")
	}

	// item.started agent_message
	msg = ConvertEvent(uiadapter.RuntimeEvent{
		Type: string(protocol.EventTypeItemStarted),
		Data: map[string]any{"item": map[string]any{"kind": "agent_message", "id": "m1"}},
	})
	started, ok := msg.(ItemStartedMsg)
	if !ok || started.ItemType != "agent_message" {
		t.Fatalf("item started = %+v", msg)
	}

	// item.completed agent_message
	msg = ConvertEvent(uiadapter.RuntimeEvent{
		Type: string(protocol.EventTypeItemCompleted),
		Data: map[string]any{"item": map[string]any{
			"kind": "agent_message", "id": "m1", "text": "hi", "reasoning": "why",
		}},
	})
	done, ok := msg.(ItemCompletedMsg)
	if !ok || done.Text != "hi" || done.Reasoning != "why" {
		t.Fatalf("item completed = %+v", msg)
	}

	// item.completed tool_call 无 display 用 error
	msg = ConvertEvent(uiadapter.RuntimeEvent{
		Type: string(protocol.EventTypeItemCompleted),
		Data: map[string]any{"item": map[string]any{
			"kind": "tool_call", "id": "t1", "name": "Bash",
			"result": map[string]any{"error": "fail"},
		}},
	})
	tr, ok := msg.(ToolResultMsg)
	if !ok || tr.Output != "fail" || tr.Status != "success" {
		t.Fatalf("tool result = %+v", msg)
	}
}
