package headless

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestEnsureSessionAndWorkspaceRoot(t *testing.T) {
	// nil engine
	if _, err := EnsureSession(context.Background(), nil); err == nil {
		t.Fatal("nil engine")
	}
	if WorkspaceRoot(context.Background(), nil) == "" {
		// cwd fallback
	}

	eng := &fakeHLSessionEngine{currentID: "cur-1", fg: "/ws"}
	s, err := EnsureSession(context.Background(), eng)
	if err != nil || s.ID != "cur-1" {
		t.Fatalf("reuse = %+v %v", s, err)
	}
	if WorkspaceRoot(context.Background(), eng) != "/ws" {
		t.Fatal("fg root")
	}

	// Current 空 → Create
	eng2 := &fakeHLSessionEngine{createdID: "new-1", fg: "/ws"}
	s, err = EnsureSession(context.Background(), eng2)
	if err != nil || s.ID != "new-1" {
		t.Fatalf("create = %+v %v", s, err)
	}

	// Create 空 ID
	eng3 := &fakeHLSessionEngine{createdID: ""}
	if _, err := EnsureSession(context.Background(), eng3); err == nil {
		t.Fatal("empty create")
	}
}

func TestApplyModelOverrideAndResolveActive(t *testing.T) {
	eng := &fakeHLSessionEngine{}
	// 空 override no-op
	if err := ApplyModelOverride(context.Background(), eng, coreapi.Session{ID: "s"}, "  "); err != nil {
		t.Fatal(err)
	}
	// nil engine
	if err := ApplyModelOverride(context.Background(), nil, coreapi.Session{}, "m"); err == nil {
		t.Fatal("nil engine")
	}

	// ResolveActiveModelName
	if name, err := ResolveActiveModelName(context.Background(), nil); err != nil || name != "" {
		t.Fatalf("nil = %q %v", name, err)
	}
	eng2 := &fakeHLSessionEngine{models: []coreapi.ModelConfig{{Name: "a", Model: "m-a"}, {Name: "b", Model: "m-b", Active: true}}}
	name, err := ResolveActiveModelName(context.Background(), eng2)
	if err != nil || name != "m-b" {
		t.Fatalf("active = %q %v", name, err)
	}
	// 无 active
	eng3 := &fakeHLSessionEngine{models: []coreapi.ModelConfig{{Name: "a", Model: "m"}}}
	if name, err := ResolveActiveModelName(context.Background(), eng3); err != nil || name != "" {
		t.Fatalf("none = %q %v", name, err)
	}
}

