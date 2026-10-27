//go:build !windows

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// 非 Windows：bash 随发行版必装于 /bin/bash，探测恒成功；
// PortableGit 安装器是 Windows 专属能力，返回不支持。

func probeTerminalShell() TerminalShellStatus {
	return TerminalShellStatus{
		Available: true,
		Source:    terminalShellSourcePosix,
		Path:      "/bin/bash",
	}
}
