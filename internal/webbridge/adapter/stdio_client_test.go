package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	"github.com/eosaios/eos/pkg/protocol"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
	"github.com/eosaios/eos/pkg/sandbox"
)

// --- In-memory pipe-based tests (no external binary needed) ---

// newPipeStdioClient creates a StdioClient whose stream is wired to a
// net.Pipe connection instead of a real process. This lets us test the
// protocol framing and client behavior without needing the external binary.
func newPipeStdioClient(t *testing.T) (*StdioClient, net.Conn) {
	t.Helper()
	clientConn, serverConn := net.Pipe()

	client := &StdioClient{
		opts: StdioClientOptions{},
		done: make(chan struct{}),
	}

	br := bufio.NewReader(serverConn)
	sa := &streamAdapter{
		reader: br,
		writer: serverConn,
		closer: serverConn,
	}
	client.stream = sa
	client.client = newStdioRPCClient(sa)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := client.client.Start(ctx); err != nil {
		t.Fatalf("stdioRPCClient.Start() error = %v", err)
	}

	return client, clientConn
}

// serveMockServer runs a minimal mock JSON-RPC server on the given connection.
func serveMockServer(t *testing.T, conn net.Conn, workspace string, sessionID string) {
	t.Helper()
	stream := protocoljsonrpc.NewStream(conn, conn)

	go func() {
		for {
			msg, err := stream.ReadMessage()
			if err != nil {
				return
			}
			if msg.Request == nil {
				continue
			}

			req := msg.Request
			var resp protocoljsonrpc.Response

			switch req.Method {
			case protocoljsonrpc.MethodInitialize:
				result := coreapijsonrpc.InitializeResult{
					ServerName:      "eos-core",
					ProtocolVersion: "v1",
					Methods: []string{
						protocoljsonrpc.MethodInitialize,
						protocoljsonrpc.MethodStateSnapshot,
						protocoljsonrpc.MethodSessionList,
						protocoljsonrpc.MethodRuntimeModesGet,
						protocoljsonrpc.MethodWorkspaceList,
						protocoljsonrpc.MethodWorkspaceDefault,
						protocoljsonrpc.MethodWorkspaceLast,
					},
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, result)

			case protocoljsonrpc.MethodStateSnapshot:
				snapshot := coreapi.StateSnapshot{
					Workspaces: []coreapi.WorkspaceSnapshot{
						{Path: workspace, Trusted: true, Active: true, CurrentSessionID: sessionID},
					},
					CurrentSession: &coreapi.SessionSnapshot{ID: sessionID, WorkspacePath: workspace},
					Messages:       []coreapi.SessionMessage{},
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, snapshot)

			case protocoljsonrpc.MethodRuntimeModesGet:
				modes := coreapi.ModeSnapshot{
					ExecutionMode:  "auto",
					SandboxMode:    "workspace",
					ReasoningLevel: "medium",
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, modes)

			case protocoljsonrpc.MethodWorkspaceList:
				workspaces := []coreapi.Workspace{
					{Path: workspace, Trusted: true, Active: true},
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, workspaces)

			case protocoljsonrpc.MethodWorkspaceDefault:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, workspace)

			case protocoljsonrpc.MethodWorkspaceLast:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, workspace)

			case protocoljsonrpc.MethodSessionList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.Session{})

			default:
				resp, _ = protocoljsonrpc.NewErrorResponse(req.ID, protocoljsonrpc.CodeMethodNotFound, "method not found", nil)
			}

			_ = stream.WriteMessage(resp)
		}
	}()
}

// TestStdioGatewayInitializeViaPipe verifies initialize over a pipe-based connection.
func TestStdioGatewayInitializeViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServer(t, serverConn, workspace, "test-session-1")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := gateway.CoreInitializeRPC(ctx)
	if err != nil {
		t.Fatalf("CoreInitializeRPC() error = %v", err)
	}
	if result.ServerName != "eos-core" {
		t.Fatalf("ServerName = %q, want eos-core", result.ServerName)
	}
	if len(result.Methods) == 0 {
		t.Fatal("expected non-empty Methods list from initialize")
	}
}

// TestStdioGatewayStateSnapshotViaPipe verifies state/snapshot over a pipe.
func TestStdioGatewayStateSnapshotViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	sessionID := "test-session-2"
	serveMockServer(t, serverConn, workspace, sessionID)

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snapshot, err := gateway.CoreStateSnapshotRPC(ctx)
	if err != nil {
		t.Fatalf("CoreStateSnapshotRPC() error = %v", err)
	}
	if snapshot.CurrentSession == nil || snapshot.CurrentSession.ID != sessionID {
		t.Fatalf("CurrentSession.ID = %v, want %q", snapshot.CurrentSession, sessionID)
	}
	if len(snapshot.Workspaces) != 1 {
		t.Fatalf("Workspaces len = %d, want 1", len(snapshot.Workspaces))
	}
}

