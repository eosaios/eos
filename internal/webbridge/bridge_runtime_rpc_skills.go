package webbridge

import (
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// Skills 域 RPC：技能列表只读 + 重载 / 启停。

func (s *BridgeService) skillsReadOnly() []adapter.SkillInfo {
	return coreValueOrNotify(
		s,
		"skills",
		"技能清单加载失败",
		"无法从内核读取技能列表，请稍后重试或检查核心状态",
		[]adapter.SkillInfo(nil),
		func(g bridgeRuntimeGateway) ([]adapter.SkillInfo, error) {
			return g.CoreListSkillsRPC(coreCtx())
		},
	)
}

func (s *BridgeService) reloadSkillsRPC() error {
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreReloadSkillsRPC(coreCtx()) },
	)
}

func (s *BridgeService) setSkillEnabledRPC(name string, enabled bool) error {
	name = strings.TrimSpace(name)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error {
			return g.CoreSetSkillEnabledRPC(coreCtx(), name, enabled)
		},
	)
}
