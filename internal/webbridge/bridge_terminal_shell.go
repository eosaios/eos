package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "sync"

// 终端 bash 解析：三层探测（程序指定目录 → 系统公共安装 → 缺失走按需安装）。
// 平台特定探测见 bridge_terminal_shell_windows.go / _other.go，
// PortableGit 下载安装器见 bridge_terminal_shell_install.go。

// 终端 shell 状态事件名：安装过程进度（stage/percent/received/total/message）。
const terminalShellEventName = "eos:bridge:terminal-shell"

// TerminalShellStatus 描述终端面板可用的 bash 状态，随 TerminalState 下发。
type TerminalShellStatus struct {
	Available  bool   `json:"available"`
	Source     string `json:"source"`
	Installing bool   `json:"installing"`
	Path       string `json:"path,omitempty"`
}

// terminalShellSource 枚举（写入 status.Source，前端按来源展示）。
const (
	terminalShellSourceAppManaged = "app-managed"
	terminalShellSourcePath       = "path"
	terminalShellSourceGitDerived = "git-derived"
	terminalShellSourceRegistry   = "registry"
	terminalShellSourceWellKnown  = "well-known"
	terminalShellSourcePosix      = "posix"
)

// terminalShellState 缓存探测结果与安装进行态，避免每次开终端重复探测。
type terminalShellState struct {
	mu         sync.Mutex
	probe      TerminalShellStatus
	resolved   bool
	installing bool
}

func (s *BridgeService) GetTerminalShellStatus() TerminalShellStatus {
	return s.terminalShellSnapshot()
}

// terminalShellSnapshot 返回探测缓存；未探测过则立即探测一次并缓存。
func (s *BridgeService) terminalShellSnapshot() TerminalShellStatus {
	if s == nil {
		return TerminalShellStatus{}
	}
	s.terminalShell.mu.Lock()
	defer s.terminalShell.mu.Unlock()
	if !s.terminalShell.resolved {
		status := probeTerminalShell()
		status.Installing = s.terminalShell.installing
		s.terminalShell.probe = status
		s.terminalShell.resolved = true
		return status
	}
	status := s.terminalShell.probe
	status.Installing = s.terminalShell.installing
	return status
}

// terminalShellInvalidate 探测结果失效（安装完成后重探）。
func (s *BridgeService) terminalShellInvalidate() {
	s.terminalShell.mu.Lock()
	defer s.terminalShell.mu.Unlock()
	s.terminalShell.resolved = false
}
