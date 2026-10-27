package webbridge

import (
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// Plugins 域 RPC：插件列表只读 + 启停。

func (s *BridgeService) pluginsReadOnly() []adapter.PluginInfo {
	return coreValueOrNotify(
		s,
		"plugins",
		"插件清单加载失败",
		"无法从内核读取插件列表，请稍后重试或检查核心状态",
		[]adapter.PluginInfo(nil),
		func(g bridgeRuntimeGateway) ([]adapter.PluginInfo, error) {
			return g.CoreListPluginsRPC(coreCtx())
		},
	)
}

func (s *BridgeService) setPluginEnabledRPC(name string, enabled bool) error {
	name = strings.TrimSpace(name)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error {
			return g.CoreSetPluginEnabledRPC(coreCtx(), name, enabled)
		},
	)
}
