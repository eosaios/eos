package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// ChatService 会话生命周期批测：Create/Ensure/Select/Rename/Delete/Archive/
// Predict/RefineInput/RollbackChatTurn 的错误臂与成功链，state sync 纯函数
// 与 debouncer，以及 state watch 路径聚合/事件过滤 helper。
//
// chatSessionsGatewayStub 嵌入 bridgeRuntimeGateway 接口并实现 LoadBootstrap
// 容错链 + 会话写链所需方法；未实现的方法被调到会 nil panic，堆栈即指路。

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/sandbox"
	"github.com/fsnotify/fsnotify"
)

type chatSessionsGatewayStub struct {
	bridgeRuntimeGateway
	mu sync.Mutex

	// workspace 解析与激活
	defaultWorkspace string
	lastWorkspace    string
	foreground       string
	useErr           error
	addErr           error
	useCalls         []string
	addCalls         []string
	rememberCalls    []string
	trustCalls       []string

	// 会话读写
	sessions         []coreapi.Session
	archivedSessions []coreapi.Session
	currentMeta      adapter.SessionMeta
	messages         []adapter.SessionMessage
	createErr        error
	createdTitles    []string
	deleteErr        error
	deletedIDs       []string
	renameErr        error
	renamed          [][3]string
	archiveErr       error
	archivedIDs      map[string]bool
	resumeErr        error
	resumed          [][2]string
	saveErr          error
	savedMessages    [][]adapter.SessionMessage
	setCurrentCalls  [][2]string

	// 审批
	approvals   coreapi.PendingApprovalList
	respondErrs map[string]error

	// 快照与杂项读
	snapshot      adapter.RuntimeSnapshot
	modeSnapshot  coreapi.ModeSnapshot
	stateErr      error
	rollbackErr   error
	rollbackCalls [][2]interface{}
	removeErr     error

	// browser 控制面 / settings 内核 / 订阅（批二扩展）
	browserErr         error
	takeoverReasons    []string
	focusURLs          []string
	defaultProfiles    []string
	tabNewURLs         []string
	tabSwitches        []int
	tabCloses          []*int
	navigations        []string
	liveStarts         []coreapi.BrowserLiveStartRequest
	inputs             []coreapi.BrowserInputRequest
	historyActions     []string
	credentialImports  []coreapi.BrowserCredentialsImportRequest
	profileUpserts     []map[string]any
	uploadProvides     [][2]interface{}
	subscribeErr       error
	eventCh            chan adapter.Event
	getSettingsErr     error
	saveSettingsErr    error
	kernelSettings     coreapi.Settings
	savedKernels       []coreapi.Settings
	configPathOverride string

	// git / remote workspace / bash / tasks（批三扩展）
	gitErr            error
	gitRepos          coreapi.GitReposResult
	gitRepoRoots      []string
	stageCalls        [][4]interface{}
	commitMessages    [][2]string
	pushRoots         []string
	gitPushResult     coreapi.GitPushResult
	abortRoots        []string
	suggestRoots      []string
	remoteErr         error
	remoteWorkspace   adapter.RemoteWorkspace
	openRemoteCalls   []string
	forgetRemoteCalls []string
	clearRemoteCalls  []string
	bashErr           error
	bashEvents        chan adapter.Event
	bashCommands      []string
	killErr           error
	killCalls         []string
	cleanupCalls      int
	respondErr        error
	respondWithReason [][3]interface{}

	// goal / capability 写（批六扩展）
	capErr        error
	tasksList     []coreapi.TaskSnapshot
	goalSets      []coreapi.GoalSetRequest
	goalPaused    []string
	goalResumed   []string
	goalCleared   []string
	mcpUpserts    [][4]interface{}
	mcpImports    []string
	mcpDeletes    []string
	mcpEnabled    [][2]interface{}
	skillEnabled  [][2]interface{}
	pluginEnabled [][2]interface{}

	// CoreCallRPC / Invoke（批九扩展）
	callRPCErr    error
	callRPCResult json.RawMessage
	callRPCs      [][2]string
	invokeErr     error
	invokeEvents  chan adapter.Event

	// turn 流（批四扩展）
	turnErr       error
	turnFailFirst int // 前 N 次 turn/start 注入失败（重试路径）
	turnEvents    chan adapter.Event
	turnRequests  []coreapi.StartTurnRequest
	resumeTurnIDs []string

	// predict / refine
	predictText string
	predictErr  error
	refineText  string
	refineErr   error
}

func (g *chatSessionsGatewayStub) CoreConfigPath() string {
	if g.configPathOverride != "" {
		return g.configPathOverride
	}
	return "/config/eos.toml"
}

func (g *chatSessionsGatewayStub) CoreRemoveWorkspaceRPC(_ context.Context, path string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.removeErr
}

func (g *chatSessionsGatewayStub) CoreDefaultWorkspaceRPC(context.Context) (string, error) {
	return g.defaultWorkspace, nil
}

func (g *chatSessionsGatewayStub) CoreLastWorkspaceRPC(context.Context) (string, error) {
	return g.lastWorkspace, nil
}

func (g *chatSessionsGatewayStub) CoreResolveForegroundWorkspaceRPC(_ context.Context, path string) (string, error) {
	return g.foreground, nil
}

func (g *chatSessionsGatewayStub) CoreUseWorkspaceRPC(_ context.Context, path string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.useCalls = append(g.useCalls, path)
	return g.useErr
}

func (g *chatSessionsGatewayStub) CoreAddWorkspaceRPC(_ context.Context, path string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.addCalls = append(g.addCalls, path)
	return g.addErr
}

func (g *chatSessionsGatewayStub) CoreTrustWorkspaceRPC(_ context.Context, path string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.trustCalls = append(g.trustCalls, path)
	return nil
}

func (g *chatSessionsGatewayStub) CoreRememberWorkspaceRPC(_ context.Context, path string, _ bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rememberCalls = append(g.rememberCalls, path)
	return nil
}

