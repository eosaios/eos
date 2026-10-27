package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// RuntimeStatus is the typed replacement for the previous map-based GetStatus result.
type RuntimeStatus struct {
	BridgeMode        string `json:"bridgeMode"`
	ActiveWorkspace   string `json:"activeWorkspace"`
	CurrentSessionID  string `json:"currentSessionID"`
	Uptime            string `json:"uptime"`
	SessionCount      int    `json:"sessionCount"`
	NotificationCount int    `json:"notificationCount"`
}

// SessionStats is the typed replacement for the previous map-based GetStats result.
type SessionStats struct {
	SessionCount           int `json:"sessionCount"`
	TotalMessages          int `json:"totalMessages"`
	TotalUserMessages      int `json:"totalUserMessages"`
	TotalAssistantMessages int `json:"totalAssistantMessages"`
}

func (svc *SystemService) ProbeInvoke(input string) BridgeProbe {
	s := svc.bridge
	if s == nil {
		return BridgeProbe{
			Source:      "unavailable",
			Input:       strings.TrimSpace(input),
			Error:       "bridge service is not available",
			CompletedAt: time.Now().Format(time.RFC3339),
		}
	}
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		trimmed = "Please summarize the current runtime bridge state."
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	source := "runtime"
	var stream <-chan adapter.Event
	var err error
	if gateway := s.runtimeGatewayClient(); gateway != nil {
		stream, err = gateway.Invoke(ctx, trimmed)
	} else {
		err = errors.New("runtime core unavailable")
	}
	if err != nil {
		return BridgeProbe{
			Source:      source,
			Input:       trimmed,
			Error:       err.Error(),
			CompletedAt: time.Now().Format(time.RFC3339),
		}
	}

	events := make([]ProbeEvent, 0, 8)
	completed := false
	for event := range stream {
		events = append(events, ProbeEvent{
			Type:      strings.TrimSpace(event.Type),
			Message:   strings.TrimSpace(event.EffectiveMessage()),
			EventType: strings.TrimSpace(event.EventType),
		})
		if event.Kind() == "text.final" {
			completed = true
		}
		if event.Kind() == "approval.required" {
			// ProbeInvoke auto-approves every required approval: this is a
			// diagnostic flow that must run unattended, not a real approval
			// gate. The typed ApprovalAccept makes that intent explicit.
			if gateway := s.runtimeGatewayClient(); gateway != nil {
				gateway.ResolveConfirmation(event.EffectiveRequestID(), coreapi.ApprovalAccept)
			}
		}
		if len(events) >= 8 {
			break
		}
	}

	return BridgeProbe{
		Source:      source,
		Input:       trimmed,
		Events:      events,
		Completed:   completed,
		CompletedAt: time.Now().Format(time.RFC3339),
	}
}

func (svc *SystemService) GetStatus() RuntimeStatus {
	s := svc.bridge
	if s == nil {
		return RuntimeStatus{}
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return RuntimeStatus{
		BridgeMode:        s.bridgeMode(),
		ActiveWorkspace:   s.activeWorkspaceValue(),
		CurrentSessionID:  s.currentSessionValue(),
		Uptime:            time.Since(s.startedAt).String(),
		SessionCount:      len(s.sessions),
		NotificationCount: len(s.notifications),
	}
}

func (svc *SystemService) GetStats() SessionStats {
	s := svc.bridge
	if s == nil {
		return SessionStats{}
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	totalMessages := 0
	totalUserMessages := 0
	totalAssistantMessages := 0
	for _, session := range s.sessions {
		totalMessages += len(session.Messages)
		for _, msg := range session.Messages {
			switch strings.ToLower(strings.TrimSpace(msg.Role)) {
			case "user":
				totalUserMessages++
			case "assistant":
				totalAssistantMessages++
			}
		}
	}
	return SessionStats{
		SessionCount:           len(s.sessions),
		TotalMessages:          totalMessages,
		TotalUserMessages:      totalUserMessages,
		TotalAssistantMessages: totalAssistantMessages,
	}
}

func (s *BridgeService) readClipboardState() ClipboardState {
	text, ok := s.readClipboardText()
	return ClipboardState{
		Supported: ok,
		Text:      text,
	}
}

// readClipboardText 读取系统剪贴板文本。
//
// web 模式没有宿主进程剪贴板访问（浏览器侧剪贴板由前端 navigator.clipboard
// 自理），恒返回「不支持」——前端把 supported=false 渲染为禁用态。
func (s *BridgeService) readClipboardText() (text string, ok bool) {
	return "", false
}

// writeClipboardText 同理：web 模式不支持服务端剪贴板写入。
func (s *BridgeService) writeClipboardText(text string) (written bool) {
	return false
}

// captureWindowSnapshot 返回 web 模式的窗口快照。浏览器窗口不由服务端管理，
// 给一个固定的可见态；前端 WindowSnapshot 归一化消费，宽高 0 表示未知。
func (s *BridgeService) captureWindowSnapshot() WindowSnapshot {
	return WindowSnapshot{
		Maximised: true,
		Visible:   true,
	}
}
