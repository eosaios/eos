// Package server 实现 eos mcp serve：把 EOS 工具能力作为标准 MCP Server
// 暴露给外部 agent / 宿主。
//
// 首期范围（MVP）：只暴露 tools/list + tools/call，不暴露 resources/prompts。
// - tools/list：映射 EOS ToolCatalog（仅 Invocable 工具）。
// - tools/call：映射 EOS ToolExecutor，注入连接默认会话。
// - 高风险审批/询问首期不自动放行，返回 isError=true + 结构化提示。
//
// 基于 github.com/mark3labs/mcp-go。会话语义与协议契约见
// internal/docs/mcp/SERVER.md。
package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/eosaios/eos/pkg/coreapi"

	"github.com/mark3labs/mcp-go/server"
)

// Options 配置 MCP server。
type Options struct {
	// Engine 是 eos-core sidecar 引擎（提供 ToolCatalog / ToolExecutor /
	// Sessions 等全部 service）。
	Engine coreapi.Engine
	// WorkspaceRoot 工作区根目录（创建默认会话时使用）。
	WorkspaceRoot string
	// SessionID 显式指定会话 ID；为空则在首次工具调用时懒创建默认会话。
	SessionID string
	// ChatTimeoutSecs 是 eos_chat / eos_task_wait 的默认阻塞/等待上限
	//（秒）。<=0 用 control.go 的 defaultChatTimeoutSecs。
	ChatTimeoutSecs int
	// ModelOverride 是 --model 覆盖（条目名/模型 ID/套餐 label）；对每个
	// 会话在首次 eos_chat 前应用一次。
	ModelOverride string
}

// MCPHost 持有装配 MCP server 所需的会话上下文。
type MCPHost struct {
	engine        coreapi.Engine
	session       string // 连接默认会话 ID（懒初始化）
	workspaceRoot string
	// modelOverride / modelApplied / chatTimeoutSecs 驱动 eos_chat 语义。
	modelOverride string
	modelApplied  map[string]bool
	chatTimeout   int
	mu            sync.Mutex // 保护 session 懒初始化与 modelApplied
}

// New 装配一个 MCP server：创建 mark3labs MCPServer 并注册 EOS 工具
// （原子工具直通 + eos_* 控制/委托工具集）。
// 不在此启动 transport（由调用方按 stdio/sse 选择）。
func New(ctx context.Context, opts Options) (*server.MCPServer, *MCPHost, error) {
	if opts.Engine == nil {
		return nil, nil, errors.New("mcp server: engine is required")
	}
	host := &MCPHost{
		engine:        opts.Engine,
		session:       strings.TrimSpace(opts.SessionID),
		modelOverride: strings.TrimSpace(opts.ModelOverride),
		chatTimeout:   opts.ChatTimeoutSecs,
	}
	if ws := strings.TrimSpace(opts.WorkspaceRoot); ws != "" {
		host.workspaceRoot = ws
	}

	s := server.NewMCPServer("eos", eosServerVersion, server.WithToolCapabilities(true))
	if err := host.registerTools(ctx, s); err != nil {
		return nil, nil, fmt.Errorf("mcp server: register tools: %w", err)
	}
	host.registerControlTools(s)
	return s, host, nil
}

// chatTimeoutSecs 归一默认聊天超时（<=0 → defaultChatTimeoutSecs）。
func (h *MCPHost) chatTimeoutSecs() int {
	if h.chatTimeout <= 0 {
		return defaultChatTimeoutSecs
	}
	return h.chatTimeout
}

// ensureSession 懒创建连接默认会话（Current 失败则 Create），返回 session ID。
// 多次调用幂等（复用已创建的会话）。
func (h *MCPHost) ensureSession(ctx context.Context) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session != "" {
		return h.session, nil
	}
	if h.engine == nil {
		return "", errors.New("mcp server: engine unavailable")
	}
	id, err := resolveSessionID(ctx, h.engine, h.workspaceRoot)
	if err != nil {
		return "", err
	}
	h.session = id
	return id, nil
}

// resolveSessionID 复用 headless 会话解析模式：Current 成功则复用，否则 Create。
func resolveSessionID(ctx context.Context, engine coreapi.Engine, workspaceRoot string) (string, error) {
	if session, err := engine.Sessions().Current(ctx, coreapi.CurrentSessionRequest{WorkspaceRoot: workspaceRoot}); err == nil && strings.TrimSpace(session.ID) != "" {
		return strings.TrimSpace(session.ID), nil
	}
	session, err := engine.Sessions().Create(ctx, coreapi.CreateSessionRequest{
		WorkspaceRoot: workspaceRoot,
		Title:         "MCP server session",
		Metadata:      map[string]any{"source": "mcp"},
	})
	if err != nil {
		return "", fmt.Errorf("mcp server: create session: %w", err)
	}
	id := strings.TrimSpace(session.ID)
	if id == "" {
		return "", errors.New("mcp server: create session returned empty id")
	}
	return id, nil
}

const eosServerVersion = "0.1.0"
