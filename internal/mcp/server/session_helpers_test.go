package server

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestChatTimeoutSecs(t *testing.T) {
	h := &MCPHost{}
	if h.chatTimeoutSecs() != defaultChatTimeoutSecs {
		t.Fatalf("default = %d", h.chatTimeoutSecs())
	}
	h.chatTimeout = 42
	if h.chatTimeoutSecs() != 42 {
		t.Fatalf("custom = %d", h.chatTimeoutSecs())
	}
}

func TestResolveSessionID(t *testing.T) {
	// Current 有 ID → 复用
	eng := &stubSessionEngine{currentID: "cur-1"}
	id, err := resolveSessionID(context.Background(), eng, "/ws")
	if err != nil || id != "cur-1" {
		t.Fatalf("reuse = %q %v", id, err)
	}

	// Current 空 → Create
	eng2 := &stubSessionEngine{currentID: "", createdID: "new-1"}
	id, err = resolveSessionID(context.Background(), eng2, "/ws")
	if err != nil || id != "new-1" {
		t.Fatalf("create = %q %v", id, err)
	}

	// Create 空 ID → 错误
	eng3 := &stubSessionEngine{currentID: "", createdID: ""}
	if _, err := resolveSessionID(context.Background(), eng3, "/ws"); err == nil {
		t.Fatal("empty create id")
	}
}

func TestParamToOptionAndDescription(t *testing.T) {
	if paramToOption("s", coreapi.ToolParameterInfo{Type: "string", Desc: "d"}) == nil {
		t.Fatal("string")
	}
	if paramToOption("n", coreapi.ToolParameterInfo{Type: "number"}) == nil {
		t.Fatal("number")
	}
	if paramToOption("i", coreapi.ToolParameterInfo{Type: "integer"}) == nil {
		t.Fatal("int")
	}
	if paramToOption("b", coreapi.ToolParameterInfo{Type: "boolean"}) == nil {
		t.Fatal("bool")
	}
	if paramToOption("u", coreapi.ToolParameterInfo{Type: "unknown"}) == nil {
		t.Fatal("unknown")
	}
	if paramToOption("p", coreapi.ToolParameterInfo{Type: "path"}) == nil {
		t.Fatal("path")
	}
}

// stubSessionEngine 仅实现 Sessions，其余返回零值服务。
type stubSessionEngine struct {
	currentID  string
	createdID  string
	createFail bool
}

func (e *stubSessionEngine) Caller() coreapi.Caller                 { return nil }
func (e *stubSessionEngine) State() coreapi.StateService            { return nil }
func (e *stubSessionEngine) Workspaces() coreapi.WorkspaceService   { return nil }
func (e *stubSessionEngine) Sessions() coreapi.SessionService       { return &stubSessions{e: e} }
func (e *stubSessionEngine) MCP() coreapi.MCPService                { return nil }
func (e *stubSessionEngine) LSP() coreapi.LSPService                { return nil }
func (e *stubSessionEngine) Config() coreapi.ConfigService          { return nil }
func (e *stubSessionEngine) Permissions() coreapi.PermissionService { return nil }
func (e *stubSessionEngine) Extensions() coreapi.ExtensionService   { return nil }
func (e *stubSessionEngine) Context() coreapi.ContextService        { return nil }
func (e *stubSessionEngine) Usage() coreapi.UsageService            { return nil }
func (e *stubSessionEngine) Versions() coreapi.VersionService       { return nil }
func (e *stubSessionEngine) Tasks() coreapi.TaskService             { return nil }
func (e *stubSessionEngine) Goals() coreapi.GoalService             { return nil }
func (e *stubSessionEngine) Modes() coreapi.ModeService             { return nil }
func (e *stubSessionEngine) Models() coreapi.ModelService           { return nil }
func (e *stubSessionEngine) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	return nil
}
func (e *stubSessionEngine) Git() coreapi.GitService                     { return nil }
func (e *stubSessionEngine) Insights() coreapi.InsightService            { return nil }
func (e *stubSessionEngine) Memory() coreapi.MemoryService               { return nil }
func (e *stubSessionEngine) Roles() coreapi.RoleService                  { return nil }
func (e *stubSessionEngine) Turns() coreapi.TurnService                  { return nil }
func (e *stubSessionEngine) Approvals() coreapi.ApprovalService          { return nil }
func (e *stubSessionEngine) Inquiries() coreapi.InquiryService           { return nil }
func (e *stubSessionEngine) Agents() coreapi.AgentService                { return nil }
func (e *stubSessionEngine) Tools() coreapi.ToolExecutor                 { return nil }
func (e *stubSessionEngine) ToolCatalog() coreapi.ToolCatalogService     { return nil }
func (e *stubSessionEngine) ToolTelemetry() coreapi.ToolTelemetryService { return nil }
func (e *stubSessionEngine) Events() coreapi.EventSubscriber             { return nil }
func (e *stubSessionEngine) Sandbox() coreapi.SandboxService             { return nil }
func (e *stubSessionEngine) Diagnostics() coreapi.DiagnosticsService     { return nil }

type stubSessions struct{ e *stubSessionEngine }

func (s *stubSessions) Current(context.Context, coreapi.CurrentSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: s.e.currentID}, nil
}
func (s *stubSessions) Create(context.Context, coreapi.CreateSessionRequest) (coreapi.Session, error) {
	if s.e.createFail {
		return coreapi.Session{}, errCreate
	}
	return coreapi.Session{ID: s.e.createdID}, nil
}
func (s *stubSessions) List(context.Context, coreapi.ListSessionsRequest) ([]coreapi.Session, error) {
	return nil, nil
}
func (s *stubSessions) Resume(context.Context, coreapi.ResumeSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *stubSessions) SetCurrent(context.Context, coreapi.SetCurrentSessionRequest) error {
	return nil
}
func (s *stubSessions) Delete(context.Context, coreapi.DeleteSessionRequest) error { return nil }
func (s *stubSessions) Rename(context.Context, coreapi.RenameSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *stubSessions) SetMeta(context.Context, coreapi.SetSessionMetaRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *stubSessions) SaveMessages(context.Context, coreapi.SaveSessionMessagesRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: s.e.createdID}, nil
}
func (s *stubSessions) LoadMessages(context.Context, coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	return nil, nil
}

var errCreate = errStr("create fail")

type errStr string

func (e errStr) Error() string { return string(e) }