// TestStdioGatewayModeSnapshotViaPipe verifies runtime/modes/get over a pipe.
func TestStdioGatewayModeSnapshotViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServer(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	modes, err := gateway.CoreModeSnapshotRPC(ctx)
	if err != nil {
		t.Fatalf("CoreModeSnapshotRPC() error = %v", err)
	}
	if modes.ExecutionMode != "auto" {
		t.Fatalf("ExecutionMode = %q, want auto", modes.ExecutionMode)
	}
	if modes.SandboxMode != "workspace" {
		t.Fatalf("SandboxMode = %q, want workspace", modes.SandboxMode)
	}
}

// TestStdioGatewayWorkspaceListViaPipe verifies workspace/list over a pipe.
func TestStdioGatewayWorkspaceListViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServer(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	workspaces, err := gateway.CoreListWorkspacesRPC(ctx)
	if err != nil {
		t.Fatalf("CoreListWorkspacesRPC() error = %v", err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("workspaces len = %d, want 1", len(workspaces))
	}
}

// TestStdioGatewayDefaultWorkspaceViaPipe verifies workspace/default over a pipe.
func TestStdioGatewayDefaultWorkspaceViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServer(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	path, err := gateway.CoreDefaultWorkspaceRPC(ctx)
	if err != nil {
		t.Fatalf("CoreDefaultWorkspaceRPC() error = %v", err)
	}
	if filepath.Clean(path) != filepath.Clean(workspace) {
		t.Fatalf("path = %q, want %q", path, workspace)
	}
}

// TestStdioGatewaySessionListViaPipe verifies session/list over a pipe.
func TestStdioGatewaySessionListViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServer(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sessions, err := gateway.CoreListSessionsRPC(ctx, workspace)
	if err != nil {
		t.Fatalf("CoreListSessionsRPC() error = %v", err)
	}
	if sessions == nil {
		t.Fatal("expected non-nil sessions list")
	}
}

// TestStdioGatewayRuntimeSnapshotViaPipe verifies the mapped runtime snapshot.
func TestStdioGatewayRuntimeSnapshotViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	sessionID := "test-session-3"
	serveMockServer(t, serverConn, workspace, sessionID)

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snapshot, err := gateway.CoreRuntimeSnapshotRPC(ctx)
	if err != nil {
		t.Fatalf("CoreRuntimeSnapshotRPC() error = %v", err)
	}
	if snapshot.Workspaces == nil {
		t.Fatal("expected non-nil Workspaces in runtime snapshot")
	}
}

