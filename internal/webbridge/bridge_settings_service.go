package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

type SettingsService struct {
	bridge *BridgeService
}

func NewSettingsService(bridge *BridgeService) *SettingsService {
	return &SettingsService{bridge: bridge}
}

func (s *BridgeService) settingsService() *SettingsService {
	if s == nil {
		return NewSettingsService(nil)
	}
	if s.settingsSvc == nil {
		s.settingsSvc = NewSettingsService(s)
	}
	return s.settingsSvc
}

func (svc *SettingsService) SaveSettings(req SettingsSaveRequest) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	current := s.settingsReadOnly()
	normalized := SettingsSaveRequest{
		Language:             fallbackText(strings.TrimSpace(req.Language), fallbackText(strings.TrimSpace(current.Language), "zh")),
		LogDir:               strings.TrimSpace(req.LogDir),
		Theme:                fallbackText(strings.TrimSpace(req.Theme), fallbackText(strings.TrimSpace(current.Theme), "system")),
		DiffTheme:            fallbackText(strings.TrimSpace(req.DiffTheme), defaultDiffTheme),
		ExecutionMode:        normalizeExecutionMode(fallbackText(strings.TrimSpace(req.ExecutionMode), fallbackText(s.executionModeReadOnly(), "auto"))),
		SandboxMode:          NormalizeSandboxMode(fallbackText(strings.TrimSpace(req.SandboxMode), fallbackText(s.sandboxModeReadOnly(), "workspace-write"))),
		ReasoningLevel:       normalizeReasoningLevel(req.ReasoningLevel),
		AutoContext:          req.AutoContext,
		DesktopNotifications: req.DesktopNotifications,
		GitCommitReminder:    req.GitCommitReminder,
		GitCommitMarker:      req.GitCommitMarker,
		StayInTray:           req.StayInTray,
		UseMemory:            req.UseMemory,
		MaxInjectKB:          req.MaxInjectKB,
		WatchDebounceMs:      req.WatchDebounceMs,
		PollIntervalSec:      req.PollIntervalSec,
		PlanPromptStyle:      strings.TrimSpace(req.PlanPromptStyle),
		PromptTimeoutSecs:    normalizePromptTimeoutSecs(req.PromptTimeoutSecs),
		UpdateProxyEnabled:   req.UpdateProxyEnabled,
		UpdateProxyURL:       strings.TrimSpace(req.UpdateProxyURL),
	}

	// 代理开关开启时必须带合法地址：fail-fast，不带兜底（未配置地址=
	// 配置错误，保存即报错由前端展示，不静默降级直连掩盖问题）。
	if normalized.UpdateProxyEnabled {
		if strings.TrimSpace(normalized.UpdateProxyURL) == "" {
			return s.LoadBootstrap(), errors.New("更新代理已开启但地址为空，请填写代理地址")
		}
		if err := validateUpdateProxyURL(normalized.UpdateProxyURL); err != nil {
			return s.LoadBootstrap(), err
		}
	}

	activeWorkspace := s.settingsWorkspaceTarget()
	if _, err := SaveGUISettings(
		s.configPathReadOnly(),
		activeWorkspace,
		GUISettingsSaveInput{
			Language:             normalized.Language,
			LogDir:               normalized.LogDir,
			Theme:                normalized.Theme,
			DiffTheme:            normalized.DiffTheme,
			SandboxMode:          normalized.SandboxMode,
			AutoContext:          normalized.AutoContext,
			DesktopNotifications: normalized.DesktopNotifications,
			GitCommitReminder:    normalized.GitCommitReminder,
			GitCommitMarker:      normalized.GitCommitMarker,
			StayInTray:           normalized.StayInTray,
			UseMemory:            normalized.UseMemory,
			MaxInjectKB:          normalized.MaxInjectKB,
			WatchDebounceMs:      normalized.WatchDebounceMs,
			PollIntervalSec:      normalized.PollIntervalSec,
			PlanPromptStyle:      normalized.PlanPromptStyle,
			PromptTimeoutSecs:    normalized.PromptTimeoutSecs,
			UpdateProxyEnabled:   normalized.UpdateProxyEnabled,
			UpdateProxyURL:       normalized.UpdateProxyURL,
		},
		GUISettingsDefaults{
			Language: fallbackText(strings.TrimSpace(current.Language), "zh"),
			Theme:    fallbackText(strings.TrimSpace(current.Theme), "system"),
		},
	); err != nil {
		return s.LoadBootstrap(), err
	}
	if logFile, err := InitLogger(); err != nil {
		return s.LoadBootstrap(), err
	} else {
		s.logFile = logFile
	}

	s.setExecutionModeRPC(normalized.ExecutionMode)
	// 沙箱模式走 applySandboxModeSemantics 单一入口：设置里选"完全访问权限"
	// 是复合态（approval=never + danger policy + 收口待审卡），此前单推沙箱轴
	// 会让完全访问态下审批卡继续弹；切回工作区时审批轴同步复位。
	if err := s.applySandboxModeSemantics(s.activeWorkspaceValue(), normalized.SandboxMode); err != nil {
		return s.LoadBootstrap(), err
	}
	if err := s.setReasoningLevelRPC(normalized.ReasoningLevel); err != nil {
		return s.LoadBootstrap(), err
	}
	// 询问等待超时推进内核（内核 Settings 不落盘，每次保存都同步）。
	if err := s.setPromptTimeoutRPC(normalized.PromptTimeoutSecs); err != nil {
		return s.LoadBootstrap(), err
	}

	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	// 托盘显隐即时生效（回调在锁外：可能触碰窗口/托盘句柄）。
	s.notifyStayInTrayChanged(normalized.StayInTray)
	return s.LoadBootstrap(), nil
}

