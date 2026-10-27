package adapter

// failingXxx 系列：coreapi 各服务接口的「全方法注入错误」桩，供错误臂
// 扫荡点亮 adapter 包装层的引擎调用错误臂。由脚本从 pkg/coreapi/types.go
// 接口定义机械生成后手工补齐 import；接口演进时按同法再生。

import (
	"context"
	"errors"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
	"github.com/eosaios/eos/pkg/sandbox"
)

var errFailingEngine = errors.New("failing engine injected")

type failingStateService struct{}

func (failingStateService) Snapshot(p0 context.Context, p1 coreapi.StateSnapshotRequest) (coreapi.StateSnapshot, error) {
	var r0 coreapi.StateSnapshot
	return r0, errFailingEngine
}

type failingWorkspaceService struct{}

func (failingWorkspaceService) List(p0 context.Context, p1 coreapi.WorkspaceListRequest) ([]coreapi.Workspace, error) {
	var r0 []coreapi.Workspace
	return r0, errFailingEngine
}
func (failingWorkspaceService) Default(p0 context.Context) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingWorkspaceService) Last(p0 context.Context) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingWorkspaceService) ResolveForeground(p0 context.Context, p1 coreapi.ResolveForegroundWorkspaceRequest) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingWorkspaceService) Remember(p0 context.Context, p1 coreapi.RememberWorkspaceRequest) error {
	return errFailingEngine
}
func (failingWorkspaceService) Forget(p0 context.Context, p1 coreapi.WorkspacePathRequest) error {
	return errFailingEngine
}
func (failingWorkspaceService) Add(p0 context.Context, p1 coreapi.WorkspacePathRequest) error {
	return errFailingEngine
}
func (failingWorkspaceService) Remove(p0 context.Context, p1 coreapi.WorkspacePathRequest) error {
	return errFailingEngine
}
func (failingWorkspaceService) Use(p0 context.Context, p1 coreapi.WorkspacePathRequest) error {
	return errFailingEngine
}
func (failingWorkspaceService) SetForeground(p0 context.Context, p1 coreapi.WorkspacePathRequest) error {
	return errFailingEngine
}
func (failingWorkspaceService) Trust(p0 context.Context, p1 coreapi.WorkspacePathRequest) error {
	return errFailingEngine
}
func (failingWorkspaceService) ListWorktrees(p0 context.Context) ([]coreapi.Worktree, error) {
	var r0 []coreapi.Worktree
	return r0, errFailingEngine
}
func (failingWorkspaceService) CreateWorktree(p0 context.Context, p1 coreapi.CreateWorktreeRequest) (coreapi.Worktree, error) {
	var r0 coreapi.Worktree
	return r0, errFailingEngine
}
func (failingWorkspaceService) RemoveWorktree(p0 context.Context, p1 coreapi.RemoveWorktreeRequest) error {
	return errFailingEngine
}

type failingSessionService struct{}

func (failingSessionService) Create(p0 context.Context, p1 coreapi.CreateSessionRequest) (coreapi.Session, error) {
	var r0 coreapi.Session
	return r0, errFailingEngine
}
func (failingSessionService) Resume(p0 context.Context, p1 coreapi.ResumeSessionRequest) (coreapi.Session, error) {
	var r0 coreapi.Session
	return r0, errFailingEngine
}
func (failingSessionService) List(p0 context.Context, p1 coreapi.ListSessionsRequest) ([]coreapi.Session, error) {
	var r0 []coreapi.Session
	return r0, errFailingEngine
}
func (failingSessionService) Current(p0 context.Context, p1 coreapi.CurrentSessionRequest) (coreapi.Session, error) {
	var r0 coreapi.Session
	return r0, errFailingEngine
}
func (failingSessionService) SetCurrent(p0 context.Context, p1 coreapi.SetCurrentSessionRequest) error {
	return errFailingEngine
}
func (failingSessionService) Delete(p0 context.Context, p1 coreapi.DeleteSessionRequest) error {
	return errFailingEngine
}
func (failingSessionService) Rename(p0 context.Context, p1 coreapi.RenameSessionRequest) (coreapi.Session, error) {
	var r0 coreapi.Session
	return r0, errFailingEngine
}
func (failingSessionService) SetMeta(p0 context.Context, p1 coreapi.SetSessionMetaRequest) (coreapi.Session, error) {
	var r0 coreapi.Session
	return r0, errFailingEngine
}
func (failingSessionService) LoadMessages(p0 context.Context, p1 coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	var r0 []coreapi.SessionMessage
	return r0, errFailingEngine
}
func (failingSessionService) SaveMessages(p0 context.Context, p1 coreapi.SaveSessionMessagesRequest) (coreapi.Session, error) {
	var r0 coreapi.Session
	return r0, errFailingEngine
}

