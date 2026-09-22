package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

func (svc *ChatService) CancelSession(sessionID string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return s.LoadBootstrap(), errors.New("session id is required")
	}

	var cancel context.CancelFunc
	var interrupt func(context.Context) error
	assistantMessageID := ""
	hasRunningConversation := false

	s.stateMu.Lock()
	session := s.sessions[trimmed]
	if session == nil || !session.Running {
		if fallback := s.cancelFallbackSessionLocked(trimmed); fallback != nil {
			session = fallback
			trimmed = strings.TrimSpace(fallback.ID)
		}
	}
	if session == nil {
		s.stateMu.Unlock()
		return s.LoadBootstrap(), os.ErrNotExist
	}
	running := s.runningConversations[trimmed]
	if !session.Running {
		s.stateMu.Unlock()
		return s.LoadBootstrap(), errors.New("current session has no active request")
	}
	if running != nil && running.Cancel != nil {
		if running.Cancelled {
			s.stateMu.Unlock()
			return s.LoadBootstrap(), nil
		}
		running.Cancelled = true
		cancel = running.Cancel
		interrupt = running.Interrupt
		assistantMessageID = running.AssistantMessageID
		hasRunningConversation = true
	} else {
		for _, prompt := range s.prompts {
			if prompt == nil || prompt.SessionID != trimmed {
				continue
			}
			if assistantMessageID == "" {
				assistantMessageID = prompt.AssistantMessageID
			}
		}
		// 停止 = 撤回全部等待中的确认：走权威收口（翻 item + delta），不再裸删
		// s.prompts——裸删会让浮层横幅（item 投影）永久卡在「等待确认」，且后续
		// 点「允许/拒绝」因 prompt 已删而幂等空转。
		s.settleSessionPromptsCancelledLocked(session)
		if assistantMessageID == "" {
			s.stateMu.Unlock()
			return s.LoadBootstrap(), errors.New("current session has no active request")
		}
	}
	session.Running = false
	session.NeedsAttention = false
	session.UpdatedAt = time.Now()
	s.setMessageStatus(session, assistantMessageID, "已停止生成", "warning", "failed")
	targetSessionID := session.ID
	s.stateMu.Unlock()

	// 停止信号优先级最高：在锁外立刻触发 Go context 取消和内核 interrupt。
	// 在锁内调用 cancel() 会让 runConversation 在 finishConversation 里抢同一把锁，
	// 导致延迟甚至死锁；interrupt 也同步发送，失败立即返回错误给前端。
	slog.Info("bridge.cancel_session", "session_id", targetSessionID, "has_running", hasRunningConversation, "has_interrupt", interrupt != nil)
	if hasRunningConversation && cancel != nil {
		cancel()
	}
	if hasRunningConversation && interrupt != nil {
		interruptCtx, cancelInterrupt := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelInterrupt()
		if err := interrupt(interruptCtx); err != nil {
			slog.Warn("bridge.cancel_session_interrupt_failed", "session_id", trimmed, "error", err)
			return s.LoadBootstrap(), fmt.Errorf("发送停止信号失败: %w", err)
		}
		slog.Info("bridge.cancel_session_interrupt_sent", "session_id", trimmed)
	}

	s.emitShellUpdatedForSession(targetSessionID)
	return s.localBootstrapForSession(targetSessionID), nil
}

func (s *BridgeService) cancelFallbackSessionLocked(requestedID string) *sessionState {
	requestedID = strings.TrimSpace(requestedID)
	if requestedID != "" && !strings.HasPrefix(requestedID, "session-local-") {
		return nil
	}
	if current := s.sessions[strings.TrimSpace(s.currentSessionID)]; current != nil && current.Running {
		return current
	}
	var selected *sessionState
	for _, session := range s.sessions {
		if session == nil || !session.Running {
			continue
		}
		if selected != nil {
			if sameWorkspacePath(session.WorkspacePath, s.activeWorkspace) && !sameWorkspacePath(selected.WorkspacePath, s.activeWorkspace) {
				selected = session
			}
			continue
		}
		selected = session
	}
	return selected
}
