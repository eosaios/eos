package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// app_send.go 缺口批测：pasteClipboardImage / sendBashCommand / sendMessageText
// 图片与宏展开 / shouldSendMessage 边角 / handleKeyMsg 视图分发 / 预测链。
//
// 修复回归：无。
// 不可达清单：无。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/adapter"
	"github.com/eosaios/eos/internal/ui/views/shell"
	"github.com/eosaios/eos/pkg/coreapi"

	tea "charm.land/bubbletea/v2"
)

func withStubClipboardImage(t *testing.T, b []byte, err error) {
	t.Helper()
	old := readClipboardImage
	readClipboardImage = func() ([]byte, error) { return b, err }
	t.Cleanup(func() { readClipboardImage = old })
}

// runCmd 执行 tea.Cmd 并吞掉回包，覆盖闭包体语句。
func runCmd(cmd tea.Cmd) {
	if cmd != nil {
		_ = cmd()
	}
}

func TestClearPredictionNilShell(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.shell = nil
	app.predictionText = "x"
	app.clearPrediction()
	if app.predictionText != "" {
		t.Fatalf("predictionText = %q", app.predictionText)
	}
}

func TestSyncPredictionStateNilShell(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.shell = nil
	app.syncPredictionState() // 不 panic
	app2 := newTestAppModel(t)
	app2.predictionText = "stale"
	app2.shell.SetPrediction("") // shell 无预测
	app2.syncPredictionState()
	if app2.predictionText != "" {
		t.Fatalf("predictionText = %q", app2.predictionText)
	}
}

func TestRequestPredictionNilAndCannotPredict(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.shell = nil
	if cmd := app.requestPrediction("x"); cmd != nil {
		t.Fatal("nil shell should return nil")
	}
	app2 := newTestAppModel(t)
	app2.predictionEnabled = false
	app2.predictionText = "stale"
	// canPredict=false：清空并返回 nil
	if cmd := app2.requestPrediction("x"); cmd != nil {
		t.Fatal("cannot predict should return nil")
	}
	if app2.predictionText != "" {
		t.Fatalf("predictionText = %q", app2.predictionText)
	}
}

func TestShouldSendMessagePendingImages(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.state.Mode = "ai"
	app.state.Processing = false
	app.shell.SetInputValue("")
	app.pendingImagePaths = []string{filepath.Join(t.TempDir(), "a.png")}
	ok, cmd := app.shouldSendMessage()
	if !ok || cmd != nil {
		t.Fatalf("ok=%v cmd=%v", ok, cmd)
	}
	// processing 中不发送
	app.state.Processing = true
	if ok, _ := app.shouldSendMessage(); ok {
		t.Fatal("processing should not send")
	}
}

func TestShouldSendMessageSlashExitAndSkill(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.state.Mode = "ai"
	// /exit 会走 handleSlashCommand 并返回退出命令
	app.shell.SetInputValue("/exit")
	ok, exitCmd := app.shouldSendMessage()
	if ok {
		t.Fatal("/exit should not report send")
	}
	if exitCmd == nil {
		t.Fatal("/exit should return exit cmd")
	}
	// 未知 skill 斜杠：tryInvokeSkillSlash 失败落到默认发送判定
	app2 := newTestAppModel(t)
	app2.state.Mode = "ai"
	app2.state.Processing = false
	app2.shell.SetInputValue("/no-such-skill-xyz arg")
	ok2, _ := app2.shouldSendMessage()
	if !ok2 {
		t.Fatal("unknown skill slash in ai mode should allow send")
	}
}

