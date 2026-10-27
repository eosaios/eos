package webbridge

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func (s *BridgeService) loadTasksFromSnapshot(runtimeSnapshot adapter.RuntimeSnapshot) []TaskCard {
	out := make([]TaskCard, 0)
	tasks := runtimeSnapshot.Tasks
	for _, item := range tasks {
		detail := strings.Join(item.Logs, " · ")
		if detail == "" && strings.TrimSpace(item.Workspace) != "" {
			detail = "工作区: " + strings.TrimSpace(item.Workspace)
		}
		out = append(out, TaskCard{
			ID:        item.ID,
			Label:     item.Label,
			Status:    item.Status,
			CanKill:   item.CanKill,
			Detail:    detail,
			Source:    fallbackText(strings.TrimSpace(item.Workspace), "core"),
			UpdatedAt: item.StartedAt.Format(time.RFC3339),
		})
	}
	s.stateMu.RLock()
	for _, session := range s.sessions {
		if !session.Running {
			continue
		}
		out = append(out, TaskCard{
			ID:        "chat:" + session.ID,
			Label:     session.Title,
			Status:    "running",
			CanKill:   false,
			Detail:    "聊天流式任务正在运行或等待会话内确认 · 工作区: " + session.WorkspacePath,
			Source:    fallbackText(strings.TrimSpace(session.WorkspacePath), "chat"),
			UpdatedAt: session.UpdatedAt.Format(time.RFC3339),
		})
	}
	s.stateMu.RUnlock()
	slices.SortFunc(out, func(a, b TaskCard) int {
		return strings.Compare(b.UpdatedAt, a.UpdatedAt)
	})
	return out
}

func (s *BridgeService) resourceChecks(bridgeMode string, diagnostics DiagnosticsState, clipboard ClipboardState, window WindowSnapshot) []ResourceCheck {
	checks := []ResourceCheck{
		{Name: "Wails v3 宿主", Status: "ready", Detail: "桌面窗口、自定义标题栏与事件总线已接管主壳层。"},
		{Name: "共享核心桥接", Status: bridgeModeStatus(bridgeMode), Detail: bridgeModeDetail(bridgeMode)},
		s.runtimeGatewayResourceCheck(),
		{Name: "线程 / 会话 / 聊天", Status: "ready", Detail: "线程列表、消息区、输入框和工作区切换已绑定同一份桥接状态。"},
		{Name: "会话内确认 / 通知 / 命令面板", Status: "ready", Detail: "前端可在对应消息下消费审批、问询、通知与命令动作。"},
		{Name: "Bash / 会话内变更 / Worktree 入口", Status: "ready", Detail: "Bash 命令执行、消息级变更审阅与 Worktree 边界提示已回到正式工作流。"},
		{Name: "工具与管理页面", Status: "ready", Detail: "Models、MCP、LSP、Skills、Plugins、Rules、Settings、Versions 等页面直接消费 bridge / adapter 状态。"},
	}
	clipboardStatus := "baseline"
	if clipboard.Supported {
		clipboardStatus = "ready"
	}
	checks = append(checks,
		ResourceCheck{Name: "文件对话框与剪贴板", Status: clipboardStatus, Detail: "已补齐附件选择、工作区选择、诊断包导出和剪贴板读写。"},
		ResourceCheck{Name: "窗口状态桥接", Status: "ready", Detail: fmt.Sprintf("当前窗口 %dx%d，最大化=%t。", window.Width, window.Height, window.Maximised)},
	)
	if reason := s.modelCatalogFallbackSnapshot(); reason != "" {
		checks = append(checks, ResourceCheck{
			Name:   modelCatalogUnavailableTitle,
			Status: "warning",
			Detail: reason,
		})
	}
	if diagnostics.PendingCrashPath != "" || diagnostics.LogFile != "" {
		checks = append(checks, ResourceCheck{Name: "日志与诊断桥接", Status: "ready", Detail: "日志尾部、待处理崩溃报告和诊断包导出路径已联通。"})
	}
	return checks
}
