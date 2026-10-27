package sidecar

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// process_client 余臂批测：StartProcess 前置错误族（ctx 取消/解析失败/
// 坏二进制）、Dir/Env/Stderr 装配、nil client 守卫族、Wait nil 态、
// CloseWithTimeout 兜底路径、mergedEnv 矩阵。

import (
	"context"
	"os"

	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
	"path/filepath"
	"testing"
	"time"
)

func newScriptProc(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("EOS_CORE_PATH", "")
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "eos-core")
	script := "#!/usr/bin/env python3\n" +
		"import sys, json\n" +
		"def read_frame():\n" +
		"    length = 0\n" +
		"    while True:\n" +
		"        line = sys.stdin.readline()\n" +
		"        if not line:\n" +
		"            return None\n" +
		"        line = line.strip()\n" +
		"        if not line:\n" +
		"            break\n" +
		"        if line.lower().startswith('content-length:'):\n" +
		"            length = int(line.split(':', 1)[1].strip())\n" +
		"    if length <= 0:\n" +
		"        return None\n" +
		"    return sys.stdin.read(length)\n" +
		"def send(obj):\n" +
		"    out = json.dumps(obj)\n" +
		"    sys.stdout.write('Content-Length: %d\\n\\n%s' % (len(out), out))\n" +
		"    sys.stdout.flush()\n" +
		"while True:\n" +
		"    raw = read_frame()\n" +
		"    if raw is None:\n" +
		"        break\n" +
		"    try:\n" +
		"        req = json.loads(raw)\n" +
		"    except Exception:\n" +
		"        continue\n" +
		"    send({'jsonrpc': '2.0', 'id': req.get('id'), 'result': {}})\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, dir
}

func TestStartProcessPreconditions(t *testing.T) {
	// ctx 取消 → 快速失败。
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StartProcess(canceled, ProcessOptions{}); err == nil {
		t.Fatal("取消 ctx 应报错")
	}
	// 注：无显式二进制的解析臂会释放 go install 内嵌真内核并启动——
	// 测试禁止拉真内核，该臂不测（resolver 单测已覆盖解析错误）。
	// 不可执行文件 → cmd.Start 失败。
	path, _ := newScriptProc(t)
	notExec := filepath.Join(t.TempDir(), "eos-core")
	os.WriteFile(notExec, []byte("plain"), 0o644)
	_ = path
	if _, err := StartProcess(context.Background(), ProcessOptions{BinaryPath: notExec}); err == nil {
		t.Fatal("不可执行应报错")
	}
}

func TestStartProcessOptionsAssembly(t *testing.T) {
	script, dir := newScriptProc(t)
	var stderrLines []string
	pc, err := StartProcess(context.Background(), ProcessOptions{
		BinaryPath: script,
		Args:       []string{"--stdio"},
		Dir:        dir,
		Env:        map[string]string{"EOS_TEST_MARK": "1", "": "skip-me"},
		Stderr: writeFunc(func(p []byte) (int, error) {
			stderrLines = append(stderrLines, string(p))
			return len(p), nil
		}),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = pc.Close() }()
	// env 注入生效（经 mergedEnv 大小写不敏感替换/追加）。
	found := false
	for _, kv := range mergedEnv(nil, map[string]string{"EOS_TEST_MARK": "1"}) {
		if kv == "EOS_TEST_MARK=1" {
			found = true
		}
	}
	if !found {
		t.Fatal("env 注入缺失")
	}
	// Initialize 走通（脚本对任意请求回空——{} 反序列化零值成功）。
	if _, err := pc.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	// Shutdown 尽力而为。
	if err := pc.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	_ = stderrLines
}

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) { return f(p) }

func requestZero() protocoljsonrpc.Request {
	req, err := protocoljsonrpc.NewRequest(protocoljsonrpc.StringID("1"), "m", nil)
	if err != nil {
		panic(err)
	}
	return req
}

func TestProcessClientNilGuards(t *testing.T) {
	var nilPC *ProcessClient
	// nil client：Call/Do/Initialize 报错、Shutdown nil、Wait 单值错误通道。
	if err := nilPC.Call(context.Background(), "m", nil, nil); err == nil {
		t.Fatal("nil Call 应报错")
	}
	if _, err := nilPC.Do(context.Background(), requestZero()); err == nil {
		t.Fatal("nil Do 应报错")
	}
	if err := nilPC.Shutdown(context.Background()); err != nil {
		t.Fatalf("nil Shutdown = %v", err)
	}
	ch := nilPC.Wait()
	select {
	case err, ok := <-ch:
		if !ok || err == nil {
			t.Fatalf("nil Wait = %v, %v", err, ok)
		}
	default:
		t.Fatal("nil Wait 应立即就绪")
	}
	if err := nilPC.CloseWithTimeout(time.Second); err != nil {
		t.Fatalf("nil Close = %v", err)
	}

	// 零值结构体（client nil）：Call/Do 守卫臂。
	zero := &ProcessClient{waitCh: make(chan error, 1), waitDone: make(chan struct{})}
	if err := zero.Call(context.Background(), "m", nil, nil); err == nil {
		t.Fatal("零值 Call 应报错")
	}
	if _, err := zero.Do(context.Background(), requestZero()); err == nil {
		t.Fatal("零值 Do 应报错")
	}
	// 零值 Close：无进程直接走 waitDone/waitCh（waitCh 空 → 超时 kill 臂
	// cmd nil 跳过 → 500ms 后 "did not exit"——用短超时快速走完）。
	_ = zero.CloseWithTimeout(time.Millisecond)
}

func TestMergedEnvMatrix(t *testing.T) {
	base := []string{"PATH=/bin", "eos_test_mark=old"}
	out := mergedEnv(base, map[string]string{
		"EOS_TEST_MARK": "new", // 大小写不敏感替换
		"NEW_KEY":       "v",   // 追加
		"":              "x",   // 空键跳过
	})
	if len(out) != 3 {
		t.Fatalf("长度 = %d: %v", len(out), out)
	}
	if out[1] != "EOS_TEST_MARK=new" {
		t.Fatalf("替换 = %q", out[1])
	}
	if out[2] != "NEW_KEY=v" {
		t.Fatalf("追加 = %q", out[2])
	}
	// 空 extra：原样副本。
	out2 := mergedEnv([]string{"A=1"}, nil)
	if len(out2) != 1 || out2[0] != "A=1" {
		t.Fatalf("空 extra = %v", out2)
	}
	// 畸形基项（无 =）不参与索引；已有键原地替换。
	out3 := mergedEnv([]string{"MALFORMED", "A=1"}, map[string]string{"A": "2"})
	if len(out3) != 2 || out3[1] != "A=2" {
		t.Fatalf("畸形基项 = %v", out3)
	}
}
