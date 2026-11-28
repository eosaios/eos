package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"errors"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi"
)

type CommandService struct {
	bridge *BridgeService
}

func NewCommandService(bridge *BridgeService) *CommandService {
	return &CommandService{bridge: bridge}
}

func (s *BridgeService) commandService() *CommandService {
	if s == nil {
		return NewCommandService(nil)
	}
	s.serviceMu.Lock()
	defer s.serviceMu.Unlock()
	if s.commandSvc == nil {
		s.commandSvc = NewCommandService(s)
	}
	return s.commandSvc
}

func (svc *CommandService) DefaultCommandPalette() []CommandAction {
	return []CommandAction{
		{ID: "cmd-page-sessions", Command: "page.open.sessions", Label: "Open Sessions", Description: "Switch to the session management page", Target: "sessions"},
		{ID: "cmd-page-rules", Command: "page.open.rules", Label: "Open Rules", Description: "Switch to the rules page", Target: "rules"},
	}
}

func (svc *CommandService) ResolvePrompt(promptID, decision, note string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	s.stateMu.Lock()
	promptID = strings.TrimSpace(promptID)
	prompt, ok := s.prompts[promptID]
	if !ok {
		// 该审批已被内核/旁路 resolved（切 never 模式 auto-approve、resolved 事件
		// 先删）：用户的放行意图已达成，幂等成功返回，不当错误抛给前端。
		// 消息状态行由 resolved 事件路径（handleConversationResolutionLocked）更新。
		s.stateMu.Unlock()
		return s.LoadBootstrap(), nil
	}
	if prompt.Source == "request-user-input" {
		answers, err := requestUserInputDisplayAnswers(prompt, decision, note)
		if err == nil {
			err = s.resolveStructuredQuestionLocked(prompt, decision, note, answers)
		}
		s.stateMu.Unlock()
		if err != nil {
			return s.LoadBootstrap(), err
		}
		s.emitShellUpdatedForSession(prompt.SessionID)
		return s.bootstrapForSession(prompt.SessionID), nil
	}
	// Wait for the Core receipt before mutating the prompt or announcing approval.
	// The same lock serializes duplicate submissions and incoming resolution events.
	if pendingSession := s.sessions[prompt.SessionID]; pendingSession != nil {
		if err := s.persistSessionLocked(pendingSession); err != nil {
			s.stateMu.Unlock()
			return s.LoadBootstrap(), err
		}
	}
	if err := s.respondPromptRPC(prompt, decision, note); err != nil {
		s.stateMu.Unlock()
		return s.LoadBootstrap(), err
	}
	resolvedText, resolvedLevel := s.resolvedStatusTextAndLevel(prompt, decision)
	session := s.settlePromptLocked(nil, prompt, resolvedText, resolvedLevel, "streaming")
	if session != nil {
		if err := s.persistSessionLocked(session); err != nil {
			s.pushNotificationLocked(s.t("notification.request_failed.title"), err.Error(), "danger")
		}
	}
	s.stateMu.Unlock()
	s.emitShellUpdatedForSession(prompt.SessionID)
	return s.bootstrapForSession(prompt.SessionID), nil
}

func (svc *CommandService) KillTask(taskID string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.killTaskRPC(taskID); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

// resolvedStatusTextAndLevel 把用户决策 token 映射为消息流状态行的显示文本与级别。
// decision 是前端回传的 canonical token（= coreapi.ApprovalDecision wire 值），
// 与 approvalDecisionFromToken 的校验集合一致。允许类用 success（绿色），
// 拒绝/取消类用 warning（橙色）。AGENTS.md L109：用户可见文案用 i18n key。
func (s *BridgeService) resolvedStatusTextAndLevel(prompt *promptState, decision string) (string, string) {
	if prompt != nil && prompt.Source == "request-user-input" {
		return s.t("request_user_input.resolved.answered"), "info"
	}
	switch coreapi.ApprovalDecision(strings.TrimSpace(decision)) {
	case coreapi.ApprovalAccept:
		return s.t("approval.resolved.allowed"), "success"
	case coreapi.ApprovalAcceptForSession:
		return s.t("approval.resolved.allowed_session"), "success"
	case coreapi.ApprovalDecline:
		return s.t("approval.resolved.denied"), "warning"
	case coreapi.ApprovalCancel:
		return s.t("approval.resolved.cancelled"), "warning"
	default:
		return s.t("approval.resolved.default"), "info"
	}
}

func (svc *CommandService) DismissNotification(notificationID string) BootstrapState {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}
	}
	s.stateMu.Lock()
	notificationID = strings.TrimSpace(notificationID)
	next := s.notifications[:0]
	for _, item := range s.notifications {
		if item.ID == notificationID {
			continue
		}
		next = append(next, item)
	}
	s.notifications = append([]NotificationItem(nil), next...)
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap()
}

func (svc *CommandService) RunCommandPalette(command string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "session.new":
		return s.chatService().CreateSession("")
	case "tasks.cleanup":
		s.cleanupTasksRPC()
		s.stateMu.Lock()
		s.emitShellUpdated()
		s.stateMu.Unlock()
		return s.LoadBootstrap(), nil
	case "clipboard.copy-diagnostics":
		report := s.buildDiagnosticsReport()
		s.stateMu.Lock()
		if s.writeClipboardText(report) {
		} else {
		}
		s.emitShellUpdated()
		s.stateMu.Unlock()
		return s.LoadBootstrap(), nil
	case "notifications.clear":
		s.stateMu.Lock()
		s.notifications = nil
		s.emitShellUpdated()
		s.stateMu.Unlock()
		return s.LoadBootstrap(), nil
	default:
		return s.LoadBootstrap(), nil
	}
}
