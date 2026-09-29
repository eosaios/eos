package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"testing"

	"github.com/eosaios/eos/internal/ui/adapter"
)

func TestPluginConfirmHelpers(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	app.setPendingPluginConfirm("src-1")
	app.pluginInstallConfirm("src-1")
	app.clearPendingPluginConfirm()

	// pluginRemoveCmd / pluginSearchCmd 不 panic
	app.pluginRemoveCmd("nope")
	app.pluginSearchCmd("query")
	app.pluginInstallCmd("nope-source") // 异步，不阻塞
}

func TestFeedbackAndScreenshotSlash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// feedback：尝试打开浏览器（沙箱可能失败，不 panic 即可）
	app.handleFeedbackSlash(nil)

	// screenshot
	app.handleScreenshotSlash(nil)
	app.handleScreenshotSlash([]string{"path"})
}

func TestHandleHiddenLegalSlash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.handleHiddenLegalSlash()
}

func TestInitEOSMDAndTryInvokeSkill(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.initEOSMD()
	if app.tryInvokeSkillSlash("nope", nil) {
		t.Fatal("missing skill")
	}
}

func TestCheckForUpdatesAndVersionCheck(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.checkForUpdates()
	app.handleVersionCheck(VersionCheckMsg{})
}

func TestHandleRuntimeEvent(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// 空事件
	if next, _ := app.handleRuntimeEvent(adapter.RuntimeEvent{}); next == nil {
		t.Fatal("runtime event")
	}
}
