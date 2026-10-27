package webbridge

import (
	"errors"
	"os"
	"strings"
	"time"
)

// 终端会话控制：输入写入 + resize。
// 会话生命周期（创建 / 关闭）见 bridge_terminal_lifecycle.go。

func (s *BridgeService) WriteTerminalInput(sessionID, data string) error {
	return s.terminalService().WriteTerminalInput(sessionID, data)
}

func (svc *TerminalService) WriteTerminalInput(sessionID, data string) error {
	s := svc.bridge
	if s == nil {
		return errors.New("bridge service is not available")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("terminal session id required")
	}
	if data == "" {
		return nil
	}

	s.stateMu.Lock()
	session := s.terminalSessions[sessionID]
	if session == nil {
		s.stateMu.Unlock()
		return os.ErrNotExist
	}
	session.UpdatedAt = time.Now().Format(time.RFC3339)
	s.terminalActiveSessionID = sessionID
	s.stateMu.Unlock()

	session.writeMu.Lock()
	defer session.writeMu.Unlock()
	_, err := session.backend.Write([]byte(data))
	return err
}

func (s *BridgeService) ResizeTerminalSession(sessionID string, cols, rows int) error {
	return s.terminalService().ResizeTerminalSession(sessionID, cols, rows)
}

func (svc *TerminalService) ResizeTerminalSession(sessionID string, cols, rows int) error {
	s := svc.bridge
	if s == nil {
		return errors.New("bridge service is not available")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("terminal session id required")
	}
	if cols <= 0 || rows <= 0 {
		return nil
	}

	s.stateMu.RLock()
	session := s.terminalSessions[sessionID]
	s.stateMu.RUnlock()
	if session == nil {
		return os.ErrNotExist
	}
	return session.backend.Resize(cols, rows)
}
