package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
	"github.com/eosaios/eos/pkg/sandbox"
)

func TestRemoteEngineSupportedMethodsCallSidecar(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodStateSnapshot: coreapi.StateSnapshot{
				ForegroundWorkspace: "C:/work",
			},
			protocoljsonrpc.MethodWorkspaceList: []coreapi.Workspace{
				{Path: "C:/work", Active: true},
			},
			protocoljsonrpc.MethodWorkspaceRemember:      map[string]any{"ok": true},
			protocoljsonrpc.MethodWorkspaceSetForeground: map[string]any{"ok": true},
			protocoljsonrpc.MethodSessionList: []coreapi.Session{
				{ID: "session-1", WorkspaceRoot: "C:/work", CreatedAt: now, UpdatedAt: now},
			},
			protocoljsonrpc.MethodSessionCurrent: coreapi.Session{
				ID:            "session-1",
				WorkspaceRoot: "C:/work",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			protocoljsonrpc.MethodSessionCreate: coreapi.Session{
				ID:            "session-2",
				WorkspaceRoot: "C:/work",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			protocoljsonrpc.MethodSessionMessagesLoad: []coreapi.SessionMessage{
				{Role: "user", Content: "hello"},
			},
			protocoljsonrpc.MethodSessionMessagesSave: coreapi.Session{
				ID:            "session-1",
				WorkspaceRoot: "C:/work",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			protocoljsonrpc.MethodSessionResume: coreapi.Session{
				ID:            "session-1",
				WorkspaceRoot: "C:/work",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			protocoljsonrpc.MethodInquiryRespond: map[string]any{"ok": true},
			protocoljsonrpc.MethodEventSubscribe: map[string]any{"subscription_id": "sub-1"},
			protocoljsonrpc.MethodTurnStart: coreapi.Turn{
				ID:        "turn-1",
				SessionID: "session-1",
				Status:    "running",
				StartedAt: now,
				UpdatedAt: now,
			},
			protocoljsonrpc.MethodTurnInterrupt: map[string]any{"ok": true},
		},
	}
	engine := NewRemoteEngine(caller)

	snapshot, err := engine.State().Snapshot(context.Background(), coreapi.StateSnapshotRequest{})
	if err != nil {
		t.Fatalf("State().Snapshot() error = %v", err)
	}
	if snapshot.ForegroundWorkspace != "C:/work" {
		t.Fatalf("ForegroundWorkspace=%q, want C:/work", snapshot.ForegroundWorkspace)
	}

	workspaces, err := engine.Workspaces().List(context.Background(), coreapi.WorkspaceListRequest{})
	if err != nil {
		t.Fatalf("Workspaces().List() error = %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].Path != "C:/work" {
		t.Fatalf("Workspaces().List()=%+v, want C:/work", workspaces)
	}
	if err := engine.Workspaces().Remember(context.Background(), coreapi.RememberWorkspaceRequest{Path: "C:/work", Foreground: true}); err != nil {
		t.Fatalf("Workspaces().Remember() error = %v", err)
	}
	if err := engine.Workspaces().SetForeground(context.Background(), coreapi.WorkspacePathRequest{Path: "C:/work"}); err != nil {
		t.Fatalf("Workspaces().SetForeground() error = %v", err)
	}

	sessions, err := engine.Sessions().List(context.Background(), coreapi.ListSessionsRequest{WorkspaceRoot: "C:/work"})
	if err != nil {
		t.Fatalf("Sessions().List() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "session-1" {
		t.Fatalf("Sessions().List()=%+v, want session-1", sessions)
	}

	created, err := engine.Sessions().Create(context.Background(), coreapi.CreateSessionRequest{WorkspaceRoot: "C:/work", Title: "New"})
	if err != nil {
		t.Fatalf("Sessions().Create() error = %v", err)
	}
	if created.ID != "session-2" {
		t.Fatalf("Sessions().Create().ID=%q, want session-2", created.ID)
	}

	resumed, err := engine.Sessions().Resume(context.Background(), coreapi.ResumeSessionRequest{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("Sessions().Resume() error = %v", err)
	}
	if resumed.ID != "session-1" {
		t.Fatalf("Sessions().Resume().ID=%q, want session-1", resumed.ID)
	}

	current, err := engine.Sessions().Current(context.Background(), coreapi.CurrentSessionRequest{WorkspaceRoot: "C:/work"})
	if err != nil {
		t.Fatalf("Sessions().Current() error = %v", err)
	}
	if current.ID != "session-1" {
		t.Fatalf("Sessions().Current().ID=%q, want session-1", current.ID)
	}
	messages, err := engine.Sessions().LoadMessages(context.Background(), coreapi.LoadSessionMessagesRequest{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("Sessions().LoadMessages() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("Sessions().LoadMessages()=%+v, want hello", messages)
	}
	if _, err := engine.Sessions().SaveMessages(context.Background(), coreapi.SaveSessionMessagesRequest{SessionID: "session-1", Messages: messages}); err != nil {
		t.Fatalf("Sessions().SaveMessages() error = %v", err)
	}
	if err := engine.Inquiries().Respond(context.Background(), coreapi.InquiryResponse{InquiryID: "inq-1", Text: "yes"}); err != nil {
		t.Fatalf("Inquiries().Respond() error = %v", err)
	}
	eventCtx, cancelEvents := context.WithCancel(context.Background())
	ch, err := engine.Events().Subscribe(eventCtx, coreapi.EventFilter{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("Events().Subscribe() error = %v", err)
	}
	cancelEvents()
	if _, ok := <-ch; ok {
		t.Fatalf("Events().Subscribe() channel is still open after cancel")
	}

	turn, err := engine.Turns().Start(context.Background(), coreapi.StartTurnRequest{SessionID: "session-1", Input: "hello"})
	if err != nil {
		t.Fatalf("Turns().Start() error = %v", err)
	}
	if turn.ID != "turn-1" || turn.SessionID != "session-1" {
		t.Fatalf("Turns().Start()=%+v, want turn-1/session-1", turn)
	}
	if err := engine.Turns().Interrupt(context.Background(), coreapi.TurnRef{SessionID: "session-1", TurnID: "turn-1"}); err != nil {
		t.Fatalf("Turns().Interrupt() error = %v", err)
	}

	wantMethods := []string{
		protocoljsonrpc.MethodStateSnapshot,
		protocoljsonrpc.MethodWorkspaceList,
		protocoljsonrpc.MethodWorkspaceRemember,
		protocoljsonrpc.MethodWorkspaceSetForeground,
		protocoljsonrpc.MethodSessionList,
		protocoljsonrpc.MethodSessionCreate,
		protocoljsonrpc.MethodSessionResume,
		protocoljsonrpc.MethodSessionCurrent,
		protocoljsonrpc.MethodSessionMessagesLoad,
		protocoljsonrpc.MethodSessionMessagesSave,
		protocoljsonrpc.MethodInquiryRespond,
		protocoljsonrpc.MethodEventSubscribe,
		protocoljsonrpc.MethodEventUnsubscribe,
		protocoljsonrpc.MethodTurnStart,
		protocoljsonrpc.MethodTurnInterrupt,
	}
	calls := caller.snapshotCalls()
	if len(calls) != len(wantMethods) {
		t.Fatalf("calls=%+v, want %d calls", calls, len(wantMethods))
	}
	for i, want := range wantMethods {
		if calls[i].method != want {
			t.Fatalf("call[%d].method=%q, want %q", i, calls[i].method, want)
		}
	}
}

func TestRemoteEngineInitializeCachesResult(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodInitialize: map[string]any{
				"server_name":      "rust-core",
				"protocol_version": "v1",
				"methods":          []string{protocoljsonrpc.MethodInitialize},
			},
		},
	}
	engine := NewRemoteEngine(caller)

	first, err := engine.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize() first error = %v", err)
	}
	second, err := engine.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize() second error = %v", err)
	}
	if first.ServerName != "rust-core" || second.ServerName != first.ServerName {
		t.Fatalf("Initialize() results = %+v / %+v, want cached rust-core", first, second)
	}
	initCalls := caller.snapshotCalls()
	if len(initCalls) != 1 || initCalls[0].method != protocoljsonrpc.MethodInitialize {
		t.Fatalf("calls=%+v, want one initialize call", initCalls)
	}
}

func TestRemoteEngineMigratedMethodsCallSidecar(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodWorkspaceDefault:        "C:/default",
			protocoljsonrpc.MethodWorkspaceLast:           "C:/last",
			protocoljsonrpc.MethodWorkspaceResolve:        "C:/resolved",
			protocoljsonrpc.MethodWorkspaceForget:         map[string]any{"ok": true},
			protocoljsonrpc.MethodWorkspaceAdd:            map[string]any{"ok": true},
			protocoljsonrpc.MethodWorkspaceRemove:         map[string]any{"ok": true},
			protocoljsonrpc.MethodWorkspaceUse:            map[string]any{"ok": true},
			protocoljsonrpc.MethodWorkspaceTrust:          map[string]any{"ok": true},
			protocoljsonrpc.MethodWorkspaceWorktreeList:   []coreapi.Worktree{},
			protocoljsonrpc.MethodWorkspaceWorktreeCreate: coreapi.Worktree{Name: "feat", Path: "/wt/feat"},
			protocoljsonrpc.MethodWorkspaceWorktreeRemove: map[string]any{"ok": true},
			protocoljsonrpc.MethodSessionSetCurrent:       map[string]any{"ok": true},
			protocoljsonrpc.MethodSessionDelete:           map[string]any{"ok": true},
			protocoljsonrpc.MethodSessionRename:           coreapi.Session{ID: "s1"},
			protocoljsonrpc.MethodConfigRulesGet:          "rule content",
			protocoljsonrpc.MethodConfigRulesSnapshot:     coreapi.RulesSnapshot{ActiveRoot: "/root"},
			protocoljsonrpc.MethodConfigRulesSave:         map[string]any{"ok": true},
			protocoljsonrpc.MethodConfigRulesReset:        map[string]any{"ok": true},
			protocoljsonrpc.MethodConfigSettingsGet:       coreapi.Settings{Language: "en"},
			protocoljsonrpc.MethodConfigSettingsSave:      map[string]any{"ok": true},
		},
	}
	engine := NewRemoteEngine(caller)

	d, err := engine.Workspaces().Default(context.Background())
	if err != nil || d != "C:/default" {
		t.Fatalf("Default() = %q, %v", d, err)
	}
	l, err := engine.Workspaces().Last(context.Background())
	if err != nil || l != "C:/last" {
		t.Fatalf("Last() = %q, %v", l, err)
	}
	r, err := engine.Workspaces().ResolveForeground(context.Background(), coreapi.ResolveForegroundWorkspaceRequest{Preferred: "C:/pref"})
	if err != nil || r != "C:/resolved" {
		t.Fatalf("ResolveForeground() = %q, %v", r, err)
	}
	if err := engine.Workspaces().Forget(context.Background(), coreapi.WorkspacePathRequest{Path: "C:/old"}); err != nil {
		t.Fatalf("Forget() error = %v", err)
	}
	if err := engine.Workspaces().Add(context.Background(), coreapi.WorkspacePathRequest{Path: "C:/new"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := engine.Workspaces().Remove(context.Background(), coreapi.WorkspacePathRequest{Path: "C:/old"}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := engine.Workspaces().Use(context.Background(), coreapi.WorkspacePathRequest{Path: "C:/use"}); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := engine.Workspaces().Trust(context.Background(), coreapi.WorkspacePathRequest{Path: "C:/trust"}); err != nil {
		t.Fatalf("Trust() error = %v", err)
	}
	wts, err := engine.Workspaces().ListWorktrees(context.Background())
	if err != nil || wts == nil {
		t.Fatalf("ListWorktrees() = %v, %v", wts, err)
	}
	wt, err := engine.Workspaces().CreateWorktree(context.Background(), coreapi.CreateWorktreeRequest{Name: "feat"})
	if err != nil || wt.Name != "feat" {
		t.Fatalf("CreateWorktree() = %+v, %v", wt, err)
	}
	if err := engine.Workspaces().RemoveWorktree(context.Background(), coreapi.RemoveWorktreeRequest{Path: "/wt/feat"}); err != nil {
		t.Fatalf("RemoveWorktree() error = %v", err)
	}
	if err := engine.Sessions().SetCurrent(context.Background(), coreapi.SetCurrentSessionRequest{SessionID: "s1"}); err != nil {
		t.Fatalf("SetCurrent() error = %v", err)
	}
	if err := engine.Sessions().Delete(context.Background(), coreapi.DeleteSessionRequest{SessionID: "s1"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	sess, err := engine.Sessions().Rename(context.Background(), coreapi.RenameSessionRequest{SessionID: "s1", Title: "new"})
	if err != nil || sess.ID != "s1" {
		t.Fatalf("Rename() = %+v, %v", sess, err)
	}
	rules, err := engine.Config().GetRules(context.Background())
	if err != nil || rules != "rule content" {
		t.Fatalf("GetRules() = %q, %v", rules, err)
	}
	snap, err := engine.Config().RulesSnapshot(context.Background())
	if err != nil || snap.ActiveRoot != "/root" {
		t.Fatalf("RulesSnapshot() = %+v, %v", snap, err)
	}
	if err := engine.Config().SaveRules(context.Background(), coreapi.SaveRulesRequest{Content: "new rule"}); err != nil {
		t.Fatalf("SaveRules() error = %v", err)
	}
	if err := engine.Config().ResetRules(context.Background()); err != nil {
		t.Fatalf("ResetRules() error = %v", err)
	}
	settings, err := engine.Config().GetSettings(context.Background())
	if err != nil || settings.Language != "en" {
		t.Fatalf("GetSettings() = %+v, %v", settings, err)
	}
	if err := engine.Config().SaveSettings(context.Background(), coreapi.Settings{Language: "zh"}); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}

	wantMethods := []string{
		protocoljsonrpc.MethodWorkspaceDefault,
		protocoljsonrpc.MethodWorkspaceLast,
		protocoljsonrpc.MethodWorkspaceResolve,
		protocoljsonrpc.MethodWorkspaceForget,
		protocoljsonrpc.MethodWorkspaceAdd,
		protocoljsonrpc.MethodWorkspaceRemove,
		protocoljsonrpc.MethodWorkspaceUse,
		protocoljsonrpc.MethodWorkspaceTrust,
		protocoljsonrpc.MethodWorkspaceWorktreeList,
		protocoljsonrpc.MethodWorkspaceWorktreeCreate,
		protocoljsonrpc.MethodWorkspaceWorktreeRemove,
		protocoljsonrpc.MethodSessionSetCurrent,
		protocoljsonrpc.MethodSessionDelete,
		protocoljsonrpc.MethodSessionRename,
		protocoljsonrpc.MethodConfigRulesGet,
		protocoljsonrpc.MethodConfigRulesSnapshot,
		protocoljsonrpc.MethodConfigRulesSave,
		protocoljsonrpc.MethodConfigRulesReset,
		protocoljsonrpc.MethodConfigSettingsGet,
		protocoljsonrpc.MethodConfigSettingsSave,
	}
	cfgCalls := caller.snapshotCalls()
	if len(cfgCalls) != len(wantMethods) {
		t.Fatalf("calls=%d, want %d", len(cfgCalls), len(wantMethods))
	}
	for i, want := range wantMethods {
		if cfgCalls[i].method != want {
			t.Fatalf("call[%d].method=%q, want %q", i, cfgCalls[i].method, want)
		}
	}
}

func TestRemoteEngineMapsRemoteUnsupportedError(t *testing.T) {
	caller := &fakeEngineCaller{
		errs: map[string]error{
			protocoljsonrpc.MethodToolExecute: errors.New("jsonrpc error -32603: unsupported operation"),
		},
	}
	engine := NewRemoteEngine(caller)

	_, err := engine.Tools().Execute(context.Background(), coreapi.ToolRequest{Name: "shell"})
	if !errors.Is(err, coreapi.ErrUnsupported) {
		t.Fatalf("Tools().Execute() error = %v, want ErrUnsupported", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].method != protocoljsonrpc.MethodToolExecute {
		t.Fatalf("calls=%+v, want tool/execute", caller.calls)
	}
}

func TestRemoteEngineAgentServiceUsesAgentControl(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodAgentControl: map[string]any{
				"agent_id": "agent_1",
				"status":   "running",
				"agents": []coreapi.Agent{
					{
						ID:            "agent_1",
						ParentAgentID: "parent_1",
						RoleID:        "reviewer",
						Task:          "inspect",
						Status:        "running",
						CreatedAt:     now,
						UpdatedAt:     now,
					},
				},
			},
			protocoljsonrpc.MethodAgentRun: coreapi.AgentRunResult{
				Agent: coreapi.Agent{
					ID:            "agent_1",
					ParentAgentID: "parent_1",
					RoleID:        "reviewer",
					Task:          "inspect",
					Status:        "completed",
					CreatedAt:     now,
					UpdatedAt:     now,
				},
				Output: "plan drafted",
			},
			protocoljsonrpc.MethodToolExecute: coreapi.ToolResult{
				Name:    "read",
				Status:  "ok",
				Display: "done",
				Output:  json.RawMessage(`{"text":"hello"}`),
			},
			protocoljsonrpc.MethodAgentInput: map[string]any{"ok": true},
		},
	}
	engine := NewRemoteEngine(caller)

	spawned, err := engine.Agents().Spawn(context.Background(), coreapi.SpawnAgentRequest{
		ParentAgentID: " parent_1 ",
		RoleID:        " reviewer ",
		Task:          " inspect ",
	})
	if err != nil {
		t.Fatalf("Agents().Spawn() error = %v", err)
	}
	if spawned.ID != "agent_1" || spawned.RoleID != "reviewer" {
		t.Fatalf("Agents().Spawn()=%+v, want agent_1/reviewer", spawned)
	}

	waited, err := engine.Agents().Wait(context.Background(), coreapi.AgentRef{AgentID: " agent_1 "})
	if err != nil {
		t.Fatalf("Agents().Wait() error = %v", err)
	}
	if waited.Status != "running" {
		t.Fatalf("Agents().Wait().Status=%q, want running", waited.Status)
	}

	run, err := engine.Agents().Run(context.Background(), coreapi.RunAgentRequest{AgentID: " agent_1 "})
	if err != nil {
		t.Fatalf("Agents().Run() error = %v", err)
	}
	if run.Agent.ID != "agent_1" {
		t.Fatalf("Agents().Run().Agent.ID=%q, want agent_1", run.Agent.ID)
	}
	if run.Agent.Status != "completed" {
		t.Fatalf("Agents().Run().Agent.Status=%q, want completed", run.Agent.Status)
	}
	if run.Output != "plan drafted" {
		t.Fatalf("Agents().Run().Output=%q, want plan drafted", run.Output)
	}

	items, err := engine.Agents().List(context.Background(), coreapi.ListAgentsRequest{})
	if err != nil {
		t.Fatalf("Agents().List() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "agent_1" {
		t.Fatalf("Agents().List()=%+v, want agent_1", items)
	}

	tool, err := engine.Agents().RunTool(context.Background(), coreapi.AgentToolRequest{
		AgentID:   " agent_1 ",
		SessionID: " session_1 ",
		TurnID:    " turn_1 ",
		Name:      " read ",
		Args:      json.RawMessage(`{"path":"README.md"}`),
	})
	if err != nil {
		t.Fatalf("Agents().RunTool() error = %v", err)
	}
	if tool.Name != "read" || tool.Display != "done" {
		t.Fatalf("Agents().RunTool()=%+v, want read/done", tool)
	}

	if err := engine.Agents().Close(context.Background(), coreapi.AgentRef{AgentID: " agent_1 "}); err != nil {
		t.Fatalf("Agents().Close() error = %v", err)
	}
	if err := engine.Agents().SendInput(context.Background(), coreapi.AgentInput{AgentID: "agent_1", Input: "hello"}); err != nil {
		t.Fatalf("Agents().SendInput() error = %v", err)
	}

	want := []string{
		protocoljsonrpc.MethodAgentControl,
		protocoljsonrpc.MethodAgentControl,
		protocoljsonrpc.MethodAgentRun,
		protocoljsonrpc.MethodAgentControl,
		protocoljsonrpc.MethodToolExecute,
		protocoljsonrpc.MethodAgentControl,
		protocoljsonrpc.MethodAgentInput,
	}
	if len(caller.calls) != len(want) {
		t.Fatalf("calls=%+v, want %d calls", caller.calls, len(want))
	}
	for i, method := range want {
		if caller.calls[i].method != method {
			t.Fatalf("call[%d].method=%q, want %q", i, caller.calls[i].method, method)
		}
	}
}

func TestRemoteEngineNilCallerReturnsCallerUnavailable(t *testing.T) {
	engine := NewRemoteEngine(nil)

	_, err := engine.State().Snapshot(context.Background(), coreapi.StateSnapshotRequest{})
	if !errors.Is(err, ErrCallerUnavailable) {
		t.Fatalf("State().Snapshot() error = %v, want ErrCallerUnavailable", err)
	}
}

func TestRemoteEngineSandboxBackendDegradesWhenUnavailable(t *testing.T) {
	caller := &fakeEngineCaller{
		errs: map[string]error{
			protocoljsonrpc.MethodSandboxBackend: errors.New("jsonrpc error -32601: method not found"),
		},
	}
	engine := NewRemoteEngine(caller)

	status := engine.Sandbox().BackendStatus(context.Background())
	if !status.Degraded {
		t.Fatalf("BackendStatus().Degraded=false, want true")
	}
	if status.Enforced {
		t.Fatalf("BackendStatus().Enforced=true, want false")
	}
	if status.Backend != "rust-sidecar" {
		t.Fatalf("BackendStatus().Backend=%q, want rust-sidecar", status.Backend)
	}
}

type fakeEngineCall struct {
	method string
	params any
}

type fakeEngineCaller struct {
	// mu：Events().Subscribe 的内部 goroutine 会并发 Call——所有字段
	// 读写必须持锁（否则 -race 下与测试主线程读 calls 竞争）。
	mu      sync.Mutex
	calls   []fakeEngineCall
	results map[string]any
	errs    map[string]error
}

func (f *fakeEngineCaller) Call(_ context.Context, method string, params any, out any) error {
	f.mu.Lock()
	f.calls = append(f.calls, fakeEngineCall{method: method, params: params})
	err := f.errs[method]
	result := f.results[method]
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if result == nil {
		return nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// snapshotCalls 带锁取调用快照（测试断言用）。
func (f *fakeEngineCaller) snapshotCalls() []fakeEngineCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeEngineCall(nil), f.calls...)
}

func TestRemoteEngineEventSubscribeReceivesNotifications(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodEventSubscribe: map[string]any{"subscription_id": "sub-1"},
		},
	}
	engine := NewRemoteEngine(caller)

	ch, err := engine.Events().Subscribe(context.Background(), coreapi.EventFilter{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	n := protocoljsonrpc.Notification{
		Method: protocoljsonrpc.NotificationEvent,
		Params: json.RawMessage(`{"event_type":"turn.started","source":"runtime","session_id":"s1","turn_id":"t1","payload":{"key":"val"}}`),
	}
	if err := engine.handleNotification(context.Background(), n); err != nil {
		t.Fatalf("handleNotification() error = %v", err)
	}

	select {
	case env := <-ch:
		if env.EventType != "turn.started" {
			t.Fatalf("EventType=%q, want turn.started", env.EventType)
		}
		if env.SessionID != "s1" {
			t.Fatalf("SessionID=%q, want s1", env.SessionID)
		}
		if env.TurnID != "t1" {
			t.Fatalf("TurnID=%q, want t1", env.TurnID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestRemoteEngineEventUnsubscribeClosesChannel(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodEventSubscribe:   map[string]any{"subscription_id": "sub-1"},
			protocoljsonrpc.MethodEventUnsubscribe: map[string]any{"ok": true},
		},
	}
	engine := NewRemoteEngine(caller)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := engine.Events().Subscribe(ctx, coreapi.EventFilter{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestRemoteEngineEventNotificationDispatchesToMultipleSubscribers(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodEventSubscribe: map[string]any{"subscription_id": "sub-1"},
		},
	}
	engine := NewRemoteEngine(caller)

	ch1, err := engine.Events().Subscribe(context.Background(), coreapi.EventFilter{})
	if err != nil {
		t.Fatalf("Subscribe() 1 error = %v", err)
	}
	ch2, err := engine.Events().Subscribe(context.Background(), coreapi.EventFilter{})
	if err != nil {
		t.Fatalf("Subscribe() 2 error = %v", err)
	}

	n := protocoljsonrpc.Notification{
		Method: protocoljsonrpc.NotificationEvent,
		Params: json.RawMessage(`{"event_type":"tool.executed","source":"runtime"}`),
	}
	if err := engine.handleNotification(context.Background(), n); err != nil {
		t.Fatalf("handleNotification() error = %v", err)
	}

	for i, ch := range []<-chan protocol.Envelope{ch1, ch2} {
		select {
		case env := <-ch:
			if env.EventType != "tool.executed" {
				t.Fatalf("subscriber %d: EventType=%q, want tool.executed", i, env.EventType)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestRemoteEngineEventNotificationBackpressure(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodEventSubscribe: map[string]any{"subscription_id": "sub-1"},
		},
	}
	engine := NewRemoteEngine(caller)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := engine.Events().Subscribe(ctx, coreapi.EventFilter{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	n := protocoljsonrpc.Notification{
		Method: protocoljsonrpc.NotificationEvent,
		Params: json.RawMessage(`{"event_type":"text.delta","source":"runtime"}`),
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			_ = engine.handleNotification(context.Background(), n)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleNotification blocked (backpressure failure)")
	}
}

func TestRemoteEngineEventCloseAllSubscribers(t *testing.T) {
	engine := NewRemoteEngine(&fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodEventSubscribe: map[string]any{"subscription_id": "sub-1"},
		},
	})

	ch1, _ := engine.Events().Subscribe(context.Background(), coreapi.EventFilter{})
	ch2, _ := engine.Events().Subscribe(context.Background(), coreapi.EventFilter{})

	engine.closeAllSubscribers()

	for i, ch := range []<-chan protocol.Envelope{ch1, ch2} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatalf("subscriber %d: channel should be closed", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestRemoteEngineEventIgnoresNonEventNotifications(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodEventSubscribe: map[string]any{"subscription_id": "sub-1"},
		},
	}
	engine := NewRemoteEngine(caller)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := engine.Events().Subscribe(ctx, coreapi.EventFilter{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	n := protocoljsonrpc.Notification{
		Method: "state/changed",
		Params: json.RawMessage(`{"key":"val"}`),
	}
	if err := engine.handleNotification(context.Background(), n); err != nil {
		t.Fatalf("handleNotification() error = %v", err)
	}

	select {
	case <-ch:
		t.Fatal("should not receive non-event notification")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRemoteEnginePermissionServiceCallsSidecar(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodPermissionSnapshot: coreapi.PermissionSnapshot{
				ExecutionMode: "plan",
				AccessMode:    "read-only",
			},
			protocoljsonrpc.MethodPermissionPendingReview: coreapi.PendingReview{
				Path:    "/tmp/review.diff",
				HasDiff: true,
			},
			protocoljsonrpc.MethodPermissionClearReview:     map[string]any{"ok": true},
			protocoljsonrpc.MethodPermissionAccessModeSet:   map[string]any{"ok": true},
			protocoljsonrpc.MethodPermissionApprovalModeSet: map[string]any{"ok": true},
		},
	}
	engine := NewRemoteEngine(caller)

	snap, err := engine.Permissions().Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Permissions().Snapshot() error = %v", err)
	}
	if snap.ExecutionMode != "plan" {
		t.Fatalf("Snapshot().ExecutionMode=%q, want plan", snap.ExecutionMode)
	}
	if snap.AccessMode != "read-only" {
		t.Fatalf("Snapshot().AccessMode=%q, want read-only", snap.AccessMode)
	}

	review, err := engine.Permissions().PendingReview(context.Background())
	if err != nil {
		t.Fatalf("Permissions().PendingReview() error = %v", err)
	}
	if review.Path != "/tmp/review.diff" || !review.HasDiff {
		t.Fatalf("PendingReview()=%+v, want /tmp/review.diff with HasDiff=true", review)
	}

	if err := engine.Permissions().ClearPendingReview(context.Background()); err != nil {
		t.Fatalf("Permissions().ClearPendingReview() error = %v", err)
	}

	if err := engine.Permissions().SetAccessMode(context.Background(), coreapi.SetModeRequest{Mode: "full"}); err != nil {
		t.Fatalf("Permissions().SetAccessMode() error = %v", err)
	}

	if err := engine.Permissions().SetApprovalMode(context.Background(), coreapi.SetModeRequest{Mode: "auto"}); err != nil {
		t.Fatalf("Permissions().SetApprovalMode() error = %v", err)
	}

	wantMethods := []string{
		protocoljsonrpc.MethodPermissionSnapshot,
		protocoljsonrpc.MethodPermissionPendingReview,
		protocoljsonrpc.MethodPermissionClearReview,
		protocoljsonrpc.MethodPermissionAccessModeSet,
		protocoljsonrpc.MethodPermissionApprovalModeSet,
	}
	calls := caller.snapshotCalls()
	if len(calls) != len(wantMethods) {
		t.Fatalf("calls=%+v, want %d calls", calls, len(wantMethods))
	}
	for i, want := range wantMethods {
		if calls[i].method != want {
			t.Fatalf("call[%d].method=%q, want %q", i, calls[i].method, want)
		}
	}
}

func TestRemoteEngineModeServiceCallsSidecar(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodRuntimeModesGet: coreapi.ModeSnapshot{
				ExecutionMode:  "auto",
				SandboxMode:    "full",
				ReasoningLevel: "high",
			},
			protocoljsonrpc.MethodRuntimeExecutionModeSet:  map[string]any{"ok": true},
			protocoljsonrpc.MethodRuntimeSandboxModeSet:    map[string]any{"ok": true},
			protocoljsonrpc.MethodRuntimeReasoningLevelSet: map[string]any{"ok": true},
		},
	}
	engine := NewRemoteEngine(caller)

	snap, err := engine.Modes().Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Modes().Snapshot() error = %v", err)
	}
	if snap.ExecutionMode != "auto" {
		t.Fatalf("Snapshot().ExecutionMode=%q, want auto", snap.ExecutionMode)
	}
	if snap.SandboxMode != "full" {
		t.Fatalf("Snapshot().SandboxMode=%q, want full", snap.SandboxMode)
	}
	if snap.ReasoningLevel != "high" {
		t.Fatalf("Snapshot().ReasoningLevel=%q, want high", snap.ReasoningLevel)
	}

	if err := engine.Modes().SetExecutionMode(context.Background(), coreapi.SetModeRequest{Mode: "plan"}); err != nil {
		t.Fatalf("Modes().SetExecutionMode() error = %v", err)
	}
	if err := engine.Modes().SetSandboxMode(context.Background(), coreapi.SetModeRequest{Mode: "strict"}); err != nil {
		t.Fatalf("Modes().SetSandboxMode() error = %v", err)
	}
	if err := engine.Modes().SetReasoningLevel(context.Background(), coreapi.SetModeRequest{Mode: "low"}); err != nil {
		t.Fatalf("Modes().SetReasoningLevel() error = %v", err)
	}

	wantMethods := []string{
		protocoljsonrpc.MethodRuntimeModesGet,
		protocoljsonrpc.MethodRuntimeExecutionModeSet,
		protocoljsonrpc.MethodRuntimeSandboxModeSet,
		protocoljsonrpc.MethodRuntimeReasoningLevelSet,
	}
	calls := caller.snapshotCalls()
	if len(calls) != len(wantMethods) {
		t.Fatalf("calls=%+v, want %d calls", calls, len(wantMethods))
	}
	for i, want := range wantMethods {
		if calls[i].method != want {
			t.Fatalf("call[%d].method=%q, want %q", i, calls[i].method, want)
		}
	}
}

func TestRemoteEngineModelServiceCallsSidecar(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodModelList: []coreapi.ModelConfig{
				{Name: "gpt-4o", Model: "gpt-4o", Active: true},
			},
			protocoljsonrpc.MethodModelCatalog: coreapi.ModelCatalogState{
				Providers: []coreapi.ModelProviderOption{
					{ID: "openai", Name: "OpenAI"},
				},
				AllowCustomProvider: true,
			},
			protocoljsonrpc.MethodModelUpsert:   map[string]any{"ok": true},
			protocoljsonrpc.MethodModelSave:     map[string]any{"ok": true},
			protocoljsonrpc.MethodModelDelete:   map[string]any{"ok": true},
			protocoljsonrpc.MethodModelActivate: map[string]any{"ok": true},
			protocoljsonrpc.MethodModelSyncEnv:  map[string]any{"ok": true},
		},
	}
	engine := NewRemoteEngine(caller)

	models, err := engine.Models().List(context.Background())
	if err != nil {
		t.Fatalf("Models().List() error = %v", err)
	}
	if len(models) != 1 || models[0].Name != "gpt-4o" {
		t.Fatalf("Models().List()=%+v, want gpt-4o", models)
	}

	catalog, err := engine.Models().Catalog(context.Background())
	if err != nil {
		t.Fatalf("Models().Catalog() error = %v", err)
	}
	if len(catalog.Providers) != 1 || catalog.Providers[0].ID != "openai" {
		t.Fatalf("Models().Catalog()=%+v, want openai provider", catalog)
	}

	if err := engine.Models().Upsert(context.Background(), coreapi.UpsertModelRequest{Name: "gpt-4o", Model: "gpt-4o"}); err != nil {
		t.Fatalf("Models().Upsert() error = %v", err)
	}
	if err := engine.Models().Save(context.Background(), coreapi.ModelSaveRequest{Name: "gpt-4o", Mode: "custom"}); err != nil {
		t.Fatalf("Models().Save() error = %v", err)
	}
	if err := engine.Models().Delete(context.Background(), coreapi.ModelNameRequest{Name: "gpt-4o"}); err != nil {
		t.Fatalf("Models().Delete() error = %v", err)
	}
	if err := engine.Models().Activate(context.Background(), coreapi.ModelNameRequest{Name: "gpt-4o"}); err != nil {
		t.Fatalf("Models().Activate() error = %v", err)
	}
	if err := engine.Models().SyncEnv(context.Background()); err != nil {
		t.Fatalf("Models().SyncEnv() error = %v", err)
	}

	wantMethods := []string{
		protocoljsonrpc.MethodModelList,
		protocoljsonrpc.MethodModelCatalog,
		protocoljsonrpc.MethodModelUpsert,
		protocoljsonrpc.MethodModelSave,
		protocoljsonrpc.MethodModelDelete,
		protocoljsonrpc.MethodModelActivate,
		protocoljsonrpc.MethodModelSyncEnv,
	}
	calls := caller.snapshotCalls()
	if len(calls) != len(wantMethods) {
		t.Fatalf("calls=%+v, want %d calls", calls, len(wantMethods))
	}
	for i, want := range wantMethods {
		if calls[i].method != want {
			t.Fatalf("call[%d].method=%q, want %q", i, calls[i].method, want)
		}
	}
}

func TestRemoteEngineEventKeepsTopLevelTurnID(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodEventSubscribe: map[string]any{"subscription_id": "sub-1"},
		},
	}
	engine := NewRemoteEngine(caller)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := engine.Events().Subscribe(ctx, coreapi.EventFilter{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	n := protocoljsonrpc.Notification{
		Method: protocoljsonrpc.NotificationEvent,
		Params: json.RawMessage(`{"event_type":"turn.started","source":"runtime","turn_id":"turn_42","session_id":"sess_1"}`),
	}
	_ = engine.handleNotification(context.Background(), n)

	select {
	case env := <-ch:
		if env.TurnID != "turn_42" {
			t.Fatalf("TurnID=%q, want turn_42", env.TurnID)
		}
		if env.SessionID != "sess_1" {
			t.Fatalf("SessionID=%q, want sess_1", env.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestRemoteEngineeringServicesCallSidecar(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodGitStatus: []coreapi.GitChange{
				{Path: "a.txt", State: "M "},
				{Path: "b.txt", State: "??"},
			},
			protocoljsonrpc.MethodGitDiff: coreapi.GitTextResult{
				Text: "diff --git a/a.txt b/a.txt",
			},
			protocoljsonrpc.MethodGitBranches: coreapi.GitBranchesResult{
				Current:  "main",
				Branches: []string{"feature", "main"},
			},
			protocoljsonrpc.MethodGitLog: coreapi.GitLogResult{
				Branch: "main",
				Entries: []coreapi.GitLogEntry{
					{Hash: "abc123", Message: "init"},
				},
				Text: "abc123 init",
			},
			protocoljsonrpc.MethodGitShow: coreapi.GitShowResult{
				Branch:   "main",
				Revision: "HEAD",
				Text:     "diff content",
			},
			protocoljsonrpc.MethodTaskList: []coreapi.TaskSnapshot{
				{ID: "task-1", Status: "running", StartedAt: now},
			},
			protocoljsonrpc.MethodTaskTodos: []coreapi.TodoItem{
				{ID: "todo-1", Content: "step 1", Status: "pending"},
			},
			protocoljsonrpc.MethodTaskTail:    []string{"line 1", "line 2"},
			protocoljsonrpc.MethodTaskKill:    map[string]any{"ok": true},
			protocoljsonrpc.MethodTaskCleanup: map[string]any{"removed": 3},
			protocoljsonrpc.MethodVersionsList: []coreapi.VersionItem{
				{ID: "v1", File: "a.txt", Summary: "first"},
			},
			protocoljsonrpc.MethodVersionsRollback:   map[string]any{"ok": true},
			protocoljsonrpc.MethodVersionsDelete:     map[string]any{"ok": true},
			protocoljsonrpc.MethodVersionsDeleteFile: map[string]any{"removed": 2},
			protocoljsonrpc.MethodVersionsClear:      map[string]any{"removed": 5},
			protocoljsonrpc.MethodUsageSummary: coreapi.UsageSummary{
				Rounds:      4,
				InputTokens: intPtr(120),
				CostUSD:     floatPtr(0.42),
			},
			protocoljsonrpc.MethodUsageCostSummary: map[string]any{
				"text": "rounds: 4\ncost_usd: 0.420000",
			},
			protocoljsonrpc.MethodUsageCostItems: []coreapi.CostItem{
				{Model: "gpt-4", TotalTokens: intPtr(80), CostUSD: floatPtr(0.42)},
			},
		},
	}
	engine := NewRemoteEngine(caller)

	git := engine.Git()
	changes, err := git.Status(context.Background(), coreapi.GitStatusRequest{WorkspaceRoot: "C:/work"})
	if err != nil {
		t.Fatalf("Git().Status() error = %v", err)
	}
	if len(changes) != 2 || changes[0].Path != "a.txt" {
		t.Fatalf("Git().Status() = %+v, want 2 changes", changes)
	}
	diff, err := git.Diff(context.Background(), coreapi.GitDiffRequest{WorkspaceRoot: "C:/work"})
	if err != nil {
		t.Fatalf("Git().Diff() error = %v", err)
	}
	if diff.Text == "" {
		t.Fatal("Git().Diff().Text is empty")
	}
	branches, err := git.Branches(context.Background(), coreapi.GitBranchesRequest{WorkspaceRoot: "C:/work"})
	if err != nil {
		t.Fatalf("Git().Branches() error = %v", err)
	}
	if branches.Current != "main" || len(branches.Branches) != 2 {
		t.Fatalf("Git().Branches() = %+v", branches)
	}
	logOut, err := git.Log(context.Background(), coreapi.GitLogRequest{WorkspaceRoot: "C:/work", Limit: 10})
	if err != nil {
		t.Fatalf("Git().Log() error = %v", err)
	}
	if len(logOut.Entries) != 1 || logOut.Entries[0].Hash != "abc123" {
		t.Fatalf("Git().Log() = %+v", logOut)
	}
	show, err := git.Show(context.Background(), coreapi.GitShowRequest{WorkspaceRoot: "C:/work", Revision: "HEAD"})
	if err != nil {
		t.Fatalf("Git().Show() error = %v", err)
	}
	if show.Text != "diff content" {
		t.Fatalf("Git().Show().Text = %q", show.Text)
	}

	tasks := engine.Tasks()
	taskList, err := tasks.List(context.Background())
	if err != nil {
		t.Fatalf("Tasks().List() error = %v", err)
	}
	if len(taskList) != 1 || taskList[0].ID != "task-1" {
		t.Fatalf("Tasks().List() = %+v", taskList)
	}
	todos, err := tasks.Todos(context.Background())
	if err != nil {
		t.Fatalf("Tasks().Todos() error = %v", err)
	}
	if len(todos) != 1 || todos[0].ID != "todo-1" {
		t.Fatalf("Tasks().Todos() = %+v", todos)
	}
	tail, err := tasks.Tail(context.Background(), coreapi.TaskIDRequest{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("Tasks().Tail() error = %v", err)
	}
	if len(tail) != 2 {
		t.Fatalf("Tasks().Tail() = %+v", tail)
	}
	if err := tasks.Kill(context.Background(), coreapi.TaskIDRequest{TaskID: "task-1"}); err != nil {
		t.Fatalf("Tasks().Kill() error = %v", err)
	}
	cleaned, err := tasks.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Tasks().Cleanup() error = %v", err)
	}
	if cleaned != 3 {
		t.Fatalf("Tasks().Cleanup() = %d, want 3", cleaned)
	}

	versions := engine.Versions()
	vs, err := versions.List(context.Background())
	if err != nil {
		t.Fatalf("Versions().List() error = %v", err)
	}
	if len(vs) != 1 || vs[0].ID != "v1" {
		t.Fatalf("Versions().List() = %+v", vs)
	}
	if err := versions.Rollback(context.Background(), coreapi.VersionIDRequest{ID: "v1"}); err != nil {
		t.Fatalf("Versions().Rollback() error = %v", err)
	}
	if err := versions.Delete(context.Background(), coreapi.VersionIDRequest{ID: "v1"}); err != nil {
		t.Fatalf("Versions().Delete() error = %v", err)
	}
	removedFile, err := versions.DeleteFile(context.Background(), coreapi.VersionFileRequest{File: "a.txt"})
	if err != nil {
		t.Fatalf("Versions().DeleteFile() error = %v", err)
	}
	if removedFile != 2 {
		t.Fatalf("Versions().DeleteFile() = %d, want 2", removedFile)
	}
	cleared, err := versions.Clear(context.Background())
	if err != nil {
		t.Fatalf("Versions().Clear() error = %v", err)
	}
	if cleared != 5 {
		t.Fatalf("Versions().Clear() = %d, want 5", cleared)
	}

	usage := engine.Usage()
	summary, err := usage.Summary(context.Background())
	if err != nil {
		t.Fatalf("Usage().Summary() error = %v", err)
	}
	if summary.Rounds != 4 || *summary.CostUSD != 0.42 {
		t.Fatalf("Usage().Summary() = %+v", summary)
	}
	costText, err := usage.CostSummary(context.Background())
	if err != nil {
		t.Fatalf("Usage().CostSummary() error = %v", err)
	}
	if costText == "" {
		t.Fatal("Usage().CostSummary() returned empty text")
	}
	items, err := usage.CostItems(context.Background())
	if err != nil {
		t.Fatalf("Usage().CostItems() error = %v", err)
	}
	if len(items) != 1 || items[0].Model != "gpt-4" {
		t.Fatalf("Usage().CostItems() = %+v", items)
	}

	wantMethods := []string{
		protocoljsonrpc.MethodGitStatus,
		protocoljsonrpc.MethodGitDiff,
		protocoljsonrpc.MethodGitBranches,
		protocoljsonrpc.MethodGitLog,
		protocoljsonrpc.MethodGitShow,
		protocoljsonrpc.MethodTaskList,
		protocoljsonrpc.MethodTaskTodos,
		protocoljsonrpc.MethodTaskTail,
		protocoljsonrpc.MethodTaskKill,
		protocoljsonrpc.MethodTaskCleanup,
		protocoljsonrpc.MethodVersionsList,
		protocoljsonrpc.MethodVersionsRollback,
		protocoljsonrpc.MethodVersionsDelete,
		protocoljsonrpc.MethodVersionsDeleteFile,
		protocoljsonrpc.MethodVersionsClear,
		protocoljsonrpc.MethodUsageSummary,
		protocoljsonrpc.MethodUsageCostSummary,
		protocoljsonrpc.MethodUsageCostItems,
	}
	calls := caller.snapshotCalls()
	if len(calls) != len(wantMethods) {
		t.Fatalf("calls=%+v, want %d calls", calls, len(wantMethods))
	}
	for i, want := range wantMethods {
		if calls[i].method != want {
			t.Fatalf("call[%d].method=%q, want %q", i, calls[i].method, want)
		}
	}
}

func TestRemoteEngineeringServicesMapUnsupportedError(t *testing.T) {
	caller := &fakeEngineCaller{
		errs: map[string]error{
			protocoljsonrpc.MethodGitStatus: errors.New("jsonrpc error -32603: unsupported operation"),
		},
	}
	engine := NewRemoteEngine(caller)
	_, err := engine.Git().Status(context.Background(), coreapi.GitStatusRequest{WorkspaceRoot: "C:/work"})
	if !errors.Is(err, coreapi.ErrUnsupported) {
		t.Fatalf("Git().Status() error = %v, want ErrUnsupported", err)
	}
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestRemoteEngineMigrationSurfaceCallsSidecar(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	addedRecord := coreapi.MemoryRecord{
		ID:        "mem_1",
		Scope:     "user",
		Kind:      "preference",
		Content:   "use dark mode",
		Tags:      []string{"ui"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	plan := coreapi.PlanSnapshot{HasPlan: true, Content: "step 1\nstep 2"}
	role := coreapi.RoleConfig{ID: "planner", Description: "Planner", SystemPrompt: "Plan", ContextStrategy: "full"}

	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodMemorySnapshot:         coreapi.MemorySnapshot{Documents: []coreapi.MemoryDocument{{Scope: "global", Content: "remember", Exists: true}}},
			protocoljsonrpc.MethodMemorySave:             map[string]any{"ok": true},
			protocoljsonrpc.MethodMemoryRebuildIndex:     map[string]any{"ok": true},
			protocoljsonrpc.MethodMemoryRecordAdd:        addedRecord,
			protocoljsonrpc.MethodMemoryRecordList:       []coreapi.MemoryRecord{addedRecord},
			protocoljsonrpc.MethodMemoryRecordSearch:     []coreapi.MemoryRecord{addedRecord},
			protocoljsonrpc.MethodMemoryRecordDelete:     map[string]any{"ok": true},
			protocoljsonrpc.MethodContextPreview:         []string{"user: hi"},
			protocoljsonrpc.MethodContextStats:           coreapi.ContextStats{MessageCount: 2, Estimated: 7},
			protocoljsonrpc.MethodContextWindow:          map[string]any{"tokens": 8192},
			protocoljsonrpc.MethodContextPin:             map[string]any{"ok": true},
			protocoljsonrpc.MethodContextCompact:         map[string]any{"summary": "rolled up"},
			protocoljsonrpc.MethodContextClear:           map[string]any{"ok": true},
			protocoljsonrpc.MethodContextExport:          map[string]any{"ok": true},
			protocoljsonrpc.MethodInsightPredictNextUser: map[string]any{"text": "follow-up"},
			protocoljsonrpc.MethodInsightPlanSnapshot:    plan,
			protocoljsonrpc.MethodRoleList:               []coreapi.RoleConfig{role},
			protocoljsonrpc.MethodRoleResolve:            role,
		},
	}
	engine := NewRemoteEngine(caller)

	snap, err := engine.Memory().Snapshot(context.Background())
	if err != nil || len(snap.Documents) != 1 {
		t.Fatalf("Memory().Snapshot() = %+v, %v", snap, err)
	}
	if err := engine.Memory().Save(context.Background(), coreapi.SaveMemoryRequest{Scope: "global", Content: "remember"}); err != nil {
		t.Fatalf("Memory().Save() error = %v", err)
	}
	if err := engine.Memory().RebuildIndex(context.Background()); err != nil {
		t.Fatalf("Memory().RebuildIndex() error = %v", err)
	}
	got, err := engine.Memory().RecordAdd(context.Background(), coreapi.AddMemoryRecordRequest{Scope: "user", Content: "use dark mode"})
	if err != nil || got.ID != "mem_1" {
		t.Fatalf("Memory().RecordAdd() = %+v, %v", got, err)
	}
	listed, err := engine.Memory().RecordList(context.Background(), coreapi.ListMemoryRecordsRequest{Scope: "user"})
	if err != nil || len(listed) != 1 {
		t.Fatalf("Memory().RecordList() = %+v, %v", listed, err)
	}
	searched, err := engine.Memory().RecordSearch(context.Background(), coreapi.SearchMemoryRecordsRequest{Keywords: []string{"dark"}})
	if err != nil || len(searched) != 1 {
		t.Fatalf("Memory().RecordSearch() = %+v, %v", searched, err)
	}
	if err := engine.Memory().RecordDelete(context.Background(), coreapi.DeleteMemoryRecordRequest{ID: "mem_1"}); err != nil {
		t.Fatalf("Memory().RecordDelete() error = %v", err)
	}

	preview, err := engine.Context().Preview(context.Background())
	if err != nil || len(preview) != 1 || preview[0] != "user: hi" {
		t.Fatalf("Context().Preview() = %v, %v", preview, err)
	}
	stats, err := engine.Context().Stats(context.Background())
	if err != nil || stats.MessageCount != 2 || stats.Estimated != 7 {
		t.Fatalf("Context().Stats() = %+v, %v", stats, err)
	}
	tokens, err := engine.Context().WindowTokens(context.Background())
	if err != nil || tokens != 8192 {
		t.Fatalf("Context().WindowTokens() = %d, %v", tokens, err)
	}
	if err := engine.Context().PinDocument(context.Background(), coreapi.PinDocumentRequest{ID: "doc1", Content: "pinned"}); err != nil {
		t.Fatalf("Context().PinDocument() error = %v", err)
	}
	summary, err := engine.Context().Compact(context.Background())
	if err != nil || summary != "rolled up" {
		t.Fatalf("Context().Compact() = %q, %v", summary, err)
	}
	if err := engine.Context().Clear(context.Background()); err != nil {
		t.Fatalf("Context().Clear() error = %v", err)
	}
	if err := engine.Context().Export(context.Background(), coreapi.ExportContextRequest{Path: "/tmp/ctx.json"}); err != nil {
		t.Fatalf("Context().Export() error = %v", err)
	}

	prediction, err := engine.Insights().PredictNextUserMessage(context.Background(), coreapi.PredictNextUserMessageRequest{Draft: "follow-up"})
	if err != nil || prediction != "follow-up" {
		t.Fatalf("Insights().PredictNextUserMessage() = %q, %v", prediction, err)
	}
	gotPlan, err := engine.Insights().PlanSnapshot(context.Background())
	if err != nil || !gotPlan.HasPlan || gotPlan.Content != "step 1\nstep 2" {
		t.Fatalf("Insights().PlanSnapshot() = %+v, %v", gotPlan, err)
	}

	roles, err := engine.Roles().List(context.Background())
	if err != nil || len(roles) != 1 || roles[0].ID != "planner" {
		t.Fatalf("Roles().List() = %+v, %v", roles, err)
	}
	gotRole, err := engine.Roles().Resolve(context.Background(), coreapi.RoleRef{ID: "planner"})
	if err != nil || gotRole.ID != "planner" {
		t.Fatalf("Roles().Resolve() = %+v, %v", gotRole, err)
	}
}

func TestRemoteEngineRemoteWorkspaceServiceCallsSidecar(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodRemoteWorkspaceList: []coreapi.RemoteWorkspace{
				{ID: "rw-1", Kind: "git", Platform: "github", Owner: "acme", Repo: "proj", Active: true, Exists: true, LastUsedAt: now},
			},
			protocoljsonrpc.MethodRemoteWorkspaceOpen: coreapi.RemoteWorkspace{
				ID: "rw-1", Kind: "git", Platform: "github", Owner: "acme", Repo: "proj", Active: true, Exists: true, LastUsedAt: now,
			},
			protocoljsonrpc.MethodRemoteWorkspaceForget:     map[string]any{"ok": true},
			protocoljsonrpc.MethodRemoteWorkspaceClearCache: map[string]any{"ok": true},
			protocoljsonrpc.MethodRemoteRepoCurrent: coreapi.RemoteRepoState{
				Mode:          "github",
				Platform:      "github",
				RepoURL:       "https://github.com/acme/proj",
				Owner:         "acme",
				Repo:          "proj",
				WorkingBranch: "main",
			},
		},
	}
	engine := NewRemoteEngine(caller)
	rw := engine.RemoteWorkspaces()

	list, err := rw.List(context.Background())
	if err != nil {
		t.Fatalf("RemoteWorkspaces().List() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != "rw-1" {
		t.Fatalf("RemoteWorkspaces().List() = %+v, want rw-1", list)
	}

	opened, err := rw.Open(context.Background(), coreapi.RemoteWorkspaceRef{IDOrPath: "rw-1"})
	if err != nil {
		t.Fatalf("RemoteWorkspaces().Open() error = %v", err)
	}
	if opened.ID != "rw-1" || opened.Owner != "acme" {
		t.Fatalf("RemoteWorkspaces().Open() = %+v, want rw-1/acme", opened)
	}

	if err := rw.Forget(context.Background(), coreapi.RemoteWorkspaceRef{IDOrPath: "rw-1"}); err != nil {
		t.Fatalf("RemoteWorkspaces().Forget() error = %v", err)
	}

	if err := rw.ClearCache(context.Background(), coreapi.RemoteWorkspaceRef{IDOrPath: "rw-1"}); err != nil {
		t.Fatalf("RemoteWorkspaces().ClearCache() error = %v", err)
	}

	repoState, found, err := rw.CurrentRepo(context.Background())
	if err != nil {
		t.Fatalf("RemoteWorkspaces().CurrentRepo() error = %v", err)
	}
	if !found {
		t.Fatalf("RemoteWorkspaces().CurrentRepo() found=false, want true")
	}
	if repoState.Mode != "github" || repoState.Owner != "acme" {
		t.Fatalf("RemoteWorkspaces().CurrentRepo() = %+v, want github/acme", repoState)
	}

	wantMethods := []string{
		protocoljsonrpc.MethodRemoteWorkspaceList,
		protocoljsonrpc.MethodRemoteWorkspaceOpen,
		protocoljsonrpc.MethodRemoteWorkspaceForget,
		protocoljsonrpc.MethodRemoteWorkspaceClearCache,
		protocoljsonrpc.MethodRemoteRepoCurrent,
	}
	cfgCalls := caller.snapshotCalls()
	if len(cfgCalls) != len(wantMethods) {
		t.Fatalf("calls=%d, want %d", len(cfgCalls), len(wantMethods))
	}
	for i, want := range wantMethods {
		if cfgCalls[i].method != want {
			t.Fatalf("call[%d].method=%q, want %q", i, cfgCalls[i].method, want)
		}
	}
}

func TestRemoteEngineRemoteWorkspaceCurrentRepoNotFound(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodRemoteRepoCurrent: coreapi.RemoteRepoState{},
		},
	}
	engine := NewRemoteEngine(caller)

	_, found, err := engine.RemoteWorkspaces().CurrentRepo(context.Background())
	if err != nil {
		t.Fatalf("CurrentRepo() error = %v", err)
	}
	if found {
		t.Fatalf("CurrentRepo() found=true, want false for empty state")
	}
}

func TestRemoteEngineToolTelemetryServiceCallsSidecar(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodToolTraces: []coreapi.ToolTrace{
				{ID: "t-1", Tool: "read", Success: true, Cached: false},
				{ID: "t-2", Tool: "write", Success: false, RetryCount: 1},
			},
			protocoljsonrpc.MethodToolStats: []coreapi.ToolStat{
				{Tool: "read", TotalCalls: 10, SuccessCalls: 9, FailureCalls: 1, CachedCalls: 3},
				{Tool: "write", TotalCalls: 5, SuccessCalls: 4, FailureCalls: 1},
			},
		},
	}
	engine := NewRemoteEngine(caller)
	tel := engine.ToolTelemetry()

	traces, err := tel.Traces(context.Background())
	if err != nil {
		t.Fatalf("ToolTelemetry().Traces() error = %v", err)
	}
	if len(traces) != 2 || traces[0].Tool != "read" || traces[1].Tool != "write" {
		t.Fatalf("ToolTelemetry().Traces() = %+v, want 2 traces", traces)
	}

	stats, err := tel.Stats(context.Background())
	if err != nil {
		t.Fatalf("ToolTelemetry().Stats() error = %v", err)
	}
	if len(stats) != 2 || stats[0].Tool != "read" || stats[0].TotalCalls != 10 {
		t.Fatalf("ToolTelemetry().Stats() = %+v, want read with 10 calls", stats)
	}

	wantMethods := []string{
		protocoljsonrpc.MethodToolTraces,
		protocoljsonrpc.MethodToolStats,
	}
	cfgCalls := caller.snapshotCalls()
	if len(cfgCalls) != len(wantMethods) {
		t.Fatalf("calls=%d, want %d", len(cfgCalls), len(wantMethods))
	}
	for i, want := range wantMethods {
		if cfgCalls[i].method != want {
			t.Fatalf("call[%d].method=%q, want %q", i, cfgCalls[i].method, want)
		}
	}
}

func TestRemoteEngineSandboxPolicyCallsSidecar(t *testing.T) {
	caller := &fakeEngineCaller{
		results: map[string]any{
			protocoljsonrpc.MethodSandboxPolicy: map[string]any{
				"mode":           "full",
				"workspace_root": "C:/work",
				"network":        "allow",
			},
			protocoljsonrpc.MethodSandboxSetPolicy: map[string]any{"ok": true},
		},
	}
	engine := NewRemoteEngine(caller)

	policy, err := engine.Sandbox().Policy(context.Background(), coreapi.SessionRef{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("Sandbox().Policy() error = %v", err)
	}
	if policy.Mode != "full" {
		t.Fatalf("Sandbox().Policy().Mode=%q, want full", policy.Mode)
	}
	if policy.WorkspaceRoot != "C:/work" {
		t.Fatalf("Sandbox().Policy().WorkspaceRoot=%q, want C:/work", policy.WorkspaceRoot)
	}

	if err := engine.Sandbox().SetPolicy(context.Background(), coreapi.SessionRef{SessionID: "sess-1"}, sandbox.Policy{Mode: "strict"}); err != nil {
		t.Fatalf("Sandbox().SetPolicy() error = %v", err)
	}

	wantMethods := []string{
		protocoljsonrpc.MethodSandboxPolicy,
		protocoljsonrpc.MethodSandboxSetPolicy,
	}
	cfgCalls := caller.snapshotCalls()
	if len(cfgCalls) != len(wantMethods) {
		t.Fatalf("calls=%d, want %d", len(cfgCalls), len(wantMethods))
	}
	for i, want := range wantMethods {
		if cfgCalls[i].method != want {
			t.Fatalf("call[%d].method=%q, want %q", i, cfgCalls[i].method, want)
		}
	}
}
