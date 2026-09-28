package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
	"github.com/eosaios/eos/pkg/sandbox"
)

// fakeEngine 实现 coreapi.Engine 全接口供 adapter 测试注入（无需 sidecar
// 子进程）。行为可经字段注入；零值即「全部成功返回零值」。
type fakeEngine struct {
	mu sync.Mutex

	state           coreapi.StateSnapshot
	workspaceList   []coreapi.Workspace
	models          []coreapi.ModelConfig
	modelContext    coreapi.ModelContextSnapshot
	settings        coreapi.Settings
	permissionSnap  coreapi.PermissionSnapshot
	modeSnap        coreapi.ModeSnapshot
	usageSummary    coreapi.UsageSummary
	costItems       []coreapi.CostItem
	taskList        []coreapi.TaskSnapshot
	todos           []coreapi.TodoItem
	skills          []coreapi.SkillInfo
	plugins         []coreapi.PluginInfo
	mcpList         []coreapi.MCPServer
	lspList         []coreapi.LSPServer
	contextPreview  []string
	contextStats    coreapi.ContextStats
	goalGet         coreapi.GoalGetResponse
	goalSetResult   coreapi.ThreadGoal
	memorySnap      coreapi.MemorySnapshot
	insightPlan     coreapi.PlanSnapshot
	remoteWorkspaces []coreapi.RemoteWorkspace
	remoteRepo      coreapi.RemoteRepoState
	remoteRepoOK    bool
	gitBranches     coreapi.GitBranchesResult
	gitSummary      coreapi.GitSummaryResult
	gitLog          coreapi.GitLogResult
	versions        []coreapi.VersionItem
	rulesSnap       coreapi.RulesSnapshot
	diagnostics     coreapi.StartupDiagnosticsResult
	browserStatus   coreapi.BrowserRuntimeStatus
	browserTabs     []coreapi.BrowserTabInfo
	browserProfiles []coreapi.BrowserProfileRecord
	traces          []coreapi.ToolTrace
	toolStats       []coreapi.ToolStat
	toolCatalog     []coreapi.ToolDefinition
	toolExecResult  coreapi.ToolResult
	turnResult      coreapi.Turn
	approvalErr     error

	// 可观测记录
	resumeCalls  []string
	turnStarts   []coreapi.StartTurnRequest
	modeSets     []string
	activeModel  string
	eventsSub    *fakeEvents
}

func (e *fakeEngine) Caller() coreapi.Caller                 { return nil }
func (e *fakeEngine) State() coreapi.StateService            { return &fakeState{e: e} }
func (e *fakeEngine) Workspaces() coreapi.WorkspaceService   { return &fakeWorkspaces{e: e} }
func (e *fakeEngine) Sessions() coreapi.SessionService       { return &fakeSessions{e: e} }
func (e *fakeEngine) MCP() coreapi.MCPService                { return &fakeMCP{e: e} }
func (e *fakeEngine) LSP() coreapi.LSPService                { return &fakeLSP{e: e} }
func (e *fakeEngine) Config() coreapi.ConfigService          { return &fakeConfig{e: e} }
func (e *fakeEngine) Permissions() coreapi.PermissionService { return &fakePermissions{e: e} }
func (e *fakeEngine) Extensions() coreapi.ExtensionService   { return &fakeExtensions{e: e} }
func (e *fakeEngine) Context() coreapi.ContextService        { return &fakeContext{e: e} }
func (e *fakeEngine) Usage() coreapi.UsageService            { return &fakeUsage{e: e} }
func (e *fakeEngine) Versions() coreapi.VersionService       { return &fakeVersions{e: e} }
func (e *fakeEngine) Tasks() coreapi.TaskService             { return &fakeTasks{e: e} }
func (e *fakeEngine) Goals() coreapi.GoalService             { return &fakeGoals{e: e} }
func (e *fakeEngine) Modes() coreapi.ModeService             { return &fakeModes{e: e} }
func (e *fakeEngine) Models() coreapi.ModelService           { return &fakeModels{e: e} }
func (e *fakeEngine) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	return &fakeRemote{e: e}
}
func (e *fakeEngine) Git() coreapi.GitService          { return &fakeGit{e: e} }
func (e *fakeEngine) Insights() coreapi.InsightService { return &fakeInsights{e: e} }
func (e *fakeEngine) Memory() coreapi.MemoryService    { return &fakeMemory{e: e} }
func (e *fakeEngine) Roles() coreapi.RoleService       { return &fakeRoles{} }
func (e *fakeEngine) Turns() coreapi.TurnService       { return &fakeTurns{e: e} }
func (e *fakeEngine) Approvals() coreapi.ApprovalService { return &fakeApprovals{e: e} }
func (e *fakeEngine) Inquiries() coreapi.InquiryService  { return &fakeInquiries{} }
func (e *fakeEngine) Agents() coreapi.AgentService       { return &fakeAgents{} }
func (e *fakeEngine) Tools() coreapi.ToolExecutor        { return &fakeTools{e: e} }
func (e *fakeEngine) ToolCatalog() coreapi.ToolCatalogService { return &fakeToolCatalog{e: e} }
func (e *fakeEngine) ToolTelemetry() coreapi.ToolTelemetryService { return &fakeToolTelemetry{e: e} }
func (e *fakeEngine) Events() coreapi.EventSubscriber {
	if e.eventsSub == nil {
		e.eventsSub = &fakeEvents{}
	}
	return e.eventsSub
}
func (e *fakeEngine) Sandbox() coreapi.SandboxService    { return &fakeSandbox{} }
func (e *fakeEngine) Diagnostics() coreapi.DiagnosticsService { return &fakeDiagnostics{e: e} }

