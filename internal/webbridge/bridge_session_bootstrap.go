package webbridge

import (
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func (s *BridgeService) bootstrapSessions() {
	persistedLastWorkspace := s.persistedLastWorkspace()
	s.ensureDefaultWorkspaceAvailable()
	runtimeSnapshot := s.runtimeSnapshotReadOnly()
	activeWorkspace := s.resolveForegroundWorkspaceWithSnapshotAndLast(s.startupWorkspace, runtimeSnapshot, persistedLastWorkspace)
	runtimeSnapshot = s.runtimeSnapshotReadOnly()
	if activeWorkspace == "" && len(runtimeSnapshot.Workspaces) > 0 {
		activeWorkspace = runtimeSnapshot.Workspaces[0].Path
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.activeWorkspace = activeWorkspace
	if len(s.sessions) > 0 {
		return
	}
	if activeWorkspace == "" {
		return
	}
	restored := false
	for _, item := range runtimeSnapshot.Workspaces {
		workspace := strings.TrimSpace(item.Path)
		if workspace == "" {
			continue
		}
		currentID, ok := s.restoreSessionsFromRuntimeLocked(workspace)
		if !ok {
			continue
		}
		restored = true
		if sameWorkspacePath(workspace, activeWorkspace) && strings.TrimSpace(currentID) != "" {
			s.currentSessionID = currentID
		}
	}
	if activeWorkspace != "" {
		s.tryCoreRPC("activate-workspace", activeWorkspace, "", func() error {
			return s.activateWorkspaceRPC(activeWorkspace)
		})
	}
	if runtimeSnapshot.CurrentSession != nil && strings.TrimSpace(runtimeSnapshot.CurrentSession.ID) != "" {
		s.currentSessionID = runtimeSnapshot.CurrentSession.ID
		if strings.TrimSpace(runtimeSnapshot.CurrentSession.WorkspacePath) != "" {
			s.activeWorkspace = runtimeSnapshot.CurrentSession.WorkspacePath
		}
	}
	if s.currentSessionID == "" {
		// 内核 snapshot 没给 currentSession（sidecar 刚启动内存为空），用 GUI 持久化的上次会话。
		if last := strings.TrimSpace(s.persistedLastSession()); last != "" {
			if _, ok := s.sessions[last]; ok {
				s.currentSessionID = last
			}
		}
	}
	if s.currentSessionID == "" {
		if current := s.latestSessionForWorkspaceLocked(activeWorkspace); current != nil {
			s.currentSessionID = current.ID
		}
	}
	if s.currentSessionID == "" {
		if current := s.latestSessionForWorkspaceLocked(""); current != nil {
			s.currentSessionID = current.ID
			s.activeWorkspace = current.WorkspacePath
		}
	}
	if restored {
		return
	}
}

func (s *BridgeService) ensureWorkspaceAndSession() {
	persistedLastWorkspace := s.persistedLastWorkspace()
	s.ensureWorkspaceAndSessionWithSnapshotAndLast(s.runtimeSnapshotReadOnly(), persistedLastWorkspace)
}

func (s *BridgeService) ensureWorkspaceAndSessionWithSnapshotAndLast(runtimeSnapshot adapter.RuntimeSnapshot, persistedLastWorkspace string) {
	s.ensureDefaultWorkspaceAvailable()
	workspaces, _ := s.loadWorkspacesFromSnapshot(runtimeSnapshot)
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if strings.TrimSpace(s.activeWorkspace) == "" {
		s.activeWorkspace = s.resolveForegroundWorkspaceWithSnapshotAndLast("", runtimeSnapshot, persistedLastWorkspace)
		if strings.TrimSpace(s.activeWorkspace) == "" && len(workspaces) > 0 {
			s.activeWorkspace = workspaces[0].Path
		}
	}
	if current := s.ensureSessionByIDLocked(s.currentSessionID); current != nil && sameWorkspacePath(current.WorkspacePath, s.activeWorkspace) {
		s.currentSessionID = current.ID
		return
	}
	if restored := s.restoreCurrentRuntimeSessionLocked(s.activeWorkspace); restored != nil {
		s.currentSessionID = restored.ID
		return
	}
	if current := s.latestSessionForWorkspaceLocked(s.activeWorkspace); current != nil {
		s.currentSessionID = current.ID
		return
	}
	s.currentSessionID = ""
}

func (s *BridgeService) markBootstrapHydrated() {
	s.stateMu.Lock()
	s.bootstrapHydrated = true
	s.stateMu.Unlock()
}

func (s *BridgeService) shouldPreferLocalCurrentSessionLocked(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	session := s.ensureSessionByIDLocked(sessionID)
	if session == nil {
		return false
	}
	// The desktop GUI can keep browsing a different session while another one
	// continues running in the background. Preserve the locally selected session
	// whenever it is still available instead of letting runtime foreground state
	// steal focus back during shell-updated refreshes.
	return true
}

func (s *BridgeService) bootstrapForSession(sessionID string) BootstrapState {
	return s.bootstrapForSessionWithSource(sessionID, "rpc")
}

func (s *BridgeService) bootstrapForSessionWithSource(sessionID, source string) BootstrapState {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return s.loadBootstrap(source)
	}

	state := s.loadBootstrap(source)

	s.stateMu.RLock()
	session := s.ensureSessionByIDLocked(sessionID)
	if session == nil {
		s.stateMu.RUnlock()
		return state
	}
	local := localBootstrapSession{
		Session:       cloneSessionState(session),
		ActiveSession: strings.TrimSpace(s.currentSessionID),
		WorkspacePath: strings.TrimSpace(s.activeWorkspace),
	}
	for _, prompt := range s.prompts {
		if strings.TrimSpace(prompt.SessionID) == sessionID {
			local.PromptCount++
		}
	}
	s.stateMu.RUnlock()

	if local.Session == nil {
		return state
	}
	if strings.TrimSpace(local.Session.WorkspacePath) != "" {
		local.WorkspacePath = strings.TrimSpace(local.Session.WorkspacePath)
	}
	if strings.TrimSpace(local.ActiveSession) == "" {
		local.ActiveSession = local.Session.ID
	}

	state.ActiveWorkspace = local.WorkspacePath
	state.CurrentSessionID = local.Session.ID
	// 单一数据源：审批/问询 UI 数据全部在 ThreadItem.Approval（前端从 items 投影），
	// 不再把 prompts attach 到 messages（遗留通道，前端已不读 message.Prompts）。
	state.Messages = cloneMessages(local.Session.Messages)
	state.Sessions = mergeBootstrapSessionCards(state.Sessions, local)
	state.Workspaces = mergeBootstrapWorkspaceCards(state.Workspaces, local.WorkspacePath, local.Session.ID)
	// 亚秒精度：与 loadBootstrap 的 UpdatedAt 一致，避免前端快照守卫的同秒竞态。
	state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	return normalizeBootstrapState(state)
}

func (s *BridgeService) localBootstrapForSession(sessionID string) BootstrapState {
	sessionID = strings.TrimSpace(sessionID)
	now := time.Now().Format(time.RFC3339)

	s.stateMu.RLock()
	activeWorkspace := strings.TrimSpace(s.activeWorkspace)
	currentSessionID := strings.TrimSpace(s.currentSessionID)
	session := s.ensureSessionByIDLocked(sessionID)
	if session != nil {
		currentSessionID = strings.TrimSpace(session.ID)
		if workspace := strings.TrimSpace(session.WorkspacePath); workspace != "" {
			activeWorkspace = workspace
		}
	}
	sessions := s.exportSessionsLocked(nil)
	notifications := cloneNotifications(s.notifications)
	automationRuns := cloneAutomationRuns(s.automationRuns)
	terminal := s.terminalStateLocked()
	messages := []ChatMessage{}
	if session != nil {
		messages = cloneMessages(session.Messages)
	}
	s.stateMu.RUnlock()

	if activeWorkspace == "" {
		activeWorkspace = defaultWorkspacePathFromEnvironment()
	}
	workspaces := localWorkspaceCardsForBootstrap(sessions, activeWorkspace, currentSessionID)
	sessions = withActiveSessionCards(sessions, currentSessionID)
	workspaces = withActiveWorkspaceCards(workspaces, activeWorkspace)

	return normalizeBootstrapState(BootstrapState{
		Runtime:          bootstrapRuntimeShell,
		BridgeMode:       s.bridgeCoreMode(),
		ActiveWorkspace:  activeWorkspace,
		WorkspaceCount:   len(workspaces),
		CurrentSessionID: currentSessionID,
		Workspaces:       workspaces,
		Sessions:         sessions,
		Messages:         messages,
		// 单一数据源：prompts 不再输出（前端从 ThreadItem.Approval 投影）。
		Prompts:        nil,
		Notifications:  notifications,
		AutomationRuns: automationRuns,
		Terminal:       terminal,
		UpdatedAt:      now,
		AppVersion:     BuildVersion,
		UpdateInstall:  UpdateInstallState{Status: "idle"},
	})
}
