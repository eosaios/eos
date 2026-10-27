package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "sync/atomic"

// shutdownRequested 标记应用即将真正退出（托盘菜单退出 / 更新安装重启）。
// stay-in-tray 开启时，窗口关闭 hook 会拦截 WindowClosing 并隐藏窗口——
// app.Quit() 内部同样会走窗口关闭链路，不置位本标志会把退出一并拦掉
// （表现为「退出无响应」）。所有主动 Quit 的调用点必须先 RequestShutdown。
var shutdownRequested atomic.Bool

// RequestShutdown 标记即将真正退出，窗口关闭 hook 据此放行。
func RequestShutdown() { shutdownRequested.Store(true) }

// IsShutdownRequested 报告是否已请求真正退出。
func IsShutdownRequested() bool { return shutdownRequested.Load() }
