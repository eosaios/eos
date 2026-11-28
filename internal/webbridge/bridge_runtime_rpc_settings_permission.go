package webbridge

import (
	"context"
	"log/slog"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// 设置 / 权限域 RPC：GUI 设置、权限快照（全局 + 会话级）、会话 core 查找。

func (s *BridgeService) settingsReadOnly() adapter.GUISettings {
	return coreValueOrNil(
		s,
		adapter.GUISettings{},
		func(g bridgeRuntimeGateway) (adapter.GUISettings, error) {
			return g.CoreGetSettingsRPC(coreCtx())
		},
	)
}

func (s *BridgeService) permissionSnapshotReadOnly() adapter.PermissionSnapshot {
	return coreValueOrNil(
		s,
		adapter.PermissionSnapshot{},
		func(g bridgeRuntimeGateway) (adapter.PermissionSnapshot, error) {
			return g.CorePermissionSnapshotRPC(coreCtx())
		},
	)
}

func (s *BridgeService) permissionSnapshotForSessionReadOnly(sessionID string) adapter.PermissionSnapshot {
	unknown := adapter.PermissionSnapshot{SandboxMode: "unknown", ApprovalMode: "unknown"}
	if s == nil || s.runtimeGatewayClient() == nil {
		return unknown
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		if current, ok := s.runtimeGatewayClient().(interface {
			CorePermissionSessionSnapshotRPC(context.Context, string) (adapter.PermissionSnapshot, error)
		}); ok {
			snapshot, err := current.CorePermissionSessionSnapshotRPC(coreCtx(), sessionID)
			if err != nil {
				slog.Warn("session permission snapshot unavailable", "error", err)
				return unknown
			}
			return snapshot
		}
		return unknown
	}
	return unknown
}

func (s *BridgeService) threadCoreIfExists(sessionID string) adapter.Core {
	if s == nil || s.runtimeGatewayClient() == nil {
		return nil
	}
	return s.runtimeGatewayClient().ThreadCoreIfExists(strings.TrimSpace(sessionID))
}
