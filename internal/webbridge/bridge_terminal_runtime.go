package webbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func (s *BridgeService) terminalStateLocked() TerminalState {
	items := make([]*terminalSessionHandle, 0, len(s.terminalSessions))
	for _, session := range s.terminalSessions {
		if session != nil {
			items = append(items, session)
		}
	}
	slices.SortFunc(items, func(left, right *terminalSessionHandle) int {
		switch {
		case left.order < right.order:
			return -1
		case left.order > right.order:
			return 1
		default:
			return strings.Compare(left.ID, right.ID)
		}
	})
	out := make([]TerminalSessionCard, 0, len(items))
	for _, item := range items {
		out = append(out, item.TerminalSessionCard)
	}
	// 探测缓存自带独立锁，stateMu 持有期间获取安全（无反向依赖）。
	shell := s.terminalShellSnapshot()
	return TerminalState{
		Sessions:        out,
		ActiveSessionID: strings.TrimSpace(s.terminalActiveSessionID),
		Shell:           &shell,
	}
}

func (s *BridgeService) resolveTerminalWorkspace(workspacePath string) string {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath != "" {
		return workspacePath
	}
	s.stateMu.RLock()
	activeWorkspace := strings.TrimSpace(s.activeWorkspace)
	s.stateMu.RUnlock()
	if activeWorkspace != "" {
		return activeWorkspace
	}
	if foreground, err := s.runtimeGatewayClient().CoreResolveForegroundWorkspaceRPC(context.Background(), ""); err == nil && strings.TrimSpace(foreground) != "" {
		return strings.TrimSpace(foreground)
	}
	if defaultWorkspace := strings.TrimSpace(s.defaultWorkspacePathReadOnly()); defaultWorkspace != "" {
		return defaultWorkspace
	}
	return s.ensureDefaultWorkspaceAvailable()
}

func (s *BridgeService) streamTerminalSession(session *terminalSessionHandle) {
	buffer := make([]byte, 4096)
	for {
		n, err := session.backend.Read(buffer)
		if n > 0 {
			chunk := string(buffer[:n])
			s.stateMu.Lock()
			if live := s.terminalSessions[session.ID]; live != nil {
				live.UpdatedAt = time.Now().Format(time.RFC3339)
			}
			s.stateMu.Unlock()
			s.emitTerminalOutput(session.ID, chunk)
		}
		if err != nil {
			status := "exited"
			if !errors.Is(err, io.EOF) {
				status = "failed"
			}
			s.finishTerminalSession(session.ID, status)
			return
		}
	}
}

func (s *BridgeService) finishTerminalSession(sessionID, status string) {
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	s.stateMu.RLock()
	session := s.terminalSessions[sessionID]
	s.stateMu.RUnlock()
	if session == nil {
		return
	}

	_ = session.backend.Wait(waitCtx)

	s.stateMu.Lock()
	live := s.terminalSessions[sessionID]
	if live == nil {
		s.stateMu.Unlock()
		return
	}
	live.Status = status
	live.UpdatedAt = time.Now().Format(time.RFC3339)
	s.stateMu.Unlock()

	s.emitShellUpdated()
}

func (s *BridgeService) emitTerminalOutput(sessionID, data string) {
	emitter := s.currentEmitter()
	if emitter == nil {
		return
	}
	emitter(terminalOutputEventName, terminalOutputPayload{
		SessionID: sessionID,
		Data:      data,
	})
}

func latestTerminalSessionIDLocked(items map[string]*terminalSessionHandle) string {
	var latest *terminalSessionHandle
	for _, session := range items {
		if session == nil {
			continue
		}
		if latest == nil || session.order > latest.order {
			latest = session
		}
	}
	if latest == nil {
		return ""
	}
	return latest.ID
}

func formatTerminalSessionTitle(workspacePath string, order int) string {
	base := strings.TrimSpace(filepath.Base(strings.TrimSpace(workspacePath)))
	if base == "" || base == "." || base == string(os.PathSeparator) {
		return fmt.Sprintf("bash %d", order)
	}
	return fmt.Sprintf("%s · bash %d", base, order)
}