type fakeState struct{ e *fakeEngine }

func (s *fakeState) Snapshot(context.Context, coreapi.StateSnapshotRequest) (coreapi.StateSnapshot, error) {
	return s.e.state, nil
}

type fakeWorkspaces struct{ e *fakeEngine }

func (s *fakeWorkspaces) List(context.Context, coreapi.WorkspaceListRequest) ([]coreapi.Workspace, error) {
	return s.e.workspaceList, nil
}
func (s *fakeWorkspaces) Default(context.Context) (string, error)   { return "/ws/default", nil }
func (s *fakeWorkspaces) Last(context.Context) (string, error)      { return "/ws/last", nil }
func (s *fakeWorkspaces) ResolveForeground(context.Context, coreapi.ResolveForegroundWorkspaceRequest) (string, error) {
	return "/ws/fg", nil
}
func (s *fakeWorkspaces) Remember(context.Context, coreapi.RememberWorkspaceRequest) error {
	return nil
}
func (s *fakeWorkspaces) Forget(context.Context, coreapi.WorkspacePathRequest) error { return nil }
func (s *fakeWorkspaces) Add(context.Context, coreapi.WorkspacePathRequest) error    { return nil }
func (s *fakeWorkspaces) Remove(context.Context, coreapi.WorkspacePathRequest) error { return nil }
func (s *fakeWorkspaces) Use(context.Context, coreapi.WorkspacePathRequest) error    { return nil }
func (s *fakeWorkspaces) SetForeground(context.Context, coreapi.WorkspacePathRequest) error {
	return nil
}
func (s *fakeWorkspaces) Trust(context.Context, coreapi.WorkspacePathRequest) error { return nil }
func (s *fakeWorkspaces) ListWorktrees(context.Context) ([]coreapi.Worktree, error) {
	return nil, nil
}
func (s *fakeWorkspaces) CreateWorktree(context.Context, coreapi.CreateWorktreeRequest) (coreapi.Worktree, error) {
	return coreapi.Worktree{Name: "wt"}, nil
}
func (s *fakeWorkspaces) RemoveWorktree(context.Context, coreapi.RemoveWorktreeRequest) error {
	return nil
}

type fakeSessions struct{ e *fakeEngine }

