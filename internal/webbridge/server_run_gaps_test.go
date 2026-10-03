package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// server Run 主链批测：说话脚本内核驱动整条 web 服务装配（桥启动/
// 事件扇出/HTTP 监听/ctx 取消优雅关停），外加 openBrowser 拦截臂。

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newSpeakingCoreForServer 与 cli 包同款帧应答脚本内核（对所有请求回 {}）。
func newSpeakingCoreForServer(t *testing.T) string {
	t.Helper()
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
	path := filepath.Join(t.TempDir(), "eos-core")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write core: %v", err)
	}
	return path
}

func TestServerRunFullLifecycleWithSpeakingCore(t *testing.T) {
	if _, err := os.Stat("../../go.mod"); err != nil {
		t.Skip("非仓库环境")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("EOS_CORE_PATH", "")
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")

	uiDir := t.TempDir()
	os.WriteFile(filepath.Join(uiDir, "index.html"), []byte("<html><head></head><body>ok</body></html>"), 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, ServerOptions{
			UIDir:         uiDir,
			ListenAddr:    "127.0.0.1:0",
			NoOpenBrowser: true,
			CorePath:      newSpeakingCoreForServer(t),
		})
	}()

	// 给桥装配+HTTP 监听留时间，然后取消触发优雅关停。
	time.Sleep(1200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run 收尾 = %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run 未在取消后收口")
	}
}

func TestOpenBrowserIntercepted(t *testing.T) {
	fakeBin := t.TempDir()
	calls := filepath.Join(fakeBin, "open-calls")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + calls + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "open"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	openBrowser("http://127.0.0.1:1")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if data, err := os.ReadFile(calls); err == nil && len(data) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("openBrowser 未被拦截")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