type failingMCPService struct{}

func (failingMCPService) List(p0 context.Context) ([]coreapi.MCPServer, error) {
	var r0 []coreapi.MCPServer
	return r0, errFailingEngine
}
func (failingMCPService) Upsert(p0 context.Context, p1 coreapi.UpsertMCPRequest) error {
	return errFailingEngine
}
func (failingMCPService) ImportJSON(p0 context.Context, p1 coreapi.ImportMCPJSONRequest) error {
	return errFailingEngine
}
func (failingMCPService) Delete(p0 context.Context, p1 coreapi.MCPNameRequest) error {
	return errFailingEngine
}
func (failingMCPService) SetEnabled(p0 context.Context, p1 coreapi.SetMCPEnabledRequest) error {
	return errFailingEngine
}

type failingLSPService struct{}

func (failingLSPService) List(p0 context.Context) ([]coreapi.LSPServer, error) {
	var r0 []coreapi.LSPServer
	return r0, errFailingEngine
}
func (failingLSPService) Detect(p0 context.Context, p1 coreapi.LSPLanguageRequest) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingLSPService) Start(p0 context.Context, p1 coreapi.LSPLanguageRequest) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingLSPService) Install(p0 context.Context, p1 coreapi.LSPLanguageRequest) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingLSPService) Diagnostics(p0 context.Context) ([]string, error) {
	var r0 []string
	return r0, errFailingEngine
}
func (failingLSPService) DiagnosticsSummary(p0 context.Context) (coreapi.LSPDiagnosticsSummary, error) {
	var r0 coreapi.LSPDiagnosticsSummary
	return r0, errFailingEngine
}

type failingConfigService struct{}

func (failingConfigService) GetRules(p0 context.Context) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingConfigService) RulesSnapshot(p0 context.Context) (coreapi.RulesSnapshot, error) {
	var r0 coreapi.RulesSnapshot
	return r0, errFailingEngine
}
func (failingConfigService) SaveRules(p0 context.Context, p1 coreapi.SaveRulesRequest) error {
	return errFailingEngine
}
func (failingConfigService) ResetRules(p0 context.Context) error { return errFailingEngine }
func (failingConfigService) GetSettings(p0 context.Context) (coreapi.Settings, error) {
	var r0 coreapi.Settings
	return r0, errFailingEngine
}
func (failingConfigService) SaveSettings(p0 context.Context, p1 coreapi.Settings) error {
	return errFailingEngine
}

type failingPermissionService struct{}

func (failingPermissionService) Snapshot(p0 context.Context) (coreapi.PermissionSnapshot, error) {
	var r0 coreapi.PermissionSnapshot
	return r0, errFailingEngine
}
func (failingPermissionService) PendingReview(p0 context.Context) (coreapi.PendingReview, error) {
	var r0 coreapi.PendingReview
	return r0, errFailingEngine
}
func (failingPermissionService) ClearPendingReview(p0 context.Context) error { return errFailingEngine }
func (failingPermissionService) SetAccessMode(p0 context.Context, p1 coreapi.SetModeRequest) error {
	return errFailingEngine
}
func (failingPermissionService) SetApprovalMode(p0 context.Context, p1 coreapi.SetModeRequest) error {
	return errFailingEngine
}
func (failingPermissionService) EnterFullAccess(p0 context.Context, p1 coreapi.EnterFullAccessRequest) error {
	return errFailingEngine
}

type failingExtensionService struct{}

