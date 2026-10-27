package webbridge

import (
	"log/slog"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// remote workspace RPC：远程仓库列表 + 当前仓库只读，以及打开 / 忘记 / 清缓存。

func (s *BridgeService) listRemoteWorkspacesReadOnly() []adapter.RemoteWorkspace {
	return coreValueOrNotify(
		s,
		"remote-workspaces",
		"远程仓库列表加载失败",
		"无法从内核读取远程仓库列表，请稍后重试或检查核心状态",
		[]adapter.RemoteWorkspace(nil),
		func(g bridgeRuntimeGateway) ([]adapter.RemoteWorkspace, error) {
			return g.CoreListRemoteWorkspacesRPC(coreCtx())
		},
	)
}

func (s *BridgeService) currentRemoteRepoReadOnly() (adapter.RemoteRepoState, bool) {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return adapter.RemoteRepoState{}, false
	}
	state, ok, err := gateway.CoreCurrentRemoteRepoRPC(coreCtx())
	if err == nil {
		s.clearDegraded("current-remote-repo")
		return state, ok
	}
	slog.Warn("bridge.core_rpc.read_failed", "domain", "current-remote-repo", "error", err)
	if s.coreReady() {
		s.notifyDegraded("current-remote-repo", "当前远程仓库状态加载失败", "无法从内核读取当前远程仓库信息，请稍后重试")
	}
	return adapter.RemoteRepoState{}, false
}

func (s *BridgeService) openRemoteWorkspaceRPC(idOrPath string) (adapter.RemoteWorkspace, error) {
	idOrPath = strings.TrimSpace(idOrPath)
	return coreValueOrRequire(
		s,
		func(g bridgeRuntimeGateway) (adapter.RemoteWorkspace, error) {
			return g.CoreOpenRemoteWorkspaceRPC(coreCtx(), idOrPath)
		},
	)
}

func (s *BridgeService) forgetRemoteWorkspaceRPC(idOrPath string) error {
	idOrPath = strings.TrimSpace(idOrPath)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error {
			return g.CoreForgetRemoteWorkspaceRPC(coreCtx(), idOrPath)
		},
	)
}

func (s *BridgeService) clearRemoteWorkspaceCacheRPC(idOrPath string) error {
	idOrPath = strings.TrimSpace(idOrPath)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error {
			return g.CoreClearRemoteWorkspaceCacheRPC(coreCtx(), idOrPath)
		},
	)
}
