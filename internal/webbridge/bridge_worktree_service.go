package webbridge

import (
	"errors"
	"strings"
)

func (w *WorkspaceService) CreateWorktree(name string) (BootstrapState, error) {
	s := w.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if _, err := s.createWorktreeRPC(name); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (w *WorkspaceService) RemoveWorktree(path string, force bool) (BootstrapState, error) {
	s := w.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return s.LoadBootstrap(), errors.New("工作树路径不能为空")
	}
	if err := s.removeWorktreeRPC(path, force); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}