func (failingExtensionService) ListSkills(p0 context.Context) ([]coreapi.SkillInfo, error) {
	var r0 []coreapi.SkillInfo
	return r0, errFailingEngine
}
func (failingExtensionService) ReloadSkills(p0 context.Context) error { return errFailingEngine }
func (failingExtensionService) SetSkillEnabled(p0 context.Context, p1 coreapi.SetExtensionEnabledRequest) error {
	return errFailingEngine
}
func (failingExtensionService) InvokeSkill(p0 context.Context, p1 coreapi.InvokeSkillRequest) (coreapi.InvokeSkillResult, error) {
	var r0 coreapi.InvokeSkillResult
	return r0, errFailingEngine
}
func (failingExtensionService) ListPlugins(p0 context.Context) ([]coreapi.PluginInfo, error) {
	var r0 []coreapi.PluginInfo
	return r0, errFailingEngine
}
func (failingExtensionService) SetPluginEnabled(p0 context.Context, p1 coreapi.SetExtensionEnabledRequest) error {
	return errFailingEngine
}
func (failingExtensionService) BrowserStatus(p0 context.Context) (coreapi.BrowserRuntimeStatus, error) {
	var r0 coreapi.BrowserRuntimeStatus
	return r0, errFailingEngine
}
func (failingExtensionService) BrowserLaunch(ctx context.Context, req coreapi.BrowserLaunchRequest) error {
	return errFailingEngine
}
func (failingExtensionService) BrowserClose(ctx context.Context, req coreapi.BrowserCloseRequest) error {
	return errFailingEngine
}
func (failingExtensionService) BrowserControlTakeover(ctx context.Context, req coreapi.BrowserControlTakeoverRequest) error {
	return errFailingEngine
}
func (failingExtensionService) BrowserControlConfirm(ctx context.Context) error {
	return errFailingEngine
}
func (failingExtensionService) BrowserControlResume(ctx context.Context) error {
	return errFailingEngine
}
func (failingExtensionService) BrowserTabs(ctx context.Context) ([]coreapi.BrowserTabInfo, error) {
	var r0 []coreapi.BrowserTabInfo
	return r0, errFailingEngine
}
func (failingExtensionService) BrowserProfiles(ctx context.Context) ([]coreapi.BrowserProfileRecord, error) {
	var r0 []coreapi.BrowserProfileRecord
	return r0, errFailingEngine
}

type failingContextService struct{}

func (failingContextService) Preview(p0 context.Context) ([]string, error) {
	var r0 []string
	return r0, errFailingEngine
}
func (failingContextService) Stats(p0 context.Context) (coreapi.ContextStats, error) {
	var r0 coreapi.ContextStats
	return r0, errFailingEngine
}
func (failingContextService) WindowTokens(p0 context.Context) (int, error) {
	var r0 int
	return r0, errFailingEngine
}
func (failingContextService) PinDocument(p0 context.Context, p1 coreapi.PinDocumentRequest) error {
	return errFailingEngine
}
func (failingContextService) Compact(p0 context.Context) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingContextService) Clear(p0 context.Context) error { return errFailingEngine }
func (failingContextService) Export(p0 context.Context, p1 coreapi.ExportContextRequest) error {
	return errFailingEngine
}

type failingUsageService struct{}

func (failingUsageService) Summary(p0 context.Context) (coreapi.UsageSummary, error) {
	var r0 coreapi.UsageSummary
	return r0, errFailingEngine
}
func (failingUsageService) CostSummary(p0 context.Context) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingUsageService) CostItems(p0 context.Context) ([]coreapi.CostItem, error) {
	var r0 []coreapi.CostItem
	return r0, errFailingEngine
}

type failingVersionService struct{}

func (failingVersionService) List(p0 context.Context) ([]coreapi.VersionItem, error) {
	var r0 []coreapi.VersionItem
	return r0, errFailingEngine
}
func (failingVersionService) Rollback(p0 context.Context, p1 coreapi.VersionIDRequest) error {
	return errFailingEngine
}
func (failingVersionService) Delete(p0 context.Context, p1 coreapi.VersionIDRequest) error {
	return errFailingEngine
}
func (failingVersionService) DeleteFile(p0 context.Context, p1 coreapi.VersionFileRequest) (int, error) {
	var r0 int
	return r0, errFailingEngine
}
func (failingVersionService) Clear(p0 context.Context) (int, error) {
	var r0 int
	return r0, errFailingEngine
}

type failingTaskService struct{}

func (failingTaskService) List(p0 context.Context) ([]coreapi.TaskSnapshot, error) {
	var r0 []coreapi.TaskSnapshot
	return r0, errFailingEngine
}
func (failingTaskService) Todos(p0 context.Context) ([]coreapi.TodoItem, error) {
	var r0 []coreapi.TodoItem
	return r0, errFailingEngine
}
func (failingTaskService) Tail(p0 context.Context, p1 coreapi.TaskIDRequest) ([]string, error) {
	var r0 []string
	return r0, errFailingEngine
}
func (failingTaskService) Kill(p0 context.Context, p1 coreapi.TaskIDRequest) error {
	return errFailingEngine
}
func (failingTaskService) Cleanup(p0 context.Context) (int, error) {
	var r0 int
	return r0, errFailingEngine
}