func (g *chatSessionsGatewayStub) CoreListSessionsRPC(ctx context.Context, workspace string) ([]coreapi.Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	all := append([]coreapi.Session(nil), g.sessions...)
	if strings.TrimSpace(workspace) == "" {
		return all, nil
	}
	// 与内核语义一致：按 workspace 过滤，防跨 workspace 复活。
	filtered := make([]coreapi.Session, 0, len(all))
	for _, item := range all {
		if sameWorkspacePath(strings.TrimSpace(item.WorkspaceRoot), strings.TrimSpace(workspace)) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (g *chatSessionsGatewayStub) CoreListArchivedSessionsRPC(context.Context) ([]coreapi.Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]coreapi.Session(nil), g.archivedSessions...), nil
}

func (g *chatSessionsGatewayStub) CoreCurrentSessionRPC(context.Context, string) (adapter.SessionMeta, error) {
	return g.currentMeta, nil
}

func (g *chatSessionsGatewayStub) CoreCreateSessionRPC(_ context.Context, _, title, _ string, _ []adapter.SessionMessage) (adapter.SessionMeta, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.createErr != nil {
		return adapter.SessionMeta{}, g.createErr
	}
	g.createdTitles = append(g.createdTitles, title)
	id := "created-" + title
	g.sessions = append(g.sessions, coreapi.Session{ID: id})
	return adapter.SessionMeta{ID: id, Title: title}, nil
}

func (g *chatSessionsGatewayStub) CoreDeleteSessionRPC(_ context.Context, _, sessionID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.deleteErr != nil {
		return g.deleteErr
	}
	g.deletedIDs = append(g.deletedIDs, sessionID)
	return nil
}

func (g *chatSessionsGatewayStub) CoreRenameSessionRPC(_ context.Context, workspace, sessionID, title string) (adapter.SessionMeta, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.renameErr != nil {
		return adapter.SessionMeta{}, g.renameErr
	}
	g.renamed = append(g.renamed, [3]string{workspace, sessionID, title})
	return adapter.SessionMeta{ID: sessionID, Title: title}, nil
}

func (g *chatSessionsGatewayStub) CoreArchiveSessionRPC(_ context.Context, sessionID string, archived bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.archiveErr != nil {
		return g.archiveErr
	}
	if g.archivedIDs == nil {
		g.archivedIDs = map[string]bool{}
	}
	g.archivedIDs[sessionID] = archived
	return nil
}

func (g *chatSessionsGatewayStub) CoreResumeSessionRPC(_ context.Context, workspace, sessionID string) (adapter.SessionMeta, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.resumeErr != nil {
		return adapter.SessionMeta{}, g.resumeErr
	}
	g.resumed = append(g.resumed, [2]string{workspace, sessionID})
	return adapter.SessionMeta{ID: sessionID}, nil
}

func (g *chatSessionsGatewayStub) CoreSetCurrentSessionRPC(_ context.Context, workspace, sessionID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.setCurrentCalls = append(g.setCurrentCalls, [2]string{workspace, sessionID})
	return nil
}

func (g *chatSessionsGatewayStub) CoreSaveSessionMessagesRPC(_ context.Context, _, sessionID string, messages []adapter.SessionMessage) (adapter.SessionMeta, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.saveErr != nil {
		return adapter.SessionMeta{}, g.saveErr
	}
	g.savedMessages = append(g.savedMessages, append([]adapter.SessionMessage(nil), messages...))
	return adapter.SessionMeta{ID: sessionID}, nil
}

func (g *chatSessionsGatewayStub) CoreLoadSessionMessagesRPC(context.Context, string, string) ([]adapter.SessionMessage, error) {
	return append([]adapter.SessionMessage(nil), g.messages...), nil
}

func (g *chatSessionsGatewayStub) CoreApprovalListRPC(context.Context, coreapi.PendingApprovalListRequest) (coreapi.PendingApprovalList, error) {
	return g.approvals, nil
}

func (g *chatSessionsGatewayStub) CoreRespondApprovalRPC(_ context.Context, approvalID string, _ coreapi.ApprovalDecision) error {
	if err, ok := g.respondErrs[approvalID]; ok {
		return err
	}
	return nil
}

func (g *chatSessionsGatewayStub) CoreRuntimeSnapshotRPC(context.Context) (adapter.RuntimeSnapshot, error) {
	return g.snapshot, nil
}

func (g *chatSessionsGatewayStub) CoreStateSnapshotRPC(context.Context) (coreapi.StateSnapshot, error) {
	return coreapi.StateSnapshot{}, g.stateErr
}

func (g *chatSessionsGatewayStub) CoreModeSnapshotRPC(context.Context) (coreapi.ModeSnapshot, error) {
	return g.modeSnapshot, nil
}

func (g *chatSessionsGatewayStub) ResolveSessionWorkspace(string) (string, error) {
	return "", nil
}

func (g *chatSessionsGatewayStub) ThreadCoreIfExists(string) adapter.Core {
	return nil
}

func (g *chatSessionsGatewayStub) CoreWorkspaceRollbackApplyRPC(_ context.Context, workspace string, rollbacks []coreapi.TurnRollback) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.rollbackErr != nil {
		return g.rollbackErr
	}
	g.rollbackCalls = append(g.rollbackCalls, [2]interface{}{workspace, len(rollbacks)})
	return nil
}

func (g *chatSessionsGatewayStub) CorePredictNextUserMessageRPC(context.Context, string) (string, error) {
	return g.predictText, g.predictErr
}

func (g *chatSessionsGatewayStub) CoreRefineInputRPC(context.Context, string) (string, error) {
	return g.refineText, g.refineErr
}

func (g *chatSessionsGatewayStub) CoreGetSettingsRPC(context.Context) (adapter.GUISettings, error) {
	return adapter.GUISettings{Language: "zh-CN"}, nil
}