// serveMockServerWithWriteMethods runs a mock JSON-RPC server that handles
// the newly implemented write methods in addition to the read-only ones.
func serveMockServerWithWriteMethods(t *testing.T, conn net.Conn, workspace string, sessionID string) {
	t.Helper()
	stream := protocoljsonrpc.NewStream(conn, conn)

	go func() {
		for {
			msg, err := stream.ReadMessage()
			if err != nil {
				return
			}
			if msg.Request == nil {
				continue
			}

			req := msg.Request
			var resp protocoljsonrpc.Response

			switch req.Method {
			case protocoljsonrpc.MethodInitialize:
				result := coreapijsonrpc.InitializeResult{
					ServerName:      "eos-core",
					ProtocolVersion: "v1",
					Methods:         []string{protocoljsonrpc.MethodInitialize},
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, result)

			case protocoljsonrpc.MethodStateSnapshot:
				snapshot := coreapi.StateSnapshot{
					Workspaces: []coreapi.WorkspaceSnapshot{
						{Path: workspace, Trusted: true, Active: true, CurrentSessionID: sessionID},
					},
					CurrentSession: &coreapi.SessionSnapshot{ID: sessionID, WorkspacePath: workspace},
					Messages:       []coreapi.SessionMessage{},
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, snapshot)

			case protocoljsonrpc.MethodRuntimeModesGet:
				modes := coreapi.ModeSnapshot{
					ExecutionMode:  "auto",
					SandboxMode:    "workspace",
					ReasoningLevel: "medium",
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, modes)

			case protocoljsonrpc.MethodWorkspaceList:
				workspaces := []coreapi.Workspace{
					{Path: workspace, Trusted: true, Active: true},
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, workspaces)

			case protocoljsonrpc.MethodWorkspaceDefault, protocoljsonrpc.MethodWorkspaceLast:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, workspace)

			case protocoljsonrpc.MethodSessionList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.Session{})

			case protocoljsonrpc.MethodApprovalList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.PendingApprovalList{
					Approvals: []coreapi.PendingApprovalItem{
						{ApprovalID: "approval-1", SessionID: sessionID, ToolName: "request_user_input"},
					},
				})

			// Write methods — return {"ok": true}
			case protocoljsonrpc.MethodWorkspaceAdd,
				protocoljsonrpc.MethodWorkspaceRemove,
				protocoljsonrpc.MethodWorkspaceUse,
				protocoljsonrpc.MethodWorkspaceTrust,
				protocoljsonrpc.MethodWorkspaceRemember,
				protocoljsonrpc.MethodRuntimeExecutionModeSet,
				protocoljsonrpc.MethodRuntimeSandboxModeSet,
				protocoljsonrpc.MethodRuntimeReasoningLevelSet,
				protocoljsonrpc.MethodSessionDelete,
				protocoljsonrpc.MethodSessionSetCurrent,
				protocoljsonrpc.MethodApprovalRespond,
				protocoljsonrpc.MethodInquiryRespond,
				protocoljsonrpc.MethodModelUpsert,
				protocoljsonrpc.MethodModelSave,
				protocoljsonrpc.MethodModelActivate,
				protocoljsonrpc.MethodModelDelete,
				protocoljsonrpc.MethodMCPUpsert,
				protocoljsonrpc.MethodMCPImportJSON,
				protocoljsonrpc.MethodMCPDelete,
				protocoljsonrpc.MethodMCPSetEnabled,
				protocoljsonrpc.MethodExtensionsSkillsReload,
				protocoljsonrpc.MethodExtensionsSkillSetEnabled,
				protocoljsonrpc.MethodExtensionsPluginSetEnabled,
				protocoljsonrpc.MethodVersionsRollback,
				protocoljsonrpc.MethodVersionsDelete,
				protocoljsonrpc.MethodVersionsClear,
				protocoljsonrpc.MethodTaskKill,
				protocoljsonrpc.MethodTaskCleanup,
				protocoljsonrpc.MethodEventUnsubscribe,
				protocoljsonrpc.MethodTurnInterrupt,
				protocoljsonrpc.MethodSandboxSetPolicy:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]any{"ok": true})

			case protocoljsonrpc.MethodEventSubscribe:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]any{
					"subscription_id": "sub-1",
					"active":          true,
				})

			case protocoljsonrpc.MethodTurnStart:
				var params coreapi.StartTurnRequest
				_ = json.Unmarshal(req.Params, &params)
				if strings.TrimSpace(params.TurnID) == "" {
					params.TurnID = "turn-1"
				}
				turn := coreapi.Turn{
					ID:        params.TurnID,
					SessionID: params.SessionID,
					Status:    "running",
					StartedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, turn)
				_ = stream.WriteMessage(resp)
				env := protocol.NewEvent(protocol.EventTypeRequestDone, protocol.EventOptions{
					RequestID:     params.TurnID,
					SessionID:     params.SessionID,
					CorrelationID: params.TurnID,
					Payload:       map[string]any{"turn_id": params.TurnID},
				})
				notif, _ := protocoljsonrpc.NewNotification(protocoljsonrpc.NotificationEvent, env)
				_ = stream.WriteMessage(notif)
				continue

			case protocoljsonrpc.MethodModelList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.ModelConfig{
					{Name: "test-model", Model: "gpt-4", Active: true},
				})

			case protocoljsonrpc.MethodModelCatalog:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.ModelCatalogState{
					Providers: []coreapi.ModelProviderOption{{ID: "openai", Name: "OpenAI"}},
				})

			case protocoljsonrpc.MethodMCPList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.MCPServer{
					{Name: "test-mcp", Type: "stdio", Target: "echo", Enabled: true},
				})

			case protocoljsonrpc.MethodLSPList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.LSPServer{
					{Language: "go", Status: "running"},
				})

			case protocoljsonrpc.MethodLSPDetect, protocoljsonrpc.MethodLSPStart, protocoljsonrpc.MethodLSPInstall:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]string{"message": "ok"})

			case protocoljsonrpc.MethodExtensionsSkillsList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.SkillInfo{
					{Name: "test-skill", Enabled: true},
				})

			case protocoljsonrpc.MethodExtensionsPluginsList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.PluginInfo{
					{Name: "test-plugin", Enabled: true},
				})

			case protocoljsonrpc.MethodWorkspaceWorktreeList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.Worktree{
					{Name: "main", Path: workspace, Active: true},
				})

			case protocoljsonrpc.MethodUsageSummary:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.UsageSummary{
					Rounds: 10,
				})

			case protocoljsonrpc.MethodUsageCostItems:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.CostItem{
					{Model: "gpt-4"},
				})

			case protocoljsonrpc.MethodUsageCostSummary:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]string{"text": "$0.01"})

			case protocoljsonrpc.MethodVersionsList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.VersionItem{
					{ID: "v1", Summary: "test"},
				})

			case protocoljsonrpc.MethodConfigSettingsGet:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.Settings{
					Language: "en", Theme: "dark",
				})

			case protocoljsonrpc.MethodPermissionSnapshot:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.PermissionSnapshot{
					AllowedCategories: []string{"read"},
				})

			case protocoljsonrpc.MethodInsightPlanSnapshot:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.PlanSnapshot{
					HasPlan: true, Content: "test plan",
				})

			case protocoljsonrpc.MethodMemorySnapshot:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.MemorySnapshot{
					Documents: []coreapi.MemoryDocument{{Scope: "memory_summary.md", Path: "memory_summary.md"}},
				})

			case protocoljsonrpc.MethodMemorySave:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]any{"ok": true})

			case protocoljsonrpc.MethodGitBranches:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.GitBranchesResult{
					Current:  "feat/memory",
					Branches: []string{"feat/memory", "main"},
				})

			case protocoljsonrpc.MethodPermissionPendingReview:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.PendingReview{
					Path: "test.go", HasDiff: true,
				})

			case protocoljsonrpc.MethodLSPDiagnostics:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []string{"diag-1"})

			case protocoljsonrpc.MethodContextPreview:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []string{"file1.go"})

			case protocoljsonrpc.MethodContextStats:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.ContextStats{
					MessageCount: 10, Estimated: 5000,
				})

			case protocoljsonrpc.MethodInsightPredictNextUser:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]string{"message": "predicted"})

			case protocoljsonrpc.MethodRemoteWorkspaceList:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.RemoteWorkspace{
					{ID: "rw-1", Kind: "codespace"},
				})

			case protocoljsonrpc.MethodRemoteWorkspaceOpen:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.RemoteWorkspace{
					ID:       "rw-1",
					Kind:     "codespace",
					Platform: "github",
					Repo:     "test-repo",
				})

			case protocoljsonrpc.MethodRemoteWorkspaceForget,
				protocoljsonrpc.MethodRemoteWorkspaceClearCache:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]any{"ok": true})

			case protocoljsonrpc.MethodRemoteRepoCurrent:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]any{
					"ok":    true,
					"state": coreapi.RemoteRepoState{Repo: "test-repo", Platform: "github"},
				})

			case protocoljsonrpc.MethodSandboxBackend:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]any{
					"available": true, "type": "docker",
				})

			case protocoljsonrpc.MethodSandboxPolicy:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, sandbox.Policy{Mode: "workspace"})

			// Session create/resume/rename — return a Session
			case protocoljsonrpc.MethodSessionCreate,
				protocoljsonrpc.MethodSessionResume,
				protocoljsonrpc.MethodSessionRename:
				s := coreapi.Session{
					ID:            "new-session-1",
					WorkspaceRoot: workspace,
					Metadata:      map[string]any{"title": "new session"},
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, s)

			// Session messages load — return empty list
			case protocoljsonrpc.MethodSessionMessagesLoad:
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.SessionMessage{})

			// Session messages save — return a Session
			case protocoljsonrpc.MethodSessionMessagesSave:
				s := coreapi.Session{
					ID:            sessionID,
					WorkspaceRoot: workspace,
				}
				resp, _ = protocoljsonrpc.NewResultResponse(req.ID, s)

			default:
				resp, _ = protocoljsonrpc.NewErrorResponse(req.ID, protocoljsonrpc.CodeMethodNotFound, "method not found", nil)
			}

			_ = stream.WriteMessage(resp)
		}
	}()
}