type failingGoalService struct{}

func (failingGoalService) Set(p0 context.Context, p1 coreapi.GoalSetRequest) (coreapi.ThreadGoal, error) {
	var r0 coreapi.ThreadGoal
	return r0, errFailingEngine
}
func (failingGoalService) Get(p0 context.Context, p1 coreapi.GoalRefRequest) (coreapi.GoalGetResponse, error) {
	var r0 coreapi.GoalGetResponse
	return r0, errFailingEngine
}
func (failingGoalService) Pause(p0 context.Context, p1 coreapi.GoalRefRequest) (coreapi.ThreadGoal, error) {
	var r0 coreapi.ThreadGoal
	return r0, errFailingEngine
}
func (failingGoalService) Resume(p0 context.Context, p1 coreapi.GoalRefRequest) (coreapi.ThreadGoal, error) {
	var r0 coreapi.ThreadGoal
	return r0, errFailingEngine
}
func (failingGoalService) Clear(p0 context.Context, p1 coreapi.GoalRefRequest) error {
	return errFailingEngine
}

type failingModeService struct{}

func (failingModeService) Snapshot(p0 context.Context) (coreapi.ModeSnapshot, error) {
	var r0 coreapi.ModeSnapshot
	return r0, errFailingEngine
}
func (failingModeService) SetExecutionMode(p0 context.Context, p1 coreapi.SetModeRequest) error {
	return errFailingEngine
}
func (failingModeService) SetSandboxMode(p0 context.Context, p1 coreapi.SetModeRequest) error {
	return errFailingEngine
}
func (failingModeService) SetReasoningLevel(p0 context.Context, p1 coreapi.SetModeRequest) error {
	return errFailingEngine
}

type failingModelService struct{}

func (failingModelService) List(p0 context.Context) ([]coreapi.ModelConfig, error) {
	var r0 []coreapi.ModelConfig
	return r0, errFailingEngine
}
func (failingModelService) Catalog(p0 context.Context) (coreapi.ModelCatalogState, error) {
	var r0 coreapi.ModelCatalogState
	return r0, errFailingEngine
}
func (failingModelService) Upsert(p0 context.Context, p1 coreapi.UpsertModelRequest) error {
	return errFailingEngine
}
func (failingModelService) Save(p0 context.Context, p1 coreapi.ModelSaveRequest) error {
	return errFailingEngine
}
func (failingModelService) Delete(p0 context.Context, p1 coreapi.ModelNameRequest) error {
	return errFailingEngine
}
func (failingModelService) Activate(p0 context.Context, p1 coreapi.ModelNameRequest) error {
	return errFailingEngine
}
func (failingModelService) SyncEnv(p0 context.Context) error { return errFailingEngine }
func (failingModelService) Context(p0 context.Context, p1 coreapi.ModelContextRequest) (coreapi.ModelContextSnapshot, error) {
	var r0 coreapi.ModelContextSnapshot
	return r0, errFailingEngine
}
func (failingModelService) SetWorkspace(p0 context.Context, p1 coreapi.SetWorkspaceModelRequest) error {
	return errFailingEngine
}
func (failingModelService) ClearWorkspace(p0 context.Context, p1 coreapi.ClearWorkspaceModelRequest) error {
	return errFailingEngine
}
func (failingModelService) SetSession(p0 context.Context, p1 coreapi.SetSessionModelRequest) error {
	return errFailingEngine
}
func (failingModelService) ClearSession(p0 context.Context, p1 coreapi.ClearSessionModelRequest) error {
	return errFailingEngine
}

type failingRemoteWorkspaceService struct{}

func (failingRemoteWorkspaceService) List(p0 context.Context) ([]coreapi.RemoteWorkspace, error) {
	var r0 []coreapi.RemoteWorkspace
	return r0, errFailingEngine
}
func (failingRemoteWorkspaceService) Open(p0 context.Context, p1 coreapi.RemoteWorkspaceRef) (coreapi.RemoteWorkspace, error) {
	var r0 coreapi.RemoteWorkspace
	return r0, errFailingEngine
}
func (failingRemoteWorkspaceService) Forget(p0 context.Context, p1 coreapi.RemoteWorkspaceRef) error {
	return errFailingEngine
}
func (failingRemoteWorkspaceService) ClearCache(p0 context.Context, p1 coreapi.RemoteWorkspaceRef) error {
	return errFailingEngine
}
func (failingRemoteWorkspaceService) CurrentRepo(p0 context.Context) (coreapi.RemoteRepoState, bool, error) {
	var r0 coreapi.RemoteRepoState
	var r1 bool
	return r0, r1, errFailingEngine
}

