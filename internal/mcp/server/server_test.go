package server

// server_test.go + tools_test.go 验证 MCP server 的工具映射与会话注入。
// 用最小 mock engine（嵌入 coreapi.Engine 空接口 + 覆写需要的 service）。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// mcpTestEngine 嵌入 coreapi.Engine 空接口（自动满足全部 service 的零实现），
// 只覆写 MCP server 用到的 ToolCatalog / Tools / Sessions。
type mcpTestEngine struct {
	coreapi.Engine
	catalog  coreapi.ToolCatalogService
	tools    coreapi.ToolExecutor
	sessions coreapi.SessionService
}

func (e *mcpTestEngine) ToolCatalog() coreapi.ToolCatalogService { return e.catalog }
func (e *mcpTestEngine) Tools() coreapi.ToolExecutor             { return e.tools }
func (e *mcpTestEngine) Sessions() coreapi.SessionService        { return e.sessions }

type mockCatalog struct {
	defs []coreapi.ToolDefinition
	err  error
}

func (m mockCatalog) List(_ context.Context, _ coreapi.ListToolCatalogRequest) ([]coreapi.ToolDefinition, error) {
	return m.defs, m.err
}

type mockToolExecutor struct {
	calls   []coreapi.ToolRequest
	result  coreapi.ToolResult
	execErr error
}

func (m *mockToolExecutor) Execute(_ context.Context, req coreapi.ToolRequest) (coreapi.ToolResult, error) {
	m.calls = append(m.calls, req)
	return m.result, m.execErr
}

type mockSessions struct {
	currentID string
	createdID string
}

func (s mockSessions) Current(_ context.Context, _ coreapi.CurrentSessionRequest) (coreapi.Session, error) {
	if s.currentID == "" {
		return coreapi.Session{}, errors.New("no current session")
	}
	return coreapi.Session{ID: s.currentID}, nil
}

func (s mockSessions) Create(_ context.Context, req coreapi.CreateSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: s.createdID, WorkspaceRoot: req.WorkspaceRoot}, nil
}

// SessionService 其余方法的空实现（MCP server 路径不触发）。
func (mockSessions) Resume(context.Context, coreapi.ResumeSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (mockSessions) List(context.Context, coreapi.ListSessionsRequest) ([]coreapi.Session, error) {
	return nil, nil
}
func (mockSessions) SetCurrent(context.Context, coreapi.SetCurrentSessionRequest) error { return nil }
func (mockSessions) Delete(context.Context, coreapi.DeleteSessionRequest) error         { return nil }
func (mockSessions) Rename(context.Context, coreapi.RenameSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (mockSessions) SetMeta(context.Context, coreapi.SetSessionMetaRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (mockSessions) LoadMessages(context.Context, coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	return nil, nil
}
func (mockSessions) SaveMessages(context.Context, coreapi.SaveSessionMessagesRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}

func newTestHost(t *testing.T, defs []coreapi.ToolDefinition, execResult coreapi.ToolResult) (*MCPHost, *mockToolExecutor) {
	t.Helper()
	exec := &mockToolExecutor{result: execResult}
	e := &mcpTestEngine{
		catalog:  mockCatalog{defs: defs},
		tools:    exec,
		sessions: mockSessions{createdID: "sess-mcp"},
	}
	host := &MCPHost{engine: e}
	return host, exec
}

func TestNewErrorsWithoutEngine(t *testing.T) {
	if _, _, err := New(context.Background(), Options{}); err == nil {
		t.Fatal("New without engine must error")
	}
}

func TestRegisterToolsSkipsNonInvocable(t *testing.T) {
	defs := []coreapi.ToolDefinition{
		{Name: "read", Description: "read file", Invocable: true, Params: map[string]coreapi.ToolParameterInfo{
			"path": {Type: "string", Required: true, Desc: "file path"},
		}},
		{Name: "meta_cap", Description: "capability only", Invocable: false},
	}
	host, _ := newTestHost(t, defs, coreapi.ToolResult{})
	// 用真实 mark3labs MCPServer 验证注册结果。
	s := server.NewMCPServer("eos-test", "0")
	if err := host.registerTools(context.Background(), s); err != nil {
		t.Fatalf("registerTools: %v", err)
	}
	if t1 := s.GetTool("read"); t1 == nil {
		t.Fatal("invocable tool 'read' not registered")
	}
	if t2 := s.GetTool("meta_cap"); t2 != nil {
		t.Fatal("non-invocable tool 'meta_cap' should be skipped")
	}
}

func TestHandleToolCallInjectsDefaultSession(t *testing.T) {
	defs := []coreapi.ToolDefinition{{Name: "read", Invocable: true}}
	host, exec := newTestHost(t, defs, coreapi.ToolResult{Status: "success", Output: json.RawMessage(`{"output":"hello"}`)})

	_, err := host.handleToolCall(context.Background(), "read", mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleToolCall: %v", err)
	}
	if len(exec.calls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(exec.calls))
	}
	if exec.calls[0].SessionID != "sess-mcp" {
		t.Fatalf("session_id=%q, want sess-mcp (default)", exec.calls[0].SessionID)
	}
	if exec.calls[0].Name != "read" {
		t.Fatalf("tool name=%q, want read", exec.calls[0].Name)
	}
}

func TestHandleToolCallRespectsMetaSessionID(t *testing.T) {
	defs := []coreapi.ToolDefinition{{Name: "read", Invocable: true}}
	host, exec := newTestHost(t, defs, coreapi.ToolResult{Status: "success"})

	req := mcp.CallToolRequest{}
	req.Params.Meta = &mcp.Meta{AdditionalFields: map[string]any{"session_id": "explicit-sess"}}
	_, _ = host.handleToolCall(context.Background(), "read", req)
	if len(exec.calls) != 1 || exec.calls[0].SessionID != "explicit-sess" {
		t.Fatalf("session_id=%q, want explicit-sess (from _meta)", exec.calls[0].SessionID)
	}
}

func TestToolResultToMCPMarksErrorOnNonSuccess(t *testing.T) {
	r := coreapi.ToolResult{Status: "pending_approval", Display: "needs approval"}
	got := toolResultToMCP(r)
	if !got.IsError {
		t.Fatal("non-success status must set IsError=true")
	}
}

func TestToolResultToMCPSuccessNoError(t *testing.T) {
	r := coreapi.ToolResult{Status: "success", Output: json.RawMessage(`{"stdout":"done"}`)}
	got := toolResultToMCP(r)
	if got.IsError {
		t.Fatal("success status must not set IsError")
	}
	if !strings.Contains(textContent(got), "done") {
		t.Fatalf("result text missing 'done': %+v", got.Content)
	}
}

func TestHandleToolCallMapsExecError(t *testing.T) {
	defs := []coreapi.ToolDefinition{{Name: "read", Invocable: true}}
	host := &MCPHost{engine: &mcpTestEngine{
		catalog:  mockCatalog{defs: defs},
		tools:    &mockToolExecutor{execErr: errors.New("sandbox denied")},
		sessions: mockSessions{createdID: "sess-mcp"},
	}}
	got, err := host.handleToolCall(context.Background(), "read", mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleToolCall should not return go error, got %v", err)
	}
	if !got.IsError {
		t.Fatal("exec error must produce IsError=true result")
	}
	if !strings.Contains(textContent(got), "sandbox denied") {
		t.Fatalf("error message lost: %s", textContent(got))
	}
}

// textContent 从 CallToolResult 提取首个 TextContent 的文本。
func textContent(r *mcp.CallToolResult) string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	if tc, ok := r.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
