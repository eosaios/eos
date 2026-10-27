package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import "strings"

const (
	runtimeGatewayUnavailableTitle  = "核心初始化失败"
	runtimeGatewayUnavailableDetail = "核心服务未启动，模型目录不可用。"
	runtimeGatewayReadyTitle        = "核心就绪状态"
	runtimeGatewayReadyDetail       = "核心已就绪。"
)

func (s *BridgeService) notifyRuntimeGatewayFallback() {
	if s == nil {
		return
	}
	if strings.TrimSpace(s.runtimeGatewayStartError) == "" {
		return
	}
	// The raw startup error is already recorded in configureRuntimeGateway via
	// slog.Warn. The UI only needs a product-facing summary.
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.pushNotificationLocked(runtimeGatewayUnavailableTitle, runtimeGatewayUnavailableDetail, "warning")
}

func (s *BridgeService) runtimeGatewayResourceCheck() ResourceCheck {
	if s != nil && strings.TrimSpace(s.runtimeGatewayStartError) != "" {
		return ResourceCheck{
			Name:   runtimeGatewayUnavailableTitle,
			Status: "error",
			Detail: runtimeGatewayUnavailableDetail,
		}
	}
	if s != nil && s.runtimeGateway != nil {
		return ResourceCheck{
			Name:   runtimeGatewayReadyTitle,
			Status: "ready",
			Detail: runtimeGatewayReadyDetail,
		}
	}
	return ResourceCheck{
		Name:   runtimeGatewayUnavailableTitle,
		Status: "warning",
		Detail: runtimeGatewayUnavailableDetail,
	}
}
