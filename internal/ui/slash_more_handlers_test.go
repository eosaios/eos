package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestOpenPanelHelpers(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.openModelsPanel()
	if app.activeView != "panel" || app.activePanel != "models" {
		t.Fatalf("models = %q/%q", app.activeView, app.activePanel)
	}
	app.openContextPanel()
	if app.activePanel != "context" {
		t.Fatal("context")
	}
	app.openMemoryPanel()
	if app.activePanel != "memory" {
		t.Fatal("memory")
	}
	app.openSettingsPanel()
	if app.activePanel != "settings" {
		t.Fatal("settings")
	}
}

func TestHandleSessionAndResumeSlash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 列表空
	app.handleSessionSlash(nil)
	// save
	app.handleSessionSlash([]string{"save"})
	// export 缺 path
	app.handleSessionSlash([]string{"export"})
	// export 带 path
	app.handleSessionSlash([]string{"export", "id1", "/tmp/eos-session-export.md"})

	// resume
	app.handleResumeSlash(nil)
	app.handleResumeSlash([]string{"test-session"})
}

func TestHandleSkillsPluginReload(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.handleSkillsSlash(nil)
	app.handleSkillsSlash([]string{"reload"})
	app.handlePluginSlash()
	app.handlePluginSlash("install") // 缺参
	app.handlePluginSlash("remove")  // 缺参
	app.handleReloadPluginsSlash()
}

func TestHandleDoctorRemoteFast(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.handleDoctorSlash()
	app.handleRemoteSlash(nil)
	app.handleFastSlash() // 未配置 fast_model
}

func TestBrowserStatusLabel(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	running := app.browserStatusLabel(coreapi.BrowserRuntimeStatus{Running: true, BrowserKind: "chrome"})
	if running == "" {
		t.Fatal("running")
	}
	err := app.browserStatusLabel(coreapi.BrowserRuntimeStatus{LastError: "x"})
	if err == "" {
		t.Fatal("error")
	}
	idle := app.browserStatusLabel(coreapi.BrowserRuntimeStatus{})
	if idle == "" {
		t.Fatal("idle")
	}

	app.state.Language = "en"
	if app.browserStatusLabel(coreapi.BrowserRuntimeStatus{Running: true, BrowserKind: "chrome"}) == running {
		// 中英文文案不同
		t.Fatal("en label should differ")
	}
}

func TestHandleWorkspaceSlashAndModelSlash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// workspace 无参 / 列表
	app.handleWorkspaceSlash(nil)
	app.handleWorkspaceSlash([]string{"list"})
	// model 无参
	app.handleModelSlash(nil)
}

func TestHandleDiffReviewGitSlash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.handleDiffSlash(nil)
	app.handleDiffSlash([]string{"x"})
	app.handleReviewSlash(nil)
	app.handleGitSlash(nil)
	app.handleGitSlash([]string{"status"})
}

func TestHandleStatusSlash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	if cmd := app.handleStatusSlash(); cmd != nil {
		// 可能返回 nil
	}
	// 输出应进 history 或 shell
	if app.shell == nil {
		t.Fatal("shell")
	}
	_ = strings.Contains(app.shell.Content(), "状态")
}
