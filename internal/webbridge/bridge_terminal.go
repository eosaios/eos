package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"io"
	"sync"
)

// 终端域类型 + TerminalService 装配 + 会话列表查询。
// 会话生命周期（Create/Close/CloseAll）见 bridge_terminal_lifecycle.go，
// 会话控制（Write/Resize）见 bridge_terminal_control.go，
// 输出流 / 状态快照 / 标题等 runtime helper 见 bridge_terminal_runtime.go。

const (
	terminalOutputEventName = "eos:bridge:terminal-output"
	defaultTerminalCols     = 120
	defaultTerminalRows     = 32
)

type TerminalSessionCard struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Cwd       string `json:"cwd"`
	Shell     string `json:"shell"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type TerminalState struct {
	Sessions        []TerminalSessionCard `json:"sessions"`
	ActiveSessionID string                `json:"activeSessionId"`
	// Shell 是终端 bash 探测状态（三层探测 + 安装进行态）；指针区分
	// 「未探测」与「已探测不可用」。
	Shell *TerminalShellStatus `json:"shell,omitempty"`
}

type terminalOutputPayload struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type bridgeTerminalBackend interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Wait(ctx context.Context) error
}

type terminalLauncher func(workspacePath string, cols, rows int) (bridgeTerminalBackend, error)

type terminalSessionHandle struct {
	TerminalSessionCard
	order   int
	backend bridgeTerminalBackend
	writeMu sync.Mutex
}

type TerminalService struct {
	bridge *BridgeService
}

func NewTerminalService(bridge *BridgeService) *TerminalService {
	return &TerminalService{bridge: bridge}
}

func (s *BridgeService) terminalService() *TerminalService {
	if s == nil {
		return NewTerminalService(nil)
	}
	s.serviceMu.Lock()
	defer s.serviceMu.Unlock()
	if s.terminalSvc == nil {
		s.terminalSvc = NewTerminalService(s)
	}
	return s.terminalSvc
}

func (s *BridgeService) ListTerminalSessions() TerminalState {
	return s.terminalService().ListTerminalSessions()
}

func (svc *TerminalService) ListTerminalSessions() TerminalState {
	s := svc.bridge
	if s == nil {
		return TerminalState{}
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.terminalStateLocked()
}
