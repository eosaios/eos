package webbridge

import (
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
	if s == nil || s.runtimeGatewayClient() == nil {
		return adapter.PermissionSnapshot{}
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		if currentCore := s.runtimeGatewayClient().ThreadCoreIfExists(sessionID); currentCore != nil {
			return currentCore.PermissionSnapshot()
		}
	}
	return s.permissionSnapshotReadOnly()
}

func (s *BridgeService) threadCoreIfExists(sessionID string) adapter.Core {
	if s == nil || s.runtimeGatewayClient() == nil {
		return nil
	}
	return s.runtimeGatewayClient().ThreadCoreIfExists(strings.TrimSpace(sessionID))
}