type failingGitService struct{}

func (failingGitService) Status(p0 context.Context, p1 coreapi.GitStatusRequest) ([]coreapi.GitChange, error) {
	var r0 []coreapi.GitChange
	return r0, errFailingEngine
}
func (failingGitService) Summary(p0 context.Context, p1 coreapi.GitSummaryRequest) (coreapi.GitSummaryResult, error) {
	var r0 coreapi.GitSummaryResult
	return r0, errFailingEngine
}
func (failingGitService) Diff(p0 context.Context, p1 coreapi.GitDiffRequest) (coreapi.GitTextResult, error) {
	var r0 coreapi.GitTextResult
	return r0, errFailingEngine
}
func (failingGitService) Branches(p0 context.Context, p1 coreapi.GitBranchesRequest) (coreapi.GitBranchesResult, error) {
	var r0 coreapi.GitBranchesResult
	return r0, errFailingEngine
}
func (failingGitService) Log(p0 context.Context, p1 coreapi.GitLogRequest) (coreapi.GitLogResult, error) {
	var r0 coreapi.GitLogResult
	return r0, errFailingEngine
}
func (failingGitService) Show(p0 context.Context, p1 coreapi.GitShowRequest) (coreapi.GitShowResult, error) {
	var r0 coreapi.GitShowResult
	return r0, errFailingEngine
}

type failingInsightService struct{}

func (failingInsightService) PredictNextUserMessage(p0 context.Context, p1 coreapi.PredictNextUserMessageRequest) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingInsightService) RefineInput(p0 context.Context, p1 coreapi.RefineInputRequest) (string, error) {
	var r0 string
	return r0, errFailingEngine
}
func (failingInsightService) PlanSnapshot(p0 context.Context) (coreapi.PlanSnapshot, error) {
	var r0 coreapi.PlanSnapshot
	return r0, errFailingEngine
}

type failingMemoryService struct{}

func (failingMemoryService) Snapshot(p0 context.Context) (coreapi.MemorySnapshot, error) {
	var r0 coreapi.MemorySnapshot
	return r0, errFailingEngine
}
func (failingMemoryService) Save(p0 context.Context, p1 coreapi.SaveMemoryRequest) error {
	return errFailingEngine
}
func (failingMemoryService) RebuildIndex(p0 context.Context) error { return errFailingEngine }
func (failingMemoryService) RecordAdd(p0 context.Context, p1 coreapi.AddMemoryRecordRequest) (coreapi.MemoryRecord, error) {
	var r0 coreapi.MemoryRecord
	return r0, errFailingEngine
}
func (failingMemoryService) RecordList(p0 context.Context, p1 coreapi.ListMemoryRecordsRequest) ([]coreapi.MemoryRecord, error) {
	var r0 []coreapi.MemoryRecord
	return r0, errFailingEngine
}
func (failingMemoryService) RecordSearch(p0 context.Context, p1 coreapi.SearchMemoryRecordsRequest) ([]coreapi.MemoryRecord, error) {
	var r0 []coreapi.MemoryRecord
	return r0, errFailingEngine
}
func (failingMemoryService) RecordDelete(p0 context.Context, p1 coreapi.DeleteMemoryRecordRequest) error {
	return errFailingEngine
}

type failingRoleService struct{}

func (failingRoleService) List(p0 context.Context) ([]coreapi.RoleConfig, error) {
	var r0 []coreapi.RoleConfig
	return r0, errFailingEngine
}
func (failingRoleService) Resolve(p0 context.Context, p1 coreapi.RoleRef) (coreapi.RoleConfig, error) {
	var r0 coreapi.RoleConfig
	return r0, errFailingEngine
}

type failingTurnService struct{}

func (failingTurnService) Start(p0 context.Context, p1 coreapi.StartTurnRequest) (coreapi.Turn, error) {
	var r0 coreapi.Turn
	return r0, errFailingEngine
}
func (failingTurnService) Interrupt(p0 context.Context, p1 coreapi.TurnRef) error {
	return errFailingEngine
}
func (failingTurnService) Resume(p0 context.Context, p1 coreapi.TurnRef) (coreapi.Turn, error) {
	var r0 coreapi.Turn
	return r0, errFailingEngine
}

