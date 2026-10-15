package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// RunPrintMode 假内核批测：原生测试内核（Content-Length 帧）应答
// initialize/session/turn 全链并推送 item.delta + turn.completed 事件，
// 驱动 text/json/stream-json 三种输出格式的主路径。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/testexec"
	sidecarclient "github.com/eosaios/eos/pkg/coreapi/sidecar/client"
)

// writeFakeKernel starts a native framed peer on every supported platform.
func writeFakeKernel(t *testing.T) string {
	t.Helper()
	return testexec.RPCServer(t, sidecarclient.RequiredMethods)
}

func TestRunPrintModeWithFakeKernel(t *testing.T) {
	if _, err := os.Stat("../../go.mod"); err != nil {
		t.Skip("非仓库环境")
	}
	core := writeFakeKernel(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("EOS_CORE_PATH", core)
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")
	t.Setenv("EOS_CORE_STORE_DIR", filepath.Join(home, ".eos", "core"))
	t.Chdir(t.TempDir())

	for _, format := range []string{"text", "json", "stream-json"} {
		t.Run(format, func(t *testing.T) {
			if err := RunPrintMode(PrintOptions{Query: "hi", OutputFormat: format}); err != nil {
				logBytes, _ := os.ReadFile(filepath.Join(home, "Library", "Logs", "EOS", "core", "eos-core.log"))
				t.Fatalf("RunPrintMode(%s): %v\n--- core log ---\n%s", format, err, logBytes)
			}
		})
	}

	// 空查询校验臂（引擎就绪前不触发——runSingleTurn 内校验）。
	if err := RunPrintMode(PrintOptions{Query: "", OutputFormat: "json"}); err == nil {
		t.Fatal("空查询应报错")
	}
}

func newFakeKernelEnv(t *testing.T) {
	t.Helper()
	core := writeFakeKernel(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("EOS_CORE_PATH", core)
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")
	t.Setenv("EOS_CORE_STORE_DIR", filepath.Join(home, ".eos", "core"))
	t.Chdir(t.TempDir())
}

func TestRunExecWithFakeKernel(t *testing.T) {
	if _, err := os.Stat("../../go.mod"); err != nil {
		t.Skip("非仓库环境")
	}
	newFakeKernelEnv(t)
	for _, output := range []string{"text", "json", "stream-json"} {
		t.Run(output, func(t *testing.T) {
			if err := runExec(context.Background(), execOptions{Prompt: "hi", Output: output}); err != nil {
				t.Fatalf("runExec(%s): %v", output, err)
			}
		})
	}
}

func TestResolveExecPrompt(t *testing.T) {
	if got, err := resolveExecPrompt("普通"); err != nil || got != "普通" {
		t.Fatalf("普通参数 = %q, %v", got, err)
	}
	// stdin 读提示词。
	r, w, _ := os.Pipe()
	w.WriteString("  来自 stdin 的提示  ")
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
	got, err := resolveExecPrompt("-")
	if err != nil || strings.TrimSpace(got) == "" {
		t.Fatalf("stdin 提示 = %q, %v", got, err)
	}
	// 空 stdin。
	r2, w2, _ := os.Pipe()
	w2.Close()
	os.Stdin = r2
	if _, err := resolveExecPrompt("-"); err == nil {
		t.Fatal("空 stdin 应报错")
	}
	os.Stdin = old
}