// ---- LoadBootstrap 容错读链（coreValueOrNil 全吞错，这里全返回零值）----

func (g *chatSessionsGatewayStub) CoreListModelsRPC(context.Context) ([]adapter.ModelConfig, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreModelCatalogRPC(context.Context) (adapter.ModelCatalogState, error) {
	return adapter.ModelCatalogState{}, nil
}

func (g *chatSessionsGatewayStub) CoreListRemoteWorkspacesRPC(context.Context) ([]adapter.RemoteWorkspace, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreCurrentRemoteRepoRPC(context.Context) (adapter.RemoteRepoState, bool, error) {
	return adapter.RemoteRepoState{}, false, nil
}

func (g *chatSessionsGatewayStub) CoreGetFullSettingsRPC(context.Context) (coreapi.Settings, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.getSettingsErr != nil {
		return coreapi.Settings{}, g.getSettingsErr
	}
	return g.kernelSettings, nil
}

func (g *chatSessionsGatewayStub) CorePlanSnapshotRPC(context.Context) (adapter.PlanSnapshot, error) {
	return adapter.PlanSnapshot{}, nil
}

func (g *chatSessionsGatewayStub) CoreGoalGetRPC(context.Context, string) (coreapi.GoalGetResponse, error) {
	return coreapi.GoalGetResponse{}, nil
}

func (g *chatSessionsGatewayStub) CoreMemorySnapshotRPC(context.Context) (adapter.MemorySnapshot, error) {
	return adapter.MemorySnapshot{}, nil
}

func (g *chatSessionsGatewayStub) CoreGitBranchesRPC(context.Context, string) (coreapi.GitBranchesResult, error) {
	return coreapi.GitBranchesResult{}, nil
}

func (g *chatSessionsGatewayStub) CoreListWorktreesRPC(context.Context) ([]adapter.Worktree, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreListMCPRPC(context.Context) ([]adapter.MCPServer, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreListLSPRPC(context.Context) ([]adapter.LSPServer, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreListSkillsRPC(context.Context) ([]adapter.SkillInfo, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreListPluginsRPC(context.Context) ([]adapter.PluginInfo, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreUsageSummaryRPC(context.Context) (adapter.UsageSummary, error) {
	return adapter.UsageSummary{}, nil
}

func (g *chatSessionsGatewayStub) CoreCostItemsRPC(context.Context) ([]adapter.CostItem, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreListVersionsRPC(context.Context) ([]adapter.VersionItem, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CorePermissionSnapshotRPC(context.Context) (adapter.PermissionSnapshot, error) {
	return adapter.PermissionSnapshot{}, nil
}

func (g *chatSessionsGatewayStub) CoreContextStatsRPC(context.Context) (adapter.ContextStats, error) {
	return adapter.ContextStats{}, nil
}

func (g *chatSessionsGatewayStub) CoreContextPreviewRPC(context.Context) ([]string, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreLSPDiagnosticsRPC(context.Context) ([]string, error) {
	return nil, nil
}

func (g *chatSessionsGatewayStub) CoreCostSummaryRPC(context.Context) (string, error) {
	return "", nil
}

func (g *chatSessionsGatewayStub) CoreTaskListRPC(context.Context) ([]coreapi.TaskSnapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]coreapi.TaskSnapshot(nil), g.tasksList...), nil
}

func (g *chatSessionsGatewayStub) CorePendingReviewRPC(context.Context) (adapter.PendingReview, error) {
	return adapter.PendingReview{}, nil
}

func (g *chatSessionsGatewayStub) StartupDiagnostics() adapter.StartupDiagnosticsResult {
	return adapter.StartupDiagnosticsResult{}
}

func (g *chatSessionsGatewayStub) CoreSetSandboxModeRPC(context.Context, string) error { return nil }

func (g *chatSessionsGatewayStub) CoreSetSandboxPolicyRPC(_ context.Context, _ string, _ sandbox.Policy) error {
	return nil
}

func (g *chatSessionsGatewayStub) CoreDeriveSandboxPolicyRPC(context.Context, string, string) (sandbox.Policy, error) {
	return sandbox.Policy{}, nil
}

func (g *chatSessionsGatewayStub) CoreEnterFullAccessRPC(context.Context, string) (sandbox.Policy, error) {
	return sandbox.Policy{}, nil
}

func (g *chatSessionsGatewayStub) CoreSetApprovalModeRPC(context.Context, string) error { return nil }

func (g *chatSessionsGatewayStub) CoreSetSessionSandboxModeRPC(context.Context, string, string) error {
	return nil
}

func (g *chatSessionsGatewayStub) CoreSetExecutionModeRPC(context.Context, string) error { return nil }

func (g *chatSessionsGatewayStub) CoreSetReasoningLevelRPC(context.Context, string) error { return nil }

func (g *chatSessionsGatewayStub) CoreInterruptTurnRPC(context.Context, string, string) error {
	return nil
}

func (g *chatSessionsGatewayStub) CoreSetCurrentSessionCalls() [][2]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([][2]string(nil), g.setCurrentCalls...)
}

// newChatSessionsTestBridge 构造隔离 HOME 的 BridgeService：mkdir 默认工作区
// 会落在 tempdir，emitEvent 捕获到 emitted。
func newChatSessionsTestBridge(t *testing.T, gateway *chatSessionsGatewayStub) (*BridgeService, *ChatService, *emitRecorder) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	rec := &emitRecorder{}
	s := &BridgeService{
		runtimeGateway:       gateway,
		sessions:             map[string]*sessionState{},
		runningConversations: map[string]*runningConversationState{},
		prompts:              map[string]*promptState{},
		emitEvent:            rec.record,
	}
	return s, NewChatService(s), rec
}

type emitRecorder struct {
	mu       sync.Mutex
	events   []string
	payloads []BrowserEventPayload
}

func (r *emitRecorder) record(name string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, name)
	if browserPayload, ok := payload.(BrowserEventPayload); ok {
		r.payloads = append(r.payloads, browserPayload)
	}
}

func (r *emitRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func (r *emitRecorder) has(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.events {
		if item == name {
			return true
		}
	}
	return false
}

func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestChatServiceNilBridgeArms(t *testing.T) {
	svc := NewChatService(nil)
	if _, err := svc.CreateSession(""); err == nil {
		t.Fatal("CreateSession(nil bridge) error = nil")
	}
	if _, err := svc.EnsureWorkspaceSession(""); err == nil {
		t.Fatal("EnsureWorkspaceSession(nil bridge) error = nil")
	}
	if _, err := svc.SelectSession("", ""); err == nil {
		t.Fatal("SelectSession(nil bridge) error = nil")
	}
	if _, err := svc.RenameSession("", ""); err == nil {
		t.Fatal("RenameSession(nil bridge) error = nil")
	}
	if _, err := svc.DeleteSession("", ""); err == nil {
		t.Fatal("DeleteSession(nil bridge) error = nil")
	}
	if _, err := svc.ArchiveSession("", false); err == nil {
		t.Fatal("ArchiveSession(nil bridge) error = nil")
	}
	if _, err := svc.PredictNextUserMessage(""); err == nil {
		t.Fatal("PredictNextUserMessage(nil bridge) error = nil")
	}
	if _, err := svc.RefineInput(""); err == nil {
		t.Fatal("RefineInput(nil bridge) error = nil")
	}
	if _, err := svc.RollbackChatTurn("", ""); err == nil {
		t.Fatal("RollbackChatTurn(nil bridge) error = nil")
	}
}

func TestCreateSessionSuccessAndErrorArms(t *testing.T) {
	t.Run("workspace 激活失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{useErr: errors.New("use denied"), addErr: errors.New("add denied")}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		_, err := svc.CreateSession(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "use denied") {
			t.Fatalf("CreateSession error = %v, want activation failure", err)
		}
	})

	t.Run("内核建会话失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{createErr: errors.New("core create failed")}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		_, err := svc.CreateSession(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "core create failed") {
			t.Fatalf("CreateSession error = %v, want create failure", err)
		}
	})

	t.Run("成功——显式 workspace", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		s, svc, rec := newChatSessionsTestBridge(t, gateway)
		state, err := svc.CreateSession(workspace)
		if err != nil {
			t.Fatalf("CreateSession error = %v", err)
		}
		if state.CurrentSessionID == "" {
			t.Fatal("bootstrap CurrentSessionID empty after create")
		}
		s.stateMu.RLock()
		created := s.sessions[state.CurrentSessionID]
		s.stateMu.RUnlock()
		if created == nil || created.WorkspacePath != workspace {
			t.Fatalf("created session missing or wrong workspace: %+v", created)
		}
		if len(gateway.createdTitles) != 1 || gateway.createdTitles[0] != "New Chat" {
			t.Fatalf("createdTitles = %v, want [New Chat]", gateway.createdTitles)
		}
		if len(gateway.rememberCalls) == 0 {
			t.Fatal("remember-workspace RPC not called on create")
		}
		eventually(t, "shellUpdated emit", func() bool { return rec.has(shellUpdatedEventName) })
	})

	t.Run("空 workspace 回退 activeWorkspace", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.activeWorkspace = workspace
		s.stateMu.Unlock()
		state, err := svc.CreateSession("")
		if err != nil {
			t.Fatalf("CreateSession('') error = %v", err)
		}
		if state.CurrentSessionID == "" {
			t.Fatal("CurrentSessionID empty with activeWorkspace fallback")
		}
	})
}

