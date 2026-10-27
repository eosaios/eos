//go:build darwin

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// macOS 更新安装：Sparkle 式原地替换（方案 A）——
// 静默挂载 dmg → 定位 *.app → 复制到目标目录的暂存名 → 换名替换运行中的
// bundle（运行中进程持有旧可执行文件的 inode，替换目录项安全，Sparkle 同款）
// → 卸载 dmg → 调度重拉，随后由调用方延迟退出当前应用。
// 任一步失败回落方案 B：打开 dmg 由 Finder 引导手动拖装，返回带指引的
// 错误（应用保持运行以展示指引；旧应用未退出前拖拽必然「正在使用中」，
// 指引里明确要求先退出）。

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// launchUpdateInstaller macOS 入口：先尝试原地替换，失败回落打开 dmg。
func launchUpdateInstaller(path string) error {
	return installMacOSUpdate(path)
}

func installMacOSUpdate(dmgPath string) error {
	current, err := runningMacOSAppBundle()
	if err != nil {
		return fallbackMacOSOpenDmg(dmgPath, err)
	}
	if err := swapMacOSBundle(dmgPath, current); err != nil {
		return fallbackMacOSOpenDmg(dmgPath, err)
	}
	scheduleMacOSRelaunch(current)
	return nil
}

// runningMacOSAppBundle 反推当前运行中的 .app bundle 路径。
func runningMacOSAppBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("定位当前可执行文件失败: %w", err)
	}
	bundle, ok := macOSAppBundlePath(exe)
	if !ok {
		return "", errors.New("当前不是以 .app bundle 方式运行")
	}
	return bundle, nil
}

// macOSAppBundlePath 从可执行路径向上找 .app 后缀目录
// （典型布局 <bundle>/Contents/MacOS/exe）。纯函数，便于测试。
func macOSAppBundlePath(exe string) (string, bool) {
	dir := filepath.Dir(exe)
	for i := 0; i < 4; i++ {
		if strings.HasSuffix(dir, ".app") {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
	return "", false
}

func swapMacOSBundle(dmgPath, currentBundle string) error {
	mountPoint, err := mountMacOSDmg(dmgPath)
	if err != nil {
		return err
	}
	defer detachMacOSDmg(mountPoint)
	source, err := findMacOSAppBundle(mountPoint)
	if err != nil {
		return err
	}
	return replaceMacOSBundle(source, currentBundle)
}

// mountMacOSDmg 只读静默挂载（不弹 Finder），返回挂载点。
func mountMacOSDmg(dmgPath string) (string, error) {
	out, err := exec.Command("hdiutil", "attach", dmgPath, "-readonly", "-nobrowse", "-plist").Output()
	if err != nil {
		return "", fmt.Errorf("挂载 dmg 失败: %w", err)
	}
	mount := hdiutilMountPoint(string(out))
	if mount == "" {
		return "", errors.New("挂载 dmg 失败：输出中未找到挂载点")
	}
	return mount, nil
}

func detachMacOSDmg(mountPoint string) {
	// 卸载失败不影响安装结果（挂载点会在重启后消失），尽力而为。
	_ = exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
}

// hdiutilMountPoint 解析 `hdiutil attach -plist` 输出里的 mount-point。
// 结构固定为 <key>mount-point</key><string>/Volumes/xxx</string>。
func hdiutilMountPoint(plistOutput string) string {
	const key = "<key>mount-point</key>"
	idx := strings.Index(plistOutput, key)
	if idx < 0 {
		return ""
	}
	rest := plistOutput[idx+len(key):]
	open := strings.Index(rest, "<string>")
	close := strings.Index(rest, "</string>")
	if open < 0 || close < 0 || close < open {
		return ""
	}
	value := rest[open+len("<string>") : close]
	// 卷名可能含 XML 转义字符（& → &amp; 等）
	return strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&apos;", "'",
	).Replace(value)
}

// findMacOSAppBundle 在挂载点根目录定位 *.app。
func findMacOSAppBundle(mountPoint string) (string, error) {
	entries, err := os.ReadDir(mountPoint)
	if err != nil {
		return "", fmt.Errorf("读取 dmg 挂载点失败: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".app") {
			return filepath.Join(mountPoint, entry.Name()), nil
		}
	}
	return "", errors.New("dmg 中未找到 .app")
}

// replaceMacOSBundle 原地替换 bundle：ditto 复制到同级暂存名（保留
// 元数据/扩展属性/权限），旧 bundle 先改名备份，再换名启用新 bundle；
// 启用失败回滚备份。全部完成后清理备份与暂存。
func replaceMacOSBundle(source, target string) error {
	parent := filepath.Dir(target)
	name := filepath.Base(target)
	tag := strconv.Itoa(os.Getpid())
	staging := filepath.Join(parent, "."+name+".updating-"+tag)
	backup := filepath.Join(parent, "."+name+".old-"+tag)
	// 清理同名残留（上次异常退出可能遗留）；不存在时 RemoveAll 返回 nil。
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("清理暂存目录失败: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("清理备份目录失败: %w", err)
	}
	if err := exec.Command("ditto", source, staging).Run(); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("复制新版本失败: %w", err)
	}
	if err := os.Rename(target, backup); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("移出旧版本失败: %w", err)
	}
	if err := os.Rename(staging, target); err != nil {
		// 回滚：恢复旧 bundle，保持当前安装可用
		_ = os.Rename(backup, target)
		_ = os.RemoveAll(staging)
		return fmt.Errorf("启用新版本失败: %w", err)
	}
	_ = os.RemoveAll(backup)
	return nil
}

// scheduleMacOSRelaunch 调度 1s 后拉起新版本：独立进程存活到应用退出之后。
func scheduleMacOSRelaunch(bundle string) {
	quoted := "'" + strings.ReplaceAll(bundle, "'", `'\''`) + "'"
	_ = exec.Command("sh", "-c", "sleep 1; open "+quoted).Start()
}

// fallbackMacOSOpenDmg 方案 B：打开 dmg 由 Finder 引导手动安装，
// 返回带指引的错误（前端会展示；应用保持运行让用户读到指引）。
func fallbackMacOSOpenDmg(dmgPath string, cause error) error {
	if openErr := exec.Command("open", dmgPath).Start(); openErr != nil {
		return fmt.Errorf("自动安装失败（%v），打开安装器也失败: %w", cause, openErr)
	}
	return fmt.Errorf(
		"自动替换未能完成（%v）。已打开安装器：请先退出 EOS，再把 EOS 拖入「Applications」完成安装",
		cause,
	)
}
