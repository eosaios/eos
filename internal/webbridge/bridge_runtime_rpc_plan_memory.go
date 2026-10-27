package webbridge

import (
	"errors"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// 计划 / 记忆域 RPC：plan snapshot + memory snapshot 只读查询、
// 记忆笔记写入（内核 memory/save）。

func (s *BridgeService) planSnapshotReadOnly() adapter.PlanSnapshot {
	return coreValueOrNil(
		s,
		adapter.PlanSnapshot{},
		func(g bridgeRuntimeGateway) (adapter.PlanSnapshot, error) {
			return g.CorePlanSnapshotRPC(coreCtx())
		},
	)
}

func (s *BridgeService) memorySnapshotReadOnly() adapter.MemorySnapshot {
	return coreValueOrNil(
		s,
		adapter.MemorySnapshot{},
		func(g bridgeRuntimeGateway) (adapter.MemorySnapshot, error) {
			return g.CoreMemorySnapshotRPC(coreCtx())
		},
	)
}

// SaveMemoryNote 把「添加记忆笔记」内容经内核 memory/save 落为 ad_hoc note
// （append-only，写 ~/.eos/memories/extensions/ad_hoc/notes/）。返回刷新后的
// BootstrapState 供前端更新面板。
func (s *BridgeService) SaveMemoryNote(content string) (BootstrapState, error) {
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return s.LoadBootstrap(), errors.New(s.t("error.memory.note_empty"))
	}
	gateway := s.runtimeGatewayClient()
	if gateway == nil {
		return s.LoadBootstrap(), errors.New("runtime core unavailable")
	}
	if err := gateway.CoreMemorySaveRPC(coreCtx(), content); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}