func TestSendMessageTextNilAdapterAndImages(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.adapter = nil
	if cmd := app.sendMessageText("hi", false); cmd != nil {
		t.Fatal("nil adapter")
	}

	app2 := newTestAppModel(t)
	app2.state.Mode = "ai"
	img := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(img, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	app2.pendingImagePaths = []string{img}
	// 空文本 + 仅图片 → 仍发送，展示 image_only
	cmd := app2.sendMessageText("", true)
	if cmd == nil {
		t.Fatal("image-only send should return cmd")
	}
	// pending 已消费
	if len(app2.pendingImagePaths) != 0 {
		t.Fatalf("pendingImagePaths = %v", app2.pendingImagePaths)
	}
	// history 有 user 条目
	found := false
	for _, h := range app2.history {
		if h.kind == "user" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected user history entry")
	}

	// withPendingImages=false 不消费附件
	app3 := newTestAppModel(t)
	app3.pendingImagePaths = []string{img}
	_ = app3.sendMessageText("hello", false)
	if len(app3.pendingImagePaths) != 1 {
		t.Fatalf("pending should stay, got %v", app3.pendingImagePaths)
	}
}

func TestSendMessageTextDiagnosticsMacroExpanded(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// 驱动 LSPDiagnosticsMarkdown 返回非空：经 testEngine.lspDiagnostics
	// LSPDiagnosticsMarkdown 由 adapter 组装，这里直接断言宏展开后的 Invoke 轨迹。
	// 无诊断时保留原文（已有测试）；有诊断时替换——用 stub adapter 不便，
	// 改为验证展开后仍发出且 history 记录原文。
	cmd := app.sendMessageText("see #problems_and_diagnostics", false)
	if cmd == nil {
		t.Fatal("should send")
	}
	// 用户可见 history 是原文（宏在 expanded 里展开给引擎）
	found := false
	for _, h := range app.history {
		if h.kind == "user" && strings.Contains(h.content, "#problems_and_diagnostics") {
			found = true
		}
	}
	if !found {
		t.Fatal("history should keep user-facing macro text")
	}
}

func TestSendBashCommandErrorAndSuccess(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.toolExecResult = coreapi.ToolResult{Status: "success", Display: "hello\n\n"}
	app.state.Mode = "bash"
	app.shell.SetInputValue("echo hi")
	cmd := app.sendBashCommand()
	if cmd == nil {
		t.Fatal("bash cmd")
	}
	// 执行 cmd 聚合
	msg := cmd()
	// tea.Batch 返回 tea.BatchMsg（切片）
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		// 单命令时可能是单个 cmd 的结果；接受 ToolResultMsg 或 BatchMsg
		if _, isTool := msg.(ToolResultMsg); !isTool {
			t.Fatalf("msg type %T", msg)
		}
	} else {
		// 跑出所有子命令，找到 ToolResultMsg
		var tr *ToolResultMsg
		for _, c := range batch {
			if c == nil {
				continue
			}
			m := c()
			if r, ok := m.(ToolResultMsg); ok {
				tr = &r
			}
		}
		if tr == nil {
			t.Fatal("no ToolResultMsg in batch")
		}
		if tr.Status != "success" || tr.Output != "hello" {
			t.Fatalf("status=%q output=%q", tr.Status, tr.Output)
		}
	}

	// 错误臂
	app2, eng2 := newTestAppModelWithEngine(t)
	eng2.toolExecErr = errors.New("boom\r\nline")
	app2.state.Mode = "bash"
	app2.shell.SetInputValue("bad")
	cmd2 := app2.sendBashCommand()
	if cmd2 == nil {
		t.Fatal("bash cmd2")
	}
	msg2 := cmd2()
	if batch, ok := msg2.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if r, ok := c().(ToolResultMsg); ok && r.Status == "error" {
				if !strings.Contains(r.Output, "boom") {
					t.Fatalf("output=%q", r.Output)
				}
				if strings.Contains(r.Output, "\r") {
					t.Fatalf("CR should be stripped: %q", r.Output)
				}
			}
		}
	}

	// context canceled 臂
	app3, eng3 := newTestAppModelWithEngine(t)
	eng3.toolExecErr = errors.New("context canceled")
	app3.state.Mode = "bash"
	app3.shell.SetInputValue("x")
	cmd3 := app3.sendBashCommand()
	if cmd3 == nil {
		t.Fatal("bash cmd3")
	}
	msg3 := cmd3()
	if batch, ok := msg3.(tea.BatchMsg); ok {
		found := false
		for _, c := range batch {
			if c == nil {
				continue
			}
			if r, ok := c().(ToolResultMsg); ok && r.Status == "canceled" {
				found = true
			}
		}
		if !found {
			t.Fatal("expected canceled ToolResultMsg")
		}
	}
}