func (s *fakeSessions) Create(_ context.Context, req coreapi.CreateSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: "s-created", WorkspaceRoot: req.WorkspaceRoot}, nil
}
func (s *fakeSessions) Resume(_ context.Context, req coreapi.ResumeSessionRequest) (coreapi.Session, error) {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.e.resumeCalls = append(s.e.resumeCalls, req.SessionID)
	return coreapi.Session{ID: req.SessionID}, nil
}
func (s *fakeSessions) List(context.Context, coreapi.ListSessionsRequest) ([]coreapi.Session, error) {
	return nil, nil
}
func (s *fakeSessions) Current(context.Context, coreapi.CurrentSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: ""}, nil // 空：强制 ensureSessionID 走 Create
}
func (s *fakeSessions) SetCurrent(context.Context, coreapi.SetCurrentSessionRequest) error {
	return nil
}
func (s *fakeSessions) Delete(context.Context, coreapi.DeleteSessionRequest) error { return nil }
func (s *fakeSessions) Rename(context.Context, coreapi.RenameSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: "s-renamed"}, nil
}
func (s *fakeSessions) SetMeta(context.Context, coreapi.SetSessionMetaRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *fakeSessions) LoadMessages(context.Context, coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	return nil, nil
}
func (s *fakeSessions) SaveMessages(_ context.Context, req coreapi.SaveSessionMessagesRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: req.SessionID}, nil
}

type fakeMCP struct{ e *fakeEngine }

func (s *fakeMCP) List(context.Context) ([]coreapi.MCPServer, error) { return s.e.mcpList, nil }
func (s *fakeMCP) Upsert(context.Context, coreapi.UpsertMCPRequest) error {
	return nil
}
func (s *fakeMCP) ImportJSON(context.Context, coreapi.ImportMCPJSONRequest) error { return nil }
func (s *fakeMCP) Delete(context.Context, coreapi.MCPNameRequest) error           { return nil }
func (s *fakeMCP) SetEnabled(context.Context, coreapi.SetMCPEnabledRequest) error { return nil }

type fakeLSP struct{ e *fakeEngine }

func (s *fakeLSP) List(context.Context) ([]coreapi.LSPServer, error) { return s.e.lspList, nil }
func (s *fakeLSP) Detect(context.Context, coreapi.LSPLanguageRequest) (string, error) {
	return "detected", nil
}
func (s *fakeLSP) Start(context.Context, coreapi.LSPLanguageRequest) (string, error) {
	return "started", nil
}
func (s *fakeLSP) Install(context.Context, coreapi.LSPLanguageRequest) (string, error) {
	return "installed", nil
}
func (s *fakeLSP) Diagnostics(context.Context) ([]string, error) { return nil, nil }
func (s *fakeLSP) DiagnosticsSummary(context.Context) (coreapi.LSPDiagnosticsSummary, error) {
	return coreapi.LSPDiagnosticsSummary{}, nil
}

type fakeConfig struct{ e *fakeEngine }

func (s *fakeConfig) GetRules(context.Context) (string, error) { return "", nil }
func (s *fakeConfig) RulesSnapshot(context.Context) (coreapi.RulesSnapshot, error) {
	return s.e.rulesSnap, nil
}
func (s *fakeConfig) SaveRules(context.Context, coreapi.SaveRulesRequest) error { return nil }
func (s *fakeConfig) ResetRules(context.Context) error                          { return nil }
func (s *fakeConfig) GetSettings(context.Context) (coreapi.Settings, error) {
	return s.e.settings, nil
}
func (s *fakeConfig) SaveSettings(_ context.Context, in coreapi.Settings) error {
	s.e.settings = in
	return nil
}

type fakePermissions struct{ e *fakeEngine }

func (s *fakePermissions) Snapshot(context.Context) (coreapi.PermissionSnapshot, error) {
	return s.e.permissionSnap, nil
}
func (s *fakePermissions) PendingReview(context.Context) (coreapi.PendingReview, error) {
	return coreapi.PendingReview{}, nil
}
func (s *fakePermissions) ClearPendingReview(context.Context) error { return nil }
func (s *fakePermissions) SetAccessMode(_ context.Context, req coreapi.SetModeRequest) error {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.e.modeSets = append(s.e.modeSets, "access:"+req.Mode)
	return nil
}
func (s *fakePermissions) SetApprovalMode(_ context.Context, req coreapi.SetModeRequest) error {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.e.modeSets = append(s.e.modeSets, "approval:"+req.Mode)
	return nil
}
func (s *fakePermissions) EnterFullAccess(context.Context, coreapi.EnterFullAccessRequest) error {
	return nil
}