func TestEnsureWorkspaceSessionRestoreAndCreateArms(t *testing.T) {
	t.Run("resolve 失败透传", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{useErr: errors.New("use denied"), addErr: errors.New("add denied")}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.activeWorkspace = ""
		s.stateMu.Unlock()
		t.Setenv("HOME", t.TempDir()) // 默认工作区推导为空路径的分支由上面锁态+env共同决定
		if _, err := svc.EnsureWorkspaceSession(""); err == nil {
			t.Fatal("EnsureWorkspaceSession('') error = nil, want resolve failure")
		}
	})

	t.Run("已有会话复用并 resume", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{
			defaultWorkspace: workspace,
			sessions:         []coreapi.Session{{ID: "sess-restore", WorkspaceRoot: workspace}},
			currentMeta:      adapter.SessionMeta{ID: "sess-restore"},
		}
		_, svc, rec := newChatSessionsTestBridge(t, gateway)
		state, err := svc.EnsureWorkspaceSession(workspace)
		if err != nil {
			t.Fatalf("EnsureWorkspaceSession error = %v", err)
		}
		if state.CurrentSessionID != "sess-restore" {
			t.Fatalf("CurrentSessionID = %q, want restored sess-restore", state.CurrentSessionID)
		}
		if len(gateway.createdTitles) != 0 {
			t.Fatalf("createdTitles = %v, want no create on restore", gateway.createdTitles)
		}
		eventually(t, "resume-session RPC", func() bool {
			gateway.mu.Lock()
			defer gateway.mu.Unlock()
			return len(gateway.resumed) > 0
		})
		// emit goroutine 内 loadBootstrap 写 HOME 目录，锚定防清理竞态。
		eventually(t, "shellUpdated after ensure", func() bool { return rec.has(shellUpdatedEventName) })
	})

	t.Run("无历史会话则新建", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		_, svc, rec := newChatSessionsTestBridge(t, gateway)
		state, err := svc.EnsureWorkspaceSession(workspace)
		if err != nil {
			t.Fatalf("EnsureWorkspaceSession error = %v", err)
		}
		// emit goroutine 内 loadBootstrap 写 HOME 目录，锚定防清理竞态。
		eventually(t, "shellUpdated after silent create", func() bool { return rec.has(shellUpdatedEventName) })
		if state.CurrentSessionID == "" {
			t.Fatal("CurrentSessionID empty after silent create")
		}
		if len(gateway.createdTitles) != 1 {
			t.Fatalf("createdTitles = %v, want one silent create", gateway.createdTitles)
		}
	})

	t.Run("内核建会话失败", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace, createErr: errors.New("boom")}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		if _, err := svc.EnsureWorkspaceSession(workspace); err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("EnsureWorkspaceSession error = %v, want boom", err)
		}
	})
}