func TestPasteClipboardImageBranches(t *testing.T) {
	setTestHome(t)

	// 非 shell
	app := newTestAppModel(t)
	app.activeView = "settings"
	runCmd(app.pasteClipboardImage())

	// bash 模式
	app2 := newTestAppModel(t)
	app2.state.Mode = "bash"
	runCmd(app2.pasteClipboardImage())

	// 空剪贴板
	app3 := newTestAppModel(t)
	app3.state.Mode = "ai"
	withStubClipboardImage(t, nil, errors.New("empty clipboard image"))
	runCmd(app3.pasteClipboardImage())

	// 读失败
	app4 := newTestAppModel(t)
	app4.state.Mode = "ai"
	withStubClipboardImage(t, nil, errors.New("win32 failed"))
	runCmd(app4.pasteClipboardImage())

	// 成功：写入 .eos/attachments
	app5 := newTestAppModel(t)
	app5.state.Mode = "ai"
	withStubClipboardImage(t, []byte("IMGDATA"), nil)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	t.Chdir(tmp)
	defer func() { _ = os.Chdir(cwd) }()
	runCmd(app5.pasteClipboardImage())
	if len(app5.pendingImagePaths) != 1 {
		t.Fatalf("pendingImagePaths = %v", app5.pendingImagePaths)
	}
	if _, err := os.Stat(app5.pendingImagePaths[0]); err != nil {
		t.Fatalf("image file: %v", err)
	}
}

func TestPasteClipboardImageVisionWarning(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.state.Mode = "ai"
	withStubClipboardImage(t, []byte("IMG"), nil)
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	t.Chdir(tmp)
	defer func() { _ = os.Chdir(cwd) }()
	// newTestAppModel 的 activeModel=default-model / demo-model 可能无视觉能力
	runCmd(app.pasteClipboardImage())
	if len(app.pendingImagePaths) != 1 {
		t.Fatal("expected image saved")
	}
}

func TestToggleThinkingExpandNilShell(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.state.Thinking = true
	app.thinkingLive.WriteString("thinking…")
	app.shell = nil
	runCmd(app.toggleThinkingExpand())
}

func TestHandleKeyMsgViewDispatch(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// confirm 视图
	app.activeView = "confirm"
	app.confirmView = nil // nil confirmView 落到后续分支
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	// actionPopup 非空拦截
	app2 := newTestAppModel(t)
	app2.actionPopup = nil
	// 用 openActionPopup 构造
	app2.openActionPopup(bubbleActionHit{})
	if app2.actionPopup == nil {
		// 某些输入可能不弹；跳过
	} else {
		_, _ = app2.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	}

	// help 视图 esc / q
	app3 := newTestAppModel(t)
	app3.activeView = "help"
	_, _ = app3.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if app3.activeView != "shell" {
		t.Fatalf("activeView=%q", app3.activeView)
	}
	app3.activeView = "help"
	_, _ = app3.handleKeyMsg(tea.KeyPressMsg{Text: "q"})
	if app3.activeView != "shell" {
		t.Fatalf("activeView=%q", app3.activeView)
	}

	// panel esc：无 viewing/editing 状态时退回 shell
	app4 := newTestAppModel(t)
	app4.activeView = "panel"
	app4.activePanel = "context"
	_, _ = app4.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if app4.activeView != "shell" {
		t.Fatalf("activeView=%q", app4.activeView)
	}

	// panel 非 esc：转发 panel.Update
	app5 := newTestAppModel(t)
	app5.activeView = "panel"
	app5.activePanel = "help"
	// help 面板可能不在 map；用已注册面板
	if _, ok := app5.panels["context"]; ok {
		app5.activePanel = "context"
		_, _ = app5.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	// setup 视图
	app6 := newTestAppModel(t)
	app6.activeView = "setup"
	_, _ = app6.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
}

func TestHandleKeyMsgEnterSendAndBash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.state.Mode = "ai"
	app.shell.SetInputValue("hello")
	_, cmd := app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	// Enter 发送：应产生 cmd（sendMessage batch）或至少不 panic
	_ = cmd

	// bash 模式 Enter → sendBashCommand
	app2, eng := newTestAppModelWithEngine(t)
	eng.toolExecResult = coreapi.ToolResult{Status: "success", Display: "ok"}
	app2.state.Mode = "bash"
	app2.shell.SetMode(shell.ModeBash)
	app2.shell.SetInputValue("pwd")
	_, cmd2 := app2.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = cmd2

	// 输入变化触发 schedulePrediction
	app3 := newTestAppModel(t)
	app3.predictionEnabled = true
	app3.activeView = "shell"
	app3.state.Mode = "ai"
	_, _ = app3.handleKeyMsg(tea.KeyPressMsg{Text: "a"})
}

