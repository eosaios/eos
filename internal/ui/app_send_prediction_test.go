package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import "testing"

func TestRequestPredictionAndDebounce(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// canPredict 关闭时清空
	app.clearPrediction()
	cmd := app.requestPrediction("draft")
	_ = cmd

	// 开启预测
	app.predictionEnabled = true
	app.activeView = "shell"
	app.state.Mode = "ai"
	app.shell.SetInputValue("hello")
	cmd = app.requestPrediction("hello")
	if cmd == nil {
		t.Fatal("should schedule")
	}
	// 执行 cmd 拿到 PredictionUpdateMsg
	msg := cmd()
	if _, ok := msg.(PredictionUpdateMsg); !ok {
		t.Fatalf("msg = %T", msg)
	}

	// debounce seq 不匹配
	app.handlePredictionDebounceMsg(predictionDebounceMsg{Seq: 999, Draft: "x"})
	// debounce seq 匹配
	app.predictionDebounceSeq = 1
	app.handlePredictionDebounceMsg(predictionDebounceMsg{Seq: 1, Draft: "hello"})
}

func TestHandlePredictionUpdateMsg(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.predictionEnabled = true
	app.activeView = "shell"
	app.state.Mode = "ai"
	app.shell.SetInputValue("hello")

	// seq 不匹配
	app.predictionSeq = 5
	app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 1, Draft: "hello", Text: "hello world"})

	// 空文本
	app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 5, Draft: "hello", Text: "  "})

	// 前缀不符
	app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 5, Draft: "hello", Text: "other"})

	// 有效预测
	app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 5, Draft: "hello", Text: "hello world"})

	// draft 不一致
	app.handlePredictionUpdateMsg(PredictionUpdateMsg{Seq: 5, Draft: "other", Text: "hello world"})
}
