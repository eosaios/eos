//go:build windows

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

// updateInstallerArgs 应用内更新拉起 Inno 安装器的静默参数。
// 故意不带 /CLOSEAPPLICATIONS /RESTARTAPPLICATIONS：命令行参数会覆盖
// installer.iss 的 CloseApplications=no，强行启用 Restart Manager——RM
// 关不掉无窗口的内核 eos-core.exe，在 /SUPPRESSMSGBOXES 下安装被静默
// 中止（2026-08-26 beta.21 升级失败的根因）。进程关闭统一交给 iss
// [Code] 的 taskkill 方案；装完由 iss [Run] 自动拉起新版本。
// /LOG 落地 %TEMP%\Setup Log*.txt，静默失败可查。
const updateInstallerArgs = "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LOG"

// launchUpdateInstaller 提权运行 setup 安装器（UAC 确认后由安装器接管：
// 关闭残留进程 → 覆盖安装 → [Run] 自动拉起新版本）。
// 用 ShellExecuteW 的 runas 动词（exec.Command 不能触发 UAC 提权）。
func launchUpdateInstaller(path string) error {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("ShellExecuteW")
	verb, _ := syscall.UTF16PtrFromString("runas")
	exe, _ := syscall.UTF16PtrFromString(path)
	params, _ := syscall.UTF16PtrFromString(updateInstallerArgs)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(path))
	const swShownormal = 1
	ret, _, _ := proc.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(exe)),
		uintptr(unsafe.Pointer(params)),
		uintptr(unsafe.Pointer(dir)),
		uintptr(swShownormal),
	)
	if ret <= 32 {
		// ShellExecute 返回值 <=32 为失败（用户取消 UAC、SEC_E_* 等）。
		return fmt.Errorf("启动安装器失败（ShellExecute code %d）：用户取消 UAC 或系统拒绝，请重试。", ret)
	}
	return nil
}
