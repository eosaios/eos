package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

// 领域 service 声明：每个 service 持有 bridge 反向引用，由 BridgeService
// 懒初始化访问器（xxxService()）按需构造。业务方法散落在各 domain 文件
// （如 bridge_capability_integrations.go、bridge_chat_send.go 等），本文件
// 只集中 struct + 构造器 + 访问器样板，避免每个 service 单开一个文件。

// ── CapabilityService ──

type CapabilityService struct {
	bridge *BridgeService
}

func NewCapabilityService(bridge *BridgeService) *CapabilityService {
	return &CapabilityService{bridge: bridge}
}

func (s *BridgeService) capabilityService() *CapabilityService {
	if s == nil {
		return NewCapabilityService(nil)
	}
	s.serviceMu.Lock()
	defer s.serviceMu.Unlock()
	if s.capabilitySvc == nil {
		s.capabilitySvc = NewCapabilityService(s)
	}
	return s.capabilitySvc
}

// ── ChatService ──

type ChatService struct {
	bridge *BridgeService
}

func NewChatService(bridge *BridgeService) *ChatService {
	return &ChatService{bridge: bridge}
}

func (s *BridgeService) chatService() *ChatService {
	if s == nil {
		return NewChatService(nil)
	}
	s.serviceMu.Lock()
	defer s.serviceMu.Unlock()
	if s.chatSvc == nil {
		s.chatSvc = NewChatService(s)
	}
	return s.chatSvc
}

// ── SystemService ──

type SystemService struct {
	bridge *BridgeService
}

func NewSystemService(bridge *BridgeService) *SystemService {
	return &SystemService{bridge: bridge}
}

func (s *BridgeService) systemService() *SystemService {
	if s == nil {
		return NewSystemService(nil)
	}
	s.serviceMu.Lock()
	defer s.serviceMu.Unlock()
	if s.systemSvc == nil {
		s.systemSvc = NewSystemService(s)
	}
	return s.systemSvc
}

// ── AutomationService ──
// 封装自动化模板的 CRUD 与持久化（不含调度器生命周期）。

type AutomationService struct {
	bridge *BridgeService
}

func NewAutomationService(bridge *BridgeService) *AutomationService {
	return &AutomationService{bridge: bridge}
}

func (s *BridgeService) automationService() *AutomationService {
	if s == nil {
		return NewAutomationService(nil)
	}
	s.serviceMu.Lock()
	defer s.serviceMu.Unlock()
	if s.automationSvc == nil {
		s.automationSvc = NewAutomationService(s)
	}
	return s.automationSvc
}