// TestStdioGatewayWorkspaceAddViaPipe verifies workspace/add over a pipe.
func TestStdioGatewayWorkspaceAddViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreAddWorkspaceRPC(ctx, workspace); err != nil {
		t.Fatalf("CoreAddWorkspaceRPC() error = %v", err)
	}
}

// TestStdioGatewayWorkspaceTrustViaPipe verifies workspace/trust over a pipe.
func TestStdioGatewayWorkspaceTrustViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreTrustWorkspaceRPC(ctx, workspace); err != nil {
		t.Fatalf("CoreTrustWorkspaceRPC() error = %v", err)
	}
}

// TestStdioGatewaySetExecutionModeViaPipe verifies runtime/execution_mode/set over a pipe.
func TestStdioGatewaySetExecutionModeViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreSetExecutionModeRPC(ctx, "plan"); err != nil {
		t.Fatalf("CoreSetExecutionModeRPC() error = %v", err)
	}
}

// TestStdioGatewaySetReasoningLevelViaPipe verifies runtime/reasoning_level/set over a pipe.
func TestStdioGatewaySetReasoningLevelViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreSetReasoningLevelRPC(ctx, "high"); err != nil {
		t.Fatalf("CoreSetReasoningLevelRPC() error = %v", err)
	}
}

// TestStdioGatewayCreateSessionViaPipe verifies session/create over a pipe.
func TestStdioGatewayCreateSessionViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	meta, err := gateway.CoreCreateSessionRPC(ctx, workspace, "test session", "gui", nil)
	if err != nil {
		t.Fatalf("CoreCreateSessionRPC() error = %v", err)
	}
	if meta.ID != "new-session-1" {
		t.Fatalf("session ID = %q, want new-session-1", meta.ID)
	}
}

