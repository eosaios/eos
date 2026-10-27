package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// app_resume_startup_test.go 验证 --continue/--resume 启动路径接入：
// pendingResumeSession 在 resumeStartupSession() 被消费时调 ResumeSession +
// restoreSessionHistory 回填历史，且幂等（二次调用为 no-op）。

import (
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

// TestStartupResumeNoOpWithoutSession 验证 pendingResumeSession 为 nil 时
// resumeStartupSession 不触碰 history、不调用 ResumeSession。
func TestStartupResumeNoOpWithoutSession(t *testing.T) {
	engine := newTestEngine()
	engine.currentSessionID = "sess-1"
	app := NewAppModelFromCoreEngine(engine)

	app.resumeStartupSession()

	if len(engine.resumeCalls) != 0 {
		t.Fatalf("expected 0 Resume calls, got %d (%v)", len(engine.resumeCalls), engine.resumeCalls)
	}
	if len(app.history) != 0 {
		t.Fatalf("expected empty history, got %d entries", len(app.history))
	}
}

// TestStartupResumeRestoresHistory 验证设置 pendingResumeSession 后
// resumeStartupSession 调 ResumeSession（透传 id）、回填 history。
func TestStartupResumeRestoresHistory(t *testing.T) {
	engine := newTestEngine()
	engine.currentSessionID = "sess-123"
	engine.messages = []coreapi.SessionMessage{
		{Role: "user", Content: "hello", Time: time.Now()},
		{
			Role:    "assistant",
			Content: "hi there",
			Time:    time.Now(),
			Metadata: map[string]any{
				"turn_id": "t1",
				"kind":    "",
			},
		},
	}

	app := NewAppModelFromCoreEngine(engine)
	// 模拟 --resume sess-123：注入 pendingResumeSession（构造器在 startup 路径设置）。
	id := "sess-123"
	app.pendingResumeSession = &id

	app.resumeStartupSession()

	if len(engine.resumeCalls) != 1 || engine.resumeCalls[0] != "sess-123" {
		t.Fatalf("expected Resume called once with sess-123, got %v", engine.resumeCalls)
	}
	// 消费后清空：幂等性保证。
	if app.pendingResumeSession != nil {
		t.Fatalf("expected pendingResumeSession cleared after resume, got %q", *app.pendingResumeSession)
	}
	// history 至少回填 user + assistant 两条（restoreSessionHistory 按 turn_id 合并）。
	if len(app.history) < 2 {
		t.Fatalf("expected history backfilled with >=2 entries, got %d: %+v", len(app.history), app.history)
	}
}

// TestStartupResumeIdempotent 验证重复调用 resumeStartupSession 只 resume 一次。
func TestStartupResumeIdempotent(t *testing.T) {
	engine := newTestEngine()
	engine.currentSessionID = "sess-x"
	app := NewAppModelFromCoreEngine(engine)
	id := "sess-x"
	app.pendingResumeSession = &id

	app.resumeStartupSession()
	app.resumeStartupSession() // 第二次应为 no-op

	if len(engine.resumeCalls) != 1 {
		t.Fatalf("expected exactly 1 Resume call after double invoke, got %d", len(engine.resumeCalls))
	}
}

// TestStartupResumeContinuesLatest 验证 --continue（id="latest"）透传给内核。
func TestStartupResumeContinuesLatest(t *testing.T) {
	engine := newTestEngine()
	engine.currentSessionID = "latest-resolved"
	app := NewAppModelFromCoreEngine(engine)
	id := "latest"
	app.pendingResumeSession = &id

	app.resumeStartupSession()

	if len(engine.resumeCalls) != 1 || engine.resumeCalls[0] != "latest" {
		t.Fatalf("expected Resume(latest) forwarded to core, got %v", engine.resumeCalls)
	}
}
