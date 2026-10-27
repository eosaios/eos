package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func OpenDirectory(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("目录路径不能为空")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}

	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}
