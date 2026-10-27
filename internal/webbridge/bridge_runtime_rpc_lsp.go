package webbridge

import (
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// LSP 域 RPC：LSP 服务器列表 + 检测 / 启动 / 一键安装。

func (s *BridgeService) lspServersReadOnly() []adapter.LSPServer {
	return coreValueOrNil(
		s,
		[]adapter.LSPServer(nil),
		func(g bridgeRuntimeGateway) ([]adapter.LSPServer, error) {
			return g.CoreListLSPRPC(coreCtx())
		},
	)
}

func (s *BridgeService) detectLSPRPC(language string) (string, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return "", err
	}
	language = strings.TrimSpace(language)
	text, err := coreOnlyResult(
		gateway,
		func(g bridgeRuntimeGateway) (string, error) {
			return g.CoreDetectLSPRPC(coreCtx(), language)
		},
	)
	return strings.TrimSpace(text), err
}

func (s *BridgeService) startLSPRPC(language string) (string, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return "", err
	}
	language = strings.TrimSpace(language)
	text, err := coreOnlyResult(
		gateway,
		func(g bridgeRuntimeGateway) (string, error) { return g.CoreStartLSPRPC(coreCtx(), language) },
	)
	return strings.TrimSpace(text), err
}

func (s *BridgeService) installLSPRPC(language string) (string, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return "", err
	}
	language = strings.TrimSpace(language)
	text, err := coreOnlyResult(
		gateway,
		func(g bridgeRuntimeGateway) (string, error) { return g.CoreInstallLSPRPC(coreCtx(), language) },
	)
	return strings.TrimSpace(text), err
}