func (svc *SettingsService) SetExecutionMode(mode string) BootstrapState {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}
	}
	mode = normalizeExecutionMode(mode)
	s.setExecutionModeRPC(mode)
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap()
}

func (svc *SettingsService) SetReasoningLevel(level string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		level = "off"
	}
	if err := s.setReasoningLevelRPC(level); err != nil {
		return s.LoadBootstrap(), err
	}

	// wire 档位词汇（off/auto/minimal/low/medium/high/xhigh/max，见内核 protocol）。
	label := map[string]string{
		"off":     "标准",
		"auto":    "自动思考",
		"minimal": "极低推理",
		"low":     "低推理",
		"medium":  "中推理",
		"high":    "高推理",
		"xhigh":   "超高推理",
		"max":     "最高推理",
	}[level]
	if label == "" {
		label = level
	}

	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *SettingsService) ToggleFastMode() (BootstrapState, error) {
	if svc.bridge == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	return svc.bridge.LoadBootstrap(), nil
}

func (svc *SettingsService) SetTheme(name string) BootstrapState {
	if svc.bridge == nil {
		return BootstrapState{}
	}
	return svc.bridge.LoadBootstrap()
}

// normalizePromptTimeoutSecs 归一化询问等待超时（负值/0 = 关闭即一直等待）。
func normalizePromptTimeoutSecs(secs int) int {
	if secs < 0 {
		return 0
	}
	return secs
}

// setPromptTimeoutRPC 把询问等待超时推进内核 Settings（读改写，避免覆盖内核
// 里其他字段）。内核 Settings 不落盘，桌面侧持久化在 workspace 设置文件里，
// 每次保存与桥启动时都同步一次。
func (s *BridgeService) setPromptTimeoutRPC(secs int) error {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return err
	}
	normalized := normalizePromptTimeoutSecs(secs)
	current, err := gateway.CoreGetFullSettingsRPC(coreCtx())
	if err != nil {
		return fmt.Errorf("read kernel settings: %w", err)
	}
	value := int64(normalized)
	if normalized == 0 {
		current.PromptTimeoutSecs = nil
	} else {
		current.PromptTimeoutSecs = &value
	}
	if err := gateway.CoreSaveSettingsRPC(coreCtx(), current); err != nil {
		return fmt.Errorf("save kernel settings: %w", err)
	}
	slog.Info("bridge.prompt_timeout.synced", "secs", normalized)
	return nil
}
