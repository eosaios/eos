//go:build darwin

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// macOS 更新安装 fallback：从 bridge_items_sync_prompt_gaps_test.go 拆出——
// 断言的是 update_install_darwin.go 的 open-dmg 语义；Windows 走
// ShellExecute 分支（update_launch_windows.go），错误文案与流程不同。

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchUpdateInstallerFallbackOnMissingDMG(t *testing.T) {
	// go test 环境不是 .app bundle → runningMacOSAppBundle 失败 →
	// fallback 打开 dmg；dmg 不存在时 open 也失败 → 带指引错误返回。
	err := launchUpdateInstaller(filepath.Join(t.TempDir(), "missing.dmg"))
	if err == nil {
		t.Fatal("launchUpdateInstaller(missing dmg) error = nil")
	}
	// open 异步 Start 恒成功（文件不存在由 open 进程自身报错，不弹窗），
	// 故走「自动替换未能完成 + 已打开安装器指引」分支。
	if !strings.Contains(err.Error(), "自动替换未能完成") {
		t.Fatalf("error = %v, want fallback guidance", err)
	}
}
