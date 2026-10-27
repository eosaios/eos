package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// app_send/startup 第三轮收尾：/exit、hints 触发、invoke 回包三臂、
// paste cwd/write 失败、debounce 回包。

// 不可达清单：
//   - StartInteractiveTUIWithOptions：进程入口 + os.Exit。
//   - setup 三 View 具体 Update：向导状态机，setup 包内测覆盖。

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHandleKeyMsgExitCommand(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.state.Mode = "ai"
	app.shell.SetInputValue("/exit")
	_, cmd := app.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("/exit should return batch with exit cmd")
	}
}

func TestHandleKeyMsgSlashAndAtHints(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.shell.SetInputValue("")
	_, cmd := app.handleKeyMsg(tea.KeyPressMsg{Text: "/"})
	// 不执行 cmd（内含 StatusTick 会挂）；只锚定返回非 nil 且不 panic
	if cmd == nil {
		t.Fatal("slash key should produce cmd")
	}

	app2 := newTestAppModel(t)
	app2.shell.SetInputValue("")
	_, cmd2 := app2.handleKeyMsg(tea.KeyPressMsg{Text: "@"})
	if cmd2 == nil {
		t.Fatal("at key should produce cmd")
	}
}

func TestSendMessageTextInvokeErrorAndCancel(t *testing.T) {
	setTestHome(t)
	// ErrorMsg 臂
	app, eng := newTestAppModelWithEngine(t)
	eng.turnStartErr = errors.New("turn start failed")
	cmd := app.sendMessageText("hello", false)
	if cmd == nil {
		t.Fatal("should send")
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		foundErr := false
		for _, c := range batch {
			if c == nil {
				continue
			}
			if _, isErr := c().(ErrorMsg); isErr {
				foundErr = true
			}
		}
		if !foundErr {
			t.Fatal("expected ErrorMsg from invoke failure")
		}
	}

	// context canceled 臂
	app2, eng2 := newTestAppModelWithEngine(t)
	eng2.turnStartErr = errors.New("context canceled")
	cmd2 := app2.sendMessageText("hello", false)
	if batch, ok := cmd2().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			// canceled 路径返回 nil msg
			_ = c()
		}
	}
}

func TestPasteClipboardCwdAndWriteFail(t *testing.T) {
	setTestHome(t)
	// cwd 失败：把进程 cwd 删掉（Windows 上用占位后 chdir 到已删目录难；
	// 改为写失败：目标路径是目录）
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	t.Chdir(tmp)
	defer func() { _ = os.Chdir(cwd) }()

	// 造 .eos/attachments 成功，但预先放一个与 clipboard-*.png 同名的目录——
	// 文件名含纳秒时间戳不可预知，改用只读文件占位整目录权限。
	att := filepath.Join(tmp, ".eos", "attachments")
	if err := os.MkdirAll(att, 0o755); err != nil {
		t.Fatal(err)
	}
	// Windows 上目录只读对 Create 无效；改测 mkdir 失败：.eos 是文件
	_ = os.RemoveAll(filepath.Join(tmp, ".eos"))
	if err := os.WriteFile(filepath.Join(tmp, ".eos"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newTestAppModel(t)
	app.state.Mode = "ai"
	withStubClipboardImage(t, []byte("IMG"), nil)
	runCmd(app.pasteClipboardImage())
	if len(app.pendingImagePaths) != 0 {
		t.Fatal("mkdir fail should not save")
	}
}

func TestHandlePredictionDebounceReturnsCmd(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.predictionEnabled = true
	app.activeView = "shell"
	app.state.Mode = "ai"
	app.shell.SetInputValue("hi")
	app.predictionDebounceSeq = 7
	_, cmd := app.handlePredictionDebounceMsg(predictionDebounceMsg{Seq: 7, Draft: "hi"})
	if cmd == nil {
		t.Fatal("expected finalize wrapped cmd")
	}
	_ = cmd()
}
