package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"
	"time"
)

func TestStripANSIAndRuneIndex(t *testing.T) {
	if stripANSI("plain") != "plain" {
		t.Fatal("plain")
	}
	got := stripANSI("\x1b[1;31mred\x1b[0m")
	if got != "red" {
		t.Fatalf("strip = %q", got)
	}

	if runeIndex("abc", -1) != 0 {
		t.Fatal("neg")
	}
	if runeIndex("abc", 0) != 0 {
		t.Fatal("zero")
	}
	if runeIndex("abc", 10) != 3 {
		t.Fatal("oob")
	}
	if runeIndex("中文", 3) != 1 {
		t.Fatalf("cjk = %d", runeIndex("中文", 3))
	}
}

func TestBubbleActionsForEntry(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	if app.bubbleActionsForEntry(historyEntry{kind: "tool", content: "x"}) != nil {
		t.Fatal("tool no actions")
	}
	if app.bubbleActionsForEntry(historyEntry{kind: "ai", content: "  "}) != nil {
		t.Fatal("empty content")
	}
	acts := app.bubbleActionsForEntry(historyEntry{kind: "ai", content: "hello"})
	if len(acts) != 1 || acts[0].Kind != "copy" {
		t.Fatalf("ai = %+v", acts)
	}
	// plan 模式 + rawMarkdown → copy+download
	acts = app.bubbleActionsForEntry(historyEntry{
		kind: "ai", content: "hi", executionMode: "plan", rawMarkdown: "# p",
	})
	if len(acts) != 2 {
		t.Fatalf("plan = %+v", acts)
	}
	// user / agent.final 也可复制
	if app.bubbleActionsForEntry(historyEntry{kind: "user", content: "u"}) == nil {
		t.Fatal("user")
	}
	if app.bubbleActionsForEntry(historyEntry{kind: "agent.final", content: "a"}) == nil {
		t.Fatal("agent.final")
	}
}

func TestAppendHistoryAndRebuild(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	idx := app.appendHistoryIndex(historyEntry{kind: "user", content: "hello", timestamp: time.Now()})
	if idx != 0 || len(app.history) != 1 {
		t.Fatalf("idx=%d len=%d", idx, len(app.history))
	}
	if !strings.Contains(app.shell.Content(), "hello") {
		t.Fatalf("content = %q", app.shell.Content())
	}

	app.appendHistory(historyEntry{kind: "ai", content: "reply", timestamp: time.Now()})
	if len(app.history) != 2 {
		t.Fatal("append")
	}

	// rebuild 重建内容并登记 actionHits
	app.rebuildHistoryContent()
	if len(app.history) != 2 {
		t.Fatal("rebuild keeps history")
	}
	if !strings.Contains(app.shell.Content(), "hello") || !strings.Contains(app.shell.Content(), "reply") {
		t.Fatalf("rebuild content = %q", app.shell.Content())
	}

	// 空历史 no-op
	app.history = nil
	app.rebuildHistoryContent()
}

func TestAppendSystemAndDiffTheme(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.appendSystem("plain sys", "info")
	app.appendSystemStyled("styled", "error")
	if !strings.Contains(app.shell.Content(), "plain sys") {
		t.Fatal("system")
	}

	if app.diffHighlightTheme() == "" {
		t.Fatal("theme")
	}
	// highlightDiffBlock
	out := app.highlightDiffBlock("+ added\n- removed", 10, 1000)
	if out == "" {
		t.Fatal("diff block")
	}
}

func TestHandleToolCallAndResult(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	cmd := app.handleToolCall(ToolCallMsg{ID: "t1", Name: "Bash", Params: map[string]any{"command": "ls"}})
	if cmd != nil {
		t.Fatal("tool call cmd")
	}
	if len(app.history) != 1 || app.history[0].kind != "tool" {
		t.Fatalf("history = %+v", app.history)
	}
	if _, ok := app.toolInflight["t1"]; !ok {
		t.Fatal("inflight")
	}

	// 同 ID 二次 call：更新参数
	app.handleToolCall(ToolCallMsg{ID: "t1", Name: "Bash", Params: map[string]any{"command": "pwd"}})
	if len(app.history) != 1 {
		t.Fatalf("should update not append: %d", len(app.history))
	}

	// result 更新卡片
	app.handleToolResult(ToolResultMsg{ID: "t1", Name: "Bash", Status: "success", Output: "ok"})
	if _, ok := app.toolInflight["t1"]; ok {
		t.Fatal("should clear inflight")
	}
	if app.history[0].toolOutput != "ok" || !app.history[0].toolSuccess {
		t.Fatalf("result = %+v", app.history[0])
	}

	// 无 track 的 result 新建
	app.handleToolResult(ToolResultMsg{ID: "t2", Name: "Read", Status: "error", Output: "fail"})
	found := false
	for _, e := range app.history {
		if e.toolID == "t2" {
			found = true
			if e.toolSuccess {
				t.Fatal("t2 should fail")
			}
		}
	}
	if !found {
		t.Fatal("t2 entry")
	}
}

func TestHandleAIResponseMsg(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	next, _ := app.handleAIResponseMsg(AIResponseMsg{Type: "final", Content: "answer", RID: "r"})
	if next == nil {
		t.Fatal("ai response")
	}
	if !strings.Contains(app.shell.Content(), "answer") {
		t.Fatalf("content = %q", app.shell.Content())
	}
}

func TestRenderHistoryEntryNilRenderer(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.msgRenderer = nil

	cases := []historyEntry{
		{kind: "user", content: "u"},
		{kind: "ai", content: "a"},
		{kind: "system", content: "s"},
		{kind: "tool", toolOutput: "out"},
		{kind: "agent.final", content: "f"},
		{kind: "reasoning", content: "l1\n\nlast"},
		{kind: "other", content: "o"},
	}
	for _, e := range cases {
		if out := app.renderHistoryEntry(e); out == "" && e.content != "" && e.toolOutput != "" {
			t.Fatalf("empty for %q", e.kind)
		}
	}

	// reasoning 折叠取最后一行
	got := app.renderHistoryEntry(historyEntry{kind: "reasoning", content: "a\n\nfinal line"})
	if got != "final line" {
		t.Fatalf("reasoning = %q", got)
	}
}

func TestRenderHistoryEntryToolStatusFallback(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// toolSuccess → success
	out := app.renderHistoryEntry(historyEntry{kind: "tool", toolName: "Bash", toolSuccess: true})
	if out == "" {
		t.Fatal("success tool")
	}
	// toolOutput 非空 → error
	out = app.renderHistoryEntry(historyEntry{kind: "tool", toolName: "Bash", toolOutput: "err"})
	if out == "" {
		t.Fatal("error tool")
	}
	// 无输出 → running
	out = app.renderHistoryEntry(historyEntry{kind: "tool", toolName: "Bash"})
	if out == "" {
		t.Fatal("running tool")
	}
	// preStyled system
	out = app.renderHistoryEntry(historyEntry{kind: "system", content: "x", preStyled: true})
	if out == "" {
		t.Fatal("prestyled")
	}
}
