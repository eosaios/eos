package webbridge

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func sessionCardFromState(session *sessionState, pendingCount int) SessionCard {
	if session == nil {
		return SessionCard{}
	}
	return SessionCard{
		ID:             strings.TrimSpace(session.ID),
		Title:          fallbackText(strings.TrimSpace(session.Title), "新对话"),
		Preview:        fallbackText(previewFromMessages(session.Messages), "等待发送第一条消息"),
		WorkspacePath:  strings.TrimSpace(session.WorkspacePath),
		UpdatedAt:      session.UpdatedAt.Format(time.RFC3339),
		Running:        session.Running,
		Persisted:      session.Persisted,
		NeedsAttention: session.NeedsAttention,
		MessageCount:   len(session.Messages),
		PendingPrompts: pendingCount,
	}
}

func localWorkspaceCardsForBootstrap(sessions []SessionCard, activeWorkspace, currentSessionID string) []WorkspaceCard {
	defaultWorkspace := filepath.Clean(defaultWorkspacePathFromEnvironment())
	paths := make([]string, 0, len(sessions)+2)
	seen := map[string]struct{}{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		key := workspacePathKey(path)
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		paths = append(paths, filepath.Clean(path))
	}
	add(activeWorkspace)
	add(defaultWorkspace)
	for _, session := range sessions {
		add(session.WorkspacePath)
	}
	names := workspaceDisplayNames(paths)
	counts := map[string]int{}
	currents := map[string]string{}
	for _, session := range sessions {
		key := workspacePathKey(session.WorkspacePath)
		if key == "" {
			continue
		}
		counts[key]++
		if strings.TrimSpace(session.ID) == strings.TrimSpace(currentSessionID) {
			currents[key] = strings.TrimSpace(session.ID)
		}
	}
	out := make([]WorkspaceCard, 0, len(paths))
	for _, path := range paths {
		key := workspacePathKey(path)
		out = append(out, WorkspaceCard{
			Path:             path,
			Name:             fallbackText(names[key], filepath.Base(path)),
			Kind:             "local",
			Trusted:          true,
			Active:           sameWorkspacePath(path, activeWorkspace),
			Removable:        !sameWorkspacePath(path, defaultWorkspace),
			SessionCount:     counts[key],
			CurrentSessionID: currents[key],
		})
	}
	return out
}

func mergeSessionCard(base, overlay SessionCard) SessionCard {
	base.Title = fallbackText(overlay.Title, base.Title)
	base.Preview = fallbackText(overlay.Preview, base.Preview)
	base.WorkspacePath = fallbackText(overlay.WorkspacePath, base.WorkspacePath)
	if strings.TrimSpace(base.UpdatedAt) == "" || strings.Compare(overlay.UpdatedAt, base.UpdatedAt) > 0 {
		base.UpdatedAt = overlay.UpdatedAt
	}
	base.MessageCount = overlay.MessageCount
	base.Running = overlay.Running
	base.Persisted = overlay.Persisted
	base.NeedsAttention = overlay.NeedsAttention
	base.PendingPrompts = overlay.PendingPrompts
	base.Active = overlay.Active
	return base
}

func mergeBootstrapSessionCards(cards []SessionCard, local localBootstrapSession) []SessionCard {
	updated := false
	for index := range cards {
		if strings.TrimSpace(cards[index].ID) != strings.TrimSpace(local.Session.ID) {
			continue
		}
		card := sessionCardFromState(local.Session, local.PromptCount)
		card.Active = true
		cards[index] = mergeSessionCard(cards[index], card)
		updated = true
		break
	}
	if !updated {
		card := sessionCardFromState(local.Session, local.PromptCount)
		card.Active = true
		cards = append(cards, card)
	}
	cards = withActiveSessionCards(cards, local.Session.ID)
	slices.SortFunc(cards, func(a, b SessionCard) int {
		return strings.Compare(b.UpdatedAt, a.UpdatedAt)
	})
	return cards
}

func mergeBootstrapWorkspaceCards(cards []WorkspaceCard, workspacePath, currentSessionID string) []WorkspaceCard {
	workspacePath = strings.TrimSpace(workspacePath)
	currentSessionID = strings.TrimSpace(currentSessionID)
	targetKey := workspacePathKey(workspacePath)
	if targetKey == "" {
		return withActiveWorkspaceCards(cards, workspacePath)
	}
	found := false
	for index := range cards {
		if workspacePathKey(cards[index].Path) != targetKey {
			cards[index].Active = false
			continue
		}
		cards[index].Active = true
		cards[index].CurrentSessionID = currentSessionID
		found = true
	}
	if !found {
		name := filepath.Base(workspacePath)
		if name == "." || name == string(os.PathSeparator) || strings.TrimSpace(name) == "" {
			name = workspacePath
		}
		cards = append(cards, WorkspaceCard{
			Path:             workspacePath,
			Name:             name,
			Trusted:          true,
			Active:           true,
			Removable:        true,
			CurrentSessionID: currentSessionID,
		})
	}
	return withActiveWorkspaceCards(cards, workspacePath)
}
