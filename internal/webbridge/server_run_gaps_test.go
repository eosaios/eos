package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// server Run 主链批测：原生测试内核驱动整条 web 服务装配（桥启动/
// 事件扇出/HTTP 监听/ctx 取消优雅关停），外加 openBrowser 拦截臂。

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/testexec"
)

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
	if err := os.WriteFile(filepath.Join(uiDir, "index.html"), []byte("<html><head></head><body>ok</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	core := testexec.RPCServer(t, nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, ServerOptions{
			UIDir:         uiDir,
			ListenAddr:    address,
			NoOpenBrowser: true,
			CorePath:      core,
		})
	}()

	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case err := <-done:
			t.Fatalf("Run exited before HTTP readiness: %v", err)
		default:
		}
		response, err := client.Get("http://" + address + "/")
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("read HTTP response: %v, %v", readErr, closeErr)
			}
			if response.StatusCode == http.StatusOK && strings.Contains(string(body), "<body>ok</body>") {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("web server did not serve its index page")
		}
		time.Sleep(20 * time.Millisecond)
	}

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
	_, calls := installOpenCommandFixtures(t)

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
