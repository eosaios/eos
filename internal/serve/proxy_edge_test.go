package serve

// internal/serve/proxy_edge_test.go 补齐透传代理的边角分支：
//   - rawJSON 空 payload 序列化为 {}（防御分支，直接单测）
//   - firstNonEmpty 全空输入返回空串
//   - initialize 的 ServerName 回落 eos-serve
//   - forward 收到空 result（内核无返回值）时响应空对象
//
// 不可达清单：
//   - NewRouter 中 router.Register 失败的三个错误分支：Register 仅在
//     method 为空时失败，而 initialize/shutdown 常量与 AllCoreMethods()
//     条目恒非空且循环内已跳过重复注册，无构造路径可触发（防御性兜底）。

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

func TestRawJSONEmptyMarshalsAsEmptyObject(t *testing.T) {
	out, err := json.Marshal(rawJSON(nil))
	if err != nil {
		t.Fatalf("marshal empty rawJSON: %v", err)
	}
	if string(out) != "{}" {
		t.Fatalf("empty rawJSON marshals to %s, want {}", out)
	}
	// 非空 payload 原样输出
	out, err = json.Marshal(rawJSON(json.RawMessage(`{"ok":true}`)))
	if err != nil || string(out) != `{"ok":true}` {
		t.Fatalf("raw passthrough got %s, %v", out, err)
	}
}

func TestFirstNonEmptyAllEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", ""); got != "" {
		t.Fatalf("firstNonEmpty(all empty)=%q, want empty", got)
	}
	if got := firstNonEmpty("", "x", "y"); got != "x" {
		t.Fatalf("firstNonEmpty skip empty=%q, want x", got)
	}
}

func TestInitializeFallsBackServerName(t *testing.T) {
	caller := &recordingCaller{}
	router, err := NewRouter(Options{Caller: caller})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	resp := router.Handle(context.Background(), jsonrpc.Request{
		ID: jsonrpc.NumberID(1), Method: jsonrpc.MethodInitialize,
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	var result struct {
		ServerName string   `json:"server_name"`
		Methods    []string `json:"methods"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal initialize result: %v", err)
	}
	if result.ServerName != "eos-serve" {
		t.Fatalf("serverName=%q, want eos-serve fallback", result.ServerName)
	}
	if len(result.Methods) == 0 {
		t.Fatal("methods should fall back to AllCoreMethods")
	}
}

func TestForwardEmptyResultReturnsEmptyObject(t *testing.T) {
	// recordingCaller 无预设结果时 out 不被写入：模拟内核无返回值的 method
	caller := &recordingCaller{}
	router, _ := NewRouter(Options{Caller: caller})

	// 任一非 initialize/shutdown 的核心 method（recordingCaller 无预设结果
	// 即模拟内核无返回值）
	method := ""
	for _, m := range jsonrpc.AllCoreMethods() {
		if m != jsonrpc.MethodInitialize && m != jsonrpc.MethodShutdown {
			method = m
			break
		}
	}
	if method == "" {
		t.Fatal("no core method available to test")
	}
	resp := router.Handle(context.Background(), jsonrpc.Request{
		ID: jsonrpc.NumberID(1), Method: method,
	})
	if resp.Error != nil {
		t.Fatalf("forward error: %v", resp.Error)
	}
	got := strings.TrimSpace(string(resp.Result))
	if got != "{}" {
		t.Fatalf("empty result forwarded as %s, want {}", got)
	}
}