func TestStartTurnAsync(t *testing.T) {
	// nil engine
	ch := StartTurnAsync(context.Background(), nil, coreapi.StartTurnRequest{})
	select {
	case res := <-ch:
		if res.Err == nil {
			t.Fatal("nil engine")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	eng := &fakeHLSessionEngine{turnID: "t1"}
	ch = StartTurnAsync(context.Background(), eng, coreapi.StartTurnRequest{})
	select {
	case res := <-ch:
		if res.Err != nil {
			t.Fatal(res.Err)
		}
		if res.Turn.ID != "t1" {
			t.Fatalf("turn = %+v", res.Turn)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// fakeHLSessionEngine 只实现 Sessions/State/Models/Turns。
type fakeHLSessionEngine struct {
	currentID string
	createdID string
	fg        string
	models    []coreapi.ModelConfig
	turnID    string
}

func (e *fakeHLSessionEngine) Caller() coreapi.Caller               { return nil }
func (e *fakeHLSessionEngine) State() coreapi.StateService          { return &fakeHLState{fg: e.fg} }
func (e *fakeHLSessionEngine) Workspaces() coreapi.WorkspaceService { return nil }
func (e *fakeHLSessionEngine) Sessions() coreapi.SessionService     { return &fakeHLSess{e: e} }
func (e *fakeHLSessionEngine) MCP() coreapi.MCPService              { return nil }
func (e *fakeHLSessionEngine) LSP() coreapi.LSPService              { return nil }
func (e *fakeHLSessionEngine) Config() coreapi.ConfigService        { return nil }
func (e *fakeHLSessionEngine) Permissions() coreapi.PermissionService {
	return nil
}
func (e *fakeHLSessionEngine) Extensions() coreapi.ExtensionService { return nil }
func (e *fakeHLSessionEngine) Context() coreapi.ContextService      { return nil }
func (e *fakeHLSessionEngine) Usage() coreapi.UsageService          { return nil }
func (e *fakeHLSessionEngine) Versions() coreapi.VersionService     { return nil }
func (e *fakeHLSessionEngine) Tasks() coreapi.TaskService           { return nil }
func (e *fakeHLSessionEngine) Goals() coreapi.GoalService           { return nil }
func (e *fakeHLSessionEngine) Modes() coreapi.ModeService           { return nil }
func (e *fakeHLSessionEngine) Models() coreapi.ModelService         { return &fakeHLModels{e: e} }
func (e *fakeHLSessionEngine) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	return nil
}
func (e *fakeHLSessionEngine) Git() coreapi.GitService                     { return nil }
func (e *fakeHLSessionEngine) Insights() coreapi.InsightService            { return nil }
func (e *fakeHLSessionEngine) Memory() coreapi.MemoryService               { return nil }
func (e *fakeHLSessionEngine) Roles() coreapi.RoleService                  { return nil }
func (e *fakeHLSessionEngine) Turns() coreapi.TurnService                  { return &fakeHLTurns{e: e} }
func (e *fakeHLSessionEngine) Approvals() coreapi.ApprovalService          { return nil }
func (e *fakeHLSessionEngine) Inquiries() coreapi.InquiryService           { return nil }
func (e *fakeHLSessionEngine) Agents() coreapi.AgentService                { return nil }
func (e *fakeHLSessionEngine) Tools() coreapi.ToolExecutor                 { return nil }
func (e *fakeHLSessionEngine) ToolCatalog() coreapi.ToolCatalogService     { return nil }
func (e *fakeHLSessionEngine) ToolTelemetry() coreapi.ToolTelemetryService { return nil }
func (e *fakeHLSessionEngine) Events() coreapi.EventSubscriber             { return nil }
func (e *fakeHLSessionEngine) Sandbox() coreapi.SandboxService             { return nil }
func (e *fakeHLSessionEngine) Diagnostics() coreapi.DiagnosticsService     { return nil }

type fakeHLState struct{ fg string }

func (s *fakeHLState) Snapshot(context.Context, coreapi.StateSnapshotRequest) (coreapi.StateSnapshot, error) {
	return coreapi.StateSnapshot{ForegroundWorkspace: s.fg}, nil
}

type fakeHLSess struct{ e *fakeHLSessionEngine }

func (s *fakeHLSess) Current(context.Context, coreapi.CurrentSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: s.e.currentID}, nil
}
func (s *fakeHLSess) Create(context.Context, coreapi.CreateSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: s.e.createdID}, nil
}
func (s *fakeHLSess) List(context.Context, coreapi.ListSessionsRequest) ([]coreapi.Session, error) {
	return nil, nil
}
func (s *fakeHLSess) Resume(context.Context, coreapi.ResumeSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *fakeHLSess) SetCurrent(context.Context, coreapi.SetCurrentSessionRequest) error {
	return nil
}
func (s *fakeHLSess) Delete(context.Context, coreapi.DeleteSessionRequest) error { return nil }
func (s *fakeHLSess) Rename(context.Context, coreapi.RenameSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *fakeHLSess) SetMeta(context.Context, coreapi.SetSessionMetaRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *fakeHLSess) LoadMessages(context.Context, coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	return nil, nil
}
func (s *fakeHLSess) SaveMessages(context.Context, coreapi.SaveSessionMessagesRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}

type fakeHLModels struct{ e *fakeHLSessionEngine }

func (s *fakeHLModels) List(context.Context) ([]coreapi.ModelConfig, error) { return s.e.models, nil }
func (s *fakeHLModels) Catalog(context.Context) (coreapi.ModelCatalogState, error) {
	return coreapi.ModelCatalogState{}, nil
}
func (s *fakeHLModels) Upsert(context.Context, coreapi.UpsertModelRequest) error { return nil }
func (s *fakeHLModels) Save(context.Context, coreapi.ModelSaveRequest) error     { return nil }
func (s *fakeHLModels) Delete(context.Context, coreapi.ModelNameRequest) error   { return nil }
func (s *fakeHLModels) Activate(context.Context, coreapi.ModelNameRequest) error {
	return nil
}
func (s *fakeHLModels) SyncEnv(context.Context) error { return nil }
func (s *fakeHLModels) Context(context.Context, coreapi.ModelContextRequest) (coreapi.ModelContextSnapshot, error) {
	return coreapi.ModelContextSnapshot{}, nil
}
func (s *fakeHLModels) SetWorkspace(context.Context, coreapi.SetWorkspaceModelRequest) error {
	return nil
}
func (s *fakeHLModels) ClearWorkspace(context.Context, coreapi.ClearWorkspaceModelRequest) error {
	return nil
}
func (s *fakeHLModels) SetSession(context.Context, coreapi.SetSessionModelRequest) error {
	return nil
}
func (s *fakeHLModels) ClearSession(context.Context, coreapi.ClearSessionModelRequest) error {
	return nil
}

type fakeHLTurns struct{ e *fakeHLSessionEngine }

func (s *fakeHLTurns) Start(context.Context, coreapi.StartTurnRequest) (coreapi.Turn, error) {
	return coreapi.Turn{ID: s.e.turnID}, nil
}
func (s *fakeHLTurns) Interrupt(context.Context, coreapi.TurnRef) error { return nil }
func (s *fakeHLTurns) Resume(context.Context, coreapi.TurnRef) (coreapi.Turn, error) {
	return coreapi.Turn{}, nil
}
