package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"strings"
)

const windowChromeEnvKey = "EOS_GUI_WINDOW_CHROME"

type windowChromeMode string

const (
	windowChromeModeSystem       windowChromeMode = "system_decorated"
	windowChromeModeExperimental windowChromeMode = "experimental_frameless"
)

func resolveWindowChromeMode(raw string) windowChromeMode {
	if raw := strings.TrimSpace(os.Getenv(windowChromeEnvKey)); raw != "" {
		return normalizeWindowChromeMode(raw)
	}
	if raw = strings.TrimSpace(raw); raw != "" {
		return normalizeWindowChromeMode(raw)
	}
	if mode := normalizeWindowChromeMode(raw); mode != "" {
		return mode
	}
	return windowChromeModeSystem
}

func normalizeWindowChromeMode(raw string) windowChromeMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default", "stable", "system", "system_decorated", "decorated":
		return windowChromeModeSystem
	case "experimental", "frameless", "experimental_frameless", "custom":
		return windowChromeModeExperimental
	default:
		return windowChromeModeSystem
	}
}
