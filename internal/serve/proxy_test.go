package serve

// internal/serve/proxy_test.go 验证透传代理的核心行为：
//   - 任意 method 原样转发给 Caller（method + params 无损）
//   - initialize 在本地响应、methods 来自 InitResult 或 AllCoreMethods 降级
//   - 未知 method 由 Router 返回 MethodNotFound
//   - shutdown 返回空 map

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// recordingCaller 记录所有 Call 的 method + params，并按 method 返回预设结果。
type recordingCaller struct {
	calls    []recordedCall
	results  map[string]json.RawMessage // method -> result
	failWith map[string]error           // method -> error
}

type recordedCall struct {
	method string
	params json.RawMessage
}

func (c *recordingCaller) Call(_ context.Context, method string, params any, out any) error {
	var raw json.RawMessage
	if p, ok := params.(json.RawMessage); ok {
		raw = p
	} else if params != nil {
		raw, _ = json.Marshal(params)
	}
	c.calls = append(c.calls, recordedCall{method: method, params: raw})
	if err, hasErr := c.failWith[method]; hasErr {
		return err
	}
	result, ok := c.results[method]
	if !ok {
		return nil
	}
	// 把预设结果 unmarshal 进 out（out 是 *json.RawMessage）
	if ptr, ok := out.(*json.RawMessage); ok {
		*ptr = result
	}
	return nil
}

func TestForwardTransparentlyPassesMethodAndParams(t *testing.T) {
	caller := &recordingCaller{
		results: map[string]json.RawMessage{
			"session/current": json.RawMessage(`{"id":"sess-1"}`),
		},
	}
	router, err := NewRouter(Options{Caller: caller})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	params := json.RawMessage(`{"workspace_root":"/abs/path"}`)
	resp := router.Handle(context.Background(), jsonrpc.Request{
		ID: jsonrpc.NumberID(1), Method: "session/current", Params: params,
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if len(caller.calls) != 1 || caller.calls[0].method != "session/current" {
		t.Fatalf("expected 1 forwarded call session/current, got %+v", caller.calls)
	}
	if string(caller.calls[0].params) != string(params) {
		t.Fatalf("params not passed through: got %s, want %s", caller.calls[0].params, params)
	}
	if !strings.Contains(string(resp.Result), "sess-1") {
		t.Fatalf("result not returned: %s", resp.Result)
	}
}

func TestForwardPreservesNilParams(t *testing.T) {
	caller := &recordingCaller{
		results: map[string]json.RawMessage{"workspace/list": json.RawMessage(`[]`)},
	}
	router, _ := NewRouter(Options{Caller: caller})

	router.Handle(context.Background(), jsonrpc.Request{
		ID: jsonrpc.NumberID(1), Method: "workspace/list",
	})
	if len(caller.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(caller.calls))
	}
	if caller.calls[0].params != nil {
		t.Fatalf("expected nil params forwarded, got %s", caller.calls[0].params)
	}
}

func TestInitializeReturnsInitResultMethods(t *testing.T) {
	caller := &recordingCaller{}
	wantMethods := []string{"initialize", "session/create", "tool/execute"}
	router, _ := NewRouter(Options{
		Caller:     caller,
		InitResult: initResultWith(wantMethods),
	})

	resp := router.Handle(context.Background(), jsonrpc.Request{
		ID: jsonrpc.NumberID(1), Method: "initialize",
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	var got struct {
		Methods []string `json:"methods"`
	}
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Methods) != len(wantMethods) {
		t.Fatalf("methods=%v, want %v", got.Methods, wantMethods)
	}
	// initialize 不应转发给 caller
	if len(caller.calls) != 0 {
		t.Fatalf("initialize must not forward to caller, got %+v", caller.calls)
	}
}

func TestInitializeFallsBackToAllCoreMethods(t *testing.T) {
	caller := &recordingCaller{}
	router, _ := NewRouter(Options{Caller: caller}) // InitResult 为空

	resp := router.Handle(context.Background(), jsonrpc.Request{
		ID: jsonrpc.NumberID(1), Method: "initialize",
	})
	var got struct {
		Methods []string `json:"methods"`
	}
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Methods) == 0 {
		t.Fatal("fallback methods should be AllCoreMethods, got empty")
	}
	// 应包含 initialize 和至少一个核心 method
	found := map[string]bool{}
	for _, m := range got.Methods {
		found[m] = true
	}
	if !found["initialize"] || !found["session/create"] {
		t.Fatalf("fallback methods missing core ones: %v", got.Methods)
	}
}

func TestUnknownMethodReturnsMethodNotFound(t *testing.T) {
	caller := &recordingCaller{}
	router, _ := NewRouter(Options{Caller: caller})

	resp := router.Handle(context.Background(), jsonrpc.Request{
		ID: jsonrpc.NumberID(1), Method: "totally/made/up",
	})
	if resp.Error == nil || resp.Error.Code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("expected MethodNotFound, got: %+v", resp.Error)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("unknown method must not reach caller, got %+v", caller.calls)
	}
}

func TestForwardMapsCallerErrorToRPCError(t *testing.T) {
	caller := &recordingCaller{
		failWith: map[string]error{"tool/execute": errors.New("sandbox denied")},
	}
	router, _ := NewRouter(Options{Caller: caller})

	resp := router.Handle(context.Background(), jsonrpc.Request{
		ID: jsonrpc.NumberID(1), Method: "tool/execute",
		Params: json.RawMessage(`{"name":"bash"}`),
	})
	if resp.Error == nil {
		t.Fatal("expected error from caller failure")
	}
	if resp.Error.Code != jsonrpc.CodeInternalError {
		t.Fatalf("error code=%d, want %d", resp.Error.Code, jsonrpc.CodeInternalError)
	}
	if !strings.Contains(resp.Error.Message, "sandbox denied") {
		t.Fatalf("error message lost: %s", resp.Error.Message)
	}
}

func TestShutdownReturnsEmptyObject(t *testing.T) {
	caller := &recordingCaller{}
	router, _ := NewRouter(Options{Caller: caller})

	resp := router.Handle(context.Background(), jsonrpc.Request{
		ID: jsonrpc.NumberID(1), Method: "shutdown",
	})
	if resp.Error != nil {
		t.Fatalf("shutdown error: %v", resp.Error)
	}
	if strings.TrimSpace(string(resp.Result)) != "{}" {
		t.Fatalf("shutdown result=%s, want {}", resp.Result)
	}
}

func TestNewRouterRequiresCaller(t *testing.T) {
	if _, err := NewRouter(Options{}); err == nil {
		t.Fatal("NewRouter without caller must error")
	}
}

// initResultWith 构造一个带指定 methods 的 InitializeResult，用于测试。
func initResultWith(methods []string) coreapijsonrpc.InitializeResult {
	return coreapijsonrpc.InitializeResult{
		ServerName:      "eos-core-test",
		ProtocolVersion: "test-1",
		Methods:         methods,
	}
}
