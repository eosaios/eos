package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"errors"
	"path/filepath"
	"strings"
)

// OpenWorkspaceDialog web 模式不支持原生目录选择对话框（浏览器沙箱无文件系统
// 访问）。工作区切换走 SelectWorkspace(path) 显式路径输入。
func (svc *SystemService) OpenWorkspaceDialog() (FileDialogResult, error) {
	return FileDialogResult{}, errors.New("native directory dialog is not available in web mode; use SelectWorkspace with an explicit path")
}

// ExportDiagnosticsBundle web 模式不支持原生保存对话框，v1 不导出诊断包。
func (svc *SystemService) ExportDiagnosticsBundle() (ExportResult, error) {
	return ExportResult{}, errors.New("diagnostics bundle export is not available in web mode")
}

// SaveTextFileDialog / SaveZipFileDialog web 模式无原生另存为对话框；
// 返回明确不支持（与诊断包导出同模式），前端按错误通道提示。
func (svc *SystemService) SaveTextFileDialog(defaultName, content string) (ExportResult, error) {
	return ExportResult{Cancelled: true}, errors.New("save-as dialog is not available in web mode")
}

func (svc *SystemService) SaveZipFileDialog(defaultName string, entries []ZipEntry) (ExportResult, error) {
	return ExportResult{Cancelled: true}, errors.New("save-as dialog is not available in web mode")
}

func (svc *SystemService) OpenLogDirectory() error {
	s := svc.bridge
	dir := DefaultLogDir()
	if s != nil && strings.TrimSpace(s.logFile) != "" {
		dir = filepath.Dir(s.logFile)
	}
	return OpenDirectory(dir)
}

// RevealInFileManager 在系统文件管理器中定位到 path：文件则选中，目录则打开。
// 相对路径基于当前活动工作区解析（对话里 AI 输出的常是相对路径如 src/foo.ts）。
// path 必须存在，不存在返回错误（不创建、不崩溃）。
func (svc *SystemService) RevealInFileManager(path string) error {
	s := svc.bridge
	if s == nil {
		return errors.New("bridge service is not available")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New(s.t("error.system.path_required"))
	}
	if !filepath.IsAbs(path) {
		ws := strings.TrimSpace(s.activeWorkspaceValue())
		if ws == "" {
			return errors.New(s.t("error.system.relative_path_workspace_required"))
		}
		path = filepath.Join(ws, path)
	}
	return RevealPath(path)
}

// ListExternalApps 返回当前平台的外部应用目录（含安装状态），
// 供顶栏「在外部打开」下拉渲染；未安装项由前端隐藏。
func (svc *SystemService) ListExternalApps() ([]ExternalAppInfo, error) {
	return externalAppCatalog(), nil
}

// OpenInExternalApp 用 appID 对应的外部应用打开 path（相对路径按活动工作区解析，
// 约束同 RevealInFileManager）。
func (svc *SystemService) OpenInExternalApp(appID, path string) error {
	s := svc.bridge
	if s == nil {
		return errors.New("bridge service is not available")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New(s.t("error.system.path_required"))
	}
	if !filepath.IsAbs(path) {
		ws := strings.TrimSpace(s.activeWorkspaceValue())
		if ws == "" {
			return errors.New(s.t("error.system.relative_path_workspace_required"))
		}
		path = filepath.Join(ws, path)
	}
	if err := OpenInExternalApp(appID, path); err != nil {
		if errors.Is(err, errExternalAppUnavailable) {
			return errors.New(s.t("error.system.external_app_unavailable"))
		}
		return err
	}
	return nil
}

func (svc *SystemService) ReadClipboardText() string {
	s := svc.bridge
	if s == nil {
		return ""
	}
	text, _ := s.readClipboardText()
	return text
}

func (svc *SystemService) WriteClipboardText(text string) BootstrapState {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}
	}
	if s.writeClipboardText(text) {
		s.stateMu.Lock()
		s.pushNotificationLocked("Clipboard Updated", "Selected content copied to clipboard.", "success")
		s.emitShellUpdated()
		s.stateMu.Unlock()
	}
	return s.LoadBootstrap()
}

func (svc *SystemService) AcknowledgeCrashReport() (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := MarkCrashReportAcknowledged(); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.pushNotificationLocked("Crash Report Acknowledged", "Pending crash report marked as read.", "info")
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}