type failingApprovalService struct{}

func (failingApprovalService) Respond(p0 context.Context, p1 coreapi.ApprovalResponse) error {
	return errFailingEngine
}

type failingInquiryService struct{}

func (failingInquiryService) Respond(p0 context.Context, p1 coreapi.InquiryResponse) error {
	return errFailingEngine
}

type failingAgentService struct{}

func (failingAgentService) Spawn(p0 context.Context, p1 coreapi.SpawnAgentRequest) (coreapi.Agent, error) {
	var r0 coreapi.Agent
	return r0, errFailingEngine
}
func (failingAgentService) SendInput(p0 context.Context, p1 coreapi.AgentInput) error {
	return errFailingEngine
}
func (failingAgentService) Wait(p0 context.Context, p1 coreapi.AgentRef) (coreapi.Agent, error) {
	var r0 coreapi.Agent
	return r0, errFailingEngine
}
func (failingAgentService) Run(p0 context.Context, p1 coreapi.RunAgentRequest) (coreapi.AgentRunResult, error) {
	var r0 coreapi.AgentRunResult
	return r0, errFailingEngine
}
func (failingAgentService) RunTool(p0 context.Context, p1 coreapi.AgentToolRequest) (coreapi.AgentToolResult, error) {
	var r0 coreapi.AgentToolResult
	return r0, errFailingEngine
}
func (failingAgentService) List(p0 context.Context, p1 coreapi.ListAgentsRequest) ([]coreapi.Agent, error) {
	var r0 []coreapi.Agent
	return r0, errFailingEngine
}
func (failingAgentService) Close(p0 context.Context, p1 coreapi.AgentRef) error {
	return errFailingEngine
}

type failingToolExecutor struct{}

func (failingToolExecutor) Execute(p0 context.Context, p1 coreapi.ToolRequest) (coreapi.ToolResult, error) {
	var r0 coreapi.ToolResult
	return r0, errFailingEngine
}

type failingToolCatalogService struct{}

func (failingToolCatalogService) List(p0 context.Context, p1 coreapi.ListToolCatalogRequest) ([]coreapi.ToolDefinition, error) {
	var r0 []coreapi.ToolDefinition
	return r0, errFailingEngine
}

type failingToolTelemetryService struct{}

func (failingToolTelemetryService) Traces(p0 context.Context) ([]coreapi.ToolTrace, error) {
	var r0 []coreapi.ToolTrace
	return r0, errFailingEngine
}
func (failingToolTelemetryService) Stats(p0 context.Context) ([]coreapi.ToolStat, error) {
	var r0 []coreapi.ToolStat
	return r0, errFailingEngine
}

type failingEventSubscriber struct{}

func (failingEventSubscriber) Subscribe(p0 context.Context, p1 coreapi.EventFilter) (<-chan protocol.Envelope, error) {
	var r0 <-chan protocol.Envelope
	return r0, errFailingEngine
}

type failingEventPublisher struct{}

func (failingEventPublisher) Publish(p0 context.Context, p1 protocol.Envelope) error {
	return errFailingEngine
}

type failingEventBus struct{}

type failingSandboxService struct{}

func (failingSandboxService) Policy(p0 context.Context, p1 coreapi.SessionRef) (sandbox.Policy, error) {
	var r0 sandbox.Policy
	return r0, errFailingEngine
}
func (failingSandboxService) SetPolicy(p0 context.Context, p1 coreapi.SessionRef, p2 sandbox.Policy) error {
	return errFailingEngine
}
func (failingSandboxService) DerivePolicy(p0 context.Context, p1 coreapi.DeriveSandboxPolicyRequest) (sandbox.Policy, error) {
	var r0 sandbox.Policy
	return r0, errFailingEngine
}
func (failingSandboxService) BackendStatus(p0 context.Context) sandbox.BackendStatus {
	var r0 sandbox.BackendStatus
	return r0
}

type failingDiagnosticsService struct{}

func (failingDiagnosticsService) Startup(p0 context.Context) (coreapi.StartupDiagnosticsResult, error) {
	var r0 coreapi.StartupDiagnosticsResult
	return r0, errFailingEngine
}

// failingEngine 嵌入 fakeEngine 并把所有服务 getter 换成失败桩：
// adapter 包装层的引擎调用错误臂可整体点亮。
type failingEngine struct{ *fakeEngine }

