package webbridge

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

func (s *BridgeService) findOrRestoreSession(workspacePath, sessionID string) *sessionState {
	workspacePath = strings.TrimSpace(workspacePath)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	s.stateMu.RLock()
	if session := s.sessions[sessionID]; session != nil {
		if workspacePath == "" || sameWorkspacePath(session.WorkspacePath, workspacePath) {
			s.stateMu.RUnlock()
			return session
		}
	}
	s.stateMu.RUnlock()

	resolvedWorkspace := workspacePath
	if resolvedWorkspace == "" {
		resolvedWorkspace = s.workspaceForSessionFromSnapshotReadOnly(sessionID, s.runtimeSnapshotReadOnly())
	}
	if resolvedWorkspace == "" {
		return nil
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.restoreRuntimeSessionLocked(sessionID, resolvedWorkspace)
}

func (s *BridgeService) restoreSessionsFromRuntimeLocked(workspace string) (string, bool) {
	metas, err := s.listWorkspaceSessionsReadOnly(workspace)
	if err != nil || len(metas) == 0 {
		return "", false
	}
	currentID := s.currentSessionIDForWorkspaceReadOnly(workspace)
	for _, meta := range metas {
		s.restoreRuntimeSessionFromMetaLocked(meta, workspace)
	}
	currentID = strings.TrimSpace(currentID)
	if currentID == "" && len(metas) > 0 {
		currentID = metas[0].ID
	}
	return currentID, len(metas) > 0
}

func (s *BridgeService) restoreCurrentRuntimeSessionLocked(workspace string) *sessionState {
	currentID := s.currentSessionIDForWorkspaceReadOnly(workspace)
	if strings.TrimSpace(currentID) == "" {
		return nil
	}
	return s.restoreRuntimeSessionLocked(currentID, workspace)
}

func (s *BridgeService) restoreRuntimeSessionLocked(sessionID, workspace string) *sessionState {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if current := s.ensureSessionByIDLocked(sessionID); current != nil {
		// workspace 非空时校验一致性：core snapshot 可能携带跨 workspace 的脏数据，
		// 不一致则不返回该 session（返回 nil 让调用方按 workspace 新建），避免把别的
		// 工作区的会话注入当前 workspace 的复用链。
		if workspace != "" && !sameWorkspacePath(current.WorkspacePath, workspace) {
			return nil
		}
		if strings.TrimSpace(current.WorkspacePath) == "" {
			current.WorkspacePath = strings.TrimSpace(workspace)
		}
		return current
	}
	resolvedWorkspace := strings.TrimSpace(workspace)
	if resolvedWorkspace == "" {
		resolvedWorkspace = s.workspaceForSessionFromSnapshotReadOnly(sessionID, s.runtimeSnapshotReadOnly())
	}
	if resolvedWorkspace == "" {
		resolvedWorkspace = strings.TrimSpace(s.activeWorkspace)
	}
	var meta *adapter.SessionMeta
	metas, err := s.listWorkspaceSessionsReadOnly(resolvedWorkspace)
	if err != nil {
		return nil
	}
	for _, item := range metas {
		if item.ID == sessionID {
			copied := item
			meta = &copied
			break
		}
	}
	if meta == nil {
		return nil
	}
	return s.restoreRuntimeSessionFromMetaLocked(*meta, resolvedWorkspace)
}

func (s *BridgeService) restoreRuntimeSessionFromMetaLocked(meta adapter.SessionMeta, workspace string) *sessionState {
	if current := s.ensureSessionByIDLocked(meta.ID); current != nil {
		if strings.TrimSpace(current.WorkspacePath) == "" {
			current.WorkspacePath = strings.TrimSpace(workspace)
		}
		return current
	}
	workspace = strings.TrimSpace(workspace)
	rawMessages, err := s.loadWorkspaceSessionMessagesReadOnly(workspace, meta.ID)
	if err != nil {
		return nil
	}
	messages := chatMessagesFromRuntime(rawMessages)
	session := &sessionState{
		ID:            meta.ID,
		Title:         fallbackText(strings.TrimSpace(meta.Title), fallbackText(strings.TrimSpace(meta.Preview), "历史会话")),
		WorkspacePath: workspace,
		Messages:      messages,
		Persisted:     true,
		UpdatedAt:     meta.SavedAt,
	}
	s.sessions[session.ID] = session
	// 应用退出时进行中的 turn 被杀死，streaming/waiting 的 assistant 消息是永远
	// 不会再推进的死状态（重启后残留"思考中"占位）。归一为中断终态并回写持久化，
	// 让消息流显示明确的"已中断"而不是无限等待的假象。
	if normalizeInterruptedAssistantMessages(session.Messages, s.t("conversation.interrupted")) {
		s.cancelPendingApprovalsForSessionLocked(session.ID)
		if err := s.persistSessionLocked(session); err != nil {
			slog.Warn("bridge.session.normalize_interrupted.persist_failed", "session", session.ID, "error", err)
		}
	}
	return session
}

func (s *BridgeService) cancelPendingApprovalsForSessionLocked(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if s == nil || sessionID == "" {
		return
	}
	for promptID, prompt := range s.prompts {
		if strings.TrimSpace(prompt.SessionID) != sessionID {
			continue
		}
		delete(s.prompts, promptID)
	}
	pending, err := s.listPendingApprovalsRPC(sessionID)
	if err != nil {
		slog.Warn("bridge.session.cancel_pending_approvals.list_failed", "session", sessionID, "error", err)
		return
	}
	for _, approval := range pending {
		approvalID := strings.TrimSpace(approval.ApprovalID)
		if approvalID == "" {
			continue
		}
		if err := s.respondApprovalDecisionRPC(approvalID, coreapi.ApprovalCancel); err != nil {
			slog.Warn("bridge.session.cancel_pending_approvals.respond_failed", "session", sessionID, "approval_id", approvalID, "error", err)
		}
	}
}

// normalizeInterruptedAssistantMessages 把加载自持久化的 streaming/waiting assistant
// 消息归一为中断终态（failed + 清除占位 + 进行中的 item 置 failed + 追加中断提示），
// 并把当时挂起待确认的审批/问询卡片翻成 cancelled（程序退出 = 用户中止，重启后不应复活）。
// 已是终态（failed 等）的历史消息也回扫归一 pending 审批——旧版本收尾时不清审批，
// 落盘里可能残留「消息已失败、审批还 pending」的组合，重启后卡片会复活。
// 返回是否有改动（调用方据此决定是否回写持久化）。
func normalizeInterruptedAssistantMessages(messages []ChatMessage, notice string) bool {
	changed := false
	for i := range messages {
		msg := &messages[i]
		if msg.Role != "assistant" {
			continue
		}
		state := strings.ToLower(strings.TrimSpace(msg.State))
		if state != "streaming" && state != "waiting" {
			// 终态历史消息：只归一遗留的 pending 审批（幂等），不加中断提示。
			for j := range msg.Items {
				it := &msg.Items[j]
				if it.Approval != nil && strings.ToLower(strings.TrimSpace(it.Approval.State)) == "pending" {
					it.Approval.State = "cancelled"
					it.Approval.ResolvedAt = time.Now().Format(time.RFC3339)
					changed = true
				}
			}
			continue
		}
		msg.State = "failed"
		msg.IsPlaceholder = false
		for j := range msg.Items {
			it := &msg.Items[j]
			itemStatus := strings.ToLower(strings.TrimSpace(it.Status))
			if itemStatus == "streaming" || itemStatus == "waiting" {
				it.Status = "failed"
			}
			// 程序退出 = 用户中止：当时挂起待确认的审批/问询卡片不应在重启后复活。
			// 前端浮层只按 approval.state=="pending" 投影卡片（workbench-approvals-logic.ts），
			// 这里把 pending 翻成 cancelled 终态并落盘，重启后卡片不再出现。
			// （只归一一次：本函数仅在被中断的 streaming/waiting 消息上跑，幂等。）
			if it.Approval != nil && strings.ToLower(strings.TrimSpace(it.Approval.State)) == "pending" {
				it.Approval.State = "cancelled"
				it.Approval.ResolvedAt = time.Now().Format(time.RFC3339)
			}
		}
		appendStatusItem(msg, notice, "warning", "failed")
		changed = true
	}
	return changed
}

func (s *BridgeService) createWorkspaceSessionLocked(workspace, title string, notify SessionCreateNotify) (*sessionState, error) {
	meta, err := s.createWorkspaceSessionRPC(workspace, title, nil)
	if err != nil {
		return nil, err
	}
	session := s.restoreRuntimeSessionLocked(meta.ID, workspace)
	if session == nil {
		session = &sessionState{
			ID:            meta.ID,
			Title:         fallbackText(strings.TrimSpace(meta.Title), "新对话"),
			WorkspacePath: workspace,
			Persisted:     true,
			UpdatedAt:     meta.SavedAt,
		}
		s.sessions[session.ID] = session
	}
	s.currentSessionID = session.ID
	s.activeWorkspace = workspace
	s.tryCoreRPC("remember-workspace", workspace, "", func() error {
		return s.rememberWorkspaceRPC(workspace, WorkspaceActivationForeground)
	})
	s.tryCoreRPC("set-current-session", workspace, session.ID, func() error {
		return s.setWorkspaceCurrentSessionRPC(workspace, session.ID)
	})
	if notify == SessionCreateNotifyUser {
	}
	return session, nil
}

func (s *BridgeService) persistSessionLocked(session *sessionState) error {
	if session == nil {
		return nil
	}
	oldID := strings.TrimSpace(session.ID)
	messages := make([]adapter.SessionMessage, 0, len(session.Messages))
	for _, item := range session.Messages {
		// assistant 消息按 Items 展开为多条（带 item_id/turn_id/kind），
		// 使重载时能完整重建；user 消息仍单条。
		messages = append(messages, sessionMessagesFromChatMessage(item)...)
	}
	sessionID, err := s.saveWorkspaceSessionMessagesForSession(session.WorkspacePath, session.ID, messages)
	if err != nil {
		session.Persisted = false
		return err
	}
	if savedID := strings.TrimSpace(sessionID); savedID != "" && savedID != oldID {
		s.rekeySessionLocked(oldID, savedID, session)
	}
	if err := s.setWorkspaceCurrentSessionRPC(session.WorkspacePath, session.ID); err != nil {
		session.Persisted = false
		return err
	}
	if err := s.renameWorkspaceSessionRPC(session.WorkspacePath, session.ID, session.Title); err != nil {
		session.Persisted = false
		return err
	}
	session.Persisted = true
	return nil
}

func (s *BridgeService) saveWorkspaceSessionMessagesForSession(workspacePath, sessionID string, messages []adapter.SessionMessage) (string, error) {
	if s.saveSessionMessages != nil {
		core := s.threadCoreIfExists(sessionID)
		if core == nil {
			return "", errors.New("runtime unavailable: core not ready")
		}
		return s.saveSessionMessages(core, workspacePath, sessionID, messages)
	}
	return s.saveWorkspaceSessionMessagesRPC(workspacePath, sessionID, messages)
}

func (s *BridgeService) rekeySessionLocked(oldID, newID string, session *sessionState) {
	oldID = strings.TrimSpace(oldID)
	newID = strings.TrimSpace(newID)
	if oldID == "" || newID == "" || oldID == newID || session == nil {
		return
	}

	delete(s.sessions, oldID)
	session.ID = newID
	s.sessions[newID] = session

	if strings.TrimSpace(s.currentSessionID) == oldID {
		s.currentSessionID = newID
	}
	if running := s.runningConversations[oldID]; running != nil {
		delete(s.runningConversations, oldID)
		s.runningConversations[newID] = running
	}
	for _, prompt := range s.prompts {
		if strings.TrimSpace(prompt.SessionID) == oldID {
			prompt.SessionID = newID
		}
	}
}
