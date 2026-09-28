package sidecar

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestRemoteEngineNilSafety(t *testing.T) {
	var e *RemoteEngine
	if err := e.Close(); err != nil {
		t.Fatalf("nil Close = %v", err)
	}
	if e.ProcessClient() != nil {
		t.Fatal("nil ProcessClient")
	}
	ch := e.Wait()
	if ch == nil {
		t.Fatal("nil Wait")
	}
	select {
	case err := <-ch:
		if err == nil {
			t.Fatal("nil Wait should signal error")
		}
	case <-time.After(time.Second):
		t.Fatal("Wait timeout")
	}

	// 零值 engine：服务访问器应返回非 nil
	zero := &RemoteEngine{}
	if zero.State() == nil || zero.Workspaces() == nil || zero.Sessions() == nil ||
		zero.MCP() == nil || zero.LSP() == nil || zero.Config() == nil ||
		zero.Permissions() == nil || zero.Extensions() == nil || zero.Context() == nil ||
		zero.Usage() == nil || zero.Versions() == nil || zero.Tasks() == nil ||
		zero.Goals() == nil || zero.Modes() == nil || zero.Models() == nil ||
		zero.RemoteWorkspaces() == nil || zero.Git() == nil || zero.Insights() == nil ||
		zero.Memory() == nil || zero.Roles() == nil || zero.Turns() == nil ||
		zero.Approvals() == nil || zero.Inquiries() == nil || zero.Agents() == nil ||
		zero.Tools() == nil || zero.ToolCatalog() == nil || zero.ToolTelemetry() == nil ||
		zero.Events() == nil || zero.Sandbox() == nil || zero.Diagnostics() == nil {
		t.Fatal("service accessors should be non-nil")
	}
	if err := zero.Close(); err != nil {
		t.Fatalf("zero Close = %v", err)
	}
	if zero.ProcessClient() != nil {
		t.Fatal("zero ProcessClient")
	}
	if ch := zero.Wait(); ch == nil {
		t.Fatal("zero Wait")
	}
}

func TestNormalizeRemoteError(t *testing.T) {
	if got := normalizeRemoteError("m", nil); got != nil {
		t.Fatalf("nil = %v", got)
	}
	// unsupported 映射
	err := normalizeRemoteError("m", errors.New("method not found"))
	if !errors.Is(err, coreapi.ErrUnsupported) {
		t.Fatalf("unsupported = %v", err)
	}
	// 其他错误原样
	other := errors.New("boom")
	if got := normalizeRemoteError("m", other); got != other {
		t.Fatalf("other = %v", got)
	}
}
