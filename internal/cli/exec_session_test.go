package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestApplyExecSessionAndStartup(t *testing.T) {
	eng := &fakeExecSessionsEngine{}
	// nil engine
	if err := applyExecSession(context.Background(), nil, execOptions{Session: "x"}); err == nil {
		t.Fatal("nil engine")
	}
	// 空 session no-op
	if err := applyExecSession(context.Background(), eng, execOptions{}); err != nil {
		t.Fatal(err)
	}
	// 显式 session
	if err := applyExecSession(context.Background(), eng, execOptions{Session: "s1", Workspace: "/ws"}); err != nil {
		t.Fatal(err)
	}
	// last
	eng.sessions = []coreapi.Session{
		{ID: "old", UpdatedAt: time.Now().Add(-time.Hour)},
		{ID: "new", UpdatedAt: time.Now()},
	}
	if err := applyExecSession(context.Background(), eng, execOptions{Session: "last"}); err != nil {
		t.Fatal(err)
	}

	// applyExecStartup
	if err := applyExecStartup(context.Background(), nil, execOptions{}); err == nil {
		t.Fatal("nil engine startup")
	}
	if err := applyExecStartup(context.Background(), eng, execOptions{Workspace: "/ws", ExecutionMode: "plan"}); err != nil {
		t.Fatal(err)
	}
}

func TestLatestExecSessionErrors(t *testing.T) {
	eng := &fakeExecSessionsEngine{listErr: errors.New("x")}
	if _, err := latestExecSession(context.Background(), eng, ""); err == nil {
		t.Fatal("list err")
	}
	eng2 := &fakeExecSessionsEngine{}
	if _, err := latestExecSession(context.Background(), eng2, ""); err == nil {
		t.Fatal("empty list")
	}
}

type fakeExecSessionsEngine struct {
	sessions []coreapi.Session
	listErr  error
}

func (e *fakeExecSessionsEngine) Caller() coreapi.Caller                 { return nil }
func (e *fakeExecSessionsEngine) State() coreapi.StateService            { return &fakeExecState{} }
func (e *fakeExecSessionsEngine) Workspaces() coreapi.WorkspaceService   { return &fakeExecWSSvc{} }
func (e *fakeExecSessionsEngine) Sessions() coreapi.SessionService       { return &fakeExecSessSvc{e: e} }
func (e *fakeExecSessionsEngine) MCP() coreapi.MCPService                { return nil }
func (e *fakeExecSessionsEngine) LSP() coreapi.LSPService                { return nil }
func (e *fakeExecSessionsEngine) Config() coreapi.ConfigService          { return nil }
func (e *fakeExecSessionsEngine) Permissions() coreapi.PermissionService { return nil }
func (e *fakeExecSessionsEngine) Extensions() coreapi.ExtensionService   { return nil }
func (e *fakeExecSessionsEngine) Context() coreapi.ContextService        { return nil }
func (e *fakeExecSessionsEngine) Usage() coreapi.UsageService            { return nil }
func (e *fakeExecSessionsEngine) Versions() coreapi.VersionService       { return nil }
func (e *fakeExecSessionsEngine) Tasks() coreapi.TaskService             { return nil }
func (e *fakeExecSessionsEngine) Goals() coreapi.GoalService             { return nil }
func (e *fakeExecSessionsEngine) Modes() coreapi.ModeService             { return &fakeExecModesSvc{} }
func (e *fakeExecSessionsEngine) Models() coreapi.ModelService           { return nil }
func (e *fakeExecSessionsEngine) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	return nil
}
func (e *fakeExecSessionsEngine) Git() coreapi.GitService                     { return nil }
func (e *fakeExecSessionsEngine) Insights() coreapi.InsightService            { return nil }
func (e *fakeExecSessionsEngine) Memory() coreapi.MemoryService               { return nil }
func (e *fakeExecSessionsEngine) Roles() coreapi.RoleService                  { return nil }
func (e *fakeExecSessionsEngine) Turns() coreapi.TurnService                  { return nil }
func (e *fakeExecSessionsEngine) Approvals() coreapi.ApprovalService          { return nil }
func (e *fakeExecSessionsEngine) Inquiries() coreapi.InquiryService           { return nil }
func (e *fakeExecSessionsEngine) Agents() coreapi.AgentService                { return nil }
func (e *fakeExecSessionsEngine) Tools() coreapi.ToolExecutor                 { return nil }
func (e *fakeExecSessionsEngine) ToolCatalog() coreapi.ToolCatalogService     { return nil }
func (e *fakeExecSessionsEngine) ToolTelemetry() coreapi.ToolTelemetryService { return nil }
func (e *fakeExecSessionsEngine) Events() coreapi.EventSubscriber             { return nil }
func (e *fakeExecSessionsEngine) Sandbox() coreapi.SandboxService             { return nil }
func (e *fakeExecSessionsEngine) Diagnostics() coreapi.DiagnosticsService     { return nil }