func TestSelectSessionArms(t *testing.T) {
	t.Run("会话不存在", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: t.TempDir()}
		s.stateMu.Unlock()
		if _, err := svc.SelectSession("", "missing"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("SelectSession error = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("workspace 不匹配视为不存在", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: "/other/ws"}
		s.stateMu.Unlock()
		if _, err := svc.SelectSession("/this/ws", "sess-1"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("SelectSession error = %v, want os.ErrNotExist on workspace mismatch", err)
		}
	})

	t.Run("激活失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{useErr: errors.New("denied"), addErr: errors.New("denied")}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		workspace := t.TempDir()
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace}
		s.stateMu.Unlock()
		if _, err := svc.SelectSession(workspace, "sess-1"); err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("SelectSession error = %v, want activation denied", err)
		}
	})

	t.Run("成功切换并异步 remember/resume", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{defaultWorkspace: workspace}
		s, svc, rec := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace, Title: "one"}
		s.sessions["sess-2"] = &sessionState{ID: "sess-2", WorkspacePath: workspace, Title: "two"}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()
		state, err := svc.SelectSession(workspace, "sess-2")
		if err != nil {
			t.Fatalf("SelectSession error = %v", err)
		}
		if state.CurrentSessionID != "sess-2" {
			t.Fatalf("CurrentSessionID = %q, want sess-2", state.CurrentSessionID)
		}
		eventually(t, "resume RPC after select", func() bool {
			gateway.mu.Lock()
			defer gateway.mu.Unlock()
			return len(gateway.resumed) > 0
		})
		// emit 的 goroutine 内部还会 loadBootstrap（写 HOME 下默认工作区目录），
		// 等事件落地避免 tempdir 清理与后台写盘竞态。
		eventually(t, "shellUpdated emit after select", func() bool { return rec.has(shellUpdatedEventName) })
	})
}

func TestRenameSessionArms(t *testing.T) {
	newBridgeWithSession := func(t *testing.T) (*BridgeService, *ChatService, *chatSessionsGatewayStub) {
		gateway := &chatSessionsGatewayStub{}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: t.TempDir(), Title: "old"}
		s.stateMu.Unlock()
		return s, svc, gateway
	}

	t.Run("不存在", func(t *testing.T) {
		_, svc, _ := newBridgeWithSession(t)
		if _, err := svc.RenameSession("missing", "x"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("RenameSession error = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("空标题回退 Untitled Chat", func(t *testing.T) {
		_, svc, gateway := newBridgeWithSession(t)
		if _, err := svc.RenameSession("sess-1", "   "); err != nil {
			t.Fatalf("RenameSession error = %v", err)
		}
		if len(gateway.renamed) != 1 || gateway.renamed[0][2] != "Untitled Chat" {
			t.Fatalf("renamed = %v, want Untitled Chat", gateway.renamed)
		}
	})

	t.Run("持久化失败", func(t *testing.T) {
		_, svc, gateway := newBridgeWithSession(t)
		gateway.saveErr = errors.New("disk full")
		if _, err := svc.RenameSession("sess-1", "new title"); err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Fatalf("RenameSession error = %v, want disk full", err)
		}
	})

	t.Run("成功改名", func(t *testing.T) {
		s, svc, gateway := newBridgeWithSession(t)
		if _, err := svc.RenameSession("sess-1", "新标题"); err != nil {
			t.Fatalf("RenameSession error = %v", err)
		}
		s.stateMu.RLock()
		title := s.sessions["sess-1"].Title
		s.stateMu.RUnlock()
		if title != "新标题" {
			t.Fatalf("title = %q, want 新标题", title)
		}
		if len(gateway.renamed) != 1 || gateway.renamed[0][2] != "新标题" {
			t.Fatalf("renamed = %v, want persisted 新标题", gateway.renamed)
		}
	})
}

func TestDeleteSessionArms(t *testing.T) {
	t.Run("不存在", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		if _, err := svc.DeleteSession("", "missing"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("DeleteSession error = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("RPC 失败透传", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{deleteErr: errors.New("core down")}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		workspace := t.TempDir()
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace}
		s.stateMu.Unlock()
		if _, err := svc.DeleteSession(workspace, "sess-1"); err == nil || !strings.Contains(err.Error(), "core down") {
			t.Fatalf("DeleteSession error = %v, want core down", err)
		}
	})

	t.Run("RPC os.ErrNotExist 容忍继续删本地", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{deleteErr: os.ErrNotExist}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		workspace := t.TempDir()
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace}
		s.stateMu.Unlock()
		if _, err := svc.DeleteSession(workspace, "sess-1"); err != nil {
			t.Fatalf("DeleteSession error = %v, want tolerated ErrNotExist", err)
		}
		s.stateMu.RLock()
		_, still := s.sessions["sess-1"]
		s.stateMu.RUnlock()
		if still {
			t.Fatal("session still present after delete")
		}
	})

	t.Run("删当前会话切到同 workspace 最新", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		workspace := t.TempDir()
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace, UpdatedAt: time.Now().Add(-time.Hour)}
		s.sessions["sess-2"] = &sessionState{ID: "sess-2", WorkspacePath: workspace, UpdatedAt: time.Now()}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()
		state, err := svc.DeleteSession(workspace, "sess-1")
		if err != nil {
			t.Fatalf("DeleteSession error = %v", err)
		}
		if state.CurrentSessionID != "sess-2" {
			t.Fatalf("CurrentSessionID = %q, want fallback sess-2", state.CurrentSessionID)
		}
		if len(gateway.deletedIDs) != 1 || gateway.deletedIDs[0] != "sess-1" {
			t.Fatalf("deletedIDs = %v, want [sess-1]", gateway.deletedIDs)
		}
	})

	t.Run("删光后 current 置空走清空 set-current-session", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		workspace := t.TempDir()
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace}
		s.currentSessionID = "sess-1"
		s.activeWorkspace = workspace
		s.stateMu.Unlock()
		_, err := svc.DeleteSession(workspace, "sess-1")
		if err != nil {
			t.Fatalf("DeleteSession error = %v", err)
		}
		cleared := false
		for _, call := range gateway.CoreSetCurrentSessionCalls() {
			if call[1] == "" {
				cleared = true
			}
		}
		if !cleared {
			t.Fatal("set-current-session('') not called after deleting last session")
		}
	})
}

