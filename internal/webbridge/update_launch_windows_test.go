//go:build windows

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"
)

// 锁死 2026-08-26 beta.21 升级失败的根因：命令行 /CLOSEAPPLICATIONS 会覆盖
// installer.iss 的 CloseApplications=no，强行启用 Restart Manager——RM 关不
// 掉无窗口的内核 eos-core.exe，在 /SUPPRESSMSGBOXES 下安装被静默中止。
// 进程启停的裁决权在 installer.iss（CloseApplications=no + [Code] taskkill）。
func TestUpdateInstallerArgsNoRestartManager(t *testing.T) {
	fields := strings.Fields(updateInstallerArgs)
	has := func(flag string) bool {
		for _, f := range fields {
			if strings.EqualFold(f, flag) {
				return true
			}
		}
		return false
	}
	for _, forbidden := range []string{"/CLOSEAPPLICATIONS", "/RESTARTAPPLICATIONS"} {
		if has(forbidden) {
			t.Errorf("updateInstallerArgs 含 %s：Restart Manager 的启停由 installer.iss 统一裁决，命令行不得覆盖", forbidden)
		}
	}
	for _, required := range []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/LOG"} {
		if !has(required) {
			t.Errorf("updateInstallerArgs 缺少 %s", required)
		}
	}
}