type fakeExecSessSvc struct{ e *fakeExecSessionsEngine }

func (s *fakeExecSessSvc) Current(context.Context, coreapi.CurrentSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: "cur"}, nil
}
func (s *fakeExecSessSvc) Create(context.Context, coreapi.CreateSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: "c1"}, nil
}
func (s *fakeExecSessSvc) List(context.Context, coreapi.ListSessionsRequest) ([]coreapi.Session, error) {
	return s.e.sessions, s.e.listErr
}
func (s *fakeExecSessSvc) Resume(context.Context, coreapi.ResumeSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: "r"}, nil
}
func (s *fakeExecSessSvc) SetCurrent(context.Context, coreapi.SetCurrentSessionRequest) error {
	return nil
}
func (s *fakeExecSessSvc) Delete(context.Context, coreapi.DeleteSessionRequest) error { return nil }
func (s *fakeExecSessSvc) Rename(context.Context, coreapi.RenameSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *fakeExecSessSvc) SetMeta(context.Context, coreapi.SetSessionMetaRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *fakeExecSessSvc) LoadMessages(context.Context, coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	return nil, nil
}
func (s *fakeExecSessSvc) SaveMessages(context.Context, coreapi.SaveSessionMessagesRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}

type fakeExecWSSvc struct{}

func (s *fakeExecWSSvc) List(context.Context, coreapi.WorkspaceListRequest) ([]coreapi.Workspace, error) {
	return nil, nil
}
func (s *fakeExecWSSvc) Default(context.Context) (string, error)   { return "", nil }
func (s *fakeExecWSSvc) Last(context.Context) (string, error)      { return "", nil }
func (s *fakeExecWSSvc) ResolveForeground(context.Context, coreapi.ResolveForegroundWorkspaceRequest) (string, error) {
	return "", nil
}
func (s *fakeExecWSSvc) Remember(context.Context, coreapi.RememberWorkspaceRequest) error {
	return nil
}
func (s *fakeExecWSSvc) Forget(context.Context, coreapi.WorkspacePathRequest) error { return nil }
func (s *fakeExecWSSvc) Add(context.Context, coreapi.WorkspacePathRequest) error    { return nil }
func (s *fakeExecWSSvc) Remove(context.Context, coreapi.WorkspacePathRequest) error { return nil }
func (s *fakeExecWSSvc) Use(context.Context, coreapi.WorkspacePathRequest) error    { return nil }
func (s *fakeExecWSSvc) SetForeground(context.Context, coreapi.WorkspacePathRequest) error {
	return nil
}
func (s *fakeExecWSSvc) Trust(context.Context, coreapi.WorkspacePathRequest) error { return nil }
func (s *fakeExecWSSvc) ListWorktrees(context.Context) ([]coreapi.Worktree, error) {
	return nil, nil
}
func (s *fakeExecWSSvc) CreateWorktree(context.Context, coreapi.CreateWorktreeRequest) (coreapi.Worktree, error) {
	return coreapi.Worktree{}, nil
}
func (s *fakeExecWSSvc) RemoveWorktree(context.Context, coreapi.RemoveWorktreeRequest) error {
	return nil
}

type fakeExecModesSvc struct{}

func (s *fakeExecModesSvc) Snapshot(context.Context) (coreapi.ModeSnapshot, error) {
	return coreapi.ModeSnapshot{}, nil
}
func (s *fakeExecModesSvc) SetExecutionMode(context.Context, coreapi.SetModeRequest) error {
	return nil
}
func (s *fakeExecModesSvc) SetSandboxMode(context.Context, coreapi.SetModeRequest) error {
	return nil
}
func (s *fakeExecModesSvc) SetReasoningLevel(context.Context, coreapi.SetModeRequest) error {
	return nil
}