func (f *failingEngine) State() coreapi.StateService            { return failingStateService{} }
func (f *failingEngine) Workspaces() coreapi.WorkspaceService   { return failingWorkspaceService{} }
func (f *failingEngine) Sessions() coreapi.SessionService       { return failingSessionService{} }
func (f *failingEngine) MCP() coreapi.MCPService                { return failingMCPService{} }
func (f *failingEngine) LSP() coreapi.LSPService                { return failingLSPService{} }
func (f *failingEngine) Config() coreapi.ConfigService          { return failingConfigService{} }
func (f *failingEngine) Permissions() coreapi.PermissionService { return failingPermissionService{} }
func (f *failingEngine) Extensions() coreapi.ExtensionService   { return failingExtensionService{} }
func (f *failingEngine) Context() coreapi.ContextService        { return failingContextService{} }
func (f *failingEngine) Usage() coreapi.UsageService            { return failingUsageService{} }
func (f *failingEngine) Versions() coreapi.VersionService       { return failingVersionService{} }
func (f *failingEngine) Tasks() coreapi.TaskService             { return failingTaskService{} }
func (f *failingEngine) Goals() coreapi.GoalService             { return failingGoalService{} }
func (f *failingEngine) Modes() coreapi.ModeService             { return failingModeService{} }
func (f *failingEngine) Models() coreapi.ModelService           { return failingModelService{} }
func (f *failingEngine) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	return failingRemoteWorkspaceService{}
}
func (f *failingEngine) Git() coreapi.GitService                 { return failingGitService{} }
func (f *failingEngine) Insights() coreapi.InsightService        { return failingInsightService{} }
func (f *failingEngine) Memory() coreapi.MemoryService           { return failingMemoryService{} }
func (f *failingEngine) Roles() coreapi.RoleService              { return failingRoleService{} }
func (f *failingEngine) Turns() coreapi.TurnService              { return failingTurnService{} }
func (f *failingEngine) Approvals() coreapi.ApprovalService      { return failingApprovalService{} }
func (f *failingEngine) Inquiries() coreapi.InquiryService       { return failingInquiryService{} }
func (f *failingEngine) Agents() coreapi.AgentService            { return failingAgentService{} }
func (f *failingEngine) Tools() coreapi.ToolExecutor             { return failingToolExecutor{} }
func (f *failingEngine) ToolCatalog() coreapi.ToolCatalogService { return failingToolCatalogService{} }
func (f *failingEngine) ToolTelemetry() coreapi.ToolTelemetryService {
	return failingToolTelemetryService{}
}
func (f *failingEngine) Events() coreapi.EventSubscriber         { return failingEventSubscriber{} }
func (f *failingEngine) Sandbox() coreapi.SandboxService         { return failingSandboxService{} }
func (f *failingEngine) Diagnostics() coreapi.DiagnosticsService { return failingDiagnosticsService{} }

// mixedEngine 按服务名单选择性注入失败，其余服务走 fakeEngine，
// 供顺序链（如沙箱模式四级推进）逐阶段点亮错误臂。
type mixedEngine struct {
	*fakeEngine
	fail map[string]bool
}

