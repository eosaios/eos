package webbridge

import (
	"strings"
)

// Permission 域 projection：从 adapter 只读快照加载执行权限/沙箱状态。
// 查询失败时保留未知状态，避免跨会话显示错误的权限。

func (s *BridgeService) loadPermission() PermissionState {
	return s.permissionForSession(s.currentSessionValue())
}
func (s *BridgeService) permissionForSession(sessionID string) PermissionState {
	item := s.permissionSnapshotForSessionReadOnly(sessionID)
	return PermissionState{
		ExecutionMode: item.ExecutionMode,
		SandboxMode:   strings.TrimSpace(item.SandboxMode),
		// 内核 ApprovalMode serde 只输出标准 kebab-case（untrusted/on-request/never），
		// 壳层读取侧无需别名映射——直接透传，避免壳层和内核两套归一化漂移。
		ApprovalMode:      strings.TrimSpace(item.ApprovalMode),
		AllowAll:          item.AllowAll,
		AllowedCategories: append([]string(nil), item.AllowedCategories...),
		HasPendingDiff:    item.HasPendingDiff,
		PendingDiffPath:   item.PendingDiffPath,
	}
}
