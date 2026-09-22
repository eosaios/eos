package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

func (svc *ChatService) RollbackChatTurn(sessionID, userMessageID string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	userMessageID = strings.TrimSpace(userMessageID)
	if userMessageID == "" {
		return s.LoadBootstrap(), errors.New("message id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.currentSessionValue()
	}
	if sessionID == "" {
		return s.LoadBootstrap(), errors.New("session id is required")
	}
	session := s.findOrRestoreSession("", sessionID)
	if session == nil {
		return s.LoadBootstrap(), os.ErrNotExist
	}

	s.stateMu.Lock()
	session = s.sessions[strings.TrimSpace(session.ID)]
	if session == nil {
		s.stateMu.Unlock()
		return s.LoadBootstrap(), os.ErrNotExist
	}
	if session.Running || s.runningConversations[strings.TrimSpace(session.ID)] != nil {
		s.stateMu.Unlock()
		return s.LoadBootstrap(), errors.New("current session is still processing; finish or stop it before rollback")
	}
	userIndex := -1
	for index := range session.Messages {
		if strings.TrimSpace(session.Messages[index].ID) == userMessageID && strings.EqualFold(strings.TrimSpace(session.Messages[index].Role), "user") {
			userIndex = index
			break
		}
	}
	if userIndex < 0 {
		s.stateMu.Unlock()
		return s.LoadBootstrap(), os.ErrNotExist
	}

	removedMessages := cloneMessages(session.Messages[userIndex:])
	rollbacks, err := turnRollbacksFromMessages(removedMessages)
	if err != nil {
		s.stateMu.Unlock()
		return s.LoadBootstrap(), err
	}
	workspace := strings.TrimSpace(session.WorkspacePath)
	if err := s.applyRollbacksRPC(workspace, rollbacks); err != nil {
		s.stateMu.Unlock()
		return s.LoadBootstrap(), err
	}

	removedIDs := map[string]struct{}{}
	for _, message := range removedMessages {
		if id := strings.TrimSpace(message.ID); id != "" {
			removedIDs[id] = struct{}{}
		}
	}
	session.Messages = cloneMessages(session.Messages[:userIndex])
	session.Running = false
	session.NeedsAttention = false
	session.UpdatedAt = time.Now()
	for promptID, prompt := range s.prompts {
		if strings.TrimSpace(prompt.SessionID) != strings.TrimSpace(session.ID) {
			continue
		}
		if _, removed := removedIDs[strings.TrimSpace(prompt.AssistantMessageID)]; removed {
			delete(s.prompts, promptID)
		}
	}
	if err := s.persistSessionLocked(session); err != nil {
		session.Persisted = false
		session.NeedsAttention = true
		s.stateMu.Unlock()
		return s.LoadBootstrap(), err
	}
	s.currentSessionID = session.ID
	s.activeWorkspace = session.WorkspacePath
	if err := s.setWorkspaceCurrentSessionRPC(session.WorkspacePath, session.ID); err != nil {
		s.pushNotificationLocked(s.t("notification.request_failed.title"), "回滚已完成，但更新当前会话时出错："+err.Error(), "warning")
	}
	targetSessionID := session.ID
	s.stateMu.Unlock()
	response := s.bootstrapForSession(targetSessionID)
	s.emitShellUpdatedForSession(targetSessionID)
	return response, nil
}

// applyRollbacksRPC forwards the rollback descriptor to the Rust core's
// workspace/rollback/apply RPC. The Go shell no longer applies rollbacks
// directly. The local TurnRollback and coreapi.TurnRollback share the same
// camelCase JSON shape, so we round-trip through JSON to convert.
func (s *BridgeService) applyRollbacksRPC(workspace string, rollbacks []TurnRollback) error {
	if len(rollbacks) == 0 {
		return nil
	}
	coreRollbacks, err := coreAPITurnRollbacks(rollbacks)
	if err != nil {
		return err
	}
	return coreErrOrRequire(s, func(g bridgeRuntimeGateway) error {
		return g.CoreWorkspaceRollbackApplyRPC(context.Background(), workspace, coreRollbacks)
	})
}

// coreAPITurnRollbacks converts the Go shell's local []TurnRollback (package
// main) into []coreapi.TurnRollback via a JSON round-trip. The two types share
// identical camelCase JSON tags, so no field is lost.
func coreAPITurnRollbacks(rollbacks []TurnRollback) ([]coreapi.TurnRollback, error) {
	if len(rollbacks) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(rollbacks)
	if err != nil {
		return nil, err
	}
	var out []coreapi.TurnRollback
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