func (m *mixedEngine) State() coreapi.StateService {
	if m.fail["State"] {
		return failingStateService{}
	}
	return m.fakeEngine.State()
}
func (m *mixedEngine) Workspaces() coreapi.WorkspaceService {
	if m.fail["Workspaces"] {
		return failingWorkspaceService{}
	}
	return m.fakeEngine.Workspaces()
}
func (m *mixedEngine) Sessions() coreapi.SessionService {
	if m.fail["Sessions"] {
		return failingSessionService{}
	}
	return m.fakeEngine.Sessions()
}
func (m *mixedEngine) MCP() coreapi.MCPService {
	if m.fail["MCP"] {
		return failingMCPService{}
	}
	return m.fakeEngine.MCP()
}
func (m *mixedEngine) LSP() coreapi.LSPService {
	if m.fail["LSP"] {
		return failingLSPService{}
	}
	return m.fakeEngine.LSP()
}
func (m *mixedEngine) Config() coreapi.ConfigService {
	if m.fail["Config"] {
		return failingConfigService{}
	}
	return m.fakeEngine.Config()
}
func (m *mixedEngine) Permissions() coreapi.PermissionService {
	if m.fail["Permissions"] {
		return failingPermissionService{}
	}
	return m.fakeEngine.Permissions()
}
func (m *mixedEngine) Extensions() coreapi.ExtensionService {
	if m.fail["Extensions"] {
		return failingExtensionService{}
	}
	return m.fakeEngine.Extensions()
}
func (m *mixedEngine) Context() coreapi.ContextService {
	if m.fail["Context"] {
		return failingContextService{}
	}
	return m.fakeEngine.Context()
}
func (m *mixedEngine) Usage() coreapi.UsageService {
	if m.fail["Usage"] {
		return failingUsageService{}
	}
	return m.fakeEngine.Usage()
}
func (m *mixedEngine) Versions() coreapi.VersionService {
	if m.fail["Versions"] {
		return failingVersionService{}
	}
	return m.fakeEngine.Versions()
}
func (m *mixedEngine) Tasks() coreapi.TaskService {
	if m.fail["Tasks"] {
		return failingTaskService{}
	}
	return m.fakeEngine.Tasks()
}
func (m *mixedEngine) Goals() coreapi.GoalService {
	if m.fail["Goals"] {
		return failingGoalService{}
	}
	return m.fakeEngine.Goals()
}
func (m *mixedEngine) Modes() coreapi.ModeService {
	if m.fail["Modes"] {
		return failingModeService{}
	}
	return m.fakeEngine.Modes()
}
func (m *mixedEngine) Models() coreapi.ModelService {
	if m.fail["Models"] {
		return failingModelService{}
	}
	return m.fakeEngine.Models()
}
func (m *mixedEngine) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	if m.fail["RemoteWorkspaces"] {
		return failingRemoteWorkspaceService{}
	}
	return m.fakeEngine.RemoteWorkspaces()
}
func (m *mixedEngine) Git() coreapi.GitService {
	if m.fail["Git"] {
		return failingGitService{}
	}
	return m.fakeEngine.Git()
}
func (m *mixedEngine) Insights() coreapi.InsightService {
	if m.fail["Insights"] {
		return failingInsightService{}
	}
	return m.fakeEngine.Insights()
}
func (m *mixedEngine) Memory() coreapi.MemoryService {
	if m.fail["Memory"] {
		return failingMemoryService{}
	}
	return m.fakeEngine.Memory()
}
func (m *mixedEngine) Roles() coreapi.RoleService {
	if m.fail["Roles"] {
		return failingRoleService{}
	}
	return m.fakeEngine.Roles()
}
func (m *mixedEngine) Turns() coreapi.TurnService {
	if m.fail["Turns"] {
		return failingTurnService{}
	}
	return m.fakeEngine.Turns()
}
func (m *mixedEngine) Approvals() coreapi.ApprovalService {
	if m.fail["Approvals"] {
		return failingApprovalService{}
	}
	return m.fakeEngine.Approvals()
}
func (m *mixedEngine) Inquiries() coreapi.InquiryService {
	if m.fail["Inquiries"] {
		return failingInquiryService{}
	}
	return m.fakeEngine.Inquiries()
}
func (m *mixedEngine) Agents() coreapi.AgentService {
	if m.fail["Agents"] {
		return failingAgentService{}
	}
	return m.fakeEngine.Agents()
}
func (m *mixedEngine) Tools() coreapi.ToolExecutor {
	if m.fail["Tools"] {
		return failingToolExecutor{}
	}
	return m.fakeEngine.Tools()
}
func (m *mixedEngine) ToolCatalog() coreapi.ToolCatalogService {
	if m.fail["ToolCatalog"] {
		return failingToolCatalogService{}
	}
	return m.fakeEngine.ToolCatalog()
}
func (m *mixedEngine) ToolTelemetry() coreapi.ToolTelemetryService {
	if m.fail["ToolTelemetry"] {
		return failingToolTelemetryService{}
	}
	return m.fakeEngine.ToolTelemetry()
}
func (m *mixedEngine) Events() coreapi.EventSubscriber {
	if m.fail["Events"] {
		return failingEventSubscriber{}
	}
	return m.fakeEngine.Events()
}
func (m *mixedEngine) Sandbox() coreapi.SandboxService {
	if m.fail["Sandbox"] {
		return failingSandboxService{}
	}
	return m.fakeEngine.Sandbox()
}
func (m *mixedEngine) Diagnostics() coreapi.DiagnosticsService {
	if m.fail["Diagnostics"] {
		return failingDiagnosticsService{}
	}
	return m.fakeEngine.Diagnostics()
}