type fakeExtensions struct{ e *fakeEngine }

func (s *fakeExtensions) ListSkills(context.Context) ([]coreapi.SkillInfo, error) {
	return s.e.skills, nil
}
func (s *fakeExtensions) ReloadSkills(context.Context) error { return nil }
func (s *fakeExtensions) SetSkillEnabled(context.Context, coreapi.SetExtensionEnabledRequest) error {
	return nil
}
func (s *fakeExtensions) InvokeSkill(context.Context, coreapi.InvokeSkillRequest) (coreapi.InvokeSkillResult, error) {
	return coreapi.InvokeSkillResult{}, nil
}
func (s *fakeExtensions) ListPlugins(context.Context) ([]coreapi.PluginInfo, error) {
	return s.e.plugins, nil
}
func (s *fakeExtensions) SetPluginEnabled(context.Context, coreapi.SetExtensionEnabledRequest) error {
	return nil
}
func (s *fakeExtensions) BrowserStatus(context.Context) (coreapi.BrowserRuntimeStatus, error) {
	return s.e.browserStatus, nil
}
func (s *fakeExtensions) BrowserLaunch(context.Context, coreapi.BrowserLaunchRequest) error {
	return nil
}
func (s *fakeExtensions) BrowserClose(context.Context, coreapi.BrowserCloseRequest) error { return nil }
func (s *fakeExtensions) BrowserControlTakeover(context.Context, coreapi.BrowserControlTakeoverRequest) error {
	return nil
}
func (s *fakeExtensions) BrowserControlConfirm(context.Context) error { return nil }
func (s *fakeExtensions) BrowserControlResume(context.Context) error  { return nil }
func (s *fakeExtensions) BrowserTabs(context.Context) ([]coreapi.BrowserTabInfo, error) {
	return s.e.browserTabs, nil
}
func (s *fakeExtensions) BrowserProfiles(context.Context) ([]coreapi.BrowserProfileRecord, error) {
	return s.e.browserProfiles, nil
}

type fakeContext struct{ e *fakeEngine }

func (s *fakeContext) Preview(context.Context) ([]string, error) { return s.e.contextPreview, nil }
func (s *fakeContext) Stats(context.Context) (coreapi.ContextStats, error) {
	return s.e.contextStats, nil
}
func (s *fakeContext) WindowTokens(context.Context) (int, error) { return 1234, nil }
func (s *fakeContext) PinDocument(context.Context, coreapi.PinDocumentRequest) error {
	return nil
}
func (s *fakeContext) Compact(context.Context) (string, error) { return "compacted", nil }
func (s *fakeContext) Clear(context.Context) error             { return nil }
func (s *fakeContext) Export(context.Context, coreapi.ExportContextRequest) error {
	return nil
}

type fakeUsage struct{ e *fakeEngine }

func (s *fakeUsage) Summary(context.Context) (coreapi.UsageSummary, error) {
	return s.e.usageSummary, nil
}
func (s *fakeUsage) CostSummary(context.Context) (string, error) { return "$0.02", nil }
func (s *fakeUsage) CostItems(context.Context) ([]coreapi.CostItem, error) {
	return s.e.costItems, nil
}

type fakeVersions struct{ e *fakeEngine }

func (s *fakeVersions) List(context.Context) ([]coreapi.VersionItem, error) { return s.e.versions, nil }
func (s *fakeVersions) Rollback(context.Context, coreapi.VersionIDRequest) error {
	return nil
}
func (s *fakeVersions) Delete(context.Context, coreapi.VersionIDRequest) error { return nil }
func (s *fakeVersions) DeleteFile(context.Context, coreapi.VersionFileRequest) (int, error) {
	return 1, nil
}
func (s *fakeVersions) Clear(context.Context) (int, error) { return 2, nil }

type fakeTasks struct{ e *fakeEngine }

