package server

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// control 工具注册 lambda 批测：GetTool 取出注册对后直调 Handler，
// 批量点亮 AddTool 注册的一行委托 lambda。

import (
	"context"
	"testing"

	"github.com/eosaios/eos/pkg/protocol"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func newControlServer(t *testing.T) (*server.MCPServer, *MCPHost) {
	t.Helper()
	engine := &controlEngine{
		sessions: controlSessions{messages: assistantMessages()},
		turns:    &controlTurns{},
		events: controlEvents{events: []protocol.Envelope{
			{EventType: protocol.EventTypeRequestDone, Payload: map[string]any{}},
		}},
		approvals: &controlApprovals{},
		inquiries: &controlInquiries{},
		caller:    controlCaller{},
	}
	host := newControlHost(engine)
	s := server.NewMCPServer("test", "0")
	host.registerControlTools(s)
	return s, host
}

func TestControlToolLambdasInvoke(t *testing.T) {
	s, _ := newControlServer(t)
	ctx := context.Background()

	// 所有 eos_* 工具的注册 lambda：经 GetTool 拿 Handler 直调。
	// 处理器自身对非法/缺参返回错误结果——lambda 只要求委托可达。
	cases := []struct {
		tool string
		args map[string]any
	}{
		{"eos_chat", map[string]any{"message": "hi", "session_id": "sess-mcp"}},
		{"eos_task_wait", map[string]any{"session_id": "sess-mcp"}},
		{"eos_task_status", map[string]any{"session_id": "sess-mcp"}},
		{"eos_task_cancel", map[string]any{"session_id": "sess-mcp"}},
		{"eos_session_create", map[string]any{}},
		{"eos_sessions_list", map[string]any{}},
		{"eos_session_history", map[string]any{"session_id": "sess-mcp"}},
		{"eos_approval_respond", map[string]any{"approval_id": "ap", "decision": "accept"}},
		{"eos_inquiry_respond", map[string]any{"inquiry_id": "iq", "option": "opt"}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			st := s.GetTool(tc.tool)
			if st == nil || st.Handler == nil {
				t.Fatalf("%s 未注册或无 handler", tc.tool)
			}
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tc.args
			// 只要求 lambda 委托可达：错误也是处理器级语义（非 panic）。
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s lambda panic: %v", tc.tool, r)
					}
				}()
				_, _ = st.Handler(ctx, req)
			}()
		})
	}
}

func TestControlToolNamesCovered(t *testing.T) {
	s, _ := newControlServer(t)
	// 注册名清单固化（新增控制工具须同步补 lambda 调用例）。
	names := []string{
		"eos_chat", "eos_task_wait", "eos_task_status", "eos_task_cancel",
		"eos_session_create", "eos_sessions_list", "eos_session_history",
		"eos_approval_respond", "eos_inquiry_respond",
	}
	for _, name := range names {
		if s.GetTool(name) == nil {
			t.Errorf("缺注册 %s", name)
		}
	}
}
