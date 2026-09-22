package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func (svc *CapabilityService) UpsertModel(name, base, keyMasked, model string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.upsertModelRPC(name, base, keyMasked, model); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) SaveModel(req ModelSaveRequest) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	name := strings.TrimSpace(req.Name)
	if err := s.saveModelRPC(adapter.ModelSaveRequest{
		OriginalName:            strings.TrimSpace(req.OriginalName),
		Mode:                    strings.TrimSpace(req.Mode),
		ProviderID:              strings.TrimSpace(req.ProviderID),
		PresetID:                strings.TrimSpace(req.PresetID),
		Name:                    name,
		APIKey:                  req.APIKey,
		APIBase:                 strings.TrimSpace(req.APIBase),
		Model:                   strings.TrimSpace(req.Model),
		SupportsReasoningEffort: req.SupportsReasoningEffort,
		SupportsVision:          req.SupportsVision,
		SupportsTools:           req.SupportsTools,
	}); err != nil {
		return s.LoadBootstrap(), err
	}
	// 新增模型默认切换为当前模型：内核已将其置为全局 active，这里同步
	// session/workspace 级默认，让当前对话与后续新对话立即用上新模型。
	if strings.TrimSpace(req.OriginalName) == "" && name != "" {
		s.stateMu.RLock()
		activeWorkspace := strings.TrimSpace(s.activeWorkspace)
		currentSessionID := strings.TrimSpace(s.currentSessionID)
		if activeWorkspace == "" && currentSessionID != "" {
			// activeWorkspace 尚未恢复时，用当前会话记录的工作区兜底。
			if session, ok := s.sessions[currentSessionID]; ok {
				activeWorkspace = strings.TrimSpace(session.WorkspacePath)
			}
		}
		s.stateMu.RUnlock()
		if _, err := s.selectCurrentModelRPC(activeWorkspace, currentSessionID, name); err != nil {
			slog.Warn("bridge.save_model.select_current_failed", "model", name, "error", err)
		}
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

// VerifyModel 对「将要保存」的模型配置做连通测试（model/verify），只读：
// 不落盘、不推送通知、不刷新 bootstrap。ok=false 时返回测试结果本身而非错误，
// 让前端把失败原因渲染在向导里（配置不合法才是 RPC 错误）。
func (svc *CapabilityService) VerifyModel(req ModelSaveRequest) (ModelVerifyResult, error) {
	s := svc.bridge
	if s == nil {
		return ModelVerifyResult{}, errors.New("bridge service is not available")
	}
	response, err := s.verifyModelRPC(adapter.ModelSaveRequest{
		OriginalName:            strings.TrimSpace(req.OriginalName),
		Mode:                    strings.TrimSpace(req.Mode),
		ProviderID:              strings.TrimSpace(req.ProviderID),
		PresetID:                strings.TrimSpace(req.PresetID),
		Name:                    strings.TrimSpace(req.Name),
		APIKey:                  req.APIKey,
		APIBase:                 strings.TrimSpace(req.APIBase),
		Model:                   strings.TrimSpace(req.Model),
		SupportsReasoningEffort: req.SupportsReasoningEffort,
		SupportsVision:          req.SupportsVision,
		SupportsTools:           req.SupportsTools,
	})
	if err != nil {
		return ModelVerifyResult{}, err
	}
	return ModelVerifyResult{
		OK:        response.Ok,
		LatencyMS: int64(response.LatencyMs),
		Message:   response.Message,
	}, nil
}

func (svc *CapabilityService) ActivateModel(name string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.activateModelRPC(name); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) SelectCurrentModel(name string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	s.stateMu.RLock()
	activeWorkspace := strings.TrimSpace(s.activeWorkspace)
	currentSessionID := strings.TrimSpace(s.currentSessionID)
	if activeWorkspace == "" && currentSessionID != "" {
		// activeWorkspace 尚未恢复时，用当前会话记录的工作区兜底，
		// 保证会话级切换仍能把「最近选择」写入 workspace 默认模型。
		if session, ok := s.sessions[currentSessionID]; ok {
			activeWorkspace = strings.TrimSpace(session.WorkspacePath)
		}
	}
	s.stateMu.RUnlock()
	if _, err := s.selectCurrentModelRPC(activeWorkspace, currentSessionID, name); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (svc *CapabilityService) DeleteModel(name string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.deleteModelRPC(name); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}
