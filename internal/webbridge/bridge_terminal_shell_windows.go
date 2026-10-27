//go:build windows

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Windows bash 三层探测。背景：Git for Windows 安装器默认只把 Git\cmd
// （git.exe）加进系统 PATH，bash.exe 所在的 bin\ 从不进 PATH，因此
// GUI 进程的 LookPath("bash") 对装了 Git 的机器也必然失败。

// probeTerminalShell 按优先级探测 bash.exe：
//  1. 程序指定目录（PortableGit 自动安装落点 %LOCALAPPDATA%\eos\shell\git）
//  2. PATH bash
//  3. PATH git.exe → 同根 Git\bin\bash.exe
//  4. 注册表 GitForWindows InstallPath
//  5. 固定常见位置（Program Files 等）
func probeTerminalShell() TerminalShellStatus {
	for _, candidate := range terminalBashProbeCandidates() {
		if probe, ok := candidate.resolve(); ok {
			return probe
		}
	}
	return TerminalShellStatus{Available: false}
}

type terminalBashCandidate struct {
	path   string
	source string
}

func (c terminalBashCandidate) resolve() (TerminalShellStatus, bool) {
	if c.path == "" {
		return TerminalShellStatus{}, false
	}
	info, err := os.Stat(c.path)
	if err != nil || info.IsDir() {
		return TerminalShellStatus{}, false
	}
	return TerminalShellStatus{Available: true, Source: c.source, Path: c.path}, true
}

// appManagedBashPath 返回自动安装的 bash 落点：
// %LOCALAPPDATA%\eos\shell\git\bin\bash.exe（与内核 %LOCALAPPDATA%\eos\core\ 同体系）。
func appManagedBashPath() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if strings.TrimSpace(localAppData) == "" {
		return ""
	}
	return filepath.Join(localAppData, "eos", "shell", "git", "bin", "bash.exe")
}

// terminalBashProbeCandidates 生成按优先级排列的探测候选。
func terminalBashProbeCandidates() []terminalBashCandidate {
	candidates := []terminalBashCandidate{
		{path: appManagedBashPath(), source: terminalShellSourceAppManaged},
	}
	if bashPath, err := exec.LookPath("bash.exe"); err == nil {
		candidates = append(candidates, terminalBashCandidate{path: bashPath, source: terminalShellSourcePath})
	}
	if gitPath, err := exec.LookPath("git.exe"); err == nil {
		if derived := bashBesideGit(gitPath); derived != "" {
			candidates = append(candidates, terminalBashCandidate{path: derived, source: terminalShellSourceGitDerived})
		}
	}
	if installPath := gitForWindowsInstallPath(); installPath != "" {
		candidates = append(candidates, terminalBashCandidate{
			path:   filepath.Join(installPath, "bin", "bash.exe"),
			source: terminalShellSourceRegistry,
		})
	}
	for _, dir := range wellKnownGitDirs() {
		candidates = append(candidates, terminalBashCandidate{
			path:   filepath.Join(dir, "bin", "bash.exe"),
			source: terminalShellSourceWellKnown,
		})
	}
	return candidates
}

// bashBesideGit 从 git.exe 路径推导同发行版的 bash：
// ...\Git\cmd\git.exe → ...\Git\bin\bash.exe。非标准布局返回空。
func bashBesideGit(gitExe string) string {
	cmdDir := filepath.Dir(filepath.Clean(gitExe))
	if filepath.Base(cmdDir) != "cmd" {
		return ""
	}
	return filepath.Join(filepath.Dir(cmdDir), "bin", "bash.exe")
}

// wellKnownGitDirs 固定常见安装目录（含用户级安装位置）。
func wellKnownGitDirs() []string {
	dirs := []string{
		`C:\Program Files\Git`,
		`C:\Program Files (x86)\Git`,
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); strings.TrimSpace(localAppData) != "" {
		dirs = append(dirs, filepath.Join(localAppData, "Programs", "Git"))
	}
	return dirs
}

// gitForWindowsInstallPath 读注册表 HKLM\SOFTWARE\GitForWindows\InstallPath
// （Git 安装器必写；64/32 位视图都查）。
func gitForWindowsInstallPath() string {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.READ, registry.WOW64_32KEY} {
			key, err := registry.OpenKey(root, `SOFTWARE\GitForWindows`, view)
			if err != nil {
				continue
			}
			value, _, err := key.GetStringValue("InstallPath")
			key.Close()
			if err == nil && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}
