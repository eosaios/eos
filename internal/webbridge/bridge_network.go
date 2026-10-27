package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"github.com/eosaios/eos/pkg/coreapi"
)

// NetworkInspect 返回模型 API 流量记录快照（network/list）。
// limit<=0 取全部；未设置 EOS_NETWORK_INSPECT 时 enabled=false，
// 前端网络面板据此展示开启引导。记录已脱敏（敏感头掩码）与截断。
func (s *BridgeService) NetworkInspect(limit int) (coreapi.NetworkListResult, error) {
	return coreValueOrRequire(s, func(g bridgeRuntimeGateway) (coreapi.NetworkListResult, error) {
		return g.CoreNetworkListRPC(coreCtx(), limit)
	})
}

// NetworkClear 清空流量记录，返回清除的条数。
func (s *BridgeService) NetworkClear() (int, error) {
	return coreValueOrRequire(s, func(g bridgeRuntimeGateway) (int, error) {
		return g.CoreNetworkClearRPC(coreCtx())
	})
}
