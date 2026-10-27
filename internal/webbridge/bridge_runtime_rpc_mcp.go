package webbridge

import (
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// MCP 域 RPC：服务器列表只读 + 新增 / 导入 / 删除 / 启停。

func (s *BridgeService) mcpServersReadOnly() []adapter.MCPServer {
	return coreValueOrNotify(
		s,
		"mcp-servers",
		"MCP 服务器清单加载失败",
		"无法从内核读取 MCP 服务器列表，请稍后重试或检查核心状态",
		[]adapter.MCPServer(nil),
		func(g bridgeRuntimeGateway) ([]adapter.MCPServer, error) {
			return g.CoreListMCPRPC(coreCtx())
		},
	)
}

func (s *BridgeService) upsertMCPRPC(name, kind, target string, enabled bool) error {
	name = strings.TrimSpace(name)
	kind = strings.TrimSpace(kind)
	target = strings.TrimSpace(target)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error {
			return g.CoreUpsertMCPRPC(coreCtx(), name, kind, target, enabled)
		},
	)
}

func (s *BridgeService) importMCPJSONRPC(raw string) error {
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreImportMCPJSONRPC(coreCtx(), raw) },
	)
}

func (s *BridgeService) deleteMCPRPC(name string) error {
	name = strings.TrimSpace(name)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreDeleteMCPRPC(coreCtx(), name) },
	)
}

func (s *BridgeService) setMCPEnabledRPC(name string, enabled bool) error {
	name = strings.TrimSpace(name)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreSetMCPEnabledRPC(coreCtx(), name, enabled) },
	)
}
