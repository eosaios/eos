package server

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// 批十六批测：New 装配全链（workspace/model override/chatTimeout 透传）/
// resolveExplicitSession 三级 / sessionIDFromMeta / marshalArguments 三态 /
// transport nil 臂。ServeStdio/ServeSSE 真监听豁免（进程级阻塞）。

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestNewHostFieldPropagation(t *testing.T) {
	exec := &mockToolExecutor{}
	e := &mcpTestEngine{
		catalog:  mockCatalog{defs: nil},
		tools:    exec,
		sessions: mockSessions{createdID: "sess-new"},
	}
	s, host, err := New(context.Background(), Options{
		Engine:          e,
		WorkspaceRoot:   " /ws/proj ",
		SessionID:       " sess-explicit ",
		ModelOverride:   " glm-4.7 ",
		ChatTimeoutSecs: 77,
	})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	if s == nil {
		t.Fatal("server nil")
	}
	if host.workspaceRoot != "/ws/proj" {
		t.Fatalf("workspaceRoot = %q", host.workspaceRoot)
	}
	if host.session != "sess-explicit" {
		t.Fatalf("session = %q", host.session)
	}
	if host.modelOverride != "glm-4.7" {
		t.Fatalf("modelOverride = %q", host.modelOverride)
	}
	if host.chatTimeout != 77 || host.chatTimeoutSecs() != 77 {
		t.Fatalf("chatTimeout = %d / %d", host.chatTimeout, host.chatTimeoutSecs())
	}

	// <=0 归一默认。
	_, host2, err := New(context.Background(), Options{Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	if host2.chatTimeoutSecs() != defaultChatTimeoutSecs {
		t.Fatalf("default timeout = %d", host2.chatTimeoutSecs())
	}

	// 控制工具已注册（eos_chat 至少存在）。
	if s.GetTool("eos_chat") == nil {
		t.Fatal("control tool eos_chat not registered")
	}
}

func TestResolveExplicitSessionPriority(t *testing.T) {
	host, _ := newTestHost(t, nil, coreapi.ToolResult{Status: "success"})

	// 参数级最高。
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"session_id": " from-arg "}
	if sid, err := host.resolveExplicitSession(context.Background(), req); err != nil || sid != "from-arg" {
		t.Fatalf("arg level = %q %v", sid, err)
	}

	// _meta 级次之。
	reqMeta := mcp.CallToolRequest{}
	reqMeta.Params.Meta = &mcp.Meta{}
	reqMeta.Params.Meta.AdditionalFields = map[string]any{"session_id": "from-meta"}
	if sid, err := host.resolveExplicitSession(context.Background(), reqMeta); err != nil || sid != "from-meta" {
		t.Fatalf("meta level = %q %v", sid, err)
	}

	// 兜底默认会话（懒创建）。
	if sid, err := host.resolveExplicitSession(context.Background(), mcp.CallToolRequest{}); err != nil || sid != "sess-mcp" {
		t.Fatalf("default session = %q %v", sid, err)
	}

	// meta 非字符串值忽略（回退默认会话）。
	reqBad := mcp.CallToolRequest{}
	reqBad.Params.Meta = &mcp.Meta{}
	reqBad.Params.Meta.AdditionalFields = map[string]any{"session_id": 42}
	if sid, err := host.resolveExplicitSession(context.Background(), reqBad); err != nil || sid != "sess-mcp" {
		t.Fatalf("non-string meta = %q %v", sid, err)
	}
}

func TestMarshalArgumentsThreeStates(t *testing.T) {
	// RawArguments 无损优先。
	req := mcp.CallToolRequest{}
	req.Params.RawArguments = json.RawMessage(`{"a":1}`)
	if got := string(marshalArguments(req)); got != `{"a":1}` {
		t.Fatalf("raw = %s", got)
	}

	// Arguments map 降级序列化。
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]any{"b": "x"}
	if got := string(marshalArguments(req2)); !strings.Contains(got, `"b":"x"`) {
		t.Fatalf("map = %s", got)
	}

	// 空参数 → nil。
	if got := marshalArguments(mcp.CallToolRequest{}); got != nil {
		t.Fatalf("empty = %s", got)
	}
}

func TestServeTransportsRejectNilServer(t *testing.T) {
	if err := ServeStdio(context.Background(), nil); err == nil || err.Error() != "mcp server: nil MCPServer" {
		t.Fatalf("ServeStdio(nil) = %v", err)
	}
	if err := ServeSSE(context.Background(), nil, "127.0.0.1:0"); err == nil || err.Error() != "mcp server: nil MCPServer" {
		t.Fatalf("ServeSSE(nil) = %v", err)
	}
}
