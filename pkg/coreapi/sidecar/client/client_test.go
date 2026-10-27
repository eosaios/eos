// Copyright (c) 2026 DreamSailing
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

package client

import (
	"context"
	"errors"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

func TestRequiredMethodsCoversTUI(t *testing.T) {
	// 与 pkg/protocol/jsonrpc.AllCoreMethods() 子集关系由 architecture 测试校验。
	// 这里只确保每个方法名非空且格式合法。
	if len(RequiredMethods) == 0 {
		t.Fatalf("RequiredMethods is empty; TUI would refuse every sidecar")
	}
	seen := map[string]bool{}
	for _, m := range RequiredMethods {
		if m == "" {
			t.Fatalf("RequiredMethods contains empty entry")
		}
		if seen[m] {
			t.Fatalf("RequiredMethods duplicate entry: %s", m)
		}
		seen[m] = true
	}
}

func TestAttachNilEngineReturnsNil(t *testing.T) {
	if c := Attach(nil); c != nil {
		t.Fatalf("Attach(nil) = %v, want nil", c)
	}
}

func TestAttachWrapsEngine(t *testing.T) {
	engine := sidecar.NewRemoteEngine(stubCaller{})
	c := Attach(engine)
	if c == nil {
		t.Fatalf("Attach(engine) returned nil")
	}
	if c.Engine() != coreapi.Engine(engine) {
		t.Fatalf("Engine() did not return wrapped engine")
	}
	if c.HasMethod(protocoljsonrpc.MethodStateSnapshot) {
		t.Fatalf("HasMethod should be false for stub caller without initialize")
	}
}

func TestStartFailsWhenBinaryNotFound(t *testing.T) {
	_, err := Start(context.Background(), Options{
		BinaryPath:          "C:/definitely/does/not/exist/eos-core.exe",
		AllowDevPlaceholder: true,
	})
	if err == nil {
		t.Fatalf("Start() should fail when binary path does not exist")
	}
}

func TestStartContextCancelPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Start(ctx, Options{
		BinaryPath:          "C:/definitely/does/not/exist/eos-core.exe",
		AllowDevPlaceholder: true,
	})
	if err == nil {
		t.Fatalf("Start() should fail when ctx is cancelled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
}

func TestHasMethodMatchesInitializeResult(t *testing.T) {
	engine := sidecar.NewRemoteEngine(stubCaller{
		init: coreapijsonrpc.InitializeResult{
			ServerName: "eos-core",
			Methods: []string{
				protocoljsonrpc.MethodInitialize,
				protocoljsonrpc.MethodStateSnapshot,
			},
		},
	})
	c := Attach(engine)
	// Attach 不主动调 initialize，HasMethod 在没 handshake 时为 false。
	if c.HasMethod(protocoljsonrpc.MethodStateSnapshot) {
		t.Fatalf("HasMethod should be false before initialize handshake")
	}
	// 通过 StartWithEngine 注入已 initialize 的 engine。
	c2 := &Client{
		engine: engine,
		init: InitializeResult{
			ServerName: "eos-core",
			Methods: []string{
				protocoljsonrpc.MethodInitialize,
				protocoljsonrpc.MethodStateSnapshot,
			},
		},
	}
	if !c2.HasMethod(protocoljsonrpc.MethodStateSnapshot) {
		t.Fatalf("HasMethod should be true after initialize handshake")
	}
	if c2.HasMethod(protocoljsonrpc.MethodSessionList) {
		t.Fatalf("HasMethod should be false for unannounced method")
	}
}

func TestMissingMethodsReflectsRequired(t *testing.T) {
	c := &Client{
		init: InitializeResult{
			Methods: []string{
				protocoljsonrpc.MethodStateSnapshot,
			},
		},
	}
	missing := c.MissingMethods()
	if len(missing) == 0 {
		t.Fatalf("MissingMethods() should be non-empty when only state/snapshot is announced")
	}
}

func TestWaitOnNilClientReturnsErrorChannel(t *testing.T) {
	c := &Client{}
	ch := c.Wait()
	err, ok := <-ch
	if !ok {
		t.Fatalf("Wait() channel closed before delivering value")
	}
	if err == nil {
		t.Fatalf("Wait() should deliver an error for non-started client")
	}
}

func TestCloseOnNilClientIsNoop(t *testing.T) {
	if err := (*Client)(nil).Close(); err != nil {
		t.Fatalf("Close() on nil client = %v, want nil", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	c := Attach(sidecar.NewRemoteEngine(stubCaller{}))
	if err := c.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
}

type stubCaller struct {
	init coreapijsonrpc.InitializeResult
}

func (s stubCaller) Call(_ context.Context, method string, _ any, out any) error {
	switch method {
	case protocoljsonrpc.MethodInitialize:
		if target, ok := out.(*coreapijsonrpc.InitializeResult); ok {
			*target = s.init
		}
		return nil
	default:
		return coreapi.ErrUnsupported
	}
}
