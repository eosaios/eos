//go:build !windows

package clip

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商用使用请联系版权人获得商业授权。

// 非 Windows 平台尚无平台专属剪贴板图片兜底（Windows 见 image_fallback_windows.go）。
// 主路径 golang.design/x/clipboard 跨平台可用；返回显式哨兵而非匿名 errors.New，
// 让调用方能区分「平台不支持」与「执行失败」，不再静默吞错。
func readImageFallback() ([]byte, error) {
	return nil, ErrPlatformClipboardFallbackNotSupported
}
