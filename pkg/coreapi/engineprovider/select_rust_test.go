package engineprovider

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
)

func TestSelectRustPaths(t *testing.T) {
	// start 失败
	_, err := Select(context.Background(), Options{
		StartRemote: func(context.Context, sidecar.ProcessOptions) (RemoteEngine, error) {
			return nil, errors.New("start fail")
		},
	})
	if err == nil {
		t.Fatal("start fail")
	}

	// Initialize 失败
	closed := false
	_, err = Select(context.Background(), Options{
		StartRemote: func(context.Context, sidecar.ProcessOptions) (RemoteEngine, error) {
			return &fakeRemote{initErr: errors.New("init fail"), onClose: func() { closed = true }}, nil
		},
	})
	if err == nil || !closed {
		t.Fatalf("init fail = %v closed=%v", err, closed)
	}

	// 缺方法
	sel, err := Select(context.Background(), Options{
		RequiredMethods: []string{"must.have"},
		StartRemote: func(context.Context, sidecar.ProcessOptions) (RemoteEngine, error) {
			return &fakeRemote{methods: []string{"other"}}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing = %v", err)
	}
	if len(sel.Missing) != 1 {
		t.Fatalf("missing list = %v", sel.Missing)
	}

	// 成功
	sel, err = Select(context.Background(), Options{
		RequiredMethods: []string{"ok.method"},
		StartRemote: func(context.Context, sidecar.ProcessOptions) (RemoteEngine, error) {
			return &fakeRemote{methods: []string{"ok.method"}}, nil
		},
	})
	if err != nil || sel.Engine == nil {
		t.Fatalf("ok = %+v %v", sel, err)
	}
	if err := sel.Close(); err != nil {
		t.Fatal(err)
	}
}

type fakeRemote struct {
	initErr  error
	methods  []string
	onClose  func()
}

func (f *fakeRemote) Initialize(context.Context) (coreapijsonrpc.InitializeResult, error) {
	if f.initErr != nil {
		return coreapijsonrpc.InitializeResult{}, f.initErr
	}
	return coreapijsonrpc.InitializeResult{Methods: f.methods}, nil
}
func (f *fakeRemote) Close() error {
	if f.onClose != nil {
		f.onClose()
	}
	return nil
}
func (f *fakeRemote) Caller() coreapi.Caller               { return nil }
func (f *fakeRemote) State() coreapi.StateService          { return nil }
func (f *fakeRemote) Workspaces() coreapi.WorkspaceService { return nil }
func (f *fakeRemote) Sessions() coreapi.SessionService     { return nil }
func (f *fakeRemote) MCP() coreapi.MCPService              { return nil }
func (f *fakeRemote) LSP() coreapi.LSPService              { return nil }
func (f *fakeRemote) Config() coreapi.ConfigService        { return nil }
func (f *fakeRemote) Permissions() coreapi.PermissionService {
	return nil
}
func (f *fakeRemote) Extensions() coreapi.ExtensionService { return nil }
func (f *fakeRemote) Context() coreapi.ContextService      { return nil }
func (f *fakeRemote) Usage() coreapi.UsageService          { return nil }
func (f *fakeRemote) Versions() coreapi.VersionService     { return nil }
func (f *fakeRemote) Tasks() coreapi.TaskService           { return nil }
func (f *fakeRemote) Goals() coreapi.GoalService           { return nil }
func (f *fakeRemote) Modes() coreapi.ModeService           { return nil }
func (f *fakeRemote) Models() coreapi.ModelService         { return nil }
func (f *fakeRemote) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	return nil
}
func (f *fakeRemote) Git() coreapi.GitService                     { return nil }
func (f *fakeRemote) Insights() coreapi.InsightService            { return nil }
func (f *fakeRemote) Memory() coreapi.MemoryService               { return nil }
func (f *fakeRemote) Roles() coreapi.RoleService                  { return nil }
func (f *fakeRemote) Turns() coreapi.TurnService                  { return nil }
func (f *fakeRemote) Approvals() coreapi.ApprovalService          { return nil }
func (f *fakeRemote) Inquiries() coreapi.InquiryService           { return nil }
func (f *fakeRemote) Agents() coreapi.AgentService                { return nil }
func (f *fakeRemote) Tools() coreapi.ToolExecutor                 { return nil }
func (f *fakeRemote) ToolCatalog() coreapi.ToolCatalogService     { return nil }
func (f *fakeRemote) ToolTelemetry() coreapi.ToolTelemetryService { return nil }
func (f *fakeRemote) Events() coreapi.EventSubscriber             { return nil }
func (f *fakeRemote) Sandbox() coreapi.SandboxService             { return nil }
func (f *fakeRemote) Diagnostics() coreapi.DiagnosticsService     { return nil }
