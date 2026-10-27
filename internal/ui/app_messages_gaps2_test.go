// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

package ui

// app_messages 第二轮补缺：trackBubbleActionsAt 四守卫、refreshAILive 无
// 渲染器回退、clearCurrentThinking nil 守卫、archiveAgentMessage 同文本去重、
// item 流的思考清除/过滤/完成兜底、tool 卡二段参数回填与无渲染器回退、
// bash 取消/完成收尾（无 inflight 卡路径）。

import (
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/state"
	"github.com/eosaios/eos/internal/ui/components/messages"
)

// 不可达清单：trackBubbleActionsAt 的 kinds 空臂——bubbleActionsForEntry 对
// 可点击条目恒产出 Kind=copy 的动作，kinds 不可能为空（防御分支）。

func TestTrackBubbleActionsAtGuards(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// nil 渲染器：不登记
	app.msgRenderer = nil
	app.trackBubbleActionsAt(0, 0, historyEntry{kind: "ai", content: "x"}, "x")
	if len(app.actionHits) != 0 {
		t.Fatalf("nil renderer should not track, hits = %d", len(app.actionHits))
	}

	app.msgRenderer = messages.NewRenderer(app.styles, 80)
	// 无可用动作：不登记
	app.trackBubbleActionsAt(0, 0, historyEntry{kind: "system", content: "x"}, "x")
	if len(app.actionHits) != 0 {
		t.Fatalf("no actions should not track, hits = %d", len(app.actionHits))
	}
	// 空 payload：不登记
	app.trackBubbleActionsAt(0, 0, historyEntry{kind: "ai", content: "  "}, "x")
	if len(app.actionHits) != 0 {
		t.Fatalf("empty payload should not track, hits = %d", len(app.actionHits))
	}
}

func TestRefreshAILiveWithoutRendererFallsBackToRawText(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.msgRenderer = nil
	app.state.Thinking = true
	state.SetThinking(true)
	t.Cleanup(func() { state.SetThinking(false) })
	app.thinkingLive.WriteString("思考中")
	app.aiLive.WriteString("回复中")
	// 无渲染器路径：blocks 直接落原始文本，不 panic、不 ClearLive
	app.refreshAILive()
	if app.aiLive.String() != "回复中" {
		t.Fatalf("live buffer changed unexpectedly: %q", app.aiLive.String())
	}
}

func TestClearCurrentThinkingNilGuard(t *testing.T) {
	var nilModel *AppModel
	nilModel.clearCurrentThinking() // nil 接收者：不 panic
}

func TestArchiveAgentMessageSkipsDuplicate(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.appendHistory(historyEntry{kind: "ai", content: "same text"})
	app.aiLive.WriteString("same text")
	app.archiveAgentMessage()
	if got := countHistoryKind(app, "ai"); got != 1 {
		t.Fatalf("duplicate ai entry archived, ai count = %d", got)
	}
}

func TestItemDeltaThinkingAndFilterArms(t *testing.T) {
	setTestHome(t)

	t.Run("agent delta clears active thinking", func(t *testing.T) {
		app := newTestAppModel(t)
		app.state.Processing = true
		app.activeItemID = "i1"
		app.thinkingLive.WriteString("thinking...")
		app.state.Thinking = true
		app.handleItemDelta(ItemDeltaMsg{ItemID: "i1", DeltaType: "text", Delta: "answer"})
		if app.state.Thinking {
			t.Fatal("agent delta should clear thinking state")
		}
		if app.thinkingLive.String() != "" {
			t.Fatalf("thinking buffer = %q, want cleared", app.thinkingLive.String())
		}
		if app.aiLive.String() != "answer" {
			t.Fatalf("live = %q, want answer", app.aiLive.String())
		}
	})

	t.Run("reasoning delta with mismatched item id ignored", func(t *testing.T) {
		app := newTestAppModel(t)
		app.state.Processing = true
		app.activeItemID = "i1"
		app.thinkingLive.WriteString("seed")
		app.handleItemDelta(ItemDeltaMsg{ItemID: "i2", DeltaType: "reasoning", Delta: "other"})
		if app.thinkingLive.String() != "seed" {
			t.Fatalf("mismatched reasoning delta leaked: %q", app.thinkingLive.String())
		}
	})
}

