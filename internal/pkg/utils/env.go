package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EnvInfo 包含当前运行环境的信息
type EnvInfo struct {
	OS        string
	Shell     string
	CWD       string
	IsGitRepo bool
	OSVersion string
}

// GetEnvInfo 获取当前环境信息
func GetEnvInfo() EnvInfo {
	cwd := GetCWD()
	return EnvInfo{
		OS:        GetOS(),
		Shell:     GetShell(),
		CWD:       cwd,
		IsGitRepo: IsGitRepo(cwd),
		OSVersion: GetOSVersion(),
	}
}

// GetOS 获取当前操作系统名称
func GetOS() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

// GetShell 获取当前使用的 Shell 类型
func GetShell() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "sh"
	}
	return filepath.Base(shell)
}

// GetCWD 获取当前工作目录
func GetCWD() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return cwd
}

// IsGitRepo 检查目录是否包含 .git（即是否为 Git 仓库）
func IsGitRepo(cwd string) bool {
	_, err := os.Stat(filepath.Join(cwd, ".git"))
	return err == nil
}

// GetOSVersion 获取系统版本信息
func GetOSVersion() string {
	if runtime.GOOS == "windows" {
		out, err := Command("cmd", "/c", "ver").Output()
		if err != nil {
			return ""
		}
		v := strings.TrimSpace(string(out))
		// ver 输出如 "Microsoft Windows [Version 10.0.26100.0000]"
		return strings.TrimPrefix(v, "Microsoft Windows ")
	}
	// Linux / macOS
	out, err := Command("uname", "-sr").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FormatEnvInfo 将环境信息格式化为字符串，用于 Prompt
func FormatEnvInfo(info EnvInfo) string {
	var sb strings.Builder
	sb.WriteString("**运行环境**：\n")
	sb.WriteString("- 操作系统: " + info.OS + "\n")
	sb.WriteString("- Shell 类型: " + info.Shell + "\n")
	sb.WriteString("- 工作目录: " + info.CWD + "\n")
	gitStatus := "否"
	if info.IsGitRepo {
		gitStatus = "是"
	}
	sb.WriteString("- Git 仓库: " + gitStatus + "\n")
	if info.OSVersion != "" {
		sb.WriteString("- 系统版本: " + info.OSVersion + "\n")
	}
	return sb.String()
}

// GenerateProjectStructureFile 生成项目目录结构文件
func GenerateProjectStructureFile(outputPath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("Project Directory Structure:\n")
	sb.WriteString(cwd + "\n")

	// 忽略列表
	ignoreDirs := map[string]bool{
		".git": true, ".idea": true, ".vscode": true,
		"node_modules": true, "dist": true, "build": true,
		"vendor": true, "__pycache__": true, ".DS_Store": true,
		"bin": true, "obj": true, "target": true,
	}

	var walk func(path string, prefix string, depth int)
	walk = func(path string, prefix string, depth int) {
		if depth > 5 { // 限制深度
			return
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return
		}

		filtered := make([]os.DirEntry, 0, len(entries))
		for _, e := range entries {
			if ignoreDirs[e.Name()] {
				continue
			}
			filtered = append(filtered, e)
		}

		count := len(filtered)
		for i, e := range filtered {
			isLast := i == count-1
			connector := "├── "
			if isLast {
				connector = "└── "
			}

			sb.WriteString(prefix + connector + e.Name())
			if e.IsDir() {
				sb.WriteString("/")
				sb.WriteString("\n")

				newPrefix := prefix + "│   "
				if isLast {
					newPrefix = prefix + "    "
				}
				walk(filepath.Join(path, e.Name()), newPrefix, depth+1)
			} else {
				sb.WriteString("\n")
			}
		}
	}

	walk(cwd, "", 0)

	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}
