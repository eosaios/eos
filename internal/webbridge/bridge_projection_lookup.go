package webbridge

import (
	pathpkg "path"
	"strings"
)

func sameWorkspacePath(left, right string) bool {
	leftKey := workspacePathKey(left)
	rightKey := workspacePathKey(right)
	if leftKey == "" || rightKey == "" {
		return false
	}
	return leftKey == rightKey
}

func workspacePathKey(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	// 统一分隔符为 / 后再 Clean：macOS/Linux 上 filepath.Clean 不识别反斜杠，
	// 会导致 Windows 风格路径（C:\a\b 与 C:/a/b）在跨平台比较时判不等。
	return strings.ToLower(pathpkg.Clean(strings.ReplaceAll(trimmed, "\\", "/")))
}

func isDefaultWorkspacePath(path, defaultWorkspace string) bool {
	return sameWorkspacePath(path, defaultWorkspace)
}

func sessionCardByID(sessions []SessionCard, sessionID string) *SessionCard {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	for index := range sessions {
		if strings.TrimSpace(sessions[index].ID) == sessionID {
			return &sessions[index]
		}
	}
	return nil
}

func latestSessionCardForWorkspace(sessions []SessionCard, workspace string) *SessionCard {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil
	}
	var selected *SessionCard
	for index := range sessions {
		item := &sessions[index]
		if !sameWorkspacePath(item.WorkspacePath, workspace) {
			continue
		}
		if selected == nil || strings.Compare(item.UpdatedAt, selected.UpdatedAt) > 0 || (item.UpdatedAt == selected.UpdatedAt && strings.Compare(item.ID, selected.ID) < 0) {
			selected = item
		}
	}
	return selected
}

func workspaceCurrentSessionID(workspaces []WorkspaceCard, workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	for _, item := range workspaces {
		if !sameWorkspacePath(item.Path, workspace) {
			continue
		}
		return strings.TrimSpace(item.CurrentSessionID)
	}
	return ""
}

func withActiveSessionCards(sessions []SessionCard, currentSessionID string) []SessionCard {
	currentSessionID = strings.TrimSpace(currentSessionID)
	for index := range sessions {
		sessions[index].Active = currentSessionID != "" && strings.TrimSpace(sessions[index].ID) == currentSessionID
	}
	return sessions
}

func withActiveWorkspaceCards(workspaces []WorkspaceCard, activeWorkspace string) []WorkspaceCard {
	activeWorkspace = strings.TrimSpace(activeWorkspace)
	for index := range workspaces {
		workspaces[index].Active = activeWorkspace != "" && sameWorkspacePath(workspaces[index].Path, activeWorkspace)
	}
	return workspaces
}
