package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"os"
	"os/exec"

	"github.com/eosaios/eos/pkg/executil"
	"path/filepath"
	"runtime"
	"strings"
)

// RevealPath 在系统文件管理器中定位到 path：文件则选中（Windows explorer
// /select,、macOS open -R），目录则打开该目录（Linux 无统一 reveal 标准，
// 文件回退打开父目录）。path 必须已存在——不存在返回错误，不创建。
// 区别于 OpenDirectory 的 MkdirAll 副作用，本函数面向「打开已有文件/目录」。
func RevealPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("路径不能为空")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return openDirectoryNoMkdir(abs)
	}
	switch runtime.GOOS {
	case "windows":
		return executil.StartDetached(exec.Command("explorer.exe", "/select,"+abs))
	case "darwin":
		return executil.StartDetached(exec.Command("open", "-R", abs))
	default:
		// Linux 无跨发行版一致的「定位文件」命令，回退打开父目录。
		return openDirectoryNoMkdir(filepath.Dir(abs))
	}
}

// openDirectoryNoMkdir 是 OpenDirectory 的跨平台分派核心，但不做 MkdirAll。
// 供 RevealPath 复用（定位场景下目录必然已存在，不应创建）。
func openDirectoryNoMkdir(path string) error {
	switch runtime.GOOS {
	case "windows":
		return executil.StartDetached(exec.Command("explorer.exe", path))
	case "darwin":
		return executil.StartDetached(exec.Command("open", path))
	default:
		return executil.StartDetached(exec.Command("xdg-open", path))
	}
}
