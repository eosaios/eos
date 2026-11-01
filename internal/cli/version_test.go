package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/version"
)

// TestPrintVersionOutput 验证版本输出包含 AppVersion 与运行平台。
func TestPrintVersionOutput(t *testing.T) {
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = wOut
	t.Cleanup(func() { os.Stdout = origStdout })

	printVersion()
	_ = wOut.Close()

	output, _ := io.ReadAll(rOut)
	if !strings.Contains(string(output), version.AppVersion) {
		t.Fatalf("version output missing AppVersion %q: %s", version.AppVersion, output)
	}
	if !strings.Contains(string(output), runtime.GOOS) {
		t.Fatalf("version output missing GOOS %q: %s", runtime.GOOS, output)
	}
}

// TestRootVersionFlagRegistered 验证根命令已注册 --version 标志。
func TestRootVersionFlagRegistered(t *testing.T) {
	if rootCmd.Flags().Lookup("version") == nil {
		t.Fatal("--version flag not registered on root command")
	}
}

// TestRootRunShortCircuitsOnVersionFlag 验证 --version 标志下根命令
// 只打印版本即返回，不进入交互式 TUI（若缺失短路，Run 会阻塞启动 TUI）。
func TestRootRunShortCircuitsOnVersionFlag(t *testing.T) {
	cliShowVersion = true
	t.Cleanup(func() { cliShowVersion = false })

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = wOut
	t.Cleanup(func() { os.Stdout = origStdout })

	if err := rootCmd.RunE(rootCmd, nil); err != nil {
		t.Fatalf("version root command: %v", err)
	}
	_ = wOut.Close()

	output, _ := io.ReadAll(rOut)
	if !strings.Contains(string(output), version.AppVersion) {
		t.Fatalf("expected version output, got: %s", output)
	}
}
