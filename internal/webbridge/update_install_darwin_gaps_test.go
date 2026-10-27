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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestDarwinInstallChainWithFakeBins(t *testing.T) {
	fakeBin := t.TempDir()
	mountPoint := t.TempDir()
	appBundle := filepath.Join(mountPoint, "EOS.app")
	if err := os.MkdirAll(appBundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appBundle, "new-marker"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 假 hdiutil：attach 弹 mount-point plist，detach 记账。
	hdiutilCalls := filepath.Join(fakeBin, "hdiutil-calls")
	hdiutil := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + hdiutilCalls + "\n" +
		"if [ \"$1\" = attach ]; then\n" +
		"  printf '<?xml><dict><key>mount-point</key><string>" + mountPoint + "</string></dict>'\n" +
		"fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "hdiutil"), []byte(hdiutil), 0o755); err != nil {
		t.Fatal(err)
	}
	// 假 ditto：目录级复制（真实 ditto 语义的足够子集）。
	ditto := "#!/bin/sh\nmkdir -p \"$2\" && cp -R \"$1\"/. \"$2\"/\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "ditto"), []byte(ditto), 0o755); err != nil {
		t.Fatal(err)
	}
	// 假 open：relaunch 调度断言。
	openCalls := filepath.Join(fakeBin, "open-calls")
	openScript := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + openCalls + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "open"), []byte(openScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	// 挂载：plist 解析回假挂载点。
	got, err := mountMacOSDmg("/fake/eos.dmg")
	if err != nil || got != mountPoint {
		t.Fatalf("mountMacOSDmg = %q, %v", got, err)
	}
	detachMacOSDmg(mountPoint)

	// swap 全链：旧 bundle 原地替换为 dmg 内新 bundle。
	target := filepath.Join(t.TempDir(), "EOS.app")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old-marker"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := swapMacOSBundle("/fake/eos.dmg", target); err != nil {
		t.Fatalf("swapMacOSBundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "new-marker")); err != nil {
		t.Fatalf("替换后应存在新标记: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "old-marker")); !os.IsNotExist(err) {
		t.Fatal("旧内容不应残留")
	}

	// relaunch 调度：1s 后经 PATH 命中假 open。
	bundle := filepath.Join(t.TempDir(), "EOS.app")
	scheduleMacOSRelaunch(bundle)
	// Include system validation of the fresh fake open after the 1s schedule.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(openCalls); err == nil && strings.Contains(string(data), bundle) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("scheduleMacOSRelaunch 未触发假 open")
}
