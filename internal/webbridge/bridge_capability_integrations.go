package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"errors"
	"log/slog"
	"strings"
)

func (svc *CapabilityService) UpsertMCP(name, kind, target string, enabled bool) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.upsertMCPRPC(name, kind, target, enabled); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) ImportMCPJSON(raw string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.importMCPJSONRPC(raw); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) DeleteMCP(name string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.deleteMCPRPC(name); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) SetMCPEnabled(name string, enabled bool) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.setMCPEnabledRPC(name, enabled); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) DetectLSP(language string) BootstrapState {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}
	}
	language = strings.TrimSpace(language)
	if _, err := s.detectLSPRPC(language); err != nil {
		slog.Warn("bridge.detect_lsp_failed", "language", language, "error", err)
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap()
}

func (svc *CapabilityService) StartLSP(language string) BootstrapState {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}
	}
	language = strings.TrimSpace(language)
	if _, err := s.startLSPRPC(language); err != nil {
		slog.Warn("bridge.start_lsp_failed", "language", language, "error", err)
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap()
}

func (svc *CapabilityService) InstallLSP(language string) BootstrapState {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}
	}
	language = strings.TrimSpace(language)
	if _, err := s.installLSPRPC(language); err != nil {
		slog.Warn("bridge.install_lsp_failed", "language", language, "error", err)
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap()
}

func (svc *CapabilityService) ReloadSkills() (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.reloadSkillsRPC(); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) ReloadSkillsSilent() (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.reloadSkillsRPC(); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) SetSkillEnabled(name string, enabled bool) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.setSkillEnabledRPC(name, enabled); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) SetPluginEnabled(name string, enabled bool) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.setPluginEnabledRPC(name, enabled); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}
