package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"log/slog"

	"github.com/eosaios/eos/internal/config"
)

// saveWorkspaceConfig 持久化工作区记忆。保存失败（磁盘满/权限）不阻断
// 启动，但必须留痕——静默丢失用户的工作区记忆无从排查。
func saveWorkspaceConfig(cfg config.Config, path string) {
	if err := config.Save(cfg, path); err != nil {
		slog.Warn("ui.workspace_state.save.error", "path", path, "error", err)
	}
}

func rememberKnownWorkspace(path string, foreground bool) {
	cfg, cfgPath := config.Load()
	if !config.RememberWorkspace(&cfg, path, foreground) {
		return
	}
	saveWorkspaceConfig(cfg, cfgPath)
}

func forgetKnownWorkspace(path string) {
	cfg, cfgPath := config.Load()
	if !config.ForgetWorkspace(&cfg, path) {
		return
	}
	saveWorkspaceConfig(cfg, cfgPath)
}
