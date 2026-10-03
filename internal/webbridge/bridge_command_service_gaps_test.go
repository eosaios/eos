package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// CommandService 余臂批测：ResolvePrompt 深链（命中 prompt 的 settle →
// persist → respond 成功 / respond 失败回滚）与 RunCommandPalette
// session.new 臂。

import (
	"errors"
	"testing"
)

func newPromptBridge(t *testing.T, respondErr error) (*BridgeService, *chatSessionsGatewayStub, *emitRecorder) {
	t.Helper()
	gateway := &chatSessionsGatewayStub{
		respondErrs: map[string]error{},
	}
	if respondErr != nil {
		gateway.respondErrs["approval-1"] = respondErr
	}
	s, _, rec := newChatSessionsTestBridge(t, gateway)
	session := &sessionState{
		ID: "session-1",
		Messages: []ChatMessage{
			{ID: "msg-1", Role: "assistant", State: "waiting"},
		},
	}
	s.sessions[session.ID] = session
	s.beginMessageStatusWithKey(session, "msg-1", promptStatusKey("approval-1"), "等待确认…", "warning")
	s.prompts["approval-1"] = &promptState{
		PromptCard:         PromptCard{ID: "approval-1", Kind: "approval", SessionID: "session-1"},
		AssistantMessageID: "msg-1",
	}
	return s, gateway, rec
}

func TestResolvePromptSuccessAndRollback(t *testing.T) {
	t.Run("respond 成功链", func(t *testing.T) {
		s, _, rec := newPromptBridge(t, nil)
		state, err := s.commandService().ResolvePrompt("approval-1", "accept", "准了")
		if err != nil {
			t.Fatalf("ResolvePrompt: %v", err)
		}
		_ = state
		// prompt 已出表。
		if _, ok := s.prompts["approval-1"]; ok {
			t.Fatal("resolve 后 prompt 应移除")
		}
		waitForEmitsToSettle(t, rec)
	})

	t.Run("respond 失败回滚链", func(t *testing.T) {
		s, _, rec := newPromptBridge(t, errors.New("内核拒绝"))
		_, err := s.commandService().ResolvePrompt("approval-1", "accept", "")
		if err == nil || err.Error() != "respond approval: 内核拒绝" {
			t.Fatalf("失败透传 = %v", err)
		}
		// 回滚：prompt 翻回挂起。
		if _, ok := s.prompts["approval-1"]; !ok {
			t.Fatal("失败后 prompt 应回滚挂起")
		}
		// 通知留痕（danger 级审批响应失败）。
		s.stateMu.Lock()
		notifCount := len(s.notifications)
		s.stateMu.Unlock()
		if notifCount == 0 {
			t.Fatal("失败应留 danger 通知")
		}
		waitForEmitsToSettle(t, rec)
	})

	t.Run("未知 prompt 幂等", func(t *testing.T) {
		s, _, _ := newPromptBridge(t, nil)
		if _, err := s.commandService().ResolvePrompt("missing", "accept", ""); err != nil {
			t.Fatalf("未知 prompt 幂等成功: %v", err)
		}
	})
}

func TestRunCommandPaletteSessionNew(t *testing.T) {
	s, _, rec := newPromptBridge(t, nil)
	if _, err := s.RunCommandPalette("SESSION.NEW"); err != nil {
		t.Fatalf("session.new（大小写归一）: %v", err)
	}
	waitForEmitsToSettle(t, rec)
}
