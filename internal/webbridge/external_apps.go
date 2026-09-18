package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ExternalAppInfo 描述一个可从 EOS 打开当前工作区的外部应用。
// Installed 由后端按当前平台探测，前端只展示已安装项。
type ExternalAppInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

// errExternalAppUnavailable 是低层启动函数的哨兵错误（该层无 i18n 上下文），
// 服务层捕获后翻译为 error.system.external_app_unavailable。
var errExternalAppUnavailable = errors.New("external app unavailable")

// externalAppSpec 是单个第三方应用的跨平台描述；字段为空表示该平台不提供。
type externalAppSpec struct {
	id           string
	name         string // 展示名（应用官方名）
	darwinBundle string // macOS .app 包名（/Applications、~/Applications 下探测）
	windowsExe   string // %LOCALAPPDATA%\Programs 下的相对 exe 路径
	linuxExe     string // PATH 中的可执行名
}

// externalAppSpecs 是第三方应用目录（顺序即前端展示顺序）。
// files 与 terminal 各平台差异大（名称/探测方式），不进表，单独处理。
var externalAppSpecs = []externalAppSpec{
	{id: "cursor", name: "Cursor", darwinBundle: "Cursor.app", windowsExe: filepath.Join("cursor", "Cursor.exe"), linuxExe: "cursor"},
	{id: "trae", name: "Trae", darwinBundle: "Trae.app", windowsExe: filepath.Join("Trae", "Trae.exe"), linuxExe: "trae"},
	{id: "vscode", name: "VS Code", darwinBundle: "Visual Studio Code.app", windowsExe: filepath.Join("Microsoft VS Code", "Code.exe"), linuxExe: "code"},
	{id: "warp", name: "Warp", darwinBundle: "Warp.app", windowsExe: filepath.Join("Warp", "warp.exe"), linuxExe: "warp-terminal"},
	{id: "iterm", name: "iTerm", darwinBundle: "iTerm.app"},
}

// lookPathExists 报告 exe 是否在 PATH 中（exec.LookPath 的布尔包装，
// 便于在结构体字面量等单值上下文使用）。
func lookPathExists(exe string) bool {
	_, err := exec.LookPath(exe)
	return err == nil
}

// externalAppCatalog 返回当前平台的外部应用目录（含安装状态）。
func externalAppCatalog() []ExternalAppInfo {
	apps := make([]ExternalAppInfo, 0, len(externalAppSpecs)+2)
	switch runtime.GOOS {
	case "darwin":
		apps = append(apps,
			ExternalAppInfo{ID: "files", Name: "Finder", Installed: true},
			ExternalAppInfo{ID: "terminal", Name: "Terminal", Installed: true},
		)
	case "windows":
		apps = append(apps,
			ExternalAppInfo{ID: "files", Name: "Explorer", Installed: true},
			ExternalAppInfo{ID: "terminal", Name: "Terminal", Installed: lookPathExists("wt.exe")},
		)
	default:
		apps = append(apps,
			ExternalAppInfo{ID: "files", Name: "Files", Installed: true},
			ExternalAppInfo{ID: "terminal", Name: "Terminal", Installed: linuxTerminalCommand("") != nil},
		)
	}
	for _, spec := range externalAppSpecs {
		apps = append(apps, ExternalAppInfo{ID: spec.id, Name: spec.name, Installed: spec.installed()})
	}
	return apps
}

// installed 探测当前平台是否安装该应用（纯探测，不启动进程）。
func (spec externalAppSpec) installed() bool {
	switch runtime.GOOS {
	case "darwin":
		if spec.darwinBundle == "" {
			return false
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			if _, err := os.Stat(filepath.Join(dir, spec.darwinBundle)); err == nil {
				return true
			}
		}
		return false
	case "windows":
		return spec.windowsExe != "" && windowsInstalledExe(spec) != ""
	default:
		if spec.linuxExe == "" {
			return false
		}
		return lookPathExists(spec.linuxExe)
	}
}

// windowsInstalledExe 返回用户级安装目录下找到的可执行完整路径；未找到返回空串。
// os.UserCacheDir 在 Windows 上即 %LOCALAPPDATA%（编辑器类普遍装在
// %LOCALAPPDATA%\Programs 下）。
func windowsInstalledExe(spec externalAppSpec) string {
	if spec.windowsExe == "" {
		// 无 Windows 版本的应用（如 iterm）：Join 与空串会落到
		// %LOCALAPPDATA%\Programs 本身——该目录恒存在，会被误判已安装。
		return ""
	}
	localAppData, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(localAppData, "Programs", spec.windowsExe)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

// linuxTerminalCommand 返回第一个可用的终端模拟器启动命令（工作目录参数已带上）；
// dir 为空时仅作探测。找不到返回 nil。
func linuxTerminalCommand(dir string) *exec.Cmd {
	for _, candidate := range []struct {
		exe  string
		args []string
	}{
		{"x-terminal-emulator", []string{"--working-directory"}},
		{"gnome-terminal", []string{"--working-directory"}},
		{"konsole", []string{"--workdir"}},
		{"xfce4-terminal", []string{"--working-directory"}},
	} {
		if _, err := exec.LookPath(candidate.exe); err == nil {
			return exec.Command(candidate.exe, append(candidate.args, dir)...)
		}
	}
	return nil
}

// OpenInExternalApp 用 appID 对应的外部应用打开 path。与 RevealPath 同约束：
// 路径必须已存在；文件则打开其所在目录。未知应用返回错误，未安装返回
// errExternalAppUnavailable 哨兵。
func OpenInExternalApp(appID, path string) error {
	appID = strings.TrimSpace(appID)
	path = strings.TrimSpace(path)
	if appID == "" || path == "" {
		return errors.New("app and path are required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	switch appID {
	case "files":
		return openDirectoryNoMkdir(abs)
	case "terminal":
		return openTerminalApp(abs)
	default:
		return openThirdPartyApp(appID, abs)
	}
}

// openTerminalApp 用系统终端打开工作目录。
func openTerminalApp(dir string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-a", "Terminal", dir).Start()
	case "windows":
		if _, err := exec.LookPath("wt.exe"); err != nil {
			return errExternalAppUnavailable
		}
		return exec.Command("wt.exe", "-d", dir).Start()
	default:
		cmd := linuxTerminalCommand(dir)
		if cmd == nil {
			return errExternalAppUnavailable
		}
		return cmd.Start()
	}
}

// openThirdPartyApp 启动第三方应用打开工作目录。
func openThirdPartyApp(appID, dir string) error {
	for _, spec := range externalAppSpecs {
		if spec.id != appID {
			continue
		}
		switch runtime.GOOS {
		case "darwin":
			if spec.darwinBundle == "" {
				return errExternalAppUnavailable
			}
			// open -a 接受不带 .app 后缀的应用名。
			return exec.Command("open", "-a", strings.TrimSuffix(spec.darwinBundle, ".app"), dir).Start()
		case "windows":
			exe := windowsInstalledExe(spec)
			if exe == "" {
				return errExternalAppUnavailable
			}
			return exec.Command(exe, dir).Start()
		default:
			if spec.linuxExe == "" {
				return errExternalAppUnavailable
			}
			if _, err := exec.LookPath(spec.linuxExe); err != nil {
				return errExternalAppUnavailable
			}
			return exec.Command(spec.linuxExe, dir).Start()
		}
	}
	return errors.New("unknown external app: " + appID)
}
