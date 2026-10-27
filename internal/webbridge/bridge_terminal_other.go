//go:build !windows

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

type posixPtyBackend struct {
	cmd     *exec.Cmd
	ptyFile *os.File
	waitMu  sync.Mutex
	waitErr error
	waited  bool
}

func startBridgeTerminalBackend(workspacePath string, cols, rows int) (bridgeTerminalBackend, error) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		return nil, fmt.Errorf("bash not found in PATH")
	}

	cmd := exec.Command(bashPath, "-l")
	cmd.Dir = workspacePath
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "CHERE_INVOKING=1")

	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}

	return &posixPtyBackend{
		cmd:     cmd,
		ptyFile: ptyFile,
	}, nil
}

func (b *posixPtyBackend) Read(p []byte) (int, error) {
	return b.ptyFile.Read(p)
}

func (b *posixPtyBackend) Write(p []byte) (int, error) {
	return b.ptyFile.Write(p)
}

func (b *posixPtyBackend) Close() error {
	if b.cmd != nil && b.cmd.Process != nil {
		if err := b.cmd.Process.Signal(syscall.SIGHUP); !shouldIgnoreTerminalProcessError(err) {
			slog.Warn("bridge.terminal.signal_failed", "signal", "SIGHUP", "pid", b.cmd.Process.Pid, "error", err)
		}
		if err := b.cmd.Process.Kill(); !shouldIgnoreTerminalProcessError(err) {
			slog.Warn("bridge.terminal.kill_failed", "pid", b.cmd.Process.Pid, "error", err)
		}
	}
	if b.ptyFile == nil {
		return nil
	}
	return b.ptyFile.Close()
}

func shouldIgnoreTerminalProcessError(err error) bool {
	return err == nil || errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH)
}

func (b *posixPtyBackend) Resize(cols, rows int) error {
	return pty.Setsize(b.ptyFile, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (b *posixPtyBackend) Wait(ctx context.Context) error {
	b.waitMu.Lock()
	if b.waited {
		err := b.waitErr
		b.waitMu.Unlock()
		return err
	}
	b.waitMu.Unlock()

	done := make(chan error, 1)
	go func() {
		done <- b.cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	b.waitMu.Lock()
	defer b.waitMu.Unlock()
	b.waitErr = err
	b.waited = true
	return err
}
