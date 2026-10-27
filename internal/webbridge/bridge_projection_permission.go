package webbridge

import (
	"strings"
)

// Permission 域 projection：从 adapter 只读快照加载执行权限/沙箱状态。
// 优先取当前会话的权限快照；当会话快照为空（无执行模式/沙箱/分类/待审 diff）时
// 回退到全局只读快照——这是会话尚未建立独立权限时的正常态，不是错误兜底。

func (s *BridgeService) loadPermission() PermissionState {
	item := s.permissionSnapshotForSessionReadOnly(s.currentSessionValue())
	if item.ExecutionMode == "" && item.SandboxMode == "" && len(item.AllowedCategories) == 0 && !item.HasPendingDiff {
		item = s.permissionSnapshotReadOnly()
	}
	return PermissionState{
		ExecutionMode: item.ExecutionMode,
		SandboxMode:   NormalizeSandboxMode(item.SandboxMode),
		// 内核 ApprovalMode serde 只输出标准 kebab-case（untrusted/on-request/never），
		// 壳层读取侧无需别名映射——直接透传，避免壳层和内核两套归一化漂移。
		ApprovalMode:      strings.TrimSpace(item.ApprovalMode),
		AllowAll:          item.AllowAll,
		AllowedCategories: append([]string(nil), item.AllowedCategories...),
		HasPendingDiff:    item.HasPendingDiff,
		PendingDiffPath:   item.PendingDiffPath,
	}
}
