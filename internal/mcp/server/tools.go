package server

// tools.go 实现 EOS 工具目录到 MCP tools 的映射：
//   - registerTools 枚举 ToolCatalog 仅 Invocable 工具，逐个 AddTool。
//   - 每个 handler 取工具名 + 参数 → ToolExecutor.Execute（注入会话）→ 映射结果。
//   - 高风险/审批场景返回 isError=true + 结构化提示（首期不自动放行）。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// metaSessionIDKey 是调用方在 _meta 里覆盖默认会话的字段名。
const metaSessionIDKey = "session_id"

// registerTools 枚举 EOS 工具目录，把 Invocable 工具注册为 MCP tool。
// handler 捕获工具名，在调用时取参数 + 会话执行。
func (h *MCPHost) registerTools(ctx context.Context, s *server.MCPServer) error {
	if h.engine == nil {
		return fmt.Errorf("engine unavailable")
	}
	defs, err := h.engine.ToolCatalog().List(ctx, coreapi.ListToolCatalogRequest{
		WorkspaceRoot: h.workspaceRoot,
	})
	if err != nil {
		return fmt.Errorf("list tool catalog: %w", err)
	}
	for _, def := range defs {
		if !def.Invocable {
			continue // capability-only 项不伪装成可调用 tool
		}
		tool := newMCPTool(def)
		toolName := def.Name
		s.AddTool(tool, func(callCtx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return h.handleToolCall(callCtx, toolName, req)
		})
	}
	return nil
}

// newMCPTool 把一个 EOS ToolDefinition 映射为 MCP Tool（含 inputSchema）。
// mcp-go 的 NewTool 默认产出 object schema，WithString/WithNumber 等直接给
// 该 object 追加属性，无需外层 WithObject 包装。
func newMCPTool(def coreapi.ToolDefinition) mcp.Tool {
	opts := []mcp.ToolOption{mcp.WithDescription(toolDescription(def))}
	opts = append(opts, buildPropertyOptions(def)...)
	return mcp.NewTool(def.Name, opts...)
}

// toolDescription 拼装工具描述：EOS 描述 + 风险/只读标注，帮助外部 agent 决策。
func toolDescription(def coreapi.ToolDefinition) string {
	desc := strings.TrimSpace(def.Description)
	if desc == "" {
		desc = strings.TrimSpace(def.Name)
	}
	var tags []string
	if def.ReadOnly {
		tags = append(tags, "read-only")
	}
	if r := strings.TrimSpace(def.RiskLevel); r != "" {
		tags = append(tags, "risk:"+r)
	}
	if def.RequiresFullAccess {
		tags = append(tags, "requires-full-access")
	}
	if len(tags) > 0 {
		desc = desc + " (" + strings.Join(tags, ", ") + ")"
	}
	return desc
}

// handleToolCall 执行一次工具调用：
//  1. 解析会话（默认或 _meta.session_id 覆盖）。
//  2. 取工具参数（raw JSON，无损透传）。
//  3. 调 ToolExecutor.Execute。
//  4. 映射 ToolResult 为 MCP CallToolResult（非 success 标 isError）。
func (h *MCPHost) handleToolCall(ctx context.Context, toolName string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := h.resolveSessionForCall(ctx, req)
	if err != nil {
		return errorResult(fmt.Sprintf("resolve session: %v", err)), nil
	}
	args := marshalArguments(req)
	result, err := h.engine.Tools().Execute(ctx, coreapi.ToolRequest{
		SessionID: sessionID,
		Name:      toolName,
		Args:      args,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("execute %s: %v", toolName, err)), nil
	}
	return toolResultToMCP(result), nil
}

// resolveSessionForCall 取本次调用的会话：优先 _meta.session_id，否则默认会话。
func (h *MCPHost) resolveSessionForCall(ctx context.Context, req mcp.CallToolRequest) (string, error) {
	if sid := sessionIDFromMeta(req); strings.TrimSpace(sid) != "" {
		return strings.TrimSpace(sid), nil
	}
	return h.ensureSession(ctx)
}

