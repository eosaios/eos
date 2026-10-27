package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// validateCronSchedule 校验 cron 表达式是否合法（标准 5 段：分 时 日 月 周）。
// 空字符串视为合法（手动模板，不参与调度）。
func validateCronSchedule(schedule string) error {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return nil
	}
	_, err := cron.ParseStandard(schedule)
	return err
}

// nextCronRun 计算给定 cron 表达式从现在起的下一次触发时间。
// 解析失败返回 (zero, false)。
func nextCronRun(schedule string) (time.Time, bool) {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return time.Time{}, false
	}
	sched, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}, false
	}
	return sched.Next(time.Now()), true
}

// startAutomationScheduler 初始化 cron 调度器并加载已启用的模板。
// 在 Attach() 中调用。调度器随 eos-app 进程退出而停止（Close 时 cron.Stop），
// 因此"关闭后不运行"。
func (s *BridgeService) startAutomationScheduler() {
	s.automationSchedulerMu.Lock()
	defer s.automationSchedulerMu.Unlock()
	if s.automationScheduler != nil {
		return
	}
	c := cron.New(cron.WithLogger(cron.PrintfLogger(slogLogger{})))
	c.Start()
	s.automationScheduler = c
	s.reloadAutomationSchedulesLocked()
}

// stopAutomationScheduler 停止调度器（Close 时调用）。
func (s *BridgeService) stopAutomationScheduler() {
	s.automationSchedulerMu.Lock()
	defer s.automationSchedulerMu.Unlock()
	if s.automationScheduler == nil {
		return
	}
	stopCtx := s.automationScheduler.Stop()
	<-stopCtx.Done()
	s.automationScheduler = nil
}

// reloadAutomationSchedules 重建调度任务。
// 在模板增删改/启停后调用。调用方需确保不会与 start/stop 并发（用 automationSchedulerMu 保护）。
func (s *BridgeService) reloadAutomationSchedules() {
	s.automationSchedulerMu.Lock()
	defer s.automationSchedulerMu.Unlock()
	s.reloadAutomationSchedulesLocked()
}

// reloadAutomationSchedulesLocked 是 reload 的无锁版本（已持有 automationSchedulerMu）。
func (s *BridgeService) reloadAutomationSchedulesLocked() {
	if s.automationScheduler == nil {
		return
	}
	// cron 库没有"清空所有 job"的便捷 API，重建一个新实例更稳妥。
	c := cron.New(cron.WithLogger(cron.PrintfLogger(slogLogger{})))
	templates := s.allAutomationTemplatesReadOnly()
	for _, tmpl := range templates {
		if !tmpl.Enabled || strings.TrimSpace(tmpl.Schedule) == "" {
			continue
		}
		// 捕获循环变量（Go 1.22+ 每次迭代新变量，但显式拷贝更清晰）。
		template := tmpl
		if _, err := c.AddFunc(template.Schedule, func() {
			s.triggerAutomationTemplate(template)
		}); err != nil {
			slog.Warn("bridge.automation.schedule_add_failed", "template", template.ID, "schedule", template.Schedule, "error", err)
		}
	}
	c.Start()
	old := s.automationScheduler
	s.automationScheduler = c
	if old != nil {
		stopCtx := old.Stop()
		// 不阻塞等待旧任务，让它在后台收尾。
		go func() {
			<-stopCtx.Done()
		}()
	}
}

// triggerAutomationTemplate 是 cron 触发时的回调，复用 RunAutomationTemplate 的执行逻辑。
// 在独立 goroutine（cron 内部）执行，已加锁保护状态。
func (s *BridgeService) triggerAutomationTemplate(template AutomationTemplateCard) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("bridge.automation.trigger_panic", "template", template.ID, "panic", r)
		}
	}()
	slog.Info("bridge.automation.trigger", "template", template.ID, "title", template.Title, "schedule", template.Schedule)

	// 切到绑定工作区（若有），确保发到正确的会话上下文。
	workspace := strings.TrimSpace(template.WorkspacePath)
	if workspace != "" {
		if _, err := s.workspaceService().SelectWorkspace(workspace); err != nil {
			slog.Warn("bridge.automation.trigger_workspace_switch_failed", "workspace", workspace, "error", err)
		}
	}

	if _, err := s.commandService().RunAutomationTemplate(template.ID); err != nil {
		slog.Warn("bridge.automation.trigger_failed", "template", template.ID, "error", err)
	}
}

// slogLogger 把 cron 的 Printf 日志桥接到 slog。
type slogLogger struct{}

func (slogLogger) Printf(format string, args ...any) {
	slog.Info("cron", "msg", strings.TrimSpace(fmt.Sprintf(format, args...)))
}
