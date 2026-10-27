package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PathResolutionResult 路径解析结果
type PathResolutionResult struct {
	AbsPath     string // 绝对路径
	RelPath     string // 相对于工作目录的路径
	ResolvedAbs string // 解析符号链接后的绝对路径
	ErrMsg      string // 错误信息（如果有）
	IsValid     bool   // 是否有效
	IsSymlink   bool   // 是否为符号链接
}

// ResolvePath 解析文件路径，转换为绝对路径并验证安全性
// 确保路径在当前工作目录范围内（防止路径遍历攻击）
//
// 参数:
//
//	path: 要解析的路径
//
// 返回:
//
//	PathResolutionResult: 包含绝对路径、相对路径和验证状态
func ResolvePath(path string) PathResolutionResult {
	return ResolvePathUnder("", path)
}

// ResolvePathUnder 解析文件路径，转换为绝对路径并验证安全性
// 确保路径在指定 rootDir 范围内（防止路径遍历攻击）。
//
// 当 rootDir 为空时，等价于 ResolvePath：以当前工作目录作为 rootDir。
func ResolvePathUnder(rootDir string, path string) PathResolutionResult {
	if strings.TrimSpace(path) == "" {
		return PathResolutionResult{ErrMsg: "path is empty", IsValid: false}
	}

	wd := strings.TrimSpace(rootDir)
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			slog.Error("path.resolve.getwd.error",
				"component", ComponentSystem,
				"error", err,
			)
			return PathResolutionResult{ErrMsg: "failed to get working directory", IsValid: false}
		}
	}
	wdAbs, err := filepath.Abs(wd)
	if err == nil {
		wd = wdAbs
	}
	wd = filepath.Clean(wd)

	// 规范化路径输入
	normalized := normalizePathInput(expandHomePath(path))

	// 转换为绝对路径
	ap := normalized
	if runtime.GOOS == "windows" {
		// 在 Windows 上，如果路径以 / 或 \ 开头但不是绝对路径（没有盘符），
		// 我们将其视为相对于当前驱动器根目录的路径。
		// 但由于我们希望更兼容 POSIX 风格的输入（例如 /home/eos 对应 C:\home\eos），
		// 我们将 / 开头的路径视为相对于当前工作目录所在的驱动器根目录。

		// filepath.IsAbs 在 Windows 上只有带盘符或 UNC 路径才返回 true。
		// /foo 返回 false。

		if !filepath.IsAbs(ap) {
			if strings.HasPrefix(ap, "/") || strings.HasPrefix(ap, "\\") {
				// 获取工作目录所在的驱动器卷标
				vol := filepath.VolumeName(wd)
				if vol == "" {
					// 无法确定卷标，回退到 Join
					ap = filepath.Join(wd, ap)
				} else {
					ap = filepath.Join(vol, ap)
				}
			} else {
				ap = filepath.Join(wd, ap)
			}
		}
	} else {
		if !filepath.IsAbs(ap) {
			ap = filepath.Join(wd, ap)
		}
	}

	// 在 Windows 上规范化路径
	if runtime.GOOS == "windows" {
		ap = normalizeWindowsPath(ap)
	}

	// 验证路径在工作目录范围内
	rel, errRel := filepath.Rel(wd, ap)
	if errRel != nil {
		slog.Error("path.resolve.relative.error",
			"component", ComponentSystem,
			"path", ap,
			"error", errRel,
		)
		return PathResolutionResult{ErrMsg: "failed to resolve relative path", IsValid: false}
	}

	// 检查路径遍历
	if strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, string(filepath.Separator)+"..") {
		slog.Warn("path.resolve.out_of_root",
			"component", ComponentSystem,
			"path", ap,
			"relative", rel,
		)
		return PathResolutionResult{ErrMsg: "path outside working directory", IsValid: false}
	}

	// 检查符号链接并解析
	resolvedAbs, isSymlink := resolveSymlink(ap)

	// 验证解析后的路径仍在工作目录范围内。
	// 工作目录自身也要解析符号链接（macOS 上 /var → /private/var），
	// 否则根内符号链接会被误判为出界。
	if isSymlink {
		wdResolved := wd
		if r, err := filepath.EvalSymlinks(wd); err == nil {
			wdResolved = filepath.Clean(r)
		}
		resolvedRel, errRel2 := filepath.Rel(wdResolved, resolvedAbs)
		if errRel2 == nil && (strings.HasPrefix(resolvedRel, "..") || strings.HasPrefix(resolvedRel, string(filepath.Separator)+"..")) {
			slog.Warn("path.resolve.symlink_out_of_root",
				"component", ComponentSystem,
				"original", ap,
				"resolved", resolvedAbs,
				"relative", resolvedRel,
			)
			return PathResolutionResult{
				AbsPath:     ap,
				ResolvedAbs: resolvedAbs,
				ErrMsg:      "symbolic link points outside working directory",
				IsValid:     false,
				IsSymlink:   true,
			}
		}
	}

	return PathResolutionResult{
		AbsPath:     ap,
		RelPath:     rel,
		ResolvedAbs: resolvedAbs,
		IsValid:     true,
		IsSymlink:   isSymlink,
	}
}

func expandHomePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return path
		}
		if path == "~" {
			return home
		}
		rest := strings.TrimPrefix(path, "~")
		rest = strings.TrimPrefix(rest, "/")
		rest = strings.TrimPrefix(rest, "\\")
		if rest == "" {
			return home
		}
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	return path
}

// normalizePathInput 规范化路径输入
func normalizePathInput(path string) string {
	// 移除首尾空白
	path = strings.TrimSpace(path)
	// 统一路径分隔符为正斜杠（用于处理）
	path = filepath.ToSlash(path)
	// 移除冗余的分隔符
	path = filepath.Clean(path)
	return path
}

// normalizeWindowsPath 规范化 Windows 路径
func normalizeWindowsPath(path string) string {
	// 处理 Windows 驱动器号大小写
	if len(path) >= 2 {
		// 确保驱动器号大写 (C:\ -> C:\)
		if path[1] == ':' && path[0] >= 'a' && path[0] <= 'z' {
			path = string(path[0]-32) + path[1:]
		}
	}
	// 使用 filepath.Clean 规范化路径
	path = filepath.Clean(path)
	return path
}

// resolveSymlink 解析符号链接，返回最终路径和是否为符号链接
func resolveSymlink(path string) (string, bool) {
	// 检查是否为符号链接
	info, err := os.Lstat(path)
	if err != nil {
		return path, false
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return path, false
	}

	// 解析符号链接
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		slog.Warn("path.resolve.symlink.error",
			"component", ComponentSystem,
			"path", path,
			"error", err,
		)
		return path, true
	}

	// 转换为绝对路径
	if !filepath.IsAbs(resolved) {
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return path, true
		}
	}

	slog.Debug("path.resolve.symlink.resolved",
		"component", ComponentSystem,
		"original", path,
		"resolved", resolved,
	)

	return resolved, true
}

// ResolvePathSimple 简化版路径解析，只返回绝对路径
// 如果解析失败，返回空字符串
func ResolvePathSimple(path string) string {
	result := ResolvePath(path)
	if !result.IsValid {
		return ""
	}
	return result.AbsPath
}

// MustResolvePath 解析路径，如果失败则触发 panic
// 仅用于初始化阶段已知路径有效的场景
func MustResolvePath(path string) string {
	result := ResolvePath(path)
	if !result.IsValid {
		panic("path resolution failed: " + result.ErrMsg)
	}
	return result.AbsPath
}

// IsPathInRoot 检查路径是否在指定根目录下
func IsPathInRoot(path, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

// JoinPath 安全地连接路径片段
func JoinPath(base, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(base, filepath.FromSlash(rel))
}

// NormalizePath 规范化路径（统一使用正斜杠，去除冗余分隔符）
func NormalizePath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
