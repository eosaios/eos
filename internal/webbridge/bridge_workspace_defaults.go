package webbridge

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func (s *BridgeService) resolveCreateSessionWorkspaceWithError(workspacePath string, activation WorkspaceActivation) (string, error) {
	workspace := strings.TrimSpace(workspacePath)
	if workspace != "" {
		return workspace, nil
	}
	if active := s.activeWorkspaceValue(); active != "" {
		return active, nil
	}
	return s.ensureDefaultWorkspaceReady(activation)
}

func (s *BridgeService) defaultWorkspacePathCandidate() string {
	defaultWorkspace := strings.TrimSpace(s.defaultWorkspacePathReadOnly())
	if defaultWorkspace == "" {
		defaultWorkspace = defaultWorkspacePathFromEnvironment()
	}
	return strings.TrimSpace(defaultWorkspace)
}

func defaultWorkspacePathFromEnvironment() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(strings.TrimSpace(home), ".eos", "workspace")
	}
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return filepath.Join(home, ".eos", "workspace")
	}
	if home := strings.TrimSpace(os.Getenv("USERPROFILE")); home != "" {
		return filepath.Join(home, ".eos", "workspace")
	}
	return filepath.Join(".eos", "workspace")
}

func (s *BridgeService) ensureDefaultWorkspaceReady(activation WorkspaceActivation) (string, error) {
	defaultWorkspace := strings.TrimSpace(s.defaultWorkspacePathCandidate())
	if defaultWorkspace == "" {
		return "", errors.New("默认工作区路径为空")
	}
	defaultWorkspace = filepath.Clean(defaultWorkspace)
	if err := os.MkdirAll(defaultWorkspace, 0o755); err != nil {
		slog.Warn("bridge.default_workspace.mkdir.error", "path", defaultWorkspace, "error", err)
		return defaultWorkspace, fmt.Errorf("创建默认工作区失败: %w", err)
	}
	if err := s.rememberWorkspaceRPC(defaultWorkspace, activation); err != nil {
		slog.Warn("bridge.default_workspace.remember.error", "path", defaultWorkspace, "error", err)
	}
	if activation == WorkspaceActivationForeground {
		if err := s.activateWorkspaceRPC(defaultWorkspace); err != nil {
			slog.Warn("bridge.default_workspace.activate.error", "path", defaultWorkspace, "error", err)
			return defaultWorkspace, fmt.Errorf("激活默认工作区失败: %w", err)
		}
	}
	return defaultWorkspace, nil
}

func (s *BridgeService) ensureDefaultWorkspaceAvailable() string {
	defaultWorkspace, err := s.ensureDefaultWorkspaceReady(WorkspaceActivationBackground)
	if err != nil {
		slog.Warn("bridge.default_workspace.ensure.error", "path", defaultWorkspace, "error", err)
	}
	return defaultWorkspace
}
