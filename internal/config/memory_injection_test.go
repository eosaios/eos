package config

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestMemoryInjectionEnabledDefaultsTrue(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{name: "nil config", cfg: nil, want: true},
		{name: "unset", cfg: &Config{}, want: true},
		{name: "explicit false", cfg: &Config{MemoryInjectionEnabled: boolPtr(false)}, want: false},
		{name: "explicit true", cfg: &Config{MemoryInjectionEnabled: boolPtr(true)}, want: true},
	}
	for _, tc := range cases {
		if got := MemoryInjectionEnabled(tc.cfg); got != tc.want {
			t.Errorf("%s: MemoryInjectionEnabled() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestGitCommitReminderEnabledDefaultsTrue(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{name: "nil config", cfg: nil, want: true},
		{name: "unset", cfg: &Config{}, want: true},
		{name: "explicit false", cfg: &Config{GitCommitReminder: boolPtr(false)}, want: false},
		{name: "explicit true", cfg: &Config{GitCommitReminder: boolPtr(true)}, want: true},
	}
	for _, tc := range cases {
		if got := GitCommitReminderEnabled(tc.cfg); got != tc.want {
			t.Errorf("%s: GitCommitReminderEnabled() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
