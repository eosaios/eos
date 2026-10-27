package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"testing"

	"github.com/eosaios/eos/internal/ui/views/setup"
)

func TestHandleVerifyAndInitVerifiers(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// verify 空参
	app.handleVerifySlash(nil)
	// verify 带目标
	app.handleVerifySlash([]string{"check", "the", "form"})
	// Processing 中拒绝
	app.state.Processing = true
	app.handleVerifySlash(nil)
	app.state.Processing = false

	// init-verifiers 带参 → 用法
	app.handleInitVerifiersSlash([]string{"x"})
	// init-verifiers 无参
	app.handleInitVerifiersSlash(nil)
}

func TestAppInitAndView(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	cmd := app.Init()
	if cmd == nil {
		t.Fatal("Init should return batch")
	}
	_ = app.View()

	// Update 空消息不 panic
	if next, _ := app.Update(nil); next == nil {
		t.Fatal("nil msg")
	}
}

func TestSendMessageAndBash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.state.Mode = "ai"
	app.shell.SetInputValue("hello")
	cmd := app.sendMessage()
	_ = cmd

	app.state.Mode = "bash"
	app.shell.SetInputValue("ls")
	cmd = app.sendBashCommand()
	_ = cmd

	app.handleSetupCompleteMsg(setup.SetupCompleteMsg{})
}

func TestMarkInflightToolsCanceledFull(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// 有 inflight + history
	app.history = append(app.history, historyEntry{kind: "tool", toolStatus: "running"})
	app.toolInflight["t1"] = toolTrack{name: "Bash", idx: 0}
	app.markInflightToolsCanceled("canceled by user")
	if app.history[0].toolStatus != "canceled" {
		t.Fatalf("status = %q", app.history[0].toolStatus)
	}
	if app.history[0].toolOutput == "" {
		t.Fatal("output")
	}
}
