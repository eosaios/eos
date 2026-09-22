package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (svc *CapabilityService) LoadRules(workspaces []WorkspaceCard) RulesState {
	state := RulesState{}
	if path, err := globalRulesPath("zh"); err == nil {
		state.Global = readRuleDocument("global-rules", "global", "Global Rules", path, "", "", RuleDocumentScopeAll)
	} else {
		state.Global = readRuleDocument("global-rules", "global", "Global Rules", "", "", "", RuleDocumentScopeAll)
	}
	state.Workspaces = make([]RuleDocument, 0, len(workspaces))
	for _, workspace := range workspaces {
		path := workspaceRulesPath(workspace.Path)
		id := "workspace-rules:" + strings.ToLower(filepath.Clean(workspace.Path))
		workspaceRuleScope := RuleDocumentScopeAll
		if workspace.Active {
			workspaceRuleScope = RuleDocumentScopeActiveOnly
		}
		state.Workspaces = append(state.Workspaces, readRuleDocument(
			id,
			"workspace",
			fallbackText(strings.TrimSpace(workspace.Name), filepath.Base(strings.TrimSpace(workspace.Path))),
			path,
			workspace.Path,
			workspace.Name,
			workspaceRuleScope,
		))
	}
	return state
}

func (svc *CapabilityService) ActiveRulesContent(state RulesState, activeWorkspace string) string {
	activeWorkspace = strings.TrimSpace(activeWorkspace)
	for _, doc := range state.Workspaces {
		if doc.Active || (activeWorkspace != "" && sameWorkspacePath(doc.WorkspacePath, activeWorkspace)) {
			return strings.TrimSpace(doc.Content)
		}
	}
	return strings.TrimSpace(state.Global.Content)
}

func (svc *CapabilityService) SaveRules(req RulesSaveRequest) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	path, _, _, err := resolveRuleWriteTarget(req, s.guiLanguage())
	if err != nil {
		return s.LoadBootstrap(), err
	}
	content := strings.TrimSpace(req.Value)
	if content == "" {
		content = defaultRulesTemplate()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return s.LoadBootstrap(), err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) ResetRules(req RulesResetRequest) (BootstrapState, error) {
	return svc.SaveRules(RulesSaveRequest{
		Scope:         req.Scope,
		WorkspacePath: req.WorkspacePath,
		Value:         defaultRulesTemplate(),
	})
}

func (svc *CapabilityService) RollbackVersion(id string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.rollbackVersionRPC(id); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) DeleteVersion(id string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.deleteVersionRPC(id); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) ClearVersions() BootstrapState {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}
	}
	s.clearVersionsRPC()
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap()
}
