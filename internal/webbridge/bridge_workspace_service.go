package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"log/slog"
	"os"
	"strings"
)

type WorkspaceService struct {
	bridge *BridgeService
}

func NewWorkspaceService(bridge *BridgeService) *WorkspaceService {
	return &WorkspaceService{bridge: bridge}
}

func (s *BridgeService) workspaceService() *WorkspaceService {
	if s == nil {
		return NewWorkspaceService(nil)
	}
	if s.workspaceSvc == nil {
		s.workspaceSvc = NewWorkspaceService(s)
	}
	return s.workspaceSvc
}

func (w *WorkspaceService) SelectWorkspace(path string) (BootstrapState, error) {
	s := w.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return s.LoadBootstrap(), errors.New("工作区路径不能为空")
	}
	if err := s.activateWorkspaceRPC(trimmed); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.activeWorkspace = trimmed
	s.tryCoreRPC("remember-workspace", trimmed, "", func() error {
		return s.rememberWorkspaceRPC(trimmed, WorkspaceActivationForeground)
	})
	if currentID, restored := s.restoreSessionsFromRuntimeLocked(trimmed); restored {
		if currentID != "" {
			s.currentSessionID = currentID
		} else if item := s.latestSessionForWorkspaceLocked(trimmed); item != nil {
			s.currentSessionID = item.ID
		}
		resolvedSessionID := s.currentSessionID
		if resolvedSessionID != "" {
			s.tryCoreRPC("resume-session", trimmed, resolvedSessionID, func() error {
				return s.resumeWorkspaceSessionRPC(trimmed, resolvedSessionID)
			})
		}
		s.emitShellUpdated()
		s.stateMu.Unlock()
		// 会话确定后按会话维度恢复沙箱模式（历史会话用自己的值，无则默认 workspace）。
		s.syncSandboxModeForSession(trimmed, resolvedSessionID)
		return s.LoadBootstrap(), nil
	}
	s.currentSessionID = ""
	s.tryCoreRPC("set-current-session", trimmed, "", func() error {
		return s.setWorkspaceCurrentSessionRPC(trimmed, "")
	})
	s.emitShellUpdated()
	s.stateMu.Unlock()
	// 工作区无会话：回落到全局默认沙箱模式。
	s.syncSandboxModeForSession(trimmed, "")
	return s.LoadBootstrap(), nil
}

func (w *WorkspaceService) TrustWorkspace(path string) (BootstrapState, error) {
	s := w.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return s.LoadBootstrap(), errors.New("工作区路径不能为空")
	}
	if err := s.trustWorkspaceRPC(path); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (w *WorkspaceService) RemoveWorkspace(path string) (BootstrapState, error) {
	s := w.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return s.LoadBootstrap(), errors.New("工作区路径不能为空")
	}
	if isDefaultWorkspacePath(path, s.defaultWorkspacePathCandidate()) {
		return s.LoadBootstrap(), errors.New("默认工作区不能移除")
	}
	w.deleteWorkspaceSessions(path)
	if err := s.removeWorkspaceRPC(path); err != nil && !errors.Is(err, os.ErrNotExist) && !strings.Contains(strings.ToLower(err.Error()), "不存在") {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	w.removeWorkspaceStateLocked(path)
	s.ensureActiveSessionLocked("")
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (w *WorkspaceService) deleteWorkspaceSessions(path string) {
	s := w.bridge
	if s == nil {
		return
	}
	// The core derives its workspace list from sessions: any session whose
	// workspace_root == path re-introduces the workspace on the next snapshot,
	// even after workspace_forget. So removing a workspace must also delete every
	// session that belongs to it from the core store, otherwise the workspace
	// comes right back (this is why the workspace "X" didn't work while the
	// session "X" did). List the workspace's sessions from the core and delete
	// each one before forgetting the workspace itself.
	if metas, err := s.listWorkspaceSessionsReadOnly(path); err == nil {
		for _, meta := range metas {
			sessionID := strings.TrimSpace(meta.ID)
			if sessionID == "" {
				continue
			}
			if delErr := s.deleteWorkspaceSessionRPC(path, sessionID); delErr != nil {
				slog.Warn("bridge.workspace.remove.session_delete_failed", "workspace", path, "session", sessionID, "error", delErr)
			}
		}
	}
}

func (w *WorkspaceService) removeWorkspaceStateLocked(path string) {
	s := w.bridge
	if s == nil {
		return
	}
	for id, session := range s.sessions {
		if sameWorkspacePath(session.WorkspacePath, path) {
			delete(s.sessions, id)
		}
	}
	if sameWorkspacePath(s.activeWorkspace, path) {
		s.activeWorkspace = s.lastWorkspacePathReadOnly()
		s.currentSessionID = ""
		if strings.TrimSpace(s.activeWorkspace) != "" {
			s.tryCoreRPC("activate-workspace", s.activeWorkspace, "", func() error {
				return s.activateWorkspaceRPC(s.activeWorkspace)
			})
		}
	}
}
