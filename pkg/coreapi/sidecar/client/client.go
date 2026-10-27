// Package client 是 TUI 与 Rust eos-core 之间的 facade。
//
// 设计目标：
//   - TUI 不直接 import pkg/coreapi/sidecar 的所有导出。
//   - 唯一入口 Start/Attach 启动/连接 eos-core --app-server --stdio 子进程。
//   - Client 只暴露 coreapi.Engine 与 lifecycle；TUI 通过 adapter 包装后做 RPC。
//   - 不允许 import internal/bridge、internal/runtime、internal/tools、pkg/core。
//
// 与 sidecar 包的分工：
//   - sidecar 负责子进程管理、二进制解析、签名校验。
//   - client 在 sidecar 之上加一层 facade，启动/连接 path 收敛到 Start/Attach。
//   - adapter 在 client 之上提供 TUI 用的 RPC adapter 方法。
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/eosaios/eos/pkg/coreapi"
	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// RequiredMethods 是 TUI 启动时要求 eos-core 必须实现的方法集。
// 来源：eos-core-protocol FULL_CORE_METHODS 的子集，覆盖 TUI 全部面板和 slash action。
// 与 eos-core-rs FULL_CORE_METHODS 由 architecture 测试做交叉校验。
var RequiredMethods = []string{
	protocoljsonrpc.MethodStateSnapshot,
	protocoljsonrpc.MethodWorkspaceList,
	protocoljsonrpc.MethodWorkspaceSetForeground,
	protocoljsonrpc.MethodSessionList,
	protocoljsonrpc.MethodSessionCreate,
	protocoljsonrpc.MethodSessionResume,
	protocoljsonrpc.MethodSessionCurrent,
	protocoljsonrpc.MethodSessionMessagesLoad,
	protocoljsonrpc.MethodSessionMessagesSave,
	protocoljsonrpc.MethodTurnStart,
	protocoljsonrpc.MethodTurnInterrupt,
	protocoljsonrpc.MethodApprovalRespond,
	protocoljsonrpc.MethodInquiryRespond,
	protocoljsonrpc.MethodToolCatalog,
	protocoljsonrpc.MethodToolExecute,
	protocoljsonrpc.MethodEventSubscribe,
	protocoljsonrpc.MethodEventUnsubscribe,
	protocoljsonrpc.MethodConfigReload,
	protocoljsonrpc.MethodAgentControl,
	protocoljsonrpc.MethodAgentInput,
	protocoljsonrpc.MethodAgentRun,
	protocoljsonrpc.MethodSandboxBackend,
}

// Options 描述启动 eos-core --app-server --stdio 子进程的参数。
// 字段语义与 sidecar.ProcessOptions 一致，但只暴露 TUI 关心的子集。
type Options struct {
	// BinaryPath 显式指定 eos-core 可执行路径。
	// 留空时走 sidecar.ResolveBinary（EOS_CORE_PATH / EOS_CORE_MANIFEST / 默认搜索根）。
	BinaryPath string
	// Args 透传给 eos-core 的额外参数。
	// 留空时自动追加 --stdio。
	Args []string
	// Dir 子进程工作目录。
	Dir string
	// Env 注入到子进程的环境变量（追加在 os.Environ 之后）。
	Env map[string]string
	// Stderr 接收子进程 stderr；留空则丢弃。
	Stderr io.Writer
	// RequiredFeatures 要求 sidecar manifest 必须声明的能力集合。
	// 留空时使用 facade.RequiredMethods。
	RequiredFeatures []string
	// VerifyChecksum 强制校验已解析 manifest 的 sha256。
	VerifyChecksum bool
	// RequireSignature 强制校验 Ed25519 签名。
	RequireSignature bool
	// AllowDevPlaceholder 允许 manifest 使用 unsigned-development-placeholder 签名。
	// 仅 dev 环境有效；当 EOS_RELEASE_ARTIFACT_CHECK 被设置（release 场景）时，
	// sidecar.ResolveBinary 会在 resolveManifest 内强制把它改写成 false，
	// 占位签名必定被拒——无需依赖调用方正确传值。
	AllowDevPlaceholder bool
}

// InitializeResult 描述 eos-core 子进程 initialize 响应中的关键字段。
type InitializeResult struct {
	ServerName      string
	ProtocolVersion string
	Methods         []string
	// Raw 保留原始结果，供需要完整 capabilities 的调用方使用。
	Raw coreapijsonrpc.InitializeResult
}

// Client 是 TUI 视角的 eos-core 客户端。
// 内部持有 sidecar.RemoteEngine 与一组 lifecycle hooks。
type Client struct {
	engine *sidecar.RemoteEngine
	proc   *sidecar.ProcessClient
	init   InitializeResult
	mu     sync.Mutex
	closed bool
}

