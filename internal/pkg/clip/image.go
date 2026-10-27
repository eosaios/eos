package clip

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.design/x/clipboard"
)

// ErrPlatformClipboardFallbackNotSupported 表示当前平台没有平台专属的剪贴板图片兜底
// 实现（目前仅 Windows 提供 Win32 DIB/PNG 直读）。主路径 golang.design/x/clipboard
// 仍然可用；此哨兵让上层能把「平台兜底缺口」与「真正的兜底执行失败」区分开，不再静默吞掉。
var ErrPlatformClipboardFallbackNotSupported = errors.New("platform clipboard image fallback not supported")

var initOnce sync.Once
var initErr error

func initClipboard() error {
	initOnce.Do(func() {
		initErr = clipboard.Init()
	})
	return initErr
}

func ReadImage() ([]byte, error) {
	if err := initClipboard(); err != nil {
		return nil, err
	}
	for i := 0; i < 3; i++ {
		b := clipboard.Read(clipboard.FmtImage)
		if len(b) > 0 {
			return b, nil
		}
		// 平台专属兜底（Windows 走 Win32 DIB/PNG 直读）。非 Windows 平台返回
		// ErrPlatformClipboardFallbackNotSupported，这是已知的平台支持矩阵缺口，
		// 不是被吞掉的错误——循环继续依赖 golang.design/x/clipboard 主路径重试。
		if b2, err := readImageFallback(); err == nil && len(b2) > 0 {
			return b2, nil
		} else if err != nil && !errors.Is(err, ErrPlatformClipboardFallbackNotSupported) {
			return nil, fmt.Errorf("clipboard image fallback failed: %w", err)
		}
		time.Sleep(60 * time.Millisecond)
	}
	return nil, errors.New("empty clipboard image")
}
