package webbridge

import (
	"strings"
)

// workspace 路径只读查询：默认工作区、上次工作区、core 配置路径。

func (s *BridgeService) defaultWorkspacePathReadOnly() string {
	return strings.TrimSpace(coreValueOrNil(
		s,
		"",
		func(g bridgeRuntimeGateway) (string, error) { return g.CoreDefaultWorkspaceRPC(coreCtx()) },
	))
}

func (s *BridgeService) lastWorkspacePathReadOnly() string {
	return strings.TrimSpace(coreValueOrNil(
		s,
		"",
		func(g bridgeRuntimeGateway) (string, error) { return g.CoreLastWorkspaceRPC(coreCtx()) },
	))
}

func (s *BridgeService) coreConfigPathReadOnly() string {
	if s == nil || s.runtimeGatewayClient() == nil {
		return ""
	}
	return strings.TrimSpace(s.runtimeGatewayClient().CoreConfigPath())
}