func TestArchiveSessionArms(t *testing.T) {
	t.Run("空 ID 拒绝", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		if _, err := svc.ArchiveSession("  ", true); err == nil {
			t.Fatal("ArchiveSession('') error = nil")
		}
	})

	t.Run("RPC 失败透传", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{archiveErr: errors.New("archive denied")}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		if _, err := svc.ArchiveSession("sess-1", true); err == nil || !strings.Contains(err.Error(), "archive denied") {
			t.Fatalf("ArchiveSession error = %v, want archive denied", err)
		}
	})

	t.Run("归档当前会话切最新", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		workspace := t.TempDir()
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: workspace, UpdatedAt: time.Now().Add(-time.Hour)}
		s.sessions["sess-2"] = &sessionState{ID: "sess-2", WorkspacePath: workspace, UpdatedAt: time.Now()}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()
		state, err := svc.ArchiveSession("sess-1", true)
		if err != nil {
			t.Fatalf("ArchiveSession error = %v", err)
		}
		if state.CurrentSessionID != "sess-2" {
			t.Fatalf("CurrentSessionID = %q, want sess-2 after archive", state.CurrentSessionID)
		}
		s.stateMu.RLock()
		_, still := s.sessions["sess-1"]
		s.stateMu.RUnlock()
		if still {
			t.Fatal("archived session still in active map")
		}
		if gateway.archivedIDs["sess-1"] != true {
			t.Fatalf("archivedIDs = %v, want sess-1=true", gateway.archivedIDs)
		}
	})

	t.Run("取消归档仅清缓存", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, svc, _ := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: t.TempDir()}
		s.stateMu.Unlock()
		if _, err := svc.ArchiveSession("sess-1", false); err != nil {
			t.Fatalf("ArchiveSession(unarchive) error = %v", err)
		}
		if gateway.archivedIDs["sess-1"] != false {
			t.Fatalf("archivedIDs = %v, want sess-1=false", gateway.archivedIDs)
		}
	})
}

func TestPredictAndRefineForwarding(t *testing.T) {
	t.Run("predict 成功与失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{predictText: "下一个问题", predictErr: nil}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		got, err := svc.PredictNextUserMessage("draft")
		if err != nil || got != "下一个问题" {
			t.Fatalf("PredictNextUserMessage = (%q, %v)", got, err)
		}
		gateway.predictErr = errors.New("model unavailable")
		if _, err := svc.PredictNextUserMessage("draft"); err == nil {
			t.Fatal("PredictNextUserMessage error = nil on gateway failure")
		}
	})

	t.Run("refine 成功与失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{refineText: "润色后", refineErr: nil}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		got, err := svc.RefineInput("draft")
		if err != nil || got != "润色后" {
			t.Fatalf("RefineInput = (%q, %v)", got, err)
		}
		gateway.refineErr = errors.New("model unavailable")
		if _, err := svc.RefineInput("draft"); err == nil {
			t.Fatal("RefineInput error = nil on gateway failure")
		}
	})
}

