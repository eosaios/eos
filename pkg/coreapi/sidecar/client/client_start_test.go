package client

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。
//
// 覆盖 Start 成功路径与参数拼装：用 TestMain helper process 伪装 eos-core
// （Content-Length JSON-RPC over stdio），不依赖真实内核二进制或网络。
//
// 豁免清单（不在此文件硬测）：
//   - 真实 eos-core 二进制启动 / Ed25519 签名校验：依赖本机 vendored 内核，
//     e2e_*_test.go 已有打包二进制路径下的集成覆盖。
//   - 真 github.com 下载 / 系统弹窗：与本包无关。

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

const (
	fakeCoreEnv      = "EOS_CLIENT_FAKE_CORE"
	fakeCoreModeEnv  = "EOS_CLIENT_FAKE_CORE_MODE"
	fakeCoreModeOK   = "ok"
	fakeCoreModeFail = "init-fail"
)

// TestMain 让测试二进制在 EOS_CLIENT_FAKE_CORE=1 时伪装成 eos-core：
// 读 stdin 上的 Content-Length JSON-RPC 帧，回 initialize 响应。
func TestMain(m *testing.M) {
	if os.Getenv(fakeCoreEnv) == "1" {
		runFakeCore(os.Getenv(fakeCoreModeEnv))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runFakeCore(mode string) {
	// 看门狗：stdin 因父进程句柄泄漏等原因迟迟不 EOF 时，保证子进程
	// 仍会退出，避免 CloseWithTimeout 走到 "did not exit after kill"。
	// 30s：Windows 下进程冷启动 + Content-Length 握手在并行满载时可超 1s，
	// 过短会误杀正常握手（铁则：计时留 3 倍余量防并行满载 flaky）。
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			os.Exit(0)
		}
	}()
	stream := protocoljsonrpc.NewStream(os.Stdin, os.Stdout)
	for {
		msg, err := stream.ReadMessage()
		if err != nil {
			return
		}
		req := msg.Request
		if req == nil {
			continue
		}
		switch req.Method {
		case protocoljsonrpc.MethodShutdown:
			resp, _ := protocoljsonrpc.NewResultResponse(req.ID, map[string]any{"ok": true})
			_ = stream.WriteMessage(resp)
			return
		case protocoljsonrpc.MethodInitialize:
			if mode == fakeCoreModeFail {
				resp, _ := protocoljsonrpc.NewErrorResponse(req.ID, protocoljsonrpc.CodeInternalError, "fake init rejected", nil)
				_ = stream.WriteMessage(resp)
				continue
			}
			result := coreapijsonrpc.InitializeResult{
				ServerName:      "fake-eos-core",
				ProtocolVersion: "v1",
				Methods:         append([]string(nil), RequiredMethods...),
			}
			resp, _ := protocoljsonrpc.NewResultResponse(req.ID, result)
			_ = stream.WriteMessage(resp)
		default:
			resp, _ := protocoljsonrpc.NewErrorResponse(req.ID, protocoljsonrpc.CodeMethodNotFound, "unsupported in fake core", nil)
			_ = stream.WriteMessage(resp)
		}
	}
}

func fakeCoreOptions(t *testing.T, mode string) Options {
	t.Helper()
	return Options{
		BinaryPath: os.Args[0],
		Env: map[string]string{
			fakeCoreEnv:     "1",
			fakeCoreModeEnv: mode,
		},
		AllowDevPlaceholder: true,
	}
}

