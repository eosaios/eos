package webbridge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func (s *BridgeService) loadRemoteWorkspaces(activeWorkspace string) []WorkspaceCard {
	items := s.listRemoteWorkspacesReadOnly()
	out := make([]WorkspaceCard, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Repo)
		if name == "" {
			name = filepath.Base(strings.TrimSpace(item.LocalPath))
		}
		if name == "." || name == "" {
			name = strings.TrimSpace(item.RepoURL)
		}
		out = append(out, WorkspaceCard{
			ID:            item.ID,
			Path:          item.LocalPath,
			Name:          name,
			Kind:          "remote",
			Trusted:       true,
			Active:        item.Active || sameWorkspacePath(item.LocalPath, activeWorkspace),
			Removable:     true,
			Platform:      item.Platform,
			RepoURL:       item.RepoURL,
			Owner:         item.Owner,
			Repo:          item.Repo,
			Branch:        item.Branch,
			DefaultBranch: item.DefaultBranch,
			Account:       item.Account,
			Exists:        item.Exists,
		})
	}
	return out
}

func (s *BridgeService) currentRemoteRepoState() *RemoteRepoState {
	state, ok := s.currentRemoteRepoReadOnly()
	if !ok {
		return nil
	}
	return &RemoteRepoState{
		Mode:          state.Mode,
		Platform:      state.Platform,
		RepoURL:       state.RepoURL,
		Owner:         state.Owner,
		Repo:          state.Repo,
		DefaultBranch: state.DefaultBranch,
		WorkingBranch: state.WorkingBranch,
		LocalPath:     state.LocalPath,
		AccountLogin:  state.AccountLogin,
		AccountName:   state.AccountName,
		UpdatedAt:     formatTimeRFC3339(state.UpdatedAt),
	}
}

func (s *BridgeService) loadWorkspacesFromSnapshot(snapshot adapter.RuntimeSnapshot) ([]WorkspaceCard, string) {
	defaultWorkspace := s.ensureDefaultWorkspaceAvailable()
	mode := s.bridgeCoreMode()
	workspaces := make([]adapter.Workspace, 0, len(snapshot.Workspaces))
	sessionCounts := make(map[string]int, len(snapshot.Workspaces))
	displayNames := make(map[string]string, len(snapshot.Workspaces))
	currentSessions := make(map[string]string, len(snapshot.Workspaces))
	for _, item := range snapshot.Workspaces {
		workspaces = append(workspaces, adapter.Workspace{
			Path:    item.Path,
			Trusted: item.Trusted,
			Active:  item.Active,
		})
		key := workspacePathKey(item.Path)
		if key != "" {
			sessionCounts[key] = item.SessionCount
			currentSessions[key] = strings.TrimSpace(item.CurrentSessionID)
			if strings.TrimSpace(item.Name) != "" {
				displayNames[key] = strings.TrimSpace(item.Name)
			}
		}
	}
	visiblePaths := orderedWorkspacePaths(workspaces, s.activeWorkspaceValue(), defaultWorkspace)
	workspaceMeta := make(map[string]adapter.Workspace, len(workspaces))
	for _, item := range workspaces {
		key := workspacePathKey(item.Path)
		if key == "" {
			continue
		}
		if _, exists := workspaceMeta[key]; exists {
			continue
		}
		workspaceMeta[key] = item
	}
	computedNames := workspaceDisplayNames(visiblePaths)
	out := make([]WorkspaceCard, 0, len(visiblePaths))
	for _, path := range visiblePaths {
		key := workspacePathKey(path)
		meta, ok := workspaceMeta[key]
		name := computedNames[key]
		if displayNames[key] != "" {
			name = displayNames[key]
		}
		out = append(out, WorkspaceCard{
			Path:             path,
			Name:             name,
			Kind:             "local",
			Trusted:          ok && meta.Trusted,
			Active:           sameWorkspacePath(path, s.activeWorkspaceValue()) || sameWorkspacePath(path, snapshot.ForegroundWorkspace) || (ok && meta.Active),
			Removable:        !sameWorkspacePath(path, defaultWorkspace),
			SessionCount:     sessionCounts[key],
			CurrentSessionID: currentSessions[key],
		})
	}
	return out, mode
}

func orderedWorkspacePaths(runtimeWorkspaces []adapter.Workspace, activeWorkspace, defaultWorkspace string) []string {
	out := make([]string, 0, len(runtimeWorkspaces)+2)
	seen := make(map[string]struct{}, len(runtimeWorkspaces)+2)
	defaultKey := workspacePathKey(defaultWorkspace)
	add := func(path string) {
		key := workspacePathKey(path)
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		out = append(out, strings.TrimSpace(path))
	}
	addNonDefault := func(path string) {
		if defaultKey != "" && workspacePathKey(path) == defaultKey {
			return
		}
		add(path)
	}

	for _, item := range runtimeWorkspaces {
		addNonDefault(item.Path)
	}

	addNonDefault(activeWorkspace)
	add(defaultWorkspace)
	return out
}

func workspaceDisplayNames(paths []string) map[string]string {
	nameCounts := make(map[string]int, len(paths))
	for _, path := range paths {
		base := filepath.Base(strings.TrimSpace(path))
		if base == "." || base == string(os.PathSeparator) || base == "" {
			base = strings.TrimSpace(path)
		}
		nameCounts[base]++
	}

	out := make(map[string]string, len(paths))
	for _, path := range paths {
		key := workspacePathKey(path)
		if key == "" {
			continue
		}
		base := filepath.Base(strings.TrimSpace(path))
		if base == "." || base == string(os.PathSeparator) || base == "" {
			base = strings.TrimSpace(path)
		}
		label := base
		if nameCounts[base] > 1 {
			parent := filepath.Base(filepath.Dir(strings.TrimSpace(path)))
			if parent != "." && parent != string(os.PathSeparator) && parent != "" {
				label = fmt.Sprintf("%s (%s)", base, parent)
			} else {
				label = strings.TrimSpace(path)
			}
		}
		out[key] = label
	}
	return out
}

func (s *BridgeService) loadWorktrees() []WorktreeCard {
	items := s.worktreesReadOnly()
	out := make([]WorktreeCard, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = filepath.Base(strings.TrimSpace(item.Path))
		}
		out = append(out, WorktreeCard{
			Name:      name,
			Path:      strings.TrimSpace(item.Path),
			Branch:    strings.TrimSpace(item.Branch),
			Head:      strings.TrimSpace(item.Head),
			Active:    item.Active,
			Removable: item.Removable,
		})
	}
	return out
}
