// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

package ui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"os/exec"
	"runtime"
)

const eosIssuesURL = "https://github.com/eosaios/eos-app/issues"

// handleFeedbackSlash /feedback — 在系统浏览器打开 GitHub Issues 意见反馈页。
func (m *AppModel) handleFeedbackSlash(_ []string) tea.Cmd {
	m.clearPrediction()
	m.shell.ClearInput()
	if err := openInBrowser(eosIssuesURL); err != nil {
		m.appendSystem(fmt.Sprintf("打开反馈页面失败: %v（手动访问 %s）", err, eosIssuesURL), "error")
		return nil
	}
	m.appendSystem(fmt.Sprintf("已在浏览器打开意见反馈页面：%s", eosIssuesURL), "info")
	return nil
}

// openInBrowserImpl 测试注入点：非 nil 时替代真实系统调用。
// 测试绝不能真拉起浏览器（会反复打开 GitHub Issues 页）。
var openInBrowserImpl func(string) error

func openInBrowser(url string) error {
	if openInBrowserImpl != nil {
		return openInBrowserImpl(url)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
