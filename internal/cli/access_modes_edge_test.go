package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"testing"

	"github.com/spf13/pflag"
)

func setTestHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestMergeConfigPermissions(t *testing.T) {
	setTestHome(t)

	// 无配置 Permissions
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	fs.String("access-mode", "", "")
	fs.String("approval-mode", "", "")
	fs.String("sandbox-mode", "", "")
	am, ap := mergeConfigPermissions(fs, "sandbox-mode", "", "")
	if am != "" || ap != "" {
		t.Fatalf("empty cfg = %q %q", am, ap)
	}

	// 显式 flag 优先
	fs2 := pflag.NewFlagSet("t", pflag.ContinueOnError)
	fs2.String("access-mode", "read-only", "")
	fs2.String("approval-mode", "never", "")
	fs2.String("sandbox-mode", "", "")
	_ = fs2.Set("access-mode", "read-only")
	_ = fs2.Set("approval-mode", "never")
	am, ap = mergeConfigPermissions(fs2, "sandbox-mode", "read-only", "never")
	if am != "read-only" || ap != "never" {
		t.Fatalf("explicit = %q %q", am, ap)
	}

	// 非空入参不被配置覆盖
	am, ap = mergeConfigPermissions(fs, "sandbox-mode", "danger-full-access", "on-request")
	if am != "danger-full-access" || ap != "on-request" {
		t.Fatalf("args win = %q %q", am, ap)
	}
}

func TestResolveModeConfigDefaults(t *testing.T) {
	cfg := resolveModeConfig("", "", "", false)
	if cfg.AccessMode == "" || cfg.SandboxMode == "" {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.ApprovalMode != "" {
		t.Fatalf("approval default = %q", cfg.ApprovalMode)
	}
	// sandbox 别名归一
	cfg = resolveModeConfig("", "", "full_access", false)
	if cfg.AccessMode != "danger-full-access" {
		t.Fatalf("alias = %+v", cfg)
	}
}