// sessionIDFromMeta 从 MCP 请求的 _meta.AdditionalFields 取 session_id 覆盖。
func sessionIDFromMeta(req mcp.CallToolRequest) string {
	if req.Params.Meta == nil {
		return ""
	}
	if v, ok := req.Params.Meta.AdditionalFields[metaSessionIDKey]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// marshalArguments 把 MCP 调用参数序列化为 EOS ToolRequest.Args（json.RawMessage）。
// 用 RawArguments（无损），降级到 Arguments map。
func marshalArguments(req mcp.CallToolRequest) json.RawMessage {
	if len(req.Params.RawArguments) > 0 {
		return req.Params.RawArguments
	}
	args := req.GetArguments()
	if len(args) == 0 {
		return nil
	}
	bs, err := json.Marshal(args)
	if err != nil {
		return nil
	}
	return bs
}

// toolResultToMCP 把 EOS ToolResult 映射为 MCP CallToolResult。
// Status 非 success（含 pending approval / error）标 isError=true，附带结构化提示。
func toolResultToMCP(r coreapi.ToolResult) *mcp.CallToolResult {
	text := toolResultText(r)
	content := []mcp.Content{mcp.NewTextContent(text)}
	result := &mcp.CallToolResult{
		Content: content,
	}
	if !strings.EqualFold(strings.TrimSpace(r.Status), "success") && r.Status != "" {
		result.IsError = true
	}
	return result
}

// toolResultText 取 ToolResult 的可读文本：优先 Output 解析，降级 Display/Error。
func toolResultText(r coreapi.ToolResult) string {
	if out := extractOutputText(r.Output); strings.TrimSpace(out) != "" {
		return out
	}
	if d := strings.TrimSpace(r.Display); d != "" {
		return d
	}
	if e := strings.TrimSpace(r.Error); e != "" {
		return e
	}
	return strings.TrimSpace(r.Status)
}

// extractOutputText 从 ToolResult.Output（json.RawMessage）提取文本字段。
func extractOutputText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"stdout", "output", "text", "stderr", "message"} {
		if v, ok := payload[key].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	// 无已知文本字段时回退原始 JSON。
	return string(raw)
}

// errorResult 构造一个 isError=true 的 MCP 结果，content 携带错误提示。
func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(message)},
		IsError: true,
	}
}

// buildPropertyOptions 从 ToolDefinition.Params 构造 MCP 工具属性 options 列表。
// 每个属性映射为一个 ToolOption（WithString/WithNumber/WithBoolean），
// mcp-go 会把它们合并进 tool 的 object inputSchema。
func buildPropertyOptions(def coreapi.ToolDefinition) []mcp.ToolOption {
	if len(def.Params) == 0 {
		return nil
	}
	opts := make([]mcp.ToolOption, 0, len(def.Params))
	for name, info := range def.Params {
		if opt := paramToOption(name, info); opt != nil {
			opts = append(opts, opt)
		}
	}
	return opts
}

// paramToOption 把单个 ToolParameterInfo 映射为 MCP 属性 option。
func paramToOption(name string, info coreapi.ToolParameterInfo) mcp.ToolOption {
	desc := strings.TrimSpace(info.Desc)
	switch strings.ToLower(strings.TrimSpace(info.Type)) {
	case "string", "text", "path", "file", "dir", "directory":
		return mcp.WithString(name, mcp.Description(desc))
	case "number", "integer", "int", "float":
		return mcp.WithNumber(name, mcp.Description(desc))
	case "boolean", "bool":
		// WithBoolean 在当前 mcp-go 版本存在；若无则降级为 string。
		return mcp.WithBoolean(name, mcp.Description(desc))
	default:
		// 未知类型默认按 string 暴露（最宽松，避免阻断调用）。
		return mcp.WithString(name, mcp.Description(desc))
	}
}
