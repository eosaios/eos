package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"errors"
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

	// 注入 stub，绝不能真拉起系统浏览器（会反复打开 GitHub Issues 页）
	orig := openInBrowserImpl
	var opened string
	openInBrowserImpl = func(url string) error {
		opened = url
		return nil
	}
	t.Cleanup(func() { openInBrowserImpl = orig })

	app.handleFeedbackSlash(nil)
	if opened != eosIssuesURL {
		t.Fatalf("opened = %q, want %q", opened, eosIssuesURL)
	}

	// 打开失败分支
	openInBrowserImpl = func(string) error { return errors.New("no browser") }
	app.handleFeedbackSlash(nil)
	openInBrowserImpl = orig

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