func (s *fakeTasks) List(context.Context) ([]coreapi.TaskSnapshot, error) { return s.e.taskList, nil }
func (s *fakeTasks) Todos(context.Context) ([]coreapi.TodoItem, error)    { return s.e.todos, nil }
func (s *fakeTasks) Tail(context.Context, coreapi.TaskIDRequest) ([]string, error) {
	return []string{"line"}, nil
}
func (s *fakeTasks) Kill(context.Context, coreapi.TaskIDRequest) error { return nil }
func (s *fakeTasks) Cleanup(context.Context) (int, error)              { return 3, nil }

type fakeGoals struct{ e *fakeEngine }

func (s *fakeGoals) Set(_ context.Context, req coreapi.GoalSetRequest) (coreapi.ThreadGoal, error) {
	if s.e.goalSetResult.Objective != "" {
		return s.e.goalSetResult, nil
	}
	return coreapi.ThreadGoal{Objective: req.Objective, Status: "active"}, nil
}
func (s *fakeGoals) Get(context.Context, coreapi.GoalRefRequest) (coreapi.GoalGetResponse, error) {
	return s.e.goalGet, nil
}
func (s *fakeGoals) Pause(context.Context, coreapi.GoalRefRequest) (coreapi.ThreadGoal, error) {
	return coreapi.ThreadGoal{Status: "paused"}, nil
}
func (s *fakeGoals) Resume(context.Context, coreapi.GoalRefRequest) (coreapi.ThreadGoal, error) {
	return coreapi.ThreadGoal{Status: "active"}, nil
}
func (s *fakeGoals) Clear(context.Context, coreapi.GoalRefRequest) error { return nil }

type fakeModes struct{ e *fakeEngine }

func (s *fakeModes) Snapshot(context.Context) (coreapi.ModeSnapshot, error) {
	return s.e.modeSnap, nil
}
func (s *fakeModes) SetExecutionMode(_ context.Context, req coreapi.SetModeRequest) error {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.e.modeSets = append(s.e.modeSets, "exec:"+req.Mode)
	return nil
}
func (s *fakeModes) SetSandboxMode(_ context.Context, req coreapi.SetModeRequest) error {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.e.modeSets = append(s.e.modeSets, "sandbox:"+req.Mode)
	return nil
}
func (s *fakeModes) SetReasoningLevel(_ context.Context, req coreapi.SetModeRequest) error {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.e.modeSets = append(s.e.modeSets, "reasoning:"+req.Mode)
	return nil
}

type fakeModels struct{ e *fakeEngine }

func (s *fakeModels) List(context.Context) ([]coreapi.ModelConfig, error) {
	out := make([]coreapi.ModelConfig, 0, len(s.e.models))
	for _, m := range s.e.models {
		if s.e.activeModel != "" && s.e.activeModel == m.Name {
			m.Active = true
		}
		out = append(out, m)
	}
	return out, nil
}
func (s *fakeModels) Catalog(context.Context) (coreapi.ModelCatalogState, error) {
	return coreapi.ModelCatalogState{}, nil
}
func (s *fakeModels) Upsert(context.Context, coreapi.UpsertModelRequest) error { return nil }
func (s *fakeModels) Save(context.Context, coreapi.ModelSaveRequest) error     { return nil }
func (s *fakeModels) Delete(context.Context, coreapi.ModelNameRequest) error   { return nil }
func (s *fakeModels) Activate(_ context.Context, req coreapi.ModelNameRequest) error {
	s.e.activeModel = req.Name
	return nil
}
func (s *fakeModels) SyncEnv(context.Context) error { return nil }
func (s *fakeModels) Context(context.Context, coreapi.ModelContextRequest) (coreapi.ModelContextSnapshot, error) {
	return s.e.modelContext, nil
}
func (s *fakeModels) SetWorkspace(context.Context, coreapi.SetWorkspaceModelRequest) error {
	return nil
}
func (s *fakeModels) ClearWorkspace(context.Context, coreapi.ClearWorkspaceModelRequest) error {
	return nil
}
func (s *fakeModels) SetSession(context.Context, coreapi.SetSessionModelRequest) error {
	return nil
}
func (s *fakeModels) ClearSession(context.Context, coreapi.ClearSessionModelRequest) error {
	return nil
}