func TestRollbackChatTurnArms(t *testing.T) {
	newRollbackBridge := func(t *testing.T, messages []ChatMessage) (*BridgeService, *ChatService, *chatSessionsGatewayStub, *emitRecorder) {
		gateway := &chatSessionsGatewayStub{}
		s, svc, rec := newChatSessionsTestBridge(t, gateway)
		s.stateMu.Lock()
		s.sessions["sess-1"] = &sessionState{ID: "sess-1", WorkspacePath: t.TempDir(), Messages: cloneMessages(messages)}
		s.currentSessionID = "sess-1"
		s.stateMu.Unlock()
		return s, svc, gateway, rec
	}
	userTurn := []ChatMessage{
		{ID: "u1", Role: "user", Content: "改一下"},
		{ID: "a1", Role: "assistant", Content: "改完了"},
	}

	t.Run("空消息 ID 拒绝", func(t *testing.T) {
		_, svc, _, _ := newRollbackBridge(t, userTurn)
		if _, err := svc.RollbackChatTurn("sess-1", "  "); err == nil {
			t.Fatal("RollbackChatTurn('') error = nil")
		}
	})

	t.Run("无会话 ID 且无当前会话", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		_, svc, _ := newChatSessionsTestBridge(t, gateway)
		if _, err := svc.RollbackChatTurn("", "u1"); err == nil || !strings.Contains(err.Error(), "session id") {
			t.Fatalf("RollbackChatTurn error = %v, want session id required", err)
		}
	})

	t.Run("会话不存在", func(t *testing.T) {
		_, svc, _, _ := newRollbackBridge(t, userTurn)
		if _, err := svc.RollbackChatTurn("missing", "u1"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("RollbackChatTurn error = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("进行中会话拒绝回滚", func(t *testing.T) {
		s, svc, _, _ := newRollbackBridge(t, userTurn)
		s.stateMu.Lock()
		s.sessions["sess-1"].Running = true
		s.stateMu.Unlock()
		if _, err := svc.RollbackChatTurn("sess-1", "u1"); err == nil || !strings.Contains(err.Error(), "still processing") {
			t.Fatalf("RollbackChatTurn error = %v, want still processing", err)
		}
	})

	t.Run("消息 ID 不存在", func(t *testing.T) {
		_, svc, _, _ := newRollbackBridge(t, userTurn)
		if _, err := svc.RollbackChatTurn("sess-1", "nope"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("RollbackChatTurn error = %v, want os.ErrNotExist for missing message", err)
		}
	})

	t.Run("缺回退快照拒绝", func(t *testing.T) {
		messages := []ChatMessage{
			{ID: "u1", Role: "user"},
			{ID: "a1", Role: "assistant", ChangeSet: &MessageChangeSet{Files: []ChangedFile{{Path: "a.go"}}}},
		}
		_, svc, _, _ := newRollbackBridge(t, messages)
		_, err := svc.RollbackChatTurn("sess-1", "u1")
		if err == nil || !strings.Contains(err.Error(), "安全回退快照") {
			t.Fatalf("RollbackChatTurn error = %v, want missing snapshot rejection", err)
		}
	})

	t.Run("内核回滚失败透传", func(t *testing.T) {
		messages := []ChatMessage{
			{ID: "u1", Role: "user"},
			{ID: "a1", Role: "assistant", Rollback: &TurnRollback{UserMessageID: "u1", AssistantMessageID: "a1"}},
		}
		_, svc, gateway, _ := newRollbackBridge(t, messages)
		gateway.rollbackErr = errors.New("apply failed")
		if _, err := svc.RollbackChatTurn("sess-1", "u1"); err == nil || !strings.Contains(err.Error(), "apply failed") {
			t.Fatalf("RollbackChatTurn error = %v, want apply failed", err)
		}
	})

	t.Run("成功回滚裁剪消息并清 prompts", func(t *testing.T) {
		messages := []ChatMessage{
			{ID: "u0", Role: "user"},
			{ID: "a0", Role: "assistant"},
			{ID: "u1", Role: "user"},
			{ID: "a1", Role: "assistant", Rollback: &TurnRollback{UserMessageID: "u1", AssistantMessageID: "a1"}},
		}
		s, svc, gateway, rec := newRollbackBridge(t, messages)
		s.prompts["p1"] = &promptState{PromptCard: PromptCard{ID: "p1", SessionID: "sess-1"}, AssistantMessageID: "a1"}
		state, err := svc.RollbackChatTurn("sess-1", "u1")
		if err != nil {
			t.Fatalf("RollbackChatTurn error = %v", err)
		}
		s.stateMu.RLock()
		remaining := len(s.sessions["sess-1"].Messages)
		_, promptGone := s.prompts["p1"]
		s.stateMu.RUnlock()
		if remaining != 2 {
			t.Fatalf("remaining messages = %d, want 2", remaining)
		}
		if promptGone {
			t.Fatal("prompt for removed assistant message still present")
		}
		if state.CurrentSessionID != "sess-1" {
			t.Fatalf("CurrentSessionID = %q, want sess-1", state.CurrentSessionID)
		}
		if len(gateway.savedMessages) == 0 {
			t.Fatal("trimmed messages not persisted")
		}
		if len(gateway.rollbackCalls) != 1 {
			t.Fatalf("rollbackCalls = %v, want one apply", gateway.rollbackCalls)
		}
		// emit 的 goroutine 内部 loadBootstrap 会写 HOME 下目录，等事件落地
		// 避免 tempdir 清理与后台写盘竞态。
		eventually(t, "shellUpdated emit after rollback", func() bool { return rec.has(shellUpdatedEventName) })
	})
}

func TestStateSyncPureHelpers(t *testing.T) {
	if got := stateChangeShellSource("  "); got != "event.runtime.state" {
		t.Fatalf("stateChangeShellSource('') = %q", got)
	}
	if got := stateChangeShellSource(" turn.started "); got != "event.runtime.state.turn.started" {
		t.Fatalf("stateChangeShellSource = %q", got)
	}

	if got := shellSyncBatchSource(nil); got != "event.state" {
		t.Fatalf("shellSyncBatchSource(nil) = %q", got)
	}
	if got := shellSyncBatchSource(map[string]struct{}{"a": {}}); got != "a" {
		t.Fatalf("shellSyncBatchSource(single) = %q", got)
	}
	if got := shellSyncBatchSource(map[string]struct{}{"b": {}, "a": {}}); got != "event.state.batch" {
		t.Fatalf("shellSyncBatchSource(multi) = %q", got)
	}

	if got := runtimeEventShellSource(adapter.Event{Type: " turn.completed "}); got != "event.runtime.state.turn.completed" {
		t.Fatalf("runtimeEventShellSource(Type) = %q", got)
	}
	if got := runtimeEventShellSource(adapter.Event{EventType: "goal.updated"}); got != "event.runtime.state.goal.updated" {
		t.Fatalf("runtimeEventShellSource(EventType fallback) = %q", got)
	}
	if got := runtimeEventShellSource(adapter.Event{}); got != "event.runtime.state" {
		t.Fatalf("runtimeEventShellSource(empty) = %q", got)
	}
	// logRuntimeStateSyncEvent 的错误分支（payload/Data 双字段）跑过即可。
	logRuntimeStateSyncEvent("turn.error", adapter.Event{Payload: map[string]any{"m": "x"}})
	logRuntimeStateSyncEvent("Error", adapter.Event{Data: map[string]any{"m": "y"}})
	logRuntimeStateSyncEvent("request.failed", adapter.Event{})
	logRuntimeStateSyncEvent("turn.completed", adapter.Event{})
	logRuntimeStateSyncEvent("", adapter.Event{})
}

func TestRunShellSyncDebouncerEmitsPerSession(t *testing.T) {
	gateway := &chatSessionsGatewayStub{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	rec := &emitRecorder{}
	s := &BridgeService{
		runtimeGateway:       gateway,
		sessions:             map[string]*sessionState{},
		runningConversations: map[string]*runningConversationState{},
		prompts:              map[string]*promptState{},
		emitEvent:            rec.record,
	}
	stop := make(chan struct{})
	s.stopCh = stop
	sources := make(chan string, 8)

	done := make(chan struct{})
	go func() {
		s.runShellSyncDebouncer(sources)
		close(done)
	}()

	// 无 running 会话 → 广播空 sessionID 的 shellUpdated。
	sources <- "event.runtime.state.turn.completed"
	eventually(t, "broadcast shellUpdated", func() bool { return rec.has(shellUpdatedEventName) })

	// 带 running 会话 → 按 session 定向发射。
	s.stateMu.Lock()
	s.runningConversations["sess-run"] = &runningConversationState{}
	s.stateMu.Unlock()
	before := rec.count()
	sources <- "event.filesystem"
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && rec.count() == before {
		time.Sleep(10 * time.Millisecond)
	}
	if rec.count() == before {
		t.Fatal("per-session shellUpdated not emitted for running conversation")
	}

	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("debouncer did not exit after stopCh closed")
	}
}

func TestSubscribeRuntimeStateEventsFailureFallback(t *testing.T) {
	// 订阅 RPC 失败时降级：空 channel + noop 退订，不阻塞 state sync。
	gateway := &chatSessionsGatewayStub{}
	s := &BridgeService{runtimeGateway: &subscribeFailGateway{chatSessionsGatewayStub: gateway}}
	out, unsubscribe := s.subscribeRuntimeStateEvents()
	if out == nil {
		t.Fatal("out channel nil on subscribe failure")
	}
	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("failure fallback channel should stay silent")
		}
	default:
	}
	unsubscribe() // 必须 noop 不 panic
}