// Start 启动 eos-core --app-server --stdio 子进程并完成 initialize 握手。
// 成功后 Client 持有 RemoteEngine，可通过 Engine() 拿到 coreapi.Engine。
func Start(ctx context.Context, opts Options) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	processOpts := sidecar.ProcessOptions{
		BinaryPath:          opts.BinaryPath,
		Args:                opts.Args,
		Dir:                 opts.Dir,
		Env:                 opts.Env,
		Stderr:              opts.Stderr,
		VerifyChecksum:      opts.VerifyChecksum,
		RequireSignature:    opts.RequireSignature,
		AllowDevPlaceholder: opts.AllowDevPlaceholder,
	}
	if len(opts.RequiredFeatures) > 0 {
		processOpts.RequiredFeatures = append([]string(nil), opts.RequiredFeatures...)
	} else {
		processOpts.RequiredFeatures = append([]string(nil), RequiredMethods...)
	}
	if len(processOpts.Args) == 0 {
		processOpts.Args = []string{"--stdio"}
	} else {
		hasStdio := false
		for _, arg := range processOpts.Args {
			if arg == "--stdio" {
				hasStdio = true
				break
			}
		}
		if !hasStdio {
			processOpts.Args = append([]string(nil), processOpts.Args...)
			processOpts.Args = append(processOpts.Args, "--stdio")
		}
	}

	engine, err := sidecar.StartRemoteEngine(ctx, processOpts)
	if err != nil {
		return nil, fmt.Errorf("start eos-core sidecar: %w", err)
	}

	c := &Client{
		engine: engine,
		proc:   sidecarProcessClient(engine),
		init: InitializeResult{
			ServerName:      "eos-core",
			ProtocolVersion: "v1",
			Methods:         nil,
		},
	}
	if init, err := engine.Initialize(ctx); err == nil {
		c.init = InitializeResult{
			ServerName:      init.ServerName,
			ProtocolVersion: init.ProtocolVersion,
			Methods:         append([]string(nil), init.Methods...),
			Raw:             init,
		}
	}
	return c, nil
}

// Attach 在已有 RemoteEngine（例如测试桩）上包一层 Client。
// 用于测试与 engineprovider 自定义 StartRemoteFunc 的场景。
func Attach(engine *sidecar.RemoteEngine) *Client {
	if engine == nil {
		return nil
	}
	return &Client{
		engine: engine,
		proc:   sidecarProcessClient(engine),
	}
}

// Engine 返回底层 coreapi.Engine，供 adapter 做 RPC 调用。
// c.engine 为 nil 指针时返回真正的 nil 接口，避免 typed-nil 陷阱。
func (c *Client) Engine() coreapi.Engine {
	if c == nil || c.engine == nil {
		return nil
	}
	return c.engine
}

// Process 返回底层 ProcessClient，供需要更细粒度 lifecycle 控制的调用方使用。
func (c *Client) Process() *sidecar.ProcessClient {
	if c == nil {
		return nil
	}
	return c.proc
}

// Initialize 返回 handshake 结果，包含实际提供的方法列表。
func (c *Client) Initialize() InitializeResult {
	if c == nil {
		return InitializeResult{}
	}
	return c.init
}

// HasMethod 检查 handshake 中 eos-core 是否声明了给定方法。
func (c *Client) HasMethod(method string) bool {
	if c == nil {
		return false
	}
	for _, m := range c.init.Methods {
		if m == method {
			return true
		}
	}
	return false
}

// MissingMethods 返回 RequiredMethods 中 eos-core 未声明的方法。
// 用于在 initialize 之后立即检查 capability 缺口。
func (c *Client) MissingMethods() []string {
	if c == nil {
		return nil
	}
	available := make(map[string]struct{}, len(c.init.Methods))
	for _, m := range c.init.Methods {
		available[m] = struct{}{}
	}
	required := RequiredMethods
	if len(available) == 0 {
		return append([]string(nil), required...)
	}
	var missing []string
	for _, m := range required {
		if _, ok := available[m]; !ok {
			missing = append(missing, m)
		}
	}
	return missing
}

// Wait 阻塞至子进程退出，并返回 exit error。
// 用于 graceful shutdown 或错误恢复。
func (c *Client) Wait() <-chan error {
	if c == nil || c.engine == nil {
		ch := make(chan error, 1)
		ch <- errors.New("client is not started")
		close(ch)
		return ch
	}
	return c.engine.Wait()
}

// Close 关闭子进程连接并释放资源。
// 多次调用安全，第二次起返回 nil。
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	if c.engine == nil {
		return nil
	}
	return c.engine.Close()
}

// sidecarProcessClient 提取 engine 内部持有的 ProcessClient。
// 委托给 sidecar.RemoteEngine.ProcessClient 公开访问器。
func sidecarProcessClient(engine *sidecar.RemoteEngine) *sidecar.ProcessClient {
	if engine == nil {
		return nil
	}
	return engine.ProcessClient()
}
