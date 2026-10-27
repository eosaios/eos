package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHandleItemAndInvokeMsgs(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// item.started agent_message
	if next, _ := app.handleItemStartedMsg(ItemStartedMsg{ItemID: "i1", ItemType: "agent_message"}); next == nil {
		t.Fatal("item started")
	}
	// reasoning
	app.handleItemStartedMsg(ItemStartedMsg{ItemID: "r1", ItemType: "reasoning"})

	// delta：非 Processing 早退
	app.state.Processing = false
	if next, _ := app.handleItemDeltaMsg(ItemDeltaMsg{ItemID: "i1", Delta: "x"}); next == nil {
		t.Fatal("delta idle")
	}
	// Processing 时写入
	app.state.Processing = true
	app.handleItemDeltaMsg(ItemDeltaMsg{ItemID: "i1", Delta: "hello"})

	// completed
	if next, _ := app.handleItemCompletedMsg(ItemCompletedMsg{ItemID: "i1", Text: "done"}); next == nil {
		t.Fatal("completed")
	}

	// invoke done：非 Processing 早退
	app.state.Processing = false
	if next, _ := app.handleInvokeDoneMsg(InvokeDoneMsg{Content: "c"}); next == nil {
		t.Fatal("invoke idle")
	}
	// Processing 结束
	app.state.Processing = true
	if next, _ := app.handleInvokeDoneMsg(InvokeDoneMsg{Content: "c"}); next == nil {
		t.Fatal("invoke done")
	}
	if app.state.Processing {
		t.Fatal("should stop processing")
	}
}

func TestHandleThinkingToolAgentError(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// thinking 非 Processing
	app.state.Processing = false
	app.handleThinkingMsg(ThinkingMsg{Content: "t"})

	// thinking 流式
	app.state.Processing = true
	app.handleThinkingMsg(ThinkingMsg{Content: "think", Done: false})
	app.handleThinkingMsg(ThinkingMsg{Content: "think2", Done: true})

	// tool call/result
	app.handleToolCallMsg(ToolCallMsg{ID: "t1", Name: "Bash"})
	app.handleToolResultMsg(ToolResultMsg{ID: "t1", Status: "success", Output: "ok"})

	// agent task / final
	app.handleAgentTaskMsg(AgentTaskMsg{AgentName: "sub", Task: "do", Event: "dispatch"})
	app.handleAgentFinalMsg(AgentFinalMsg{AgentName: "sub", Content: "result", Event: "result"})
	if app.state.Processing {
		t.Fatal("agent final should stop processing")
	}

	// error
	app.handleErrorMsg(ErrorMsg{Err: errors.New("boom")})

	// clear copied
	app.history = append(app.history, historyEntry{kind: "user", content: "x"})
	app.handleClearCopiedMsg(clearCopiedMsg{idx: 0})
	app.handleClearCopiedMsg(clearCopiedMsg{idx: 99})

	// mode changed
	app.handleModeChangedMsg(ModeChangedMsg{Mode: "plan", PreviousMode: "auto"})
	if app.state.ExecutionMode != "plan" {
		t.Fatalf("mode = %q", app.state.ExecutionMode)
	}
	app.handleModeChangedMsg(ModeChangedMsg{Mode: "  "})
}

func TestHandleSetupWindowAndHints(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.handleSetupCancelMsg(struct{}{})
	// setup complete 会走保存流程，用最小配置
	_ = tea.WindowSizeMsg{}
	app.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 120, Height: 40})
	if app.width != 120 || app.height != 40 {
		t.Fatalf("size = %dx%d", app.width, app.height)
	}

	app.handleShowHintsMsg(ShowHintsMsg{Type: "slash"})
}

func TestShouldSendMessage(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 空输入
	if ok, _ := app.shouldSendMessage(); ok {
		t.Fatal("empty")
	}
	// 仅 / 或 @
	app.shell.SetInputValue("/")
	if ok, _ := app.shouldSendMessage(); ok {
		t.Fatal("slash only")
	}
	app.shell.SetInputValue("@")
	if ok, _ := app.shouldSendMessage(); ok {
		t.Fatal("at only")
	}

	// 普通消息 + AI 模式 + 空闲
	app.state.Mode = "ai"
	app.state.Processing = false
	app.shell.SetInputValue("hello")
	if ok, _ := app.shouldSendMessage(); !ok {
		t.Fatal("should send")
	}

	// Processing 中不发送
	app.state.Processing = true
	if ok, _ := app.shouldSendMessage(); ok {
		t.Fatal("processing")
	}

	// 有效斜杠命令
	app.state.Processing = false
	app.shell.SetInputValue("/help")
	ok, _ := app.shouldSendMessage()
	if ok {
		t.Fatal("slash command should not send as message")
	}
}

func TestClearPredictionAndToggleThinking(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.shell.SetPrediction("pred")
	app.clearPrediction()
	if app.shell.HasPrediction() {
		t.Fatal("clearPrediction")
	}

	app.state.Thinking = true
	app.toggleThinkingExpand()
	app.state.Thinking = false
	cmd := app.toggleThinkingExpand()
	_ = cmd

	app.syncPredictionState()
	app.canPredict()
}
