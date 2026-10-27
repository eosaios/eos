package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import ()

// GetBuildInfo 返回构建元数据（版本/commit/构建时间/平台），设置页「关于」分区展示。
func (s *BridgeService) GetBuildInfo() BuildMetadata {
	return CurrentBuildMetadata()
}