type fakeRemote struct{ e *fakeEngine }

func (s *fakeRemote) List(context.Context) ([]coreapi.RemoteWorkspace, error) {
	return s.e.remoteWorkspaces, nil
}
func (s *fakeRemote) Open(context.Context, coreapi.RemoteWorkspaceRef) (coreapi.RemoteWorkspace, error) {
	return coreapi.RemoteWorkspace{ID: "r1"}, nil
}
func (s *fakeRemote) Forget(context.Context, coreapi.RemoteWorkspaceRef) error { return nil }
func (s *fakeRemote) ClearCache(context.Context, coreapi.RemoteWorkspaceRef) error {
	return nil
}
func (s *fakeRemote) CurrentRepo(context.Context) (coreapi.RemoteRepoState, bool, error) {
	return s.e.remoteRepo, s.e.remoteRepoOK, nil
}

type fakeGit struct{ e *fakeEngine }

func (s *fakeGit) Status(context.Context, coreapi.GitStatusRequest) ([]coreapi.GitChange, error) {
	return nil, nil
}
func (s *fakeGit) Summary(context.Context, coreapi.GitSummaryRequest) (coreapi.GitSummaryResult, error) {
	return s.e.gitSummary, nil
}
func (s *fakeGit) Diff(context.Context, coreapi.GitDiffRequest) (coreapi.GitTextResult, error) {
	return coreapi.GitTextResult{Text: "diff"}, nil
}
func (s *fakeGit) Branches(context.Context, coreapi.GitBranchesRequest) (coreapi.GitBranchesResult, error) {
	return s.e.gitBranches, nil
}
func (s *fakeGit) Log(context.Context, coreapi.GitLogRequest) (coreapi.GitLogResult, error) {
	return s.e.gitLog, nil
}
func (s *fakeGit) Show(context.Context, coreapi.GitShowRequest) (coreapi.GitShowResult, error) {
	return coreapi.GitShowResult{}, nil
}

type fakeInsights struct{ e *fakeEngine }

func (s *fakeInsights) PredictNextUserMessage(context.Context, coreapi.PredictNextUserMessageRequest) (string, error) {
	return "next", nil
}
func (s *fakeInsights) RefineInput(context.Context, coreapi.RefineInputRequest) (string, error) {
	return "refined", nil
}
func (s *fakeInsights) PlanSnapshot(context.Context) (coreapi.PlanSnapshot, error) {
	return s.e.insightPlan, nil
}

type fakeMemory struct{ e *fakeEngine }

func (s *fakeMemory) Snapshot(context.Context) (coreapi.MemorySnapshot, error) {
	return s.e.memorySnap, nil
}
func (s *fakeMemory) Save(context.Context, coreapi.SaveMemoryRequest) error { return nil }
func (s *fakeMemory) RebuildIndex(context.Context) error                    { return nil }
func (s *fakeMemory) RecordAdd(context.Context, coreapi.AddMemoryRecordRequest) (coreapi.MemoryRecord, error) {
	return coreapi.MemoryRecord{}, nil
}
func (s *fakeMemory) RecordList(context.Context, coreapi.ListMemoryRecordsRequest) ([]coreapi.MemoryRecord, error) {
	return nil, nil
}
func (s *fakeMemory) RecordSearch(context.Context, coreapi.SearchMemoryRecordsRequest) ([]coreapi.MemoryRecord, error) {
	return nil, nil
}
func (s *fakeMemory) RecordDelete(context.Context, coreapi.DeleteMemoryRecordRequest) error {
	return nil
}

type fakeRoles struct{}

func (fakeRoles) List(context.Context) ([]coreapi.RoleConfig, error) { return nil, nil }
func (fakeRoles) Resolve(context.Context, coreapi.RoleRef) (coreapi.RoleConfig, error) {
	return coreapi.RoleConfig{}, nil
}

type fakeTurns struct{ e *fakeEngine }