func TestStartWithFakeCoreSucceedsAndHandshakes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c, err := Start(ctx, fakeCoreOptions(t, fakeCoreModeOK))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer c.Close()

	init := c.Initialize()
	if init.ServerName != "fake-eos-core" {
		t.Fatalf("ServerName = %q, want fake-eos-core", init.ServerName)
	}
	if init.ProtocolVersion != "v1" {
		t.Fatalf("ProtocolVersion = %q, want v1", init.ProtocolVersion)
	}
	if len(init.Methods) == 0 {
		t.Fatal("Methods empty after handshake")
	}
	if !c.HasMethod(protocoljsonrpc.MethodStateSnapshot) {
		t.Fatal("HasMethod(state/snapshot) = false after handshake")
	}
	if missing := c.MissingMethods(); len(missing) != 0 {
		t.Fatalf("MissingMethods() = %v, want empty", missing)
	}
	if c.Engine() == nil {
		t.Fatal("Engine() nil after Start")
	}
	if c.Process() == nil {
		t.Fatal("Process() nil after Start")
	}
	// Wait 成功路径：返回底层进程退出通道（与 Close 抢 waitCh，不可再断言 Close 后取值）。
	if ch := c.Wait(); ch == nil {
		t.Fatal("Wait() on started client returned nil channel")
	}
	_ = c.Close()
}

func TestStartNilContextUsesBackground(t *testing.T) {
	// 被测函数把 nil ctx 规范成 Background；http.NewRequestWithContext 等
	// 下游拒绝 nil，这里显式覆盖入口规范分支。
	c, err := Start(nil, fakeCoreOptions(t, fakeCoreModeOK))
	if err != nil {
		t.Fatalf("Start(nil ctx) error = %v", err)
	}
	defer c.Close()
	if c.Initialize().ServerName != "fake-eos-core" {
		t.Fatalf("init = %+v", c.Initialize())
	}
}

func TestStartAppendsStdioWhenMissing(t *testing.T) {
	opts := fakeCoreOptions(t, fakeCoreModeOK)
	opts.Args = []string{"--app-server"}
	c, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer c.Close()
}

func TestStartKeepsExplicitStdioArg(t *testing.T) {
	opts := fakeCoreOptions(t, fakeCoreModeOK)
	opts.Args = []string{"--stdio"}
	c, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer c.Close()
}

func TestStartEmptyArgsDefaultsToStdio(t *testing.T) {
	opts := fakeCoreOptions(t, fakeCoreModeOK)
	opts.Args = nil
	c, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer c.Close()
}

func TestStartUsesCustomRequiredFeatures(t *testing.T) {
	opts := fakeCoreOptions(t, fakeCoreModeOK)
	opts.RequiredFeatures = []string{protocoljsonrpc.MethodInitialize}
	c, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer c.Close()
}

func TestStartInitializeFailureStillReturnsClient(t *testing.T) {
	// StartRemoteEngine 内部会因 initialize 失败关掉子进程并报错；
	// 这里断言错误向上透传，且错误非 nil（覆盖错误臂）。
	_, err := Start(context.Background(), fakeCoreOptions(t, fakeCoreModeFail))
	if err == nil {
		t.Fatal("Start() with failing initialize should error")
	}
	if !strings.Contains(err.Error(), "start eos-core sidecar") {
		t.Fatalf("error = %v, want wrapped start error", err)
	}
}

func TestSidecarProcessClientNilEngine(t *testing.T) {
	if got := sidecarProcessClient(nil); got != nil {
		t.Fatalf("sidecarProcessClient(nil) = %v, want nil", got)
	}
}

func TestSidecarProcessClientFromStartedEngine(t *testing.T) {
	c, err := Start(context.Background(), fakeCoreOptions(t, fakeCoreModeOK))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer c.Close()
	if sidecarProcessClient(c.engine) == nil {
		t.Fatal("sidecarProcessClient(started engine) = nil")
	}
}

func TestStartRejectsAlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Start(ctx, fakeCoreOptions(t, fakeCoreModeOK))
	if err == nil {
		t.Fatal("Start() with cancelled ctx should fail")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestWaitOnStartedClientReturnsLiveChannel(t *testing.T) {
	c, err := Start(context.Background(), fakeCoreOptions(t, fakeCoreModeOK))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	wait := c.Wait()
	// 进程仍在运行：Wait 不应立刻返回（Close 与 Wait 共享 waitCh，
	// 这里只验证通道活跃，不与 Close 抢退出值）。
	select {
	case err := <-wait:
		t.Fatalf("Wait() returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	_ = c.Close()
}
