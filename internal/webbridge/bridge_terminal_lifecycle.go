package webbridge

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// 终端会话生命周期：创建 / 关闭单个 / 关闭全部。
// 会话控制（输入写入 / resize）见 bridge_terminal_control.go。

func (s *BridgeService) CreateTerminalSession(workspacePath string) (TerminalState, error) {
	return s.terminalService().CreateTerminalSession(workspacePath)
}

func (svc *TerminalService) CreateTerminalSession(workspacePath string) (TerminalState, error) {
	s := svc.bridge
	if s == nil {
		return TerminalState{}, errors.New("bridge service is not available")
	}
	workspacePath = s.resolveTerminalWorkspace(workspacePath)
	if workspacePath == "" {
		return s.ListTerminalSessions(), errors.New(s.t("error.terminal.workspace_path_required"))
	}
	if s.terminalLauncher == nil {
		return s.ListTerminalSessions(), errors.New("terminal launcher unavailable")
	}
	backend, err := s.terminalLauncher(workspacePath, defaultTerminalCols, defaultTerminalRows)
	if err != nil {
		return s.ListTerminalSessions(), err
	}

	now := time.Now().Format(time.RFC3339)

	s.stateMu.Lock()
	s.terminalSequence++
	handle := &terminalSessionHandle{
		TerminalSessionCard: TerminalSessionCard{
			ID:        fmt.Sprintf("terminal-%d", s.terminalSequence),
			Title:     formatTerminalSessionTitle(workspacePath, s.terminalSequence),
			Cwd:       workspacePath,
			Shell:     "bash",
			Status:    "running",
			CreatedAt: now,
			UpdatedAt: now,
		},
		order:   s.terminalSequence,
		backend: backend,
	}
	s.terminalSessions[handle.ID] = handle
	s.terminalActiveSessionID = handle.ID
	state := s.terminalStateLocked()
	s.stateMu.Unlock()

	s.emitShellUpdated()
	go s.streamTerminalSession(handle)

	return state, nil
}

func (s *BridgeService) CloseTerminalSession(sessionID string) (TerminalState, error) {
	return s.terminalService().CloseTerminalSession(sessionID)
}

func (svc *TerminalService) CloseTerminalSession(sessionID string) (TerminalState, error) {
	s := svc.bridge
	if s == nil {
		return TerminalState{}, errors.New("bridge service is not available")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return s.ListTerminalSessions(), errors.New("terminal session id required")
	}

	var session *terminalSessionHandle
	s.stateMu.Lock()
	session = s.terminalSessions[sessionID]
	if session == nil {
		state := s.terminalStateLocked()
		s.stateMu.Unlock()
		return state, os.ErrNotExist
	}
	delete(s.terminalSessions, sessionID)
	if s.terminalActiveSessionID == sessionID {
		s.terminalActiveSessionID = latestTerminalSessionIDLocked(s.terminalSessions)
	}
	state := s.terminalStateLocked()
	s.stateMu.Unlock()

	err := session.backend.Close()
	s.emitShellUpdated()

	return state, err
}

func (s *BridgeService) closeAllTerminalSessions() {
	s.terminalService().CloseAllTerminalSessions()
}

func (svc *TerminalService) CloseAllTerminalSessions() {
	s := svc.bridge
	if s == nil {
		return
	}
	s.stateMu.Lock()
	sessions := make([]*terminalSessionHandle, 0, len(s.terminalSessions))
	for id, session := range s.terminalSessions {
		if session != nil {
			sessions = append(sessions, session)
		}
		delete(s.terminalSessions, id)
	}
	s.terminalActiveSessionID = ""
	s.stateMu.Unlock()

	for _, session := range sessions {
		if err := session.backend.Close(); err != nil {
			slog.Warn("bridge.terminal.close_failed", "session_id", session.ID, "error", err)
		}
	}
}
