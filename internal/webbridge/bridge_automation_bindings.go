package webbridge

import (
	"strings"
	"time"
)

// ── Wails 绑定方法（前端通过 BridgeService.X 调用）──

func (s *BridgeService) SaveAutomationTemplate(req AutomationSaveRequest) (BootstrapState, error) {
	return s.automationService().SaveAutomationTemplate(req)
}

func (s *BridgeService) DeleteAutomationTemplate(id string) (BootstrapState, error) {
	return s.automationService().DeleteAutomationTemplate(id)
}

func (s *BridgeService) ToggleAutomationTemplate(id string, enabled bool) (BootstrapState, error) {
	return s.automationService().ToggleAutomationTemplate(id, enabled)
}

// VerifyCronExpression 给前端做实时校验，返回是否合法 + 人类可读的下一次运行时间。
type CronVerifyResult struct {
	Valid   bool   `json:"valid"`
	NextRun string `json:"nextRun"`
	Error   string `json:"error"`
}

func (s *BridgeService) VerifyCronExpression(schedule string) CronVerifyResult {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return CronVerifyResult{Valid: true}
	}
	if err := validateCronSchedule(schedule); err != nil {
		return CronVerifyResult{Valid: false, Error: err.Error()}
	}
	next, ok := nextCronRun(schedule)
	if !ok {
		return CronVerifyResult{Valid: true, NextRun: ""}
	}
	return CronVerifyResult{Valid: true, NextRun: next.Format(time.RFC3339)}
}
