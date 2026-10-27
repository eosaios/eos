package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// RunPrintMode 假内核批测：python 脚本内核（Content-Length 帧）应答
// initialize/session/turn 全链并推送 item.delta + turn.completed 事件，
// 驱动 text/json/stream-json 三种输出格式的主路径。

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sidecarclient "github.com/eosaios/eos/pkg/coreapi/sidecar/client"
)

// writeFakeKernel 写智能脚本内核：按 method 查表应答；turn/start 后推送
// delta 与完成事件（session/turn id 从请求回显，保证订阅过滤命中）。
func writeFakeKernel(t *testing.T) string {
	t.Helper()
	methods, _ := json.Marshal(sidecarclient.RequiredMethods)
	script := strings.ReplaceAll(`#!/usr/bin/env python3
import os, sys, json

METHODS = __METHODS__

def read_frame():
    length = 0
    while True:
        line = sys.stdin.readline()
        if not line:
            return None
        line = line.strip()
        if not line:
            break
        if line.lower().startswith('content-length:'):
            length = int(line.split(':', 1)[1].strip())
    if length <= 0:
        return None
    return sys.stdin.read(length)

def send(obj):
    out = json.dumps(obj)
    sys.stdout.write('Content-Length: %d\n\n%s' % (len(out), out))
    sys.stdout.flush()

def notify(event_type, session_id, turn_id, payload):
    send({'jsonrpc': '2.0', 'method': 'event', 'params': {
        'event_type': event_type,
        'session_id': session_id,
        'turn_id': turn_id,
        'source': 'runtime',
        'payload': payload,
    }})

def reply(req, result):
    send({'jsonrpc': '2.0', 'id': req.get('id'), 'result': result})

while True:
    raw = read_frame()
    if raw is None:
        break
    try:
        req = json.loads(raw)
    except Exception:
        continue
    method = req.get('method', '')
    params = req.get('params') or {}
    if method == 'initialize':
        reply(req, {'server_name': 'stub-core', 'protocol_version': '1', 'methods': METHODS})
    elif method == 'session/current':
        reply(req, {'id': '', 'workspace_root': os.getcwd()})
    elif method == 'session/create':
        reply(req, {'id': 's-stub', 'workspace_root': os.getcwd()})
    elif method == 'session/resume':
        reply(req, {'id': params.get('session_id', 's-stub'), 'workspace_root': os.getcwd()})
    elif method == 'turn/start':
        sid = params.get('session_id', 's-stub')
        tid = params.get('turn_id', 't-stub')
        reply(req, {'id': tid, 'session_id': sid, 'status': 'running'})
        notify('turn.item_started', sid, tid, {'item_id': 'it-1'})
        notify('turn.item_delta', sid, tid, {'delta': '你好，', 'delta_type': 'text'})
        notify('turn.item_delta', sid, tid, {'delta': '世界', 'delta_type': 'text'})
        notify('turn.completed', sid, tid, {})
    elif method == 'event/subscribe':
        reply(req, {'subscription_id': 'sub-stub'})
    elif method == 'event/unsubscribe':
        reply(req, {})
    else:
        reply(req, {})
`, "__METHODS__", string(methods))
	path := filepath.Join(t.TempDir(), "eos-core")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kernel: %v", err)
	}
	return path
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