func (s *fakeTurns) Start(_ context.Context, req coreapi.StartTurnRequest) (coreapi.Turn, error) {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.e.turnStarts = append(s.e.turnStarts, req)
	if s.e.turnResult.ID != "" {
		return s.e.turnResult, nil
	}
	return coreapi.Turn{ID: req.TurnID, SessionID: req.SessionID, Status: "completed"}, nil
}
func (s *fakeTurns) Interrupt(context.Context, coreapi.TurnRef) error { return nil }
func (s *fakeTurns) Resume(context.Context, coreapi.TurnRef) (coreapi.Turn, error) {
	return coreapi.Turn{Status: "completed"}, nil
}

type fakeApprovals struct{ e *fakeEngine }

func (s *fakeApprovals) Respond(context.Context, coreapi.ApprovalResponse) error {
	return s.e.approvalErr
}

type fakeInquiries struct{}

func (fakeInquiries) Respond(context.Context, coreapi.InquiryResponse) error { return nil }

type fakeAgents struct{}

func (fakeAgents) Spawn(context.Context, coreapi.SpawnAgentRequest) (coreapi.Agent, error) {
	return coreapi.Agent{}, nil
}
func (fakeAgents) SendInput(context.Context, coreapi.AgentInput) error       { return nil }
func (fakeAgents) Wait(context.Context, coreapi.AgentRef) (coreapi.Agent, error) {
	return coreapi.Agent{}, nil
}
func (fakeAgents) Run(context.Context, coreapi.RunAgentRequest) (coreapi.AgentRunResult, error) {
	return coreapi.AgentRunResult{}, nil
}
func (fakeAgents) RunTool(context.Context, coreapi.AgentToolRequest) (coreapi.AgentToolResult, error) {
	return coreapi.AgentToolResult{}, nil
}
func (fakeAgents) List(context.Context, coreapi.ListAgentsRequest) ([]coreapi.Agent, error) {
	return nil, nil
}
func (fakeAgents) Close(context.Context, coreapi.AgentRef) error { return nil }

type fakeTools struct{ e *fakeEngine }

func (s *fakeTools) Execute(context.Context, coreapi.ToolRequest) (coreapi.ToolResult, error) {
	return s.e.toolExecResult, nil
}

type fakeToolCatalog struct{ e *fakeEngine }

func (s *fakeToolCatalog) List(context.Context, coreapi.ListToolCatalogRequest) ([]coreapi.ToolDefinition, error) {
	return s.e.toolCatalog, nil
}

type fakeToolTelemetry struct{ e *fakeEngine }

func (s *fakeToolTelemetry) Traces(context.Context) ([]coreapi.ToolTrace, error) {
	return s.e.traces, nil
}
func (s *fakeToolTelemetry) Stats(context.Context) ([]coreapi.ToolStat, error) {
	return s.e.toolStats, nil
}

type fakeEvents struct {
	ch     chan protocol.Envelope
	err    error
}

func (f *fakeEvents) Subscribe(context.Context, coreapi.EventFilter) (<-chan protocol.Envelope, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.ch != nil {
		return f.ch, nil
	}
	ch := make(chan protocol.Envelope)
	close(ch)
	return ch, nil
}

type fakeSandbox struct{}

func (fakeSandbox) Policy(context.Context, coreapi.SessionRef) (sandbox.Policy, error) {
	return sandbox.Policy{}, nil
}
func (fakeSandbox) SetPolicy(context.Context, coreapi.SessionRef, sandbox.Policy) error {
	return nil
}
func (fakeSandbox) DerivePolicy(_ context.Context, req coreapi.DeriveSandboxPolicyRequest) (sandbox.Policy, error) {
	return sandbox.Policy{Mode: sandbox.NormalizeMode(req.Mode)}, nil
}
func (fakeSandbox) BackendStatus(context.Context) sandbox.BackendStatus {
	return sandbox.BackendStatus{Backend: "fake"}
}

type fakeDiagnostics struct{ e *fakeEngine }

func (s *fakeDiagnostics) Startup(context.Context) (coreapi.StartupDiagnosticsResult, error) {
	return s.e.diagnostics, nil
}

var _ coreapi.Engine = (*fakeEngine)(nil)
var errNotAvailable = errors.New("core client is not available")

func isNotAvailable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "core client is not available")
}
