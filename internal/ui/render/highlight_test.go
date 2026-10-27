package render

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"
)

func TestNormalizeChromaTheme(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"", DefaultChromaTheme},
		{"   ", DefaultChromaTheme},
		{"monokai", "monokai"},
		{"  Dracula ", "dracula"},
		{"GITHUB-DARK", "github-dark"},
		{"not-a-theme", DefaultChromaTheme},
	}
	for _, tc := range cases {
		if got := NormalizeChromaTheme(tc.name); got != tc.want {
			t.Errorf("NormalizeChromaTheme(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestHighlightDiffANSIRendersTokens(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n@@ -1,3 +1,3 @@\n-old line\n+new line\n context"
	for _, theme := range []string{"", "monokai", "dracula", "github-dark"} {
		out := HighlightDiffANSI(diff, theme)
		if out == "" {
			t.Fatalf("theme %q produced empty output", theme)
		}
		if !strings.Contains(out, "\x1b[") {
			t.Fatalf("theme %q produced no ANSI codes", theme)
		}
		if !strings.Contains(out, "new line") {
			t.Fatalf("theme %q lost diff content", theme)
		}
	}
}
