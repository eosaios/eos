//go:build !windows && !darwin

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// launchUpdateInstaller Linux 无统一安装器约定：打开下载目录，
// tar.gz 由用户自行解压。macOS 的原地替换安装见 update_install_darwin.go。
func launchUpdateInstaller(path string) error {
	if err := exec.Command("xdg-open", filepath.Dir(path)).Start(); err != nil {
		return fmt.Errorf("打开下载目录失败: %w", err)
	}
	return nil
}
