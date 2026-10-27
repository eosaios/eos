package webbridge

import (
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

type bootstrapDeferredState struct {
	mcpServers     []MCPServerCard
	lspServers     []LSPServerCard
	skills         []SkillCard
	plugins        []PluginCard
	worktrees      []WorktreeCard
	usageSummary   UsageSummaryCard
	costItems      []CostItemCard
	versions       []VersionCard
	permission     PermissionState
	tasks          []TaskCard
	clipboard      ClipboardState
	diagnostics    DiagnosticsState
	resourceChecks []ResourceCheck
}

type bootstrapLocalState struct {
	activeWorkspace    string
	currentSessionID   string
	sessions           []SessionCard
	notifications      []NotificationItem
	automationRuns     []AutomationRunCard
	bash               BashState
	terminal           TerminalState
	preferLocalCurrent bool
}

func (p *StateProjectionService) loadDeferredState(scope BootstrapLoadScope, runtimeSnapshot adapter.RuntimeSnapshot, bridgeMode string, window WindowSnapshot) bootstrapDeferredState {
	deferred := bootstrapDeferredState{}
	if p == nil || p.bridge == nil || scope != BootstrapLoadIncludeDeferred {
		return deferred
	}
	s := p.bridge
	deferred.mcpServers = s.loadMCPServers()
	deferred.lspServers = s.loadLSPServers()
	deferred.skills = s.loadSkills()
	deferred.plugins = s.loadPlugins()
	deferred.worktrees = s.loadWorktrees()
	deferred.usageSummary = s.loadUsageSummary()
	deferred.costItems = s.loadCostItems()
	deferred.versions = s.loadVersions()
	deferred.permission = s.loadPermission()
	deferred.tasks = s.loadTasksFromSnapshot(runtimeSnapshot)
	deferred.clipboard = s.readClipboardState()
	deferred.diagnostics = s.loadDiagnostics()
	deferred.resourceChecks = s.resourceChecks(bridgeMode, deferred.diagnostics, deferred.clipboard, window)
	s.markBootstrapHydrated()
	return deferred
}

func (p *StateProjectionService) loadLocalBootstrapState(runtimeSnapshot adapter.RuntimeSnapshot) bootstrapLocalState {
	local := bootstrapLocalState{}
	if p == nil || p.bridge == nil {
		return local
	}
	s := p.bridge
	s.stateMu.RLock()
	local.activeWorkspace = strings.TrimSpace(s.activeWorkspace)
	local.currentSessionID = strings.TrimSpace(s.currentSessionID)
	local.sessions = s.exportSessionsLocked(runtimeSnapshot.Sessions)
	// 单一数据源：prompts 不再 export（前端从 ThreadItem.Approval 投影）。
	local.notifications = cloneNotifications(s.notifications)
	local.automationRuns = cloneAutomationRuns(s.automationRuns)
	local.bash = s.bash
	local.terminal = s.terminalStateLocked()
	local.preferLocalCurrent = s.shouldPreferLocalCurrentSessionLocked(local.currentSessionID)
	s.stateMu.RUnlock()
	return local
}

func (p *StateProjectionService) resolveBootstrapSelection(local bootstrapLocalState, runtimeSnapshot adapter.RuntimeSnapshot, defaultWorkspace string, workspaces []WorkspaceCard) (string, string) {
	if p == nil || p.bridge == nil {
		return "", ""
	}
	s := p.bridge
	activeWorkspace := local.activeWorkspace
	currentSessionID := local.currentSessionID

	if !local.preferLocalCurrent && activeWorkspace == "" && strings.TrimSpace(runtimeSnapshot.ForegroundWorkspace) != "" {
		activeWorkspace = strings.TrimSpace(runtimeSnapshot.ForegroundWorkspace)
	}
	if !local.preferLocalCurrent && runtimeSnapshot.CurrentSession != nil {
		currentSessionID = strings.TrimSpace(runtimeSnapshot.CurrentSession.ID)
		if activeWorkspace == "" && strings.TrimSpace(runtimeSnapshot.CurrentSession.WorkspacePath) != "" {
			activeWorkspace = strings.TrimSpace(runtimeSnapshot.CurrentSession.WorkspacePath)
		}
	}
	if strings.TrimSpace(currentSessionID) == "" {
		if currentID := workspaceCurrentSessionID(workspaces, activeWorkspace); strings.TrimSpace(currentID) != "" {
			currentSessionID = strings.TrimSpace(currentID)
		}
	}
	if strings.TrimSpace(currentSessionID) == "" {
		if item := latestSessionCardForWorkspace(local.sessions, activeWorkspace); item != nil {
			currentSessionID = strings.TrimSpace(item.ID)
		}
	}
	if item := sessionCardByID(local.sessions, currentSessionID); item != nil {
		if strings.TrimSpace(item.WorkspacePath) != "" {
			activeWorkspace = strings.TrimSpace(item.WorkspacePath)
		}
	} else if strings.TrimSpace(currentSessionID) != "" {
		if resolvedWorkspace := s.workspaceForSessionFromSnapshotReadOnly(currentSessionID, runtimeSnapshot); resolvedWorkspace != "" {
			activeWorkspace = strings.TrimSpace(resolvedWorkspace)
		}
	}
	if activeWorkspace == "" {
		activeWorkspace = defaultWorkspace
	}
	if activeWorkspace == "" {
		for _, item := range workspaces {
			if item.Active {
				activeWorkspace = item.Path
				break
			}
		}
	}
	if activeWorkspace == "" {
		if item := sessionCardByID(local.sessions, currentSessionID); item != nil {
			activeWorkspace = strings.TrimSpace(item.WorkspacePath)
		}
	}
	return activeWorkspace, currentSessionID
}