func TestStartReasoningItemSameIDNoop(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.activeItemID = "i1"
	anchor := time.Now()
	app.reasoningStartTime = anchor
	app.startReasoningItem("i1")
	if !app.reasoningStartTime.Equal(anchor) {
		t.Fatal("same-id reasoning item should be a no-op")
	}
	// 新 id：重置思考缓冲并点亮思考态
	app.thinkingLive.WriteString("stale")
	app.startReasoningItem("i2")
	if app.thinkingLive.String() != "" || !app.state.Thinking {
		t.Fatalf("new item should reset thinking, buf = %q thinking = %v", app.thinkingLive.String(), app.state.Thinking)
	}
}

func TestReasoningCompletedFallbacks(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// Reasoning 空回退 thinkingLive；reasoningStartTime 零回退 currentAIStartTime
	app.activeItemID = "i1"
	app.thinkingLive.WriteString("buffered reasoning")
	app.reasoningStartTime = time.Time{}
	app.currentAIStartTime = time.Now().Add(-time.Second)
	app.handleReasoningCompleted(ItemCompletedMsg{ItemType: "reasoning"})
	found := false
	for _, h := range app.history {
		if h.kind == "reasoning" && strings.Contains(h.content, "buffered reasoning") {
			found = true
		}
	}
	if !found {
		t.Fatalf("buffered reasoning should archive, history = %+v", app.history)
	}
}

func TestItemCompletedUnknownTypeIgnored(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	before := len(app.history)
	app.handleItemCompleted(ItemCompletedMsg{ItemType: "bogus_type"})
	if len(app.history) != before {
		t.Fatal("unknown item type should be ignored")
	}
}

func TestToolCallSecondStageParamsBackfill(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// 一段：无参数建卡
	_ = app.handleToolCall(ToolCallMsg{ID: "t1", Name: "bash"})
	// 二段：带参数回填既有卡（首卡 toolParams 为空才回填）
	_ = app.handleToolCall(ToolCallMsg{ID: "t1", Name: "bash", Params: map[string]any{"command": "ls"}})
	found := false
	for _, h := range app.history {
		if h.kind == "tool" && h.toolName == "bash" && len(h.toolParams) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("second-stage params not backfilled, history = %+v", app.history)
	}
	// 三段：参数已存在 → 不重复回填（不 panic 即可）
	_ = app.handleToolCall(ToolCallMsg{ID: "t1", Name: "bash", Params: map[string]any{"command": "ls -la"}})
}

func TestToolCallWithoutRendererFallback(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.msgRenderer = nil
	_ = app.handleToolCall(ToolCallMsg{ID: "t9", Name: "bash"})
	if got := countHistoryKind(app, "tool"); got != 1 {
		t.Fatalf("tool card = %d, want 1", got)
	}
}

func TestToolResultBashFinalizationWithoutTrack(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.state.Processing = true
	app.shell.SetProcessing(true)
	app.setActiveCancel(func() {})

	// 取消（无 inflight 卡）：无 track 时 name 取 msg.ID，用 bash 命中取消臂
	_ = app.handleToolResult(ToolResultMsg{ID: "bash", Status: "canceled"})
	if app.activeCancel != nil {
		t.Fatal("canceled bash should clear active cancel")
	}

	// 完成（无 inflight 卡）：建卡并结束处理态
	app.state.Processing = true
	_ = app.handleToolResult(ToolResultMsg{ID: "bash", Status: "success", Output: "ok"})
	if app.state.Processing {
		t.Fatal("completed bash should clear processing")
	}
	found := false
	for _, h := range app.history {
		if h.kind == "tool" && h.toolName == "bash" && h.toolOutput == "ok" {
			found = true
		}
	}
	if !found {
		t.Fatalf("untracked result should append card, history = %+v", app.history)
	}
}
