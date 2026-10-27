package webbridge

import (
	"errors"
	"strings"
)

// workspace 写操作 RPC：激活 / 新增 / 切换 / 信任 / 移除 / 记忆 / 设置当前会话。
// 路径只读查询见 bridge_runtime_rpc_workspace_paths.go，
// worktree / remote workspace / 运行模式 RPC 见各自独立文件。

func (s *BridgeService) activateWorkspaceRPC(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("workspace path is required")
	}
	if err := s.useWorkspaceRPC(path); err == nil {
		return nil
	} else {
		addErr := s.addWorkspaceRPC(path)
		retryErr := s.useWorkspaceRPC(path)
		if retryErr == nil {
			return nil
		}
		return errors.Join(err, addErr, retryErr)
	}
}

func (s *BridgeService) addWorkspaceRPC(path string) error {
	path = strings.TrimSpace(path)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreAddWorkspaceRPC(coreCtx(), path) },
	)
}

func (s *BridgeService) useWorkspaceRPC(path string) error {
	path = strings.TrimSpace(path)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreUseWorkspaceRPC(coreCtx(), path) },
	)
}

func (s *BridgeService) trustWorkspaceRPC(path string) error {
	path = strings.TrimSpace(path)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreTrustWorkspaceRPC(coreCtx(), path) },
	)
}

func (s *BridgeService) removeWorkspaceRPC(path string) error {
	path = strings.TrimSpace(path)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreRemoveWorkspaceRPC(coreCtx(), path) },
	)
}

func (s *BridgeService) rememberWorkspaceRPC(path string, activation WorkspaceActivation) error {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return err
	}
	path = strings.TrimSpace(path)
	foreground := activation == WorkspaceActivationForeground
	return coreOnlyErr(
		gateway,
		func(g bridgeRuntimeGateway) error {
			return g.CoreRememberWorkspaceRPC(coreCtx(), path, foreground)
		},
	)
}

func (s *BridgeService) setWorkspaceCurrentSessionRPC(workspace, sessionID string) error {
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error {
			return g.CoreSetCurrentSessionRPC(coreCtx(), workspace, sessionID)
		},
	)
}
