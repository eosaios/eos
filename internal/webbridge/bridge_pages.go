package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"fmt"
	"log/slog"
	"strings"
)

func (s *BridgeService) RunBashCommand(input string) (BootstrapState, error) {
	return s.commandService().RunBashCommand(input)
}

func (s *BridgeService) TrustWorkspace(path string) (BootstrapState, error) {
	return s.workspaceService().TrustWorkspace(path)
}

func (s *BridgeService) RemoveWorkspace(path string) (BootstrapState, error) {
	return s.workspaceService().RemoveWorkspace(path)
}

func (s *BridgeService) CreateWorktree(name string) (BootstrapState, error) {
	return s.workspaceService().CreateWorktree(name)
}

func (s *BridgeService) RemoveWorktree(path string, force bool) (BootstrapState, error) {
	return s.workspaceService().RemoveWorktree(path, force)
}

func (s *BridgeService) UpsertModel(name, base, keyMasked, model string) (BootstrapState, error) {
	return s.capabilityService().UpsertModel(name, base, keyMasked, model)
}

func (s *BridgeService) SaveModel(req ModelSaveRequest) (BootstrapState, error) {
	return s.capabilityService().SaveModel(req)
}

func (s *BridgeService) VerifyModel(req ModelSaveRequest) (ModelVerifyResult, error) {
	return s.capabilityService().VerifyModel(req)
}

func (s *BridgeService) ActivateModel(name string) (BootstrapState, error) {
	return s.capabilityService().ActivateModel(name)
}

func (s *BridgeService) SelectCurrentModel(name string) (BootstrapState, error) {
	return s.capabilityService().SelectCurrentModel(name)
}

func (s *BridgeService) DeleteModel(name string) (BootstrapState, error) {
	return s.capabilityService().DeleteModel(name)
}

func (s *BridgeService) UpsertMCP(name, kind, target string, enabled bool) (BootstrapState, error) {
	return s.capabilityService().UpsertMCP(name, kind, target, enabled)
}

func (s *BridgeService) ImportMCPJSON(raw string) (BootstrapState, error) {
	return s.capabilityService().ImportMCPJSON(raw)
}

func (s *BridgeService) DeleteMCP(name string) (BootstrapState, error) {
	return s.capabilityService().DeleteMCP(name)
}

func (s *BridgeService) SetMCPEnabled(name string, enabled bool) (BootstrapState, error) {
	return s.capabilityService().SetMCPEnabled(name, enabled)
}

func (s *BridgeService) DetectLSP(language string) BootstrapState {
	return s.capabilityService().DetectLSP(language)
}

func (s *BridgeService) StartLSP(language string) BootstrapState {
	return s.capabilityService().StartLSP(language)
}

func (s *BridgeService) InstallLSP(language string) BootstrapState {
	return s.capabilityService().InstallLSP(language)
}

func (s *BridgeService) ReloadSkills() (BootstrapState, error) {
	return s.capabilityService().ReloadSkills()
}

func (s *BridgeService) ReloadSkillsSilent() (BootstrapState, error) {
	return s.capabilityService().ReloadSkillsSilent()
}

func (s *BridgeService) SetSkillEnabled(name string, enabled bool) (BootstrapState, error) {
	return s.capabilityService().SetSkillEnabled(name, enabled)
}

func (s *BridgeService) SetPluginEnabled(name string, enabled bool) (BootstrapState, error) {
	return s.capabilityService().SetPluginEnabled(name, enabled)
}

func (s *BridgeService) SaveRules(req RulesSaveRequest) (BootstrapState, error) {
	return s.capabilityService().SaveRules(req)
}

func (s *BridgeService) ResetRules(req RulesResetRequest) (BootstrapState, error) {
	return s.capabilityService().ResetRules(req)
}

func (s *BridgeService) SaveSettings(req SettingsSaveRequest) (BootstrapState, error) {
	return s.settingsService().SaveSettings(req)
}

func (s *BridgeService) SetExecutionMode(mode string) BootstrapState {
	return s.settingsService().SetExecutionMode(mode)
}

// SetApprovalModeForUI 是前端"本次会话全部允许"开关的 Wails 入口。
// 委托给 commandService（approval_mode 裁决属命令域，不持久化到设置）。
func (s *BridgeService) SetApprovalModeForUI(mode string) BootstrapState {
	return s.commandService().SetApprovalModeForUI(mode)
}

// SetSandboxModeForUI 是前端 composer 沙箱下拉（只读/工作区/完全访问）的 Wails 入口。
// 走 applySandboxModeSemantics 单一入口：完全访问是复合态（approval=never +
// danger policy + 收口待审卡），切回其余档位时审批轴同步复位——双轴始终一致。
// 同时把选择写入当前会话 metadata（session/set_meta），下次启动按会话恢复——
// 每个会话独立持久化，新会话默认 workspace-write（AGENTS.md §3：归一化在壳层，单一
// 真相源在内核）。
func (s *BridgeService) SetSandboxModeForUI(mode string) (BootstrapState, error) {
	normalized := NormalizeSandboxMode(mode)
	if err := s.applySandboxModeSemantics(s.activeWorkspaceValue(), normalized); err != nil {
		return s.LoadBootstrap(), fmt.Errorf("apply sandbox mode: %w", err)
	}
	s.persistSessionSandboxMode(s.currentSessionValue(), normalized)
	return s.LoadBootstrap(), nil
}

// EnterFullAccessForUI 是前端“完全访问”危险确认后的 Wails 入口。
// 语义不是“只切沙箱下拉”，而是进入真正的无审批完全访问：
// approval=never + sandbox=danger-full-access，并立即收起本地等待中的审批卡片。
// 沙箱模式按会话持久化为 danger-full-access（approval 不持久化，维持 per-session 运行时语义）。
func (s *BridgeService) EnterFullAccessForUI() (BootstrapState, error) {
	if err := s.applySandboxModeSemantics(s.activeWorkspaceValue(), "danger-full-access"); err != nil {
		return s.LoadBootstrap(), fmt.Errorf("enter full access: %w", err)
	}
	s.persistSessionSandboxMode(s.currentSessionValue(), "danger-full-access")
	return s.LoadBootstrap(), nil
}

// persistSessionSandboxMode 把沙箱模式写入指定会话的 metadata（session/set_meta）。
// sessionID 为空时跳过——会话尚未建立时是正常态，仅运行时生效，不持久化。
func (s *BridgeService) persistSessionSandboxMode(sessionID, mode string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return
	}
	if err := gateway.CoreSetSessionSandboxModeRPC(coreCtx(), sessionID, mode); err != nil {
		slog.Warn("bridge.session.persist_sandbox_mode.failed", "session", sessionID, "mode", mode, "error", err)
	}
}

func (s *BridgeService) SetReasoningLevel(level string) (BootstrapState, error) {
	return s.settingsService().SetReasoningLevel(level)
}

func (s *BridgeService) RollbackVersion(id string) (BootstrapState, error) {
	return s.capabilityService().RollbackVersion(id)
}

func (s *BridgeService) DeleteVersion(id string) (BootstrapState, error) {
	return s.capabilityService().DeleteVersion(id)
}

func (s *BridgeService) ClearVersions() BootstrapState {
	return s.capabilityService().ClearVersions()
}
