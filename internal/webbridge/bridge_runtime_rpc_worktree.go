package webbridge

import (
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// worktree RPC：worktree 列表只读，以及创建 / 删除。

func (s *BridgeService) worktreesReadOnly() []adapter.Worktree {
	return coreValueOrNil(
		s,
		[]adapter.Worktree(nil),
		func(g bridgeRuntimeGateway) ([]adapter.Worktree, error) {
			return g.CoreListWorktreesRPC(coreCtx())
		},
	)
}

func (s *BridgeService) createWorktreeRPC(name string) (adapter.Worktree, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return adapter.Worktree{}, err
	}
	name = strings.TrimSpace(name)
	return coreOnlyResult(
		gateway,
		func(g bridgeRuntimeGateway) (adapter.Worktree, error) {
			return g.CoreCreateWorktreeRPC(coreCtx(), name)
		},
	)
}

func (s *BridgeService) removeWorktreeRPC(path string, force bool) error {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return err
	}
	path = strings.TrimSpace(path)
	return coreOnlyErr(
		gateway,
		func(g bridgeRuntimeGateway) error { return g.CoreRemoveWorktreeRPC(coreCtx(), path, force) },
	)
}
