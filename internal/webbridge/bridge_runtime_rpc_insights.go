package webbridge

import (
	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// runtime 只读洞察：用量 / 成本 / 诊断类快照查询。
// 版本 RPC 见 bridge_runtime_rpc_versions.go，
// 设置 / 权限见 bridge_runtime_rpc_settings_permission.go，
// 计划 / 记忆见 bridge_runtime_rpc_plan_memory.go。

func (s *BridgeService) usageSummaryReadOnly() adapter.UsageSummary {
	return coreValueOrNotify(
		s,
		"usage-summary",
		"用量统计暂不可用",
		"无法从内核读取用量统计，显示的零值不代表真实用量，请稍后重试",
		adapter.UsageSummary{},
		func(g bridgeRuntimeGateway) (adapter.UsageSummary, error) {
			return g.CoreUsageSummaryRPC(coreCtx())
		},
	)
}

func (s *BridgeService) costItemsReadOnly() []adapter.CostItem {
	return coreValueOrNil(
		s,
		[]adapter.CostItem(nil),
		func(g bridgeRuntimeGateway) ([]adapter.CostItem, error) {
			return g.CoreCostItemsRPC(coreCtx())
		},
	)
}

func (s *BridgeService) costSummaryReadOnly() string {
	return coreValueOrNil(
		s,
		"",
		func(g bridgeRuntimeGateway) (string, error) { return g.CoreCostSummaryRPC(coreCtx()) },
	)
}

func (s *BridgeService) pendingReviewReadOnly() adapter.PendingReview {
	return coreValueOrNil(
		s,
		adapter.PendingReview{},
		func(g bridgeRuntimeGateway) (adapter.PendingReview, error) {
			return g.CorePendingReviewRPC(coreCtx())
		},
	)
}

func (s *BridgeService) lspDiagnosticsReadOnly() []string {
	return coreValueOrNil(
		s,
		[]string(nil),
		func(g bridgeRuntimeGateway) ([]string, error) { return g.CoreLSPDiagnosticsRPC(coreCtx()) },
	)
}

func (s *BridgeService) contextPreviewReadOnly() []string {
	return coreValueOrNil(
		s,
		[]string(nil),
		func(g bridgeRuntimeGateway) ([]string, error) { return g.CoreContextPreviewRPC(coreCtx()) },
	)
}

func (s *BridgeService) contextStatsReadOnly() adapter.ContextStats {
	return coreValueOrNil(
		s,
		adapter.ContextStats{},
		func(g bridgeRuntimeGateway) (adapter.ContextStats, error) {
			return g.CoreContextStatsRPC(coreCtx())
		},
	)
}

// coreStartupDiagnosticsReadOnly is intentionally not folded into
// coreValueOrNil: StartupDiagnostics has no error channel, so the generic
// helper does not apply.
func (s *BridgeService) coreStartupDiagnosticsReadOnly() adapter.StartupDiagnosticsResult {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return adapter.StartupDiagnosticsResult{}
	}
	return gateway.StartupDiagnostics()
}
