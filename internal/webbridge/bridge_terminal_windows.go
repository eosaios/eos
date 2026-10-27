//go:build windows

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/UserExistsError/conpty"
)

// errTerminalShellMissing 结构化标记：前端据此切换到安装引导视图，
// 而不是把英文报错顶到全局 alert。
var errTerminalShellMissing = errors.New("terminal shell missing")

type conPtyBackend struct {
	backend *conpty.ConPty
}

func startBridgeTerminalBackend(workspacePath string, cols, rows int) (bridgeTerminalBackend, error) {
	// 三层探测：程序指定目录 → 系统 PATH → git/注册表/固定位置推导。
	// 详见 bridge_terminal_shell_windows.go（GUI 进程的 PATH 里通常没有 bash，
	// 直接 LookPath 会让装了 Git 的用户也开不了终端）。
	probe := probeTerminalShell()
	if !probe.Available {
		return nil, errTerminalShellMissing
	}
	commandLine := quoteTerminalCommandArg(probe.Path) + " -l"
	env := append(os.Environ(), "TERM=xterm-256color", "CHERE_INVOKING=1")

	backend, err := conpty.Start(
		commandLine,
		conpty.ConPtyDimensions(cols, rows),
		conpty.ConPtyWorkDir(workspacePath),
		conpty.ConPtyEnv(env),
	)
	if err != nil {
		return nil, err
	}
	return &conPtyBackend{backend: backend}, nil
}

func (b *conPtyBackend) Read(p []byte) (int, error) {
	return b.backend.Read(p)
}

func (b *conPtyBackend) Write(p []byte) (int, error) {
	return b.backend.Write(p)
}

func (b *conPtyBackend) Close() error {
	return b.backend.Close()
}

func (b *conPtyBackend) Resize(cols, rows int) error {
	return b.backend.Resize(cols, rows)
}

func (b *conPtyBackend) Wait(ctx context.Context) error {
	_, err := b.backend.Wait(ctx)
	return err
}

func quoteTerminalCommandArg(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if !strings.ContainsAny(value, " \t\"") {
		return value
	}
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	return `"` + escaped + `"`
}
