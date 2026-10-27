package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 靧NCL-1.1 发布的非商用许可，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// CancelSession 余臂批测：空 id/未知会话/未运行/已取消幂等/带 running
// 会话的 cancel+interrupt 链（成功与 interrupt 失败）/prompt 挂起分支。

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestCancelSessionArms(t *testing.T) {
	t.Run("空 id 与未知会话", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		if _, err := s.CancelSession("  "); err == nil {
			t.Fatal("空 id 应报错")
		}
		if _, err := s.CancelSession("missing"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("未知会话 = %v", err)
		}
	})

	t.Run("会话存在但未运行", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		s.sessions["s-idle"] = &sessionState{ID: "s-idle"}
		if _, err := s.CancelSession("s-idle"); err == nil ||
			err.Error() != "current session has no active request" {
			t.Fatalf("未运行 = %v", err)
		}
	})

	t.Run("running 已取消幂等", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		s.sessions["s-run"] = &sessionState{ID: "s-run", Running: true}
		s.runningConversations["s-run"] = &runningConversationState{Cancelled: true, Cancel: func() {}}
		if _, err := s.CancelSession("s-run"); err != nil {
			t.Fatalf("重复取消幂等 = %v", err)
		}
	})

	t.Run("running 取消链+interrupt 失败", func(t *testing.T) {
		interruptErr := errors.New("interrupt down")
		for _, tc := range []struct {
			name      string
			interrupt func(context.Context) error
		}{
			{"interrupt 成功", func(context.Context) error { return nil }},
			{"interrupt 失败", func(context.Context) error { return interruptErr }},
			{"无 interrupt", nil},
		} {
			t.Run(tc.name, func(t *testing.T) {
				s, _, rec := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
				session := &sessionState{
					ID: "s-run", Running: true,
					Messages: []ChatMessage{{ID: "msg-a", Role: "assistant"}},
				}
				s.sessions["s-run"] = session
				_, cancel := context.WithCancel(context.Background())
				defer cancel()
				s.runningConversations["s-run"] = &runningConversationState{
					AssistantMessageID: "msg-a",
					Cancel:             cancel,
					Interrupt:          tc.interrupt,
				}
				_, err := s.CancelSession("s-run")
				if tc.interrupt != nil && tc.interrupt(context.Background()) != nil {
					// interrupt 失败路径要求错误透传。
					if err == nil {
						t.Fatal("interrupt 失败应透传")
					}
				}
				// 会话态收口。
				if session.Running {
					t.Fatal("取消后 Running 应 false")
				}
				waitForEmitsToSettle(t, rec)
			})
		}
	})

	t.Run("无 running 的 prompt 挂起分支", func(t *testing.T) {
		s, _, rec := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		session := &sessionState{
			ID: "s-prompt", Running: true,
			Messages: []ChatMessage{{ID: "msg-p", Role: "assistant", State: "waiting"}},
		}
		s.sessions["s-prompt"] = session
		s.beginMessageStatusWithKey(session, "msg-p", promptStatusKey("ap-x"), "等待确认…", "warning")
		s.prompts["ap-x"] = &promptState{
			PromptCard:         PromptCard{ID: "ap-x", Kind: "approval", SessionID: "s-prompt"},
			AssistantMessageID: "msg-p",
		}
		_, err := s.CancelSession("s-prompt")
		if err != nil {
			t.Fatalf("prompt 分支取消 = %v", err)
		}
		// 挂起审批被权威收口。
		if _, ok := s.prompts["ap-x"]; ok {
			t.Fatal("停止应撤回全部等待确认")
		}
		waitForEmitsToSettle(t, rec)
	})

	t.Run("无 running 无 prompt：无活动请求", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		s.sessions["s-empty"] = &sessionState{ID: "s-empty", Running: true}
		if _, err := s.CancelSession("s-empty"); err == nil {
			t.Fatal("无活动请求应报错")
		}
	})
}
