package webbridge

import (
	"slices"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func (s *BridgeService) bootstrapMessages(currentSessionID, activeWorkspace string, runtimeSnapshot adapter.RuntimeSnapshot) []ChatMessage {
	currentSessionID = strings.TrimSpace(currentSessionID)
	if currentSessionID == "" {
		return nil
	}
	s.stateMu.RLock()
	localSession := s.ensureSessionByIDLocked(currentSessionID)
	localMessages := []ChatMessage(nil)
	if localSession != nil {
		localMessages = cloneMessages(localSession.Messages)
	}
	s.stateMu.RUnlock()
	if localSession != nil {
		return localMessages
	}
	if runtimeSnapshot.CurrentSession != nil &&
		strings.TrimSpace(runtimeSnapshot.CurrentSession.ID) == currentSessionID &&
		len(runtimeSnapshot.Messages) > 0 {
		out := chatMessagesFromRuntime(runtimeSnapshot.Messages)
		return out
	}
	resolvedWorkspace := strings.TrimSpace(activeWorkspace)
	if resolvedWorkspace == "" &&
		runtimeSnapshot.CurrentSession != nil &&
		strings.TrimSpace(runtimeSnapshot.CurrentSession.ID) == currentSessionID {
		resolvedWorkspace = strings.TrimSpace(runtimeSnapshot.CurrentSession.WorkspacePath)
	}
	if resolvedWorkspace == "" {
		resolvedWorkspace = s.workspaceForSessionFromSnapshotReadOnly(currentSessionID, runtimeSnapshot)
	}
	if resolvedWorkspace == "" {
		return nil
	}
	rawMessages, err := s.loadWorkspaceSessionMessagesReadOnly(resolvedWorkspace, currentSessionID)
	if err != nil {
		return nil
	}
	return chatMessagesFromRuntime(rawMessages)
}

func (s *BridgeService) exportSessionsLocked(runtimeSessions []adapter.SessionSnapshot) []SessionCard {
	out := make([]SessionCard, 0, len(runtimeSessions)+len(s.sessions))
	seen := make(map[string]int, len(runtimeSessions)+len(s.sessions))
	for _, snapshot := range runtimeSessions {
		item := s.sessions[snapshot.ID]
		pendingCount := s.pendingPromptCountLocked(snapshot.ID)
		title := strings.TrimSpace(snapshot.Title)
		preview := strings.TrimSpace(snapshot.Preview)
		workspacePath := strings.TrimSpace(snapshot.WorkspacePath)
		updatedAt := snapshot.UpdatedAt
		messageCount := snapshot.MessageCount
		running := snapshot.Running
		needsAttention := snapshot.NeedsAttention
		persisted := true
		if item != nil {
			title = fallbackText(item.Title, title)
			if preview == "" {
				preview = previewFromMessages(item.Messages)
			}
			if workspacePath == "" {
				workspacePath = item.WorkspacePath
			}
			if updatedAt.IsZero() || item.UpdatedAt.After(updatedAt) {
				updatedAt = item.UpdatedAt
			}
			if len(item.Messages) > 0 {
				messageCount = len(item.Messages)
			}
			running = running || item.Running
			needsAttention = needsAttention || item.NeedsAttention
			persisted = item.Persisted
		}
		seen[snapshot.ID] = len(out)
		out = append(out, SessionCard{
			ID:             snapshot.ID,
			Title:          fallbackText(title, "新对话"),
			Preview:        fallbackText(preview, "等待发送第一条消息"),
			WorkspacePath:  workspacePath,
			UpdatedAt:      updatedAt.Format(time.RFC3339),
			Running:        running,
			Persisted:      persisted,
			NeedsAttention: needsAttention,
			MessageCount:   messageCount,
			PendingPrompts: pendingCount,
			Archived:       snapshot.Archived,
			Active:         snapshot.Active || snapshot.ID == s.currentSessionID,
		})
	}
	for sessionID, item := range s.sessions {
		index, exists := seen[sessionID]
		card := sessionCardFromState(item, s.pendingPromptCountLocked(sessionID))
		card.Active = strings.TrimSpace(card.ID) == strings.TrimSpace(s.currentSessionID)
		if exists {
			out[index] = mergeSessionCard(out[index], card)
			continue
		}
		out = append(out, card)
	}
	slices.SortFunc(out, func(a, b SessionCard) int {
		return strings.Compare(b.UpdatedAt, a.UpdatedAt)
	})
	return out
}

func (s *BridgeService) pendingPromptCountLocked(sessionID string) int {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0
	}
	count := 0
	for _, prompt := range s.prompts {
		if strings.TrimSpace(prompt.SessionID) == sessionID {
			count++
		}
	}
	return count
}

func (s *BridgeService) loadWorkspaces() ([]WorkspaceCard, string) {
	return s.loadWorkspacesFromSnapshot(s.runtimeSnapshotReadOnly())
}

func (s *BridgeService) planSnapshot() PlanSnapshot {
	snap := s.planSnapshotReadOnly()
	return PlanSnapshot{
		HasPlan:          snap.HasPlan,
		Content:          snap.Content,
		WorkspaceCurrent: snap.WorkspaceCurrent,
		UserLatest:       snap.UserLatest,
		UserSnapshot:     snap.UserSnapshot,
		UpdatedAt:        formatTimeRFC3339(snap.UpdatedAt),
	}
}

func (s *BridgeService) memorySnapshot() MemorySnapshot {
	snap := s.memorySnapshotReadOnly()
	docs := make([]MemoryDocument, 0, len(snap.Documents))
	for _, doc := range snap.Documents {
		docs = append(docs, MemoryDocument{
			Scope:     doc.Scope,
			Path:      doc.Path,
			Exists:    doc.Exists,
			Content:   doc.Content,
			Summary:   doc.Summary,
			UpdatedAt: formatTimeRFC3339(doc.UpdatedAt),
		})
	}
	return MemorySnapshot{Documents: docs}
}
