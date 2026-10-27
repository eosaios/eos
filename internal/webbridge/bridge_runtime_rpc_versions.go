package webbridge

import (
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// 版本域 RPC：版本列表只读 + 回滚 / 删除 / 清空写操作。

func (s *BridgeService) versionsReadOnly() []adapter.VersionItem {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return nil
	}
	return coreOnlyValue(
		gateway,
		[]adapter.VersionItem(nil),
		func(g bridgeRuntimeGateway) ([]adapter.VersionItem, error) {
			return g.CoreListVersionsRPC(coreCtx())
		},
	)
}

func (s *BridgeService) rollbackVersionRPC(id string) error {
	id = strings.TrimSpace(id)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreRollbackVersionRPC(coreCtx(), id) },
	)
}

func (s *BridgeService) deleteVersionRPC(id string) error {
	id = strings.TrimSpace(id)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreDeleteVersionRPC(coreCtx(), id) },
	)
}

func (s *BridgeService) clearVersionsRPC() int {
	return coreValueOrNil(
		s,
		0,
		func(g bridgeRuntimeGateway) (int, error) { return g.CoreClearVersionsRPC(coreCtx()) },
	)
}
