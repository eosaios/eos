package webbridge

import (
	"strings"
	"time"
)

// pushNotificationLocked 写入通知中心。
//
// 通知中心只放「用户离开应用后仍需要知道」的事项：
//   - 请求/会话完成
//   - 请求失败（需要重试或处理）
//   - 待审批 / 需要确认（等待用户决策）
//   - 后台任务结果
//   - 系统降级（核心不可用等）
//
// 操作回执（保存成功、导入完成、会话改名等）由页面内反馈表达，不进通知中心。
func (s *BridgeService) pushNotificationLocked(title, message, tone string) {
	item := NotificationItem{
		ID:        newID("notice"),
		Title:     fallbackText(strings.TrimSpace(title), "通知"),
		Message:   strings.TrimSpace(message),
		Tone:      fallbackText(strings.TrimSpace(tone), "info"),
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.notifications = append([]NotificationItem{item}, s.notifications...)
	if len(s.notifications) > maxNotificationCount {
		s.notifications = append([]NotificationItem(nil), s.notifications[:maxNotificationCount]...)
	}
}

func (s *BridgeService) activeWorkspaceValue() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return strings.TrimSpace(s.activeWorkspace)
}

func (s *BridgeService) currentSessionValue() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return strings.TrimSpace(s.currentSessionID)
}

func (s *BridgeService) bridgeMode() string {
	_, bridgeMode := s.loadWorkspaces()
	return bridgeMode
}

// bridgeCoreMode reports the actual transport that backs the runtime core.
// It is the single source of truth for "is the Rust core (or legacy core)
// ready to answer RPCs" and replaces the old "runtime" hard-coded string that
// conflated rust-stdio with the in-process runtime.
//
// Values:
//   - "rust-stdio"  — eos-core --stdio gateway is up and healthy
//   - "legacy"      — legacy Go runtime adapter is active (requires the
//     `legacy` build tag, otherwise unavailable)
//   - "unavailable" — no core transport is available (start failed or not
//     configured); frontend must show fallback / notifications
func (s *BridgeService) bridgeCoreMode() string {
	if s == nil {
		return "unavailable"
	}
	if message := strings.TrimSpace(s.runtimeGatewayStartError); message != "" {
		return "unavailable"
	}
	mode := strings.TrimSpace(s.runtimeGatewayMode)
	switch mode {
	case bridgeRuntimeGatewayModeRust:
		if s.runtimeGateway == nil {
			return "unavailable"
		}
		return "rust-stdio"
	case bridgeRuntimeGatewayModeLegacy:
		if s.runtimeGateway == nil {
			return "unavailable"
		}
		return "legacy"
	}
	// No gateway configured at all (e.g. test wiring that bypasses
	// configureRuntimeGateway) — treat as unavailable so callers render empties.
	return "unavailable"
}

// coreReady returns true when the configured transport is healthy enough to
// answer RPCs. Use this instead of comparing the legacy "runtime" sentinel
// to decide whether snapshot data is available.
func (s *BridgeService) coreReady() bool {
	return s != nil && s.runtimeGateway != nil && strings.TrimSpace(s.runtimeGatewayStartError) == ""
}