type subscribeFailGateway struct {
	*chatSessionsGatewayStub
}

func (g *subscribeFailGateway) CoreSubscribeEventsRPC(context.Context, string, string, string, int) (<-chan adapter.Event, func(), error) {
	return nil, nil, errors.New("event/subscribe unavailable")
}

func TestStateWatchDirectoriesAggregation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, ".eos"), 0o755); err != nil {
		t.Fatalf("mkdir home .eos: %v", err)
	}
	workspace := t.TempDir()
	// watch 集合只收存在的目录（cleanExistingDir），先建出 .eos 子树。
	for _, sub := range []string{".eos", ".eos/sessions", ".eos/worktrees"} {
		if err := os.MkdirAll(filepath.Join(workspace, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	gateway := &chatSessionsGatewayStub{
		snapshot: adapter.RuntimeSnapshot{
			ForegroundWorkspace: workspace,
			Workspaces:          []adapter.WorkspaceSnapshot{{Path: workspace}},
		},
	}
	s := &BridgeService{runtimeGateway: gateway}
	s.stateMu.Lock()
	s.activeWorkspace = workspace
	s.stateMu.Unlock()

	dirs := s.stateWatchDirectories()
	if len(dirs) == 0 {
		t.Fatal("stateWatchDirectories empty")
	}
	joined := strings.Join(dirs, "\n")
	if !strings.Contains(joined, workspace) {
		t.Fatalf("active workspace dir missing from watch set: %v", dirs)
	}
	if !strings.Contains(joined, filepath.Join(workspace, ".eos")) {
		t.Fatalf(".eos dir missing from watch set: %v", dirs)
	}
	if !strings.Contains(joined, filepath.Join(home, ".eos")) {
		t.Fatalf("home .eos dir missing from watch set: %v", dirs)
	}
}

func TestStateWatchFilterAndPathHelpers(t *testing.T) {
	cases := map[string]bool{
		filepath.Join("/ws", ".eos", "skills", "x", "SKILL.md"):     true,
		filepath.Join("/ws", ".eos", "plugins", "p", "plugin.json"): true,
		filepath.Join("/ws", ".agents", "skills", "y", "SKILL.md"):  true,
		filepath.Join("/ws", ".eos.json"):                           true,
		filepath.Join("/ws", "README.md"):                           false,
		"":                                                          false,
	}
	for path, want := range cases {
		if got := isStateWatchEventPath(path); got != want {
			t.Fatalf("isStateWatchEventPath(%q) = %v, want %v", path, got, want)
		}
	}
	if !isStateWatchOp(fsnotify.Write) {
		t.Fatal("isStateWatchOp(Write) = false")
	}
	if isStateWatchOp(fsnotify.Chmod) {
		t.Fatal("isStateWatchOp(Chmod) = true")
	}

	if got := pluginSourcePath("directory:/opt/skills"); got != "/opt/skills" {
		t.Fatalf("pluginSourcePath = %q", got)
	}
	if got := pluginSourcePath("builtin"); got != "" {
		t.Fatalf("pluginSourcePath(builtin) = %q, want empty", got)
	}
	if got := pluginSourcePath("  "); got != "" {
		t.Fatalf("pluginSourcePath(blank) = %q, want empty", got)
	}

	dir := t.TempDir()
	if got := cleanExistingDir(dir); got == "" {
		t.Fatal("cleanExistingDir(existing dir) empty")
	}
	if got := cleanExistingDir(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("cleanExistingDir(missing) = %q, want empty", got)
	}
	out := map[string]struct{}{}
	addPathOrParent(out, dir) // 目录本身
	addPathOrParent(out, filepath.Join(dir, "file.txt"))
	addPathOrParent(out, "  ")
	if _, ok := out[dir]; !ok {
		t.Fatalf("directory path not added: %v", out)
	}
	if _, ok := out[filepath.Dir(filepath.Join(dir, "file.txt"))]; !ok || filepath.Dir(filepath.Join(dir, "file.txt")) != dir {
		t.Fatalf("file parent not added: %v", out)
	}
}
