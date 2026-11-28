package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"fmt"
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

// SetSandboxModeForUI submits one scoped change. Core persists the complete preset.
func (s *BridgeService) SetSandboxModeForUI(mode string) (BootstrapState, error) {
	s.stateMu.RLock()
	sessionID, workspace := s.currentSessionID, s.activeWorkspace
	if session := s.sessions[sessionID]; session != nil && session.WorkspacePath != "" {
		workspace = session.WorkspacePath
	}
	s.stateMu.RUnlock()
	if err := s.applySandboxModeForSession(workspace, mode, sessionID); err != nil {
		return s.LoadBootstrap(), fmt.Errorf("apply permission preset: %w", err)
	}
	return s.LoadBootstrap(), nil
}
func (s *BridgeService) EnterFullAccessForUI() (BootstrapState, error) {
	return s.SetSandboxModeForUI("danger-full-access")
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
