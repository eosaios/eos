//go:build !windows

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// posix pty 后端轻量真进程测试（从 bridge_plugins_status_gaps_test.go 拆出：
// shouldIgnoreTerminalProcessError 定义于 !windows 的 bridge_terminal_other.go，
// 与平台无关的批测留在原文件于双平台运行）。

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// ---- posix pty 后端（轻量真进程） ----

func TestPosixPtyBackendLifecycle(t *testing.T) {
	backend, err := startBridgeTerminalBackend(t.TempDir(), 80, 24)
	if err != nil {
		t.Skipf("pty unavailable in this environment: %v", err)
	}
	if err := backend.Resize(100, 30); err != nil {
		t.Fatalf("Resize error = %v", err)
	}
	if _, err := backend.Write([]byte("\n")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	// 读回显（bash 启动横幅/回显，轮询至读到任意字节）。
	buf := make([]byte, 256)
	deadline := time.Now().Add(3 * time.Second)
	read := 0
	for time.Now().Before(deadline) && read == 0 {
		n, readErr := backend.Read(buf)
		if n > 0 {
			read += n
			break
		}
		if readErr != nil {
			t.Fatalf("Read error = %v", readErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if read == 0 {
		t.Log("pty produced no output within 3s; lifecycle still verified via write/resize/close")
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	waitErr := backend.Wait(context.Background())
	// Close 发 SIGHUP：bash 以 signal: hangup 退出属正常收尾（生产侧忽略
	// Wait 返回值），此处同样接受。
	if waitErr != nil && !shouldIgnoreTerminalProcessError(waitErr) && !strings.Contains(waitErr.Error(), "hangup") {
		t.Fatalf("Wait error = %v", waitErr)
	}
}

func TestShouldIgnoreTerminalProcessError(t *testing.T) {
	if !shouldIgnoreTerminalProcessError(nil) {
		t.Fatal("nil should be ignored")
	}
	if !shouldIgnoreTerminalProcessError(os.ErrProcessDone) {
		t.Fatal("process done should be ignored")
	}
	if shouldIgnoreTerminalProcessError(errors.New("boom")) {
		t.Fatal("generic error should not be ignored")
	}
	// context 错误不属「进程已终结」词表，不忽略。
	if shouldIgnoreTerminalProcessError(context.Canceled) {
		t.Fatal("context.Canceled should not be ignored")
	}
}
