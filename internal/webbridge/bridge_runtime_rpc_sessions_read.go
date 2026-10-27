package webbridge

import (
	"log/slog"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// loadArchivedSessionCardsReadOnly returns SessionCards for the desktop's
// archived sessions (source=gui, archived=true), for the "Archived" view.
func (s *BridgeService) loadArchivedSessionCardsReadOnly() []SessionCard {
	if s == nil || s.runtimeGatewayClient() == nil {
		return nil
	}
	items, err := s.runtimeGatewayClient().CoreListArchivedSessionsRPC(coreCtx())
	if err != nil {
		return nil
	}
	out := make([]SessionCard, 0, len(items))
	for _, item := range items {
		title := ""
		if item.Metadata != nil {
			if v, ok := item.Metadata["title"]; ok {
				if str, _ := v.(string); str != "" {
					title = str
				}
			}
		}
		out = append(out, SessionCard{
			ID:            item.ID,
			Title:         fallbackText(title, "Archived Chat"),
			WorkspacePath: item.WorkspaceRoot,
			UpdatedAt:     item.UpdatedAt.Format(time.RFC3339),
			Persisted:     true,
			Archived:      true,
		})
	}
	return out
}

func (s *BridgeService) runtimeSnapshotReadOnly() adapter.RuntimeSnapshot {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return adapter.RuntimeSnapshot{}
	}
	return coreOnlyValue(
		gateway,
		adapter.RuntimeSnapshot{},
		func(g bridgeRuntimeGateway) (adapter.RuntimeSnapshot, error) {
			return g.CoreRuntimeSnapshotRPC(coreCtx())
		},
	)
}

func (s *BridgeService) configPathReadOnly() string {
	if s == nil || s.runtimeGatewayClient() == nil {
		return ""
	}
	return strings.TrimSpace(s.runtimeGatewayClient().CoreConfigPath())
}

func (s *BridgeService) currentSessionIDForWorkspaceReadOnly(workspace string) string {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return ""
	}
	workspace = strings.TrimSpace(workspace)
	// Snapshot is the aggregate read (workspaces + current session); when it
	// fails we degrade to the lighter CoreCurrentSessionRPC below. The fallback
	// is intentional, but the swallowed error must stay observable.
	if snapshot, err := gateway.CoreStateSnapshotRPC(coreCtx()); err == nil {
		for _, item := range snapshot.Workspaces {
			if sameWorkspacePath(item.Path, workspace) {
				if currentID := strings.TrimSpace(item.CurrentSessionID); currentID != "" {
					return currentID
				}
			}
		}
		if snapshot.CurrentSession != nil {
			currentWorkspace := strings.TrimSpace(snapshot.CurrentSession.WorkspacePath)
			if workspace == "" || sameWorkspacePath(currentWorkspace, workspace) {
				return strings.TrimSpace(snapshot.CurrentSession.ID)
			}
		}
	} else {
		slog.Warn("bridge.core_rpc.read_failed", "domain", "state-snapshot", "workspace", workspace, "error", err)
	}
	meta, err := gateway.CoreCurrentSessionRPC(coreCtx(), workspace)
	if err != nil {
		slog.Warn("bridge.core_rpc.read_failed", "domain", "current-session", "workspace", workspace, "error", err)
		return ""
	}
	return strings.TrimSpace(meta.ID)
}

func (s *BridgeService) loadWorkspaceSessionMessagesReadOnly(workspace, sessionID string) ([]adapter.SessionMessage, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	return coreOnlyResult(
		gateway,
		func(g bridgeRuntimeGateway) ([]adapter.SessionMessage, error) {
			return g.CoreLoadSessionMessagesRPC(coreCtx(), workspace, sessionID)
		},
	)
}

func (s *BridgeService) workspaceForSessionFromSnapshotReadOnly(sessionID string, snapshot adapter.RuntimeSnapshot) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	if snapshot.CurrentSession != nil && strings.TrimSpace(snapshot.CurrentSession.ID) == sessionID {
		if workspace := strings.TrimSpace(snapshot.CurrentSession.WorkspacePath); workspace != "" {
			return workspace
		}
	}
	for _, session := range snapshot.Sessions {
		if strings.TrimSpace(session.ID) != sessionID {
			continue
		}
		if workspace := strings.TrimSpace(session.WorkspacePath); workspace != "" {
			return workspace
		}
	}
	if s == nil || s.runtimeGatewayClient() == nil {
		return ""
	}
	workspace, err := s.runtimeGatewayClient().ResolveSessionWorkspace(sessionID)
	if err != nil {
		slog.Warn("bridge.core_rpc.read_failed", "domain", "resolve-session-workspace", "session_id", sessionID, "error", err)
	}
	return strings.TrimSpace(workspace)
}

func (s *BridgeService) resolveSessionWorkspaceReadOnly(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || s == nil || s.runtimeGatewayClient() == nil {
		return ""
	}
	if workspace := s.workspaceForSessionFromSnapshotReadOnly(sessionID, s.runtimeSnapshotReadOnly()); workspace != "" {
		return workspace
	}
	workspace, err := s.runtimeGatewayClient().ResolveSessionWorkspace(sessionID)
	if err != nil {
		slog.Warn("bridge.core_rpc.read_failed", "domain", "resolve-session-workspace", "session_id", sessionID, "error", err)
	}
	return strings.TrimSpace(workspace)
}