// TestStdioGatewayDeleteSessionViaPipe verifies session/delete over a pipe.
func TestStdioGatewayDeleteSessionViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "sess-1")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreDeleteSessionRPC(ctx, workspace, "sess-1"); err != nil {
		t.Fatalf("CoreDeleteSessionRPC() error = %v", err)
	}
}

// TestStdioGatewayLoadSessionMessagesViaPipe verifies session/messages/load over a pipe.
func TestStdioGatewayLoadSessionMessagesViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "sess-1")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msgs, err := gateway.CoreLoadSessionMessagesRPC(ctx, workspace, "sess-1")
	if err != nil {
		t.Fatalf("CoreLoadSessionMessagesRPC() error = %v", err)
	}
	if msgs == nil {
		t.Fatal("expected non-nil messages list")
	}
}

// TestStdioGatewayResumeSessionViaPipe verifies session/resume over a pipe.
func TestStdioGatewayResumeSessionViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "sess-1")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	meta, err := gateway.CoreResumeSessionRPC(ctx, workspace, "sess-1")
	if err != nil {
		t.Fatalf("CoreResumeSessionRPC() error = %v", err)
	}
	if meta.ID != "new-session-1" {
		t.Fatalf("session ID = %q, want new-session-1", meta.ID)
	}
}

// TestStdioGatewayRespondApprovalViaPipe verifies approval/respond over a pipe.
func TestStdioGatewayRespondApprovalViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreRespondApprovalRPC(ctx, "approval-1", coreapi.ApprovalAccept); err != nil {
		t.Fatalf("CoreRespondApprovalRPC() error = %v", err)
	}
}

func TestStdioGatewayApprovalListViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	workspace := t.TempDir()
	serveMockServerWithWriteMethods(t, serverConn, workspace, "sess-1")

	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	list, err := gateway.CoreApprovalListRPC(ctx, coreapi.PendingApprovalListRequest{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("CoreApprovalListRPC() error = %v", err)
	}
	if len(list.Approvals) != 1 || list.Approvals[0].ApprovalID != "approval-1" {
		t.Fatalf("approvals = %+v, want approval-1", list.Approvals)
	}
}

func TestStdioGatewayListModelsViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models, err := gateway.CoreListModelsRPC(ctx)
	if err != nil {
		t.Fatalf("CoreListModelsRPC() error = %v", err)
	}
	if len(models) != 1 || models[0].Name != "test-model" {
		t.Fatalf("models = %v, want [{test-model}]", models)
	}
}

func TestStdioGatewayModelCatalogViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	catalog, err := gateway.CoreModelCatalogRPC(ctx)
	if err != nil {
		t.Fatalf("CoreModelCatalogRPC() error = %v", err)
	}
	if len(catalog.Providers) != 1 || catalog.Providers[0].ID != "openai" {
		t.Fatalf("catalog.Providers = %v", catalog.Providers)
	}
}

func TestStdioGatewayUpsertModelViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreUpsertModelRPC(ctx, "test", "http://localhost", "key", "gpt-4"); err != nil {
		t.Fatalf("CoreUpsertModelRPC() error = %v", err)
	}
}

func TestStdioGatewayListMCPViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	servers, err := gateway.CoreListMCPRPC(ctx)
	if err != nil {
		t.Fatalf("CoreListMCPRPC() error = %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "test-mcp" {
		t.Fatalf("servers = %v", servers)
	}
}

func TestStdioGatewaySetMCPEnabledViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreSetMCPEnabledRPC(ctx, "test-mcp", false); err != nil {
		t.Fatalf("CoreSetMCPEnabledRPC() error = %v", err)
	}
}

func TestStdioGatewayListLSPViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	servers, err := gateway.CoreListLSPRPC(ctx)
	if err != nil {
		t.Fatalf("CoreListLSPRPC() error = %v", err)
	}
	if len(servers) != 1 || servers[0].Language != "go" {
		t.Fatalf("servers = %v", servers)
	}
}

func TestStdioGatewayDetectLSPViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg, err := gateway.CoreDetectLSPRPC(ctx, "go")
	if err != nil {
		t.Fatalf("CoreDetectLSPRPC() error = %v", err)
	}
	if msg != "ok" {
		t.Fatalf("message = %q, want ok", msg)
	}
}

func TestStdioGatewayInstallLSPViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg, err := gateway.CoreInstallLSPRPC(ctx, "go")
	if err != nil {
		t.Fatalf("CoreInstallLSPRPC() error = %v", err)
	}
	if msg != "ok" {
		t.Fatalf("message = %q, want ok", msg)
	}
}

func TestStdioGatewayListSkillsViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	skills, err := gateway.CoreListSkillsRPC(ctx)
	if err != nil {
		t.Fatalf("CoreListSkillsRPC() error = %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "test-skill" {
		t.Fatalf("skills = %v", skills)
	}
}

func TestStdioGatewayListPluginsViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	plugins, err := gateway.CoreListPluginsRPC(ctx)
	if err != nil {
		t.Fatalf("CoreListPluginsRPC() error = %v", err)
	}
	if len(plugins) != 1 || plugins[0].Name != "test-plugin" {
		t.Fatalf("plugins = %v", plugins)
	}
}

func TestStdioGatewayListWorktreesViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	trees, err := gateway.CoreListWorktreesRPC(ctx)
	if err != nil {
		t.Fatalf("CoreListWorktreesRPC() error = %v", err)
	}
	if len(trees) != 1 || trees[0].Name != "main" {
		t.Fatalf("worktrees = %v", trees)
	}
}

func TestStdioGatewayUsageSummaryViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	summary, err := gateway.CoreUsageSummaryRPC(ctx)
	if err != nil {
		t.Fatalf("CoreUsageSummaryRPC() error = %v", err)
	}
	if summary.Rounds != 10 {
		t.Fatalf("Rounds = %d, want 10", summary.Rounds)
	}
}

func TestStdioGatewayCostSummaryViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	summary, err := gateway.CoreCostSummaryRPC(ctx)
	if err != nil {
		t.Fatalf("CoreCostSummaryRPC() error = %v", err)
	}
	if summary != "$0.01" {
		t.Fatalf("summary = %q, want $0.01", summary)
	}
}

func TestStdioGatewayListVersionsViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	versions, err := gateway.CoreListVersionsRPC(ctx)
	if err != nil {
		t.Fatalf("CoreListVersionsRPC() error = %v", err)
	}
	if len(versions) != 1 || versions[0].ID != "v1" {
		t.Fatalf("versions = %v", versions)
	}
}

func TestStdioGatewayClearVersionsViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := gateway.CoreClearVersionsRPC(ctx)
	if err != nil {
		t.Fatalf("CoreClearVersionsRPC() error = %v", err)
	}
}

func TestStdioGatewayGetSettingsViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	settings, err := gateway.CoreGetSettingsRPC(ctx)
	if err != nil {
		t.Fatalf("CoreGetSettingsRPC() error = %v", err)
	}
	if settings.Language != "en" || settings.Theme != "dark" {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestStdioGatewayPermissionSnapshotViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snapshot, err := gateway.CorePermissionSnapshotRPC(ctx)
	if err != nil {
		t.Fatalf("CorePermissionSnapshotRPC() error = %v", err)
	}
	if len(snapshot.AllowedCategories) != 1 || snapshot.AllowedCategories[0] != "read" {
		t.Fatalf("AllowedCategories = %v", snapshot.AllowedCategories)
	}
}

func TestStdioGatewayPlanSnapshotViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snapshot, err := gateway.CorePlanSnapshotRPC(ctx)
	if err != nil {
		t.Fatalf("CorePlanSnapshotRPC() error = %v", err)
	}
	if !snapshot.HasPlan || snapshot.Content != "test plan" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestStdioGatewayMemorySnapshotViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snapshot, err := gateway.CoreMemorySnapshotRPC(ctx)
	if err != nil {
		t.Fatalf("CoreMemorySnapshotRPC() error = %v", err)
	}
	if len(snapshot.Documents) != 1 || snapshot.Documents[0].Scope != "memory_summary.md" {
		t.Fatalf("snapshot.Documents = %v", snapshot.Documents)
	}
}

func TestStdioGatewayMemorySaveViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreMemorySaveRPC(ctx, "remember prefer tabs"); err != nil {
		t.Fatalf("CoreMemorySaveRPC() error = %v", err)
	}
}

func TestStdioGatewayGitBranchesViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := gateway.CoreGitBranchesRPC(ctx, "C:/work/repo")
	if err != nil {
		t.Fatalf("CoreGitBranchesRPC() error = %v", err)
	}
	if result.Current != "feat/memory" || len(result.Branches) != 2 {
		t.Fatalf("CoreGitBranchesRPC() = %+v", result)
	}
}

func TestStdioGatewayPendingReviewViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	review, err := gateway.CorePendingReviewRPC(ctx)
	if err != nil {
		t.Fatalf("CorePendingReviewRPC() error = %v", err)
	}
	if review.Path != "test.go" || !review.HasDiff {
		t.Fatalf("review = %+v", review)
	}
}

func TestStdioGatewayContextPreviewViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	files, err := gateway.CoreContextPreviewRPC(ctx)
	if err != nil {
		t.Fatalf("CoreContextPreviewRPC() error = %v", err)
	}
	if len(files) != 1 || files[0] != "file1.go" {
		t.Fatalf("files = %v", files)
	}
}

func TestStdioGatewayContextStatsViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := gateway.CoreContextStatsRPC(ctx)
	if err != nil {
		t.Fatalf("CoreContextStatsRPC() error = %v", err)
	}
	if stats.MessageCount != 10 {
		t.Fatalf("MessageCount = %d, want 10", stats.MessageCount)
	}
}

func TestStdioGatewayLSPDiagnosticsViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	diags, err := gateway.CoreLSPDiagnosticsRPC(ctx)
	if err != nil {
		t.Fatalf("CoreLSPDiagnosticsRPC() error = %v", err)
	}
	if len(diags) != 1 || diags[0] != "diag-1" {
		t.Fatalf("diags = %v", diags)
	}
}

func TestStdioGatewayPredictNextUserMessageViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg, err := gateway.CorePredictNextUserMessageRPC(ctx, "hello")
	if err != nil {
		t.Fatalf("CorePredictNextUserMessageRPC() error = %v", err)
	}
	if msg != "predicted" {
		t.Fatalf("message = %q, want predicted", msg)
	}
}

func TestStdioGatewayListRemoteWorkspacesViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	items, err := gateway.CoreListRemoteWorkspacesRPC(ctx)
	if err != nil {
		t.Fatalf("CoreListRemoteWorkspacesRPC() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "rw-1" {
		t.Fatalf("items = %v", items)
	}
}

func TestStdioGatewayOpenRemoteWorkspaceViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := gateway.CoreOpenRemoteWorkspaceRPC(ctx, "rw-1")
	if err != nil {
		t.Fatalf("CoreOpenRemoteWorkspaceRPC() error = %v", err)
	}
	if item.ID != "rw-1" || item.Repo != "test-repo" {
		t.Fatalf("item = %+v", item)
	}
}

func TestStdioGatewayForgetRemoteWorkspaceViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreForgetRemoteWorkspaceRPC(ctx, "rw-1"); err != nil {
		t.Fatalf("CoreForgetRemoteWorkspaceRPC() error = %v", err)
	}
}

func TestStdioGatewayClearRemoteWorkspaceCacheViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreClearRemoteWorkspaceCacheRPC(ctx, "rw-1"); err != nil {
		t.Fatalf("CoreClearRemoteWorkspaceCacheRPC() error = %v", err)
	}
}

func TestStdioGatewayCurrentRemoteRepoViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	state, ok, err := gateway.CoreCurrentRemoteRepoRPC(ctx)
	if err != nil {
		t.Fatalf("CoreCurrentRemoteRepoRPC() error = %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if state.Repo != "test-repo" {
		t.Fatalf("Repo = %q, want test-repo", state.Repo)
	}
}

func TestStdioGatewayKillTaskViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gateway.CoreKillTaskRPC(ctx, "task-1"); err != nil {
		t.Fatalf("CoreKillTaskRPC() error = %v", err)
	}
}

func TestStdioGatewayCleanupTasksViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := gateway.CoreCleanupTasksRPC(ctx)
	if err != nil {
		t.Fatalf("CoreCleanupTasksRPC() error = %v", err)
	}
	_ = count
}

