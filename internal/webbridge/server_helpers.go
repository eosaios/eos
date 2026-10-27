package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

// server_helpers.go 是 web 模式服务壳的小工具函数与用户可见文案。

import (
	"os"
	"os/exec"

	"github.com/eosaios/eos/pkg/executil"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/i18n"
)

// runBackgroundCommand 启动一个脱离服务生命周期的进程（如 open 浏览器），
// 不等待其退出。启动失败返回错误，进程自身的退出码不关心。
func runBackgroundCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return executil.StartDetached(cmd)
}

// webLang 读启动语言：EOS_LANG（zh/en），缺省 zh（与 CLI 全局约定一致）。
func webLang() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EOS_LANG"))) {
	case "en":
		return "en"
	default:
		return "zh"
	}
}

func webServerReadyMessage(url, uiDir string) string {
	return i18n.T("web.server.ready", webLang(), url, uiDir)
}

func webServerShutdownMessage() string {
	return i18n.T("web.server.shutdown", webLang())
}
