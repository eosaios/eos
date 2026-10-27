package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestResolveWindowChromeModePrefersEnvOverride(t *testing.T) {
	t.Setenv(windowChromeEnvKey, "frameless")
	if got := resolveWindowChromeMode(string(windowChromeModeSystem)); got != windowChromeModeExperimental {
		t.Fatalf("resolveWindowChromeMode()=%q, want %q", got, windowChromeModeExperimental)
	}
}

func TestNormalizeWindowChromeModeFallsBackToSystem(t *testing.T) {
	cases := map[string]windowChromeMode{
		"":                       windowChromeModeSystem,
		"stable":                 windowChromeModeSystem,
		"system_decorated":       windowChromeModeSystem,
		"experimental":           windowChromeModeExperimental,
		"experimental_frameless": windowChromeModeExperimental,
		"frameless":              windowChromeModeExperimental,
		"unexpected-value":       windowChromeModeSystem,
	}
	for raw, want := range cases {
		if got := normalizeWindowChromeMode(raw); got != want {
			t.Fatalf("normalizeWindowChromeMode(%q)=%q, want %q", raw, got, want)
		}
	}
}