func TestStdioGatewaySandboxBackendStatusViaPipe(t *testing.T) {
	client, serverConn := newPipeStdioClient(t)
	defer client.Close()
	defer serverConn.Close()

	serveMockServerWithWriteMethods(t, serverConn, t.TempDir(), "")
	gateway := NewStdioGateway(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status, err := gateway.CoreSandboxBackendStatusRPC(ctx)
	if err != nil {
		t.Fatalf("CoreSandboxBackendStatusRPC() error = %v", err)
	}
	_ = status
}

// TestStdioGatewayNotImplementedMethods verifies that remaining large-scope
// methods still return the not-implemented error.
func TestStdioGatewayNotImplementedMethods(t *testing.T) {
	client := &StdioClient{done: make(chan struct{})}
	gateway := NewStdioGateway(client)

	ctx := context.Background()

	tests := []struct {
		name string
		err  error
	}{
		{"CoreRunBashStreamRPC", func() error {
			_, err := gateway.CoreRunBashStreamRPC(ctx, "")
			return err
		}()},
	}

	for _, tt := range tests {
		if tt.err == nil {
			t.Errorf("%s: expected error, got nil", tt.name)
		} else if !strings.Contains(tt.err.Error(), "not implemented") {
			t.Errorf("%s: error = %v, want 'not implemented'", tt.name, tt.err)
		}
	}
}

// TestStdioClientNotStartedCallFails verifies that calling Call before Start
// returns an error.
func TestStdioClientNotStartedCallFails(t *testing.T) {
	client := NewStdioClient(StdioClientOptions{})
	err := client.Call(context.Background(), "initialize", nil, &coreapijsonrpc.InitializeResult{})
	if err == nil {
		t.Fatal("expected error when calling Call before Start()")
	}
	if !strings.Contains(err.Error(), "not started") {
		t.Fatalf("error = %v, want 'not started'", err)
	}
}

// TestStdioClientCloseWithoutStart verifies that Close() can be called
// without Start() without panicking.
func TestStdioClientCloseWithoutStart(t *testing.T) {
	client := NewStdioClient(StdioClientOptions{})
	if err := client.Close(); err != nil {
		t.Fatalf("Close() without Start() error = %v", err)
	}
}

// TestStdioGatewayImplementsBridgeRuntimeGateway documents the compile-time check.
func TestStdioGatewayImplementsBridgeRuntimeGateway(t *testing.T) {
	gateway := &StdioGateway{}
	var _ interface {
		CoreInitializeRPC(context.Context) (coreapijsonrpc.InitializeResult, error)
		CoreStateSnapshotRPC(context.Context) (coreapi.StateSnapshot, error)
	} = gateway
}

func TestStdioGatewayModelCatalogWithVendoredSidecar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewStdioClient(StdioClientOptions{
		VerifyChecksum:   true,
		RequireSignature: true,
	})
	if err := client.Start(ctx); err != nil {
		if errors.Is(err, sidecar.ErrCoreBinaryNotFound) {
			t.Skipf("no vendored sidecar binary for this target: %v", err)
		}
		t.Fatalf("StdioClient.Start() error = %v", err)
	}
	defer client.Close()

	resolved := client.ResolvedBinary()
	if strings.TrimSpace(resolved.Path) == "" {
		t.Fatal("ResolvedBinary().Path is empty")
	}

	gateway := NewStdioGateway(client)
	catalog, err := gateway.CoreModelCatalogRPC(ctx)
	if err != nil {
		t.Fatalf("CoreModelCatalogRPC() error = %v", err)
	}
	if len(catalog.Providers) == 0 {
		t.Fatal("catalog.Providers is empty")
	}
	if len(catalog.Presets) == 0 {
		t.Fatal("catalog.Presets is empty")
	}

	var hasQwen, hasGPTCodex bool
	for _, preset := range catalog.Presets {
		switch preset.ID {
		case "qwen3.6-plus":
			hasQwen = true
		case "gpt-5.3-codex":
			hasGPTCodex = true
		}
	}
	if !hasQwen {
		t.Fatal("catalog presets missing qwen3.6-plus")
	}
	if !hasGPTCodex {
		t.Fatal("catalog presets missing gpt-5.3-codex")
	}
}

func TestStdioClientSurvivesStartupContextCancelWithVendoredSidecar(t *testing.T) {
	startCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	client := NewStdioClient(StdioClientOptions{
		VerifyChecksum:   true,
		RequireSignature: true,
	})
	if err := client.Start(startCtx); err != nil {
		cancel()
		if errors.Is(err, sidecar.ErrCoreBinaryNotFound) {
			t.Skipf("no vendored sidecar binary for this target: %v", err)
		}
		t.Fatalf("StdioClient.Start() error = %v", err)
	}
	cancel()
	defer client.Close()

	select {
	case <-client.Done():
		if state := client.ProcessState(); state != nil {
			t.Fatalf("stdio client exited after startup context cancel: exit=%d success=%v", state.ExitCode(), state.Success())
		}
		t.Fatal("stdio client exited after startup context cancel")
	case <-time.After(200 * time.Millisecond):
	}

	gateway := NewStdioGateway(client)
	catalog, err := gateway.CoreModelCatalogRPC(context.Background())
	if err != nil {
		t.Fatalf("CoreModelCatalogRPC() after startup context cancel error = %v", err)
	}
	if len(catalog.Providers) == 0 || len(catalog.Presets) == 0 {
		t.Fatalf("catalog unexpectedly empty after startup context cancel: providers=%d presets=%d", len(catalog.Providers), len(catalog.Presets))
	}
}

func TestStdioClientDerivedEnvClearsFakeProviderAndPassesStoreDir(t *testing.T) {
	client := NewStdioClient(StdioClientOptions{
		Workspace: `C:\workspace`,
		StoreDir:  `C:\Users\tester\.eos\core`,
	})

	env := client.derivedEnv()
	if env["EOS_MODEL_PROVIDER"] != "" {
		t.Fatalf("EOS_MODEL_PROVIDER=%q, want empty override", env["EOS_MODEL_PROVIDER"])
	}
	if env["EOS_CORE_STORE_DIR"] != `C:\Users\tester\.eos\core` {
		t.Fatalf("EOS_CORE_STORE_DIR=%q", env["EOS_CORE_STORE_DIR"])
	}
	if env["EOS_WORKSPACE_ROOT"] != `C:\workspace` {
		t.Fatalf("EOS_WORKSPACE_ROOT=%q", env["EOS_WORKSPACE_ROOT"])
	}
}
