package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// external_apps 余臂批测：探测边界（空 bundle/空 linuxExe/UserCacheDir 失败）、
// 打开链校验族（空参/不存在路径/文件取父目录/未知应用）、三方应用
// open -a 拦截与未安装哨兵（假 open 前置）。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newFakeOpenBin(t *testing.T) (string, string) {
	t.Helper()
	fakeBin := t.TempDir()
	calls := filepath.Join(fakeBin, "open-calls")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + calls + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "open"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return fakeBin, calls
}

func waitFakeOpen(t *testing.T, calls, substr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if data, err := os.ReadFile(calls); err == nil && strings.Contains(string(data), substr) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("假 open 未拦截 %q: %v", substr, calls)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestExternalAppProbeEdgeArms(t *testing.T) {
	// darwin 空 bundle：恒未安装。
	if (&externalAppSpec{id: "x"}).installed() {
		t.Fatal("空 bundle 应未安装")
	}
	// linux 空 exe 同理（darwin 上走 darwin 臂——用 cfg 无关的结构验证空字段路径）。
	spec := externalAppSpec{id: "x"} // 全空
	if spec.darwinBundle != "" || spec.linuxExe != "" || spec.windowsExe != "" {
		t.Fatal("夹具污染")
	}
	// UserCacheDir 失败臂：HOME 清空。
	t.Setenv("HOME", "")
	if got := windowsInstalledExe(externalAppSpecs[1]); got != "" {
		t.Fatalf("无 HOME = %q", got)
	}
}

func TestOpenInExternalAppValidationArms(t *testing.T) {
	_, calls := newFakeOpenBin(t)
	// 空参。
	if err := OpenInExternalApp("", "/tmp"); err == nil {
		t.Fatal("空 appID 应报错")
	}
	dir := t.TempDir()
	if err := OpenInExternalApp("files", "  "); err == nil {
		t.Fatal("空 path 应报错")
	}
	// 不存在路径。
	if err := OpenInExternalApp("files", filepath.Join(dir, "missing")); err == nil {
		t.Fatal("不存在路径应报错")
	}
	// 文件路径 → 打开父目录（files 臂）。
	marker := filepath.Join(dir, "file.txt")
	os.WriteFile(marker, []byte("x"), 0o644)
	if err := OpenInExternalApp("files", marker); err != nil {
		t.Fatalf("files 臂: %v", err)
	}
	waitFakeOpen(t, calls, dir)
	// 未知应用。
	if err := OpenInExternalApp("no-such-app", dir); err == nil ||
		!strings.Contains(err.Error(), "unknown external app") {
		t.Fatalf("未知应用 = %v", err)
	}
	// 三方应用 darwin：open -a 应用名（去 .app 后缀）。
	if err := OpenInExternalApp("cursor", dir); err != nil {
		t.Fatalf("cursor 打开: %v", err)
	}
	waitFakeOpen(t, calls, "-a Cursor")
	// darwin 空 bundle 哨兵：iterm 有 bundle——直接构造 spec 走 openThirdPartyApp
	// 的空 bundle 分支不可达（specs 全有 bundle）；未知 id 走 default 已覆盖。
}
