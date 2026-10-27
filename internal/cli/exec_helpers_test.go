package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestExecWorkspaceRoot(t *testing.T) {
	// 显式 flag
	if got := execWorkspaceRoot(context.Background(), nil, "  /ws  "); got != "/ws" {
		t.Fatalf("flag = %q", got)
	}
	// 引擎前台
	eng := &fakeExecEngine{fg: "/fg"}
	if got := execWorkspaceRoot(context.Background(), eng, ""); got != "/fg" {
		t.Fatalf("fg = %q", got)
	}
	// cwd 回落
	eng2 := &fakeExecEngine{}
	got := execWorkspaceRoot(context.Background(), eng2, "")
	if got == "" {
		t.Fatal("cwd fallback")
	}
}

func TestWriteExecErrorAndOutput(t *testing.T) {
	// json 错误走 stdout
	writeExecJSON(os.Stdout, ExecResult{Error: "boom"})
	writeExecError("json", errStr("x"))
	writeExecError("text", errStr("y"))

	n := 1
	f := 1.5
	writeExecOutput("text", ExecResult{
		Content: "hello", Model: "m", DurationMs: 10,
		TotalTokens: &n, CostUSD: &f, Workspace: "/ws",
	})
	writeExecOutput("json", ExecResult{Content: "h"})
}

func TestGoVersionAndLegalCmd(t *testing.T) {
	v := goVersion()
	if v == "" {
		t.Fatal("empty go version")
	}
	cmd := newHiddenLegalCmd()
	if cmd == nil {
		t.Fatal("legal cmd")
	}
}

func TestSaveRawConfigDoc(t *testing.T) {
	setTestHome(t)
	path := filepath.Join(t.TempDir(), "cfg.json")
	doc := map[string]json.RawMessage{"a": json.RawMessage(`1`)}
	if err := saveRawConfigDoc(doc, path); err != nil {
		t.Fatal(err)
	}
	doc["a"] = json.RawMessage(`2`)
	if err := saveRawConfigDoc(doc, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "2") {
		t.Fatalf("doc = %q %v", data, err)
	}
}

type fakeExecEngine struct {
	fg string
}

func (e *fakeExecEngine) Caller() coreapi.Caller                 { return nil }
func (e *fakeExecEngine) State() coreapi.StateService            { return &fakeExecState{fg: e.fg} }
func (e *fakeExecEngine) Workspaces() coreapi.WorkspaceService   { return nil }
func (e *fakeExecEngine) Sessions() coreapi.SessionService       { return nil }
func (e *fakeExecEngine) MCP() coreapi.MCPService                { return nil }
func (e *fakeExecEngine) LSP() coreapi.LSPService                { return nil }
func (e *fakeExecEngine) Config() coreapi.ConfigService          { return nil }
func (e *fakeExecEngine) Permissions() coreapi.PermissionService { return nil }
func (e *fakeExecEngine) Extensions() coreapi.ExtensionService   { return nil }
func (e *fakeExecEngine) Context() coreapi.ContextService        { return nil }
func (e *fakeExecEngine) Usage() coreapi.UsageService            { return nil }
func (e *fakeExecEngine) Versions() coreapi.VersionService       { return nil }
func (e *fakeExecEngine) Tasks() coreapi.TaskService             { return nil }
func (e *fakeExecEngine) Goals() coreapi.GoalService             { return nil }
func (e *fakeExecEngine) Modes() coreapi.ModeService             { return nil }
func (e *fakeExecEngine) Models() coreapi.ModelService           { return nil }
func (e *fakeExecEngine) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	return nil
}
func (e *fakeExecEngine) Git() coreapi.GitService                     { return nil }
func (e *fakeExecEngine) Insights() coreapi.InsightService            { return nil }
func (e *fakeExecEngine) Memory() coreapi.MemoryService               { return nil }
func (e *fakeExecEngine) Roles() coreapi.RoleService                  { return nil }
func (e *fakeExecEngine) Turns() coreapi.TurnService                  { return nil }
func (e *fakeExecEngine) Approvals() coreapi.ApprovalService          { return nil }
func (e *fakeExecEngine) Inquiries() coreapi.InquiryService           { return nil }
func (e *fakeExecEngine) Agents() coreapi.AgentService                { return nil }
func (e *fakeExecEngine) Tools() coreapi.ToolExecutor                 { return nil }
func (e *fakeExecEngine) ToolCatalog() coreapi.ToolCatalogService     { return nil }
func (e *fakeExecEngine) ToolTelemetry() coreapi.ToolTelemetryService { return nil }
func (e *fakeExecEngine) Events() coreapi.EventSubscriber             { return nil }
func (e *fakeExecEngine) Sandbox() coreapi.SandboxService             { return nil }
func (e *fakeExecEngine) Diagnostics() coreapi.DiagnosticsService     { return nil }

type fakeExecState struct{ fg string }

func (s *fakeExecState) Snapshot(context.Context, coreapi.StateSnapshotRequest) (coreapi.StateSnapshot, error) {
	return coreapi.StateSnapshot{ForegroundWorkspace: s.fg}, nil
}

type errStr string

func (e errStr) Error() string { return string(e) }