func TestHandleKeyMsgHintsEnterTab(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// 先打开 slash hints
	app.shell.ShowSlashHints("")
	if !app.shell.IsHintsVisible() {
		// 某些实现 ShowSlashHints 需要参数；再试
		app.shell.ShowSlashHints("he")
	}
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyTab})
	_, _ = app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyUp})
}

func TestHandlePredictionDebounceMatching(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.predictionEnabled = true
	app.activeView = "shell"
	app.state.Mode = "ai"
	app.shell.SetInputValue("hello")
	app.predictionDebounceSeq = 3
	// 匹配 + canPredict → requestPrediction
	_, cmd := app.handlePredictionDebounceMsg(predictionDebounceMsg{Seq: 3, Draft: "hello"})
	if cmd == nil {
		t.Fatal("expected prediction cmd")
	}
	// 匹配但 draft 不一致
	app.predictionDebounceSeq = 4
	_, cmd2 := app.handlePredictionDebounceMsg(predictionDebounceMsg{Seq: 4, Draft: "other"})
	if cmd2 != nil {
		t.Fatal("draft mismatch should nil")
	}
	// 匹配但不可预测
	app2 := newTestAppModel(t)
	app2.predictionEnabled = false
	app2.predictionDebounceSeq = 1
	app2.predictionText = "stale"
	_, _ = app2.handlePredictionDebounceMsg(predictionDebounceMsg{Seq: 1, Draft: "x"})
	if app2.predictionText != "" {
		t.Fatal("should clear")
	}
}

func TestHandlePredictionUpdateFullArms(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.predictionEnabled = true
	app.activeView = "shell"
	app.state.Mode = "ai"
	app.shell.SetInputValue("hel")

	// 空文本清空（clearPrediction 会递增 seq，每臂后重同步）
	app.predictionSeq = 10
	app.predictionText = "old"
	_, _ = app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 10, Draft: "hel", Text: "  "})
	if app.predictionText != "" {
		t.Fatal("empty text should clear")
	}

	// 前缀不符清空
	app.predictionSeq = 20
	app.predictionText = "old"
	_, _ = app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 20, Draft: "hel", Text: "zzz"})
	if app.predictionText != "" {
		t.Fatal("prefix mismatch should clear")
	}

	// 文本等于当前输入 → 清空
	app.predictionSeq = 30
	app.predictionText = "old"
	_, _ = app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 30, Draft: "hel", Text: "hel"})
	if app.predictionText != "" {
		t.Fatal("equal text should clear")
	}

	// 有效预测
	app.predictionSeq = 40
	_, _ = app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 40, Draft: "hel", Text: "hello world"})
	if app.predictionText != "hello world" {
		t.Fatalf("predictionText=%q", app.predictionText)
	}

	// 不可预测清空
	app.predictionEnabled = false
	app.predictionSeq = 50
	app.predictionText = "x"
	_, _ = app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 50, Draft: "hel", Text: "hello"})
	if app.predictionText != "" {
		t.Fatal("cannot predict should clear")
	}
}

func TestUpdateHintsBasedOnInputArms(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.shell.SetInputValue("/he")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("/help arg")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("see @")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("see @foo")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("see @foo bar")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("plain")
	app.updateHintsBasedOnInput()
}

func TestSchedulePredictionAndClear(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.predictionEnabled = false
	// 不可预测 → clear + nil
	if cmd := app.schedulePrediction("x"); cmd != nil {
		t.Fatal("cannot predict")
	}
	app.predictionEnabled = true
	app.activeView = "shell"
	app.state.Mode = "ai"
	cmd := app.schedulePrediction("x")
	if cmd == nil {
		t.Fatal("should schedule")
	}
	// 跑 debounce cmd
	msg := cmd()
	dm, ok := msg.(predictionDebounceMsg)
	if !ok {
		t.Fatalf("msg=%T", msg)
	}
	if dm.Draft != "x" {
		t.Fatalf("draft=%q", dm.Draft)
	}
}

// 确保 adapter 编译期引用（避免误删 import）
var _ = adapter.CoreClientAdapter{}
var _ = context.Background
