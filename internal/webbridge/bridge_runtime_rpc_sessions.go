package webbridge

import (
	"errors"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

func (s *BridgeService) listWorkspaceSessionsReadOnly(workspace string) ([]adapter.SessionMeta, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	workspace = strings.TrimSpace(workspace)
	items, err := coreOnlyResult(
		gateway,
		func(g bridgeRuntimeGateway) ([]coreapi.Session, error) {
			return g.CoreListSessionsRPC(coreCtx(), workspace)
		},
	)
	if err != nil {
		return nil, err
	}
	return sessionMetasFromCoreAPI(items, workspace), nil
}

func (s *BridgeService) resumeWorkspaceSessionRPC(workspace, sessionID string) error {
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id required")
	}
	return coreErrOrRequire(s, func(g bridgeRuntimeGateway) error {
		_, err := g.CoreResumeSessionRPC(coreCtx(), workspace, sessionID)
		return err
	})
}

func (s *BridgeService) createWorkspaceSessionRPC(workspace, title string, messages []adapter.SessionMessage) (adapter.SessionMeta, error) {
	workspace = strings.TrimSpace(workspace)
	title = strings.TrimSpace(title)
	return coreValueOrRequire(
		s,
		func(g bridgeRuntimeGateway) (adapter.SessionMeta, error) {
			return g.CoreCreateSessionRPC(coreCtx(), workspace, title, "gui", messages)
		},
	)
}

func (s *BridgeService) saveWorkspaceSessionMessagesRPC(workspace, sessionID string, messages []adapter.SessionMessage) (string, error) {
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	meta, err := coreValueOrRequire(
		s,
		func(g bridgeRuntimeGateway) (adapter.SessionMeta, error) {
			return g.CoreSaveSessionMessagesRPC(coreCtx(), workspace, sessionID, messages)
		},
	)
	if err != nil {
		return "", err
	}
	return fallbackText(strings.TrimSpace(meta.ID), sessionID), nil
}

func (s *BridgeService) renameWorkspaceSessionRPC(workspace, sessionID, title string) error {
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	title = strings.TrimSpace(title)
	return coreErrOrRequire(s, func(g bridgeRuntimeGateway) error {
		_, err := g.CoreRenameSessionRPC(coreCtx(), workspace, sessionID, title)
		return err
	})
}

func (s *BridgeService) deleteWorkspaceSessionRPC(workspace, sessionID string) error {
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	return coreErrOrRequire(s, func(g bridgeRuntimeGateway) error {
		return g.CoreDeleteSessionRPC(coreCtx(), workspace, sessionID)
	})
}

func (s *BridgeService) archiveSessionRPC(sessionID string, archived bool) error {
	sessionID = strings.TrimSpace(sessionID)
	return coreErrOrRequire(s, func(g bridgeRuntimeGateway) error {
		return g.CoreArchiveSessionRPC(coreCtx(), sessionID, archived)
	})
}
