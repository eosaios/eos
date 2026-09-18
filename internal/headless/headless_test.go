package headless_test

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// 迁移自 internal/cli/print_test.go（会话/订阅原语随实现移入本包）。

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/eosaios/eos/internal/headless"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	"github.com/eosaios/eos/pkg/protocol"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// headlessSessionCaller 是记录调用并回放固定应答的 Caller 假实现。
type headlessSessionCaller struct {
	mu      sync.Mutex
	calls   []headlessSessionCall
	replies map[string]any
}

type headlessSessionCall struct {
	method string
	params any
}

func (c *headlessSessionCaller) Call(_ context.Context, method string, params any, out any) error {
	c.mu.Lock()
	c.calls = append(c.calls, headlessSessionCall{method: method, params: params})
	reply := c.replies[method]
	c.mu.Unlock()
	if out == nil || reply == nil {
		return nil
	}
	data, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (c *headlessSessionCaller) hasMethod(method string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, call := range c.calls {
		if call.method == method {
			return true
		}
	}
	return false
}

func (c *headlessSessionCaller) firstParams(method string) any {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, call := range c.calls {
		if call.method == method {
			return call.params
		}
	}
	return nil
}

func TestEnsureSessionUsesCurrentSession(t *testing.T) {
	caller := &headlessSessionCaller{replies: map[string]any{
		protocoljsonrpc.MethodStateSnapshot:  coreapi.StateSnapshot{ForegroundWorkspace: "C:/work/current"},
		protocoljsonrpc.MethodSessionCurrent: coreapi.Session{ID: "sess-current", WorkspaceRoot: "C:/work/current"},
	}}
	engine := sidecar.NewRemoteEngine(caller)

	session, err := headless.EnsureSession(context.Background(), engine)
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
	if session.ID != "sess-current" {
		t.Fatalf("session.ID=%q, want sess-current", session.ID)
	}
	if caller.hasMethod(protocoljsonrpc.MethodSessionCreate) {
		t.Fatal("EnsureSession created a new session despite current session existing")
	}
	params, ok := caller.firstParams(protocoljsonrpc.MethodSessionCurrent).(coreapi.CurrentSessionRequest)
	if !ok {
		t.Fatalf("session/current params type = %T", caller.firstParams(protocoljsonrpc.MethodSessionCurrent))
	}
	if params.WorkspaceRoot != "C:/work/current" {
		t.Fatalf("workspace_root=%q, want C:/work/current", params.WorkspaceRoot)
	}
}

func TestEnsureSessionCreatesWhenCurrentMissing(t *testing.T) {
	caller := &headlessSessionCaller{replies: map[string]any{
		protocoljsonrpc.MethodStateSnapshot: coreapi.StateSnapshot{ForegroundWorkspace: "C:/work/new"},
		protocoljsonrpc.MethodSessionCreate: coreapi.Session{ID: "sess-new", WorkspaceRoot: "C:/work/new"},
	}}
	engine := sidecar.NewRemoteEngine(caller)

	session, err := headless.EnsureSession(context.Background(), engine)
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
	if session.ID != "sess-new" {
		t.Fatalf("session.ID=%q, want sess-new", session.ID)
	}
	params, ok := caller.firstParams(protocoljsonrpc.MethodSessionCreate).(coreapi.CreateSessionRequest)
	if !ok {
		t.Fatalf("session/create params type = %T", caller.firstParams(protocoljsonrpc.MethodSessionCreate))
	}
	if params.WorkspaceRoot != "C:/work/new" {
		t.Fatalf("workspace_root=%q, want C:/work/new", params.WorkspaceRoot)
	}
}

func TestSubscribeTurnEventsIncludesSessionFilter(t *testing.T) {
	caller := &headlessSessionCaller{replies: map[string]any{
		protocoljsonrpc.MethodEventSubscribe: map[string]any{"subscription_id": "sub-1"},
	}}
	engine := sidecar.NewRemoteEngine(caller)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := headless.SubscribeTurnEvents(ctx, engine, "sess-1", "turn-1")
	if ch == nil {
		t.Fatal("SubscribeTurnEvents returned nil channel")
	}
	params, ok := caller.firstParams(protocoljsonrpc.MethodEventSubscribe).(coreapi.EventSubscribeRequest)
	if !ok {
		t.Fatalf("event/subscribe params type = %T", caller.firstParams(protocoljsonrpc.MethodEventSubscribe))
	}
	if params.SessionID != "sess-1" || params.TurnID != "turn-1" {
		t.Fatalf("event filter session=%q turn=%q, want sess-1/turn-1", params.SessionID, params.TurnID)
	}
}

func TestNormalizeEventKeepsOriginalTypeAndFillsFailure(t *testing.T) {
	ev := protocol.Envelope{
		EventType: protocol.EventTypeTurnCancelled,
		Payload:   map[string]any{"session_id": "s", "turn_id": "t"},
	}
	eventType, payload, raw := headless.NormalizeEvent(ev)
	if eventType != protocol.EventTypeRequestFailed {
		t.Fatalf("eventType=%q, want %q", eventType, protocol.EventTypeRequestFailed)
	}
	if raw != protocol.EventTypeTurnCancelled {
		t.Fatalf("raw=%q, want %q", raw, protocol.EventTypeTurnCancelled)
	}
	if payload["original_event_type"] != string(protocol.EventTypeTurnCancelled) {
		t.Fatalf("payload should keep original_event_type, got %v", payload["original_event_type"])
	}
	if headless.FailureMessage(raw, payload) != "request cancelled" {
		t.Fatalf("FailureMessage=%q, want request cancelled", headless.FailureMessage(raw, payload))
	}
}
