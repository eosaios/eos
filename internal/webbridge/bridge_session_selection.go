package webbridge

import (
	"strings"
)

func (s *BridgeService) ensureActiveSessionLocked(workspace string) *sessionState {
	session, _ := s.ensureActiveSessionLockedWithError(workspace)
	return session
}

func (s *BridgeService) ensureActiveSessionLockedWithError(workspace string) (*sessionState, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = strings.TrimSpace(s.activeWorkspace)
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		s.currentSessionID = ""
		return nil, nil
	}
	s.activeWorkspace = workspace
	if current := s.ensureSessionByIDLocked(s.currentSessionID); current != nil && (workspace == "" || sameWorkspacePath(current.WorkspacePath, workspace)) {
		return current, nil
	}
	if restored := s.restoreCurrentRuntimeSessionLocked(workspace); restored != nil {
		s.currentSessionID = restored.ID
		return restored, nil
	}
	if current := s.latestSessionForWorkspaceLocked(workspace); current != nil {
		s.currentSessionID = current.ID
		return current, nil
	}
	session, err := s.createWorkspaceSessionLocked(workspace, "新对话", SessionCreateSilent)
	if err != nil {
		s.currentSessionID = ""
		return nil, err
	}
	return session, nil
}

func (s *BridgeService) ensureSessionByIDLocked(sessionID string) *sessionState {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return s.sessions[sessionID]
}

func (s *BridgeService) latestSessionForWorkspaceLocked(workspace string) *sessionState {
	workspace = strings.TrimSpace(workspace)
	var selected *sessionState
	for _, item := range s.sessions {
		if workspace != "" && !sameWorkspacePath(item.WorkspacePath, workspace) {
			continue
		}
		if selected == nil || item.UpdatedAt.After(selected.UpdatedAt) || (item.UpdatedAt.Equal(selected.UpdatedAt) && strings.Compare(item.ID, selected.ID) < 0) {
			selected = item
		}
	}
	if selected != nil || workspace == "" {
		return selected
	}
	metas, err := s.listWorkspaceSessionsReadOnly(workspace)
	if err != nil || len(metas) == 0 {
		return nil
	}
	selectedID := metas[0].ID
	selectedAt := metas[0].SavedAt
	for _, item := range metas[1:] {
		if item.SavedAt.After(selectedAt) || (item.SavedAt.Equal(selectedAt) && strings.Compare(item.ID, selectedID) < 0) {
			selectedID = item.ID
			selectedAt = item.SavedAt
		}
	}
	return s.restoreRuntimeSessionLocked(selectedID, workspace)
}
