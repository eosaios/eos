// Package serve 实现 eos serve：把 eos-core 内核 sidecar 的 JSON-RPC 能力
// 通过 stdio 对外暴露为本地工具服务。
//
// 设计：serve 层是透传代理（forwarder），不做任何业务裁决。它为
// jsonrpc.AllCoreMethods() 的每个 method 注册一个通用 forwarder handler，
// handler 内部把请求原样转给内核 sidecar 的 Caller，结果原样返回。所有
// 业务（命令策略/审批/沙箱/turn 编排）都在 Rust 内核，裁决不在壳层
// （AGENTS.md §3）。
package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// Caller 抽象到内核 sidecar 的 JSON-RPC 调用能力。
// *sidecar.ProcessClient 天然满足此接口（Call(ctx, method, params, out)）。
// 在 internal/serve 层用接口而非具体类型，避免耦合 sidecar 包。
type Caller interface {
	Call(ctx context.Context, method string, params any, out any) error
}

// Options 配置 serve 代理。
type Options struct {
	// Caller 到内核 sidecar 的调用通道（通常 = RemoteEngine.ProcessClient()）。
	Caller Caller
	// InitResult 内核 initialize 握手结果，用于本地响应 initialize 方法。
	// 若为空，initialize handler 会返回仅含 methods 的降级结果。
	InitResult coreapijsonrpc.InitializeResult
}

// NewRouter 构造一个把全部核心 method 透传到内核 Caller 的 JSON-RPC Router。
//
// - initialize / shutdown：在 serve 层本地响应（握手是协议元信息，不需转发）。
// - 其余 AllCoreMethods()：注册通用 forwarder，原样 caller.Call 转发。
// - 不在 AllCoreMethods() 中的 method：Router.Handle 自然返回 MethodNotFound。
//
// 返回的 Router 可直接用于 jsonrpc.ServeStream / jsonrpc.ServeWS。
func NewRouter(opts Options) (*jsonrpc.Router, error) {
	if opts.Caller == nil {
		return nil, errors.New("serve: caller is required")
	}
	router := jsonrpc.NewRouter()
	if err := router.Register(jsonrpc.MethodInitialize, handleInitialize(opts.InitResult)); err != nil {
		return nil, fmt.Errorf("serve: register initialize: %w", err)
	}
	if err := router.Register(jsonrpc.MethodShutdown, handleNoContent); err != nil {
		return nil, fmt.Errorf("serve: register shutdown: %w", err)
	}
	// 为除 initialize/shutdown 外的全部核心 method 注册通用透传 handler。
	for _, method := range jsonrpc.AllCoreMethods() {
		if method == jsonrpc.MethodInitialize || method == jsonrpc.MethodShutdown {
			continue
		}
		if err := router.Register(method, forward(opts.Caller, method)); err != nil {
			return nil, fmt.Errorf("serve: register %s: %w", method, err)
		}
	}
	return router, nil
}

// forward 返回一个把请求原样转发给内核 Caller 的 handler。
// params 用 json.RawMessage 透传（不反序列化为具体类型），保证 wire 格式无损。
func forward(caller Caller, method string) jsonrpc.HandlerFunc {
	return func(ctx context.Context, req jsonrpc.Request) (any, *jsonrpc.Error) {
		params := forwardedParams(req.Params)
		var result json.RawMessage
		if err := caller.Call(ctx, method, params, &result); err != nil {
			return nil, jsonrpcError(err)
		}
		// 返回 raw JSON 作为 result，Router 会原样写入响应。
		// 若 result 为空（method 无返回值），返回空 map 保持响应结构合法。
		if len(result) == 0 {
			return map[string]any{}, nil
		}
		return rawJSON(result), nil
	}
}

// forwardedParams 把 Request.Params（json.RawMessage）转成 caller.Call 能接受的形态。
// nil params 传 nil（内核侧多数 method 接受空 params）。
func forwardedParams(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

// rawJSON 是 json.RawMessage 的别名包装，使其在 json.Marshal 时原样输出。
// json.RawMessage 本身已实现 MarshalJSON，直接返回即可，这里只是为了
// 让 forward 的返回值类型语义清晰（「这就是内核的原始响应」）。
type rawJSON json.RawMessage

func (r rawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("{}"), nil
	}
	return r, nil
}

// handleInitialize 在 serve 层本地响应 initialize，返回内核握手结果。
// 若 InitResult 缺失，methods 退化为 AllCoreMethods()。
func handleInitialize(init coreapijsonrpc.InitializeResult) jsonrpc.HandlerFunc {
	return func(ctx context.Context, req jsonrpc.Request) (any, *jsonrpc.Error) {
		methods := init.Methods
		if len(methods) == 0 {
			methods = jsonrpc.AllCoreMethods()
		}
		return coreapijsonrpc.InitializeResult{
			ServerName:      firstNonEmpty(init.ServerName, "eos-serve"),
			ProtocolVersion: init.ProtocolVersion,
			Methods:         methods,
			Capabilities:    init.Capabilities,
		}, nil
	}
}

// handleNoContent 响应无返回值的方法（如 shutdown），返回空 map。
func handleNoContent(_ context.Context, _ jsonrpc.Request) (any, *jsonrpc.Error) {
	return map[string]any{}, nil
}

// jsonrpcError 把 caller.Call 的错误映射为 JSON-RPC Error。
// 保留内核原始错误信息，由调用方据 message 判断（不替内核归类错误码）。
func jsonrpcError(err error) *jsonrpc.Error {
	msg := "core call failed"
	if err != nil {
		msg = err.Error()
	}
	return &jsonrpc.Error{
		Code:    jsonrpc.CodeInternalError,
		Message: msg,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
