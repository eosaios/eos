// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

package ui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
	"github.com/eosaios/eos/pkg/sandbox"
)

// newTestEngine 返回一个完整 mock 的 coreapi.Engine，所有 service 实现都把
// 状态存在 in-memory 结构上。tests 不再需要启动 sidecar 子进程。
func newTestEngine() *testEngine {
	return &testEngine{
		settings: coreapi.Settings{
			PlanPromptStyle: "concise",
		},
		permissionSnap: coreapi.PermissionSnapshot{},
	}
}

// testEngine 实现 coreapi.Engine；所有 service 通过嵌入指针访问共享状态。
type testEngine struct {
	settings         coreapi.Settings
	permissionSnap   coreapi.PermissionSnapshot
	lastSettingsSave any
	workspaceList    []coreapi.Workspace
	models           []coreapi.ModelConfig
	activeModel      string

	// Session 行为可观测/可注入字段（供启动期 resume 测试用，零值保持原行为）：
	//   resumeCalls      —— 记录 Resume 收到的 SessionID（"latest" 透传）
	//   messages         —— LoadMessages 返回的消息（按 turn_id 分组回填 history）
	//   currentSessionID —— Current 返回的 ID（默认 "test-session"）
	resumeCalls      []string
	messages         []coreapi.SessionMessage
	currentSessionID string
}

func (e *testEngine) Caller() coreapi.Caller                 { return nil }
func (e *testEngine) State() coreapi.StateService            { return &testStateService{} }
func (e *testEngine) Workspaces() coreapi.WorkspaceService   { return &testWorkspaceService{e: e} }
func (e *testEngine) Sessions() coreapi.SessionService       { return &testSessionService{e: e} }
func (e *testEngine) MCP() coreapi.MCPService                { return &testMCPService{} }
func (e *testEngine) LSP() coreapi.LSPService                { return nil }
func (e *testEngine) Config() coreapi.ConfigService          { return &testConfigService{e: e} }
func (e *testEngine) Permissions() coreapi.PermissionService { return &testPermissionService{e: e} }
func (e *testEngine) Extensions() coreapi.ExtensionService   { return &testExtensionService{} }
func (e *testEngine) Context() coreapi.ContextService        { return &testContextService{} }
func (e *testEngine) Usage() coreapi.UsageService            { return &testUsageService{} }
func (e *testEngine) Versions() coreapi.VersionService       { return nil }
func (e *testEngine) Tasks() coreapi.TaskService             { return &testTaskService{} }
func (e *testEngine) Goals() coreapi.GoalService             { return &testGoalService{} }
func (e *testEngine) Modes() coreapi.ModeService             { return &testModeService{e: e} }
func (e *testEngine) Models() coreapi.ModelService           { return &testModelService{e: e} }
func (e *testEngine) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	return &testRemoteWorkspaceService{}
}
func (e *testEngine) Git() coreapi.GitService                     { return nil }
func (e *testEngine) Insights() coreapi.InsightService            { return nil }
func (e *testEngine) Memory() coreapi.MemoryService               { return nil }
func (e *testEngine) Roles() coreapi.RoleService                  { return nil }
func (e *testEngine) Turns() coreapi.TurnService                  { return nil }
func (e *testEngine) Approvals() coreapi.ApprovalService          { return nil }
func (e *testEngine) Inquiries() coreapi.InquiryService           { return nil }
func (e *testEngine) Agents() coreapi.AgentService                { return nil }
func (e *testEngine) Tools() coreapi.ToolExecutor                 { return nil }
func (e *testEngine) ToolCatalog() coreapi.ToolCatalogService     { return nil }
func (e *testEngine) ToolTelemetry() coreapi.ToolTelemetryService { return nil }
func (e *testEngine) Events() coreapi.EventSubscriber             { return &testEventSubscriber{} }
func (e *testEngine) Sandbox() coreapi.SandboxService             { return &testSandboxService{} }
func (e *testEngine) Diagnostics() coreapi.DiagnosticsService     { return &testDiagnosticsService{} }

// === Diagnostics ===
//
// Stub for the `coreapi.DiagnosticsService` the production Engine
// exposes for `startup/diagnostics` (used by the TUI to surface the
// sidecar's health, manifest, sandbox backend, and migration marker
// before showing the first prompt).
type testDiagnosticsService struct{}

func (s *testDiagnosticsService) Startup(context.Context) (coreapi.StartupDiagnosticsResult, error) {
	return coreapi.StartupDiagnosticsResult{
		OS:   "test",
		Arch: "test",
	}, nil
}

// === Workspace ===
type testWorkspaceService struct{ e *testEngine }

func (s *testWorkspaceService) List(context.Context, coreapi.WorkspaceListRequest) ([]coreapi.Workspace, error) {
	return s.e.workspaceList, nil
}
func (s *testWorkspaceService) Default(context.Context) (string, error) { return "", nil }
func (s *testWorkspaceService) Last(context.Context) (string, error)    { return "", nil }
func (s *testWorkspaceService) ResolveForeground(context.Context, coreapi.ResolveForegroundWorkspaceRequest) (string, error) {
	return "", nil
}
func (s *testWorkspaceService) Remember(context.Context, coreapi.RememberWorkspaceRequest) error {
	return nil
}
func (s *testWorkspaceService) Forget(context.Context, coreapi.WorkspacePathRequest) error {
	return nil
}
func (s *testWorkspaceService) Add(_ context.Context, req coreapi.WorkspacePathRequest) error {
	s.e.workspaceList = append(s.e.workspaceList, coreapi.Workspace{Path: req.Path, Active: true})
	return nil
}
func (s *testWorkspaceService) Remove(_ context.Context, req coreapi.WorkspacePathRequest) error {
	out := s.e.workspaceList[:0]
	for _, w := range s.e.workspaceList {
		if w.Path != req.Path {
			out = append(out, w)
		}
	}
	s.e.workspaceList = out
	return nil
}
func (s *testWorkspaceService) Use(_ context.Context, req coreapi.WorkspacePathRequest) error {
	for i, w := range s.e.workspaceList {
		if w.Path == req.Path {
			s.e.workspaceList[i].Active = true
		} else {
			s.e.workspaceList[i].Active = false
		}
	}
	return nil
}
func (s *testWorkspaceService) SetForeground(context.Context, coreapi.WorkspacePathRequest) error {
	return nil
}
func (s *testWorkspaceService) Trust(context.Context, coreapi.WorkspacePathRequest) error { return nil }
func (s *testWorkspaceService) ListWorktrees(context.Context) ([]coreapi.Worktree, error) {
	return nil, nil
}
func (s *testWorkspaceService) CreateWorktree(context.Context, coreapi.CreateWorktreeRequest) (coreapi.Worktree, error) {
	return coreapi.Worktree{}, nil
}
func (s *testWorkspaceService) RemoveWorktree(context.Context, coreapi.RemoveWorktreeRequest) error {
	return nil
}

// === Session ===
type testSessionService struct{ e *testEngine }

func (s *testSessionService) Create(_ context.Context, req coreapi.CreateSessionRequest) (coreapi.Session, error) {
	out := coreapi.Session{ID: "test-session", WorkspaceRoot: req.WorkspaceRoot}
	return out, nil
}
func (s *testSessionService) Resume(_ context.Context, req coreapi.ResumeSessionRequest) (coreapi.Session, error) {
	s.e.resumeCalls = append(s.e.resumeCalls, req.SessionID)
	return coreapi.Session{ID: req.SessionID}, nil
}
func (s *testSessionService) List(context.Context, coreapi.ListSessionsRequest) ([]coreapi.Session, error) {
	return nil, nil
}
func (s *testSessionService) Current(context.Context, coreapi.CurrentSessionRequest) (coreapi.Session, error) {
	if id := strings.TrimSpace(s.e.currentSessionID); id != "" {
		return coreapi.Session{ID: id}, nil
	}
	return coreapi.Session{ID: "test-session"}, nil
}
func (s *testSessionService) SetCurrent(context.Context, coreapi.SetCurrentSessionRequest) error {
	return nil
}
func (s *testSessionService) Delete(context.Context, coreapi.DeleteSessionRequest) error { return nil }
func (s *testSessionService) Rename(context.Context, coreapi.RenameSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *testSessionService) SetMeta(context.Context, coreapi.SetSessionMetaRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *testSessionService) LoadMessages(context.Context, coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	if s.e.messages != nil {
		return s.e.messages, nil
	}
	return nil, nil
}
func (s *testSessionService) SaveMessages(_ context.Context, req coreapi.SaveSessionMessagesRequest) (coreapi.Session, error) {
	id := req.SessionID
	if id == "" {
		id = "test-session"
	}
	return coreapi.Session{ID: id}, nil
}

// === Config ===
type testConfigService struct{ e *testEngine }

func (s *testConfigService) GetRules(context.Context) (string, error) { return "", nil }
func (s *testConfigService) RulesSnapshot(context.Context) (coreapi.RulesSnapshot, error) {
	return coreapi.RulesSnapshot{}, nil
}
func (s *testConfigService) SaveRules(context.Context, coreapi.SaveRulesRequest) error { return nil }
func (s *testConfigService) ResetRules(context.Context) error                          { return nil }
func (s *testConfigService) GetSettings(context.Context) (coreapi.Settings, error) {
	return s.e.settings, nil
}
func (s *testConfigService) SaveSettings(_ context.Context, settings coreapi.Settings) error {
	s.e.settings = settings
	// 同步写 .eos/settings.json，模拟 eos-core 的 workspace 持久化路径。
	wd, _ := os.Getwd()
	dir := filepath.Join(wd, ".eos")
	if err := os.MkdirAll(dir, 0o755); err == nil {
		path := filepath.Join(dir, "settings.json")
		doc := map[string]any{
			"plan_prompt_style": settings.PlanPromptStyle,
			"plan_bubble":       settings.PlanBubbleColor,
			"watch_mode":        settings.WatchMode,
			"language":          settings.Language,
			"theme":             settings.Theme,
		}
		if data, err := json.MarshalIndent(doc, "", "  "); err == nil {
			_ = os.WriteFile(path, data, 0o644)
		}
	}
	return nil
}

// === Permission ===
type testPermissionService struct{ e *testEngine }

func (s *testPermissionService) Snapshot(context.Context) (coreapi.PermissionSnapshot, error) {
	return s.e.permissionSnap, nil
}
func (s *testPermissionService) PendingReview(context.Context) (coreapi.PendingReview, error) {
	return coreapi.PendingReview{}, nil
}
func (s *testPermissionService) ClearPendingReview(context.Context) error { return nil }
func (s *testPermissionService) SetAccessMode(_ context.Context, req coreapi.SetModeRequest) error {
	s.e.permissionSnap.AccessMode = req.Mode
	switch req.Mode {
	case "danger-full-access", "danger_full_access", "full_access", "full-access":
		s.e.permissionSnap.SandboxMode = "danger-full-access"
	default:
		if s.e.permissionSnap.SandboxMode == "" {
			s.e.permissionSnap.SandboxMode = "workspace-write"
		}
	}
	return nil
}
func (s *testPermissionService) SetApprovalMode(_ context.Context, req coreapi.SetModeRequest) error {
	s.e.permissionSnap.ApprovalMode = req.Mode
	return nil
}
func (s *testPermissionService) EnterFullAccess(_ context.Context, req coreapi.EnterFullAccessRequest) error {
	s.e.permissionSnap.ApprovalMode = "never"
	s.e.permissionSnap.SandboxMode = "danger-full-access"
	return nil
}

// === Events / Sandbox ===
type testEventSubscriber struct{}

func (b *testEventSubscriber) Subscribe(context.Context, coreapi.EventFilter) (<-chan protocol.Envelope, error) {
	ch := make(chan protocol.Envelope)
	close(ch)
	return ch, nil
}

type testSandboxService struct{}

func (s *testSandboxService) Policy(context.Context, coreapi.SessionRef) (sandbox.Policy, error) {
	return sandbox.Policy{}, nil
}
func (s *testSandboxService) SetPolicy(context.Context, coreapi.SessionRef, sandbox.Policy) error {
	return nil
}
func (s *testSandboxService) DerivePolicy(_ context.Context, req coreapi.DeriveSandboxPolicyRequest) (sandbox.Policy, error) {
	return sandbox.Policy{Mode: sandbox.NormalizeMode(req.Mode)}, nil
}
func (s *testSandboxService) BackendStatus(context.Context) sandbox.BackendStatus {
	return sandbox.BackendStatus{Backend: "test", Enforced: false}
}

// === Model ===
type testModelService struct{ e *testEngine }

func (s *testModelService) List(context.Context) ([]coreapi.ModelConfig, error) {
	out := make([]coreapi.ModelConfig, 0, len(s.e.models))
	for _, m := range s.e.models {
		if s.e.activeModel != "" && s.e.activeModel == m.Name {
			m.Active = true
		}
		out = append(out, m)
	}
	return out, nil
}
func (s *testModelService) Catalog(context.Context) (coreapi.ModelCatalogState, error) {
	return coreapi.ModelCatalogState{
		Providers: []coreapi.ModelProviderOption{{
			ID:            "openai",
			Name:          "OpenAI",
			Endpoints:     []coreapi.ProviderEndpoint{{Plan: "api", Format: "openai_chat", APIBase: "https://api.openai.com/v1"}},
			DefaultModels: []string{"gpt-5-codex"},
		}},
		Presets: []coreapi.ModelPresetOption{{
			ID:            "gpt-5-codex",
			Name:          "GPT-5-Codex",
			ProviderID:    "openai",
			ModelName:     "gpt-5-codex",
			Plan:          "api",
			Format:        "openai_chat",
			ContextWindow: 400000,
			Tags:          []string{"推荐", "编程"},
			SupportsTools: true,
		}},
		AllowCustomProvider: true,
		AllowCustomModel:    true,
	}, nil
}
func (s *testModelService) Upsert(_ context.Context, req coreapi.UpsertModelRequest) error {
	for i, m := range s.e.models {
		if m.Name == req.Name {
			s.e.models[i].APIBase = req.APIBase
			s.e.models[i].Model = req.Model
			return nil
		}
	}
	s.e.models = append(s.e.models, coreapi.ModelConfig{
		Name:    req.Name,
		APIBase: req.APIBase,
		Model:   req.Model,
	})
	return nil
}
func (s *testModelService) Save(context.Context, coreapi.ModelSaveRequest) error { return nil }
func (s *testModelService) Delete(_ context.Context, req coreapi.ModelNameRequest) error {
	out := s.e.models[:0]
	for _, m := range s.e.models {
		if m.Name != req.Name {
			out = append(out, m)
		}
	}
	s.e.models = out
	return nil
}
func (s *testModelService) Activate(_ context.Context, req coreapi.ModelNameRequest) error {
	s.e.activeModel = req.Name
	return nil
}
func (s *testModelService) SyncEnv(context.Context) error { return nil }
func (s *testModelService) Context(context.Context, coreapi.ModelContextRequest) (coreapi.ModelContextSnapshot, error) {
	return coreapi.ModelContextSnapshot{
		ResolvedModelName: s.e.activeModel,
		ResolvedScope:     "global",
	}, nil
}
func (s *testModelService) SetWorkspace(_ context.Context, req coreapi.SetWorkspaceModelRequest) error {
	s.e.activeModel = req.ModelName
	return nil
}
func (s *testModelService) ClearWorkspace(context.Context, coreapi.ClearWorkspaceModelRequest) error {
	return nil
}
func (s *testModelService) SetSession(_ context.Context, req coreapi.SetSessionModelRequest) error {
	s.e.activeModel = req.ModelName
	return nil
}
func (s *testModelService) ClearSession(context.Context, coreapi.ClearSessionModelRequest) error {
	return nil
}

// === Mode ===
type testModeService struct{ e *testEngine }

func (s *testModeService) Snapshot(context.Context) (coreapi.ModeSnapshot, error) {
	return coreapi.ModeSnapshot{}, nil
}
func (s *testModeService) SetExecutionMode(context.Context, coreapi.SetModeRequest) error {
	return nil
}

// SetSandboxMode 模拟内核 runtime/sandbox_mode/set：同步 mode 快照与
// permission_snapshot.sandbox_mode（内核行为见 eos-core-runtime 的
// runtime_sandbox_mode_sync 回归测试）。
func (s *testModeService) SetSandboxMode(_ context.Context, req coreapi.SetModeRequest) error {
	s.e.permissionSnap.SandboxMode = req.Mode
	return nil
}
func (s *testModeService) SetReasoningLevel(context.Context, coreapi.SetModeRequest) error {
	return nil
}

// === State ===
type testStateService struct{}

// === Usage ===
// 补全 Usage 空实现，让依赖 refreshCostPanel/UsageSummary 的路径（如启动期 resume）
// 在测试里不再 nil 解引用。零值无副作用，不改变既有测试行为。
type testUsageService struct{}

func (s *testUsageService) Summary(context.Context) (coreapi.UsageSummary, error) {
	return coreapi.UsageSummary{}, nil
}
func (s *testUsageService) CostSummary(context.Context) (string, error) { return "", nil }
func (s *testUsageService) CostItems(context.Context) ([]coreapi.CostItem, error) {
	return nil, nil
}

// === Context ===
type testContextService struct{}

func (s *testContextService) Preview(context.Context) ([]string, error) { return nil, nil }
func (s *testContextService) Stats(context.Context) (coreapi.ContextStats, error) {
	return coreapi.ContextStats{}, nil
}
func (s *testContextService) WindowTokens(context.Context) (int, error) { return 0, nil }
func (s *testContextService) PinDocument(context.Context, coreapi.PinDocumentRequest) error {
	return nil
}
func (s *testContextService) Compact(context.Context) (string, error) { return "", nil }
func (s *testContextService) Clear(context.Context) error             { return nil }
func (s *testContextService) Export(context.Context, coreapi.ExportContextRequest) error {
	return nil
}

// === RemoteWorkspace ===
type testRemoteWorkspaceService struct{}

func (s *testRemoteWorkspaceService) List(context.Context) ([]coreapi.RemoteWorkspace, error) {
	return nil, nil
}
func (s *testRemoteWorkspaceService) Open(context.Context, coreapi.RemoteWorkspaceRef) (coreapi.RemoteWorkspace, error) {
	return coreapi.RemoteWorkspace{}, nil
}
func (s *testRemoteWorkspaceService) Forget(context.Context, coreapi.RemoteWorkspaceRef) error {
	return nil
}
func (s *testRemoteWorkspaceService) ClearCache(context.Context, coreapi.RemoteWorkspaceRef) error {
	return nil
}
func (s *testRemoteWorkspaceService) CurrentRepo(context.Context) (coreapi.RemoteRepoState, bool, error) {
	return coreapi.RemoteRepoState{}, false, nil
}

func (s *testStateService) Snapshot(context.Context, coreapi.StateSnapshotRequest) (coreapi.StateSnapshot, error) {
	return coreapi.StateSnapshot{}, nil
}

// === Extension ===
type testExtensionService struct{}

func (s *testExtensionService) ListSkills(context.Context) ([]coreapi.SkillInfo, error) {
	return nil, nil
}
func (s *testExtensionService) ReloadSkills(context.Context) error { return nil }
func (s *testExtensionService) SetSkillEnabled(context.Context, coreapi.SetExtensionEnabledRequest) error {
	return nil
}
func (s *testExtensionService) InvokeSkill(context.Context, coreapi.InvokeSkillRequest) (coreapi.InvokeSkillResult, error) {
	return coreapi.InvokeSkillResult{}, nil
}
func (s *testExtensionService) ListPlugins(context.Context) ([]coreapi.PluginInfo, error) {
	return nil, nil
}
func (s *testExtensionService) SetPluginEnabled(context.Context, coreapi.SetExtensionEnabledRequest) error {
	return nil
}
func (s *testExtensionService) BrowserStatus(context.Context) (coreapi.BrowserRuntimeStatus, error) {
	return coreapi.BrowserRuntimeStatus{}, nil
}

func (s *testExtensionService) BrowserLaunch(ctx context.Context, req coreapi.BrowserLaunchRequest) error {
	return nil
}

func (s *testExtensionService) BrowserClose(ctx context.Context, req coreapi.BrowserCloseRequest) error {
	return nil
}

func (s *testExtensionService) BrowserControlTakeover(ctx context.Context, req coreapi.BrowserControlTakeoverRequest) error {
	return nil
}

func (s *testExtensionService) BrowserControlConfirm(ctx context.Context) error {
	return nil
}

func (s *testExtensionService) BrowserControlResume(ctx context.Context) error {
	return nil
}

func (s *testExtensionService) BrowserTabs(ctx context.Context) ([]coreapi.BrowserTabInfo, error) {
	return nil, nil
}

func (s *testExtensionService) BrowserProfiles(ctx context.Context) ([]coreapi.BrowserProfileRecord, error) {
	return nil, nil
}

// === MCP ===
type testMCPService struct{}

func (s *testMCPService) List(context.Context) ([]coreapi.MCPServer, error)      { return nil, nil }
func (s *testMCPService) Upsert(context.Context, coreapi.UpsertMCPRequest) error { return nil }
func (s *testMCPService) ImportJSON(context.Context, coreapi.ImportMCPJSONRequest) error {
	return nil
}
func (s *testMCPService) Delete(context.Context, coreapi.MCPNameRequest) error { return nil }
func (s *testMCPService) SetEnabled(context.Context, coreapi.SetMCPEnabledRequest) error {
	return nil
}

// === Task ===
type testTaskService struct{}

func (s *testTaskService) List(context.Context) ([]coreapi.TaskSnapshot, error) { return nil, nil }
func (s *testTaskService) Todos(context.Context) ([]coreapi.TodoItem, error)    { return nil, nil }
func (s *testTaskService) Tail(context.Context, coreapi.TaskIDRequest) ([]string, error) {
	return nil, nil
}
func (s *testTaskService) Kill(context.Context, coreapi.TaskIDRequest) error { return nil }
func (s *testTaskService) Cleanup(context.Context) (int, error)              { return 0, nil }

// testGoalService 是 UI 测试用的 goal service 假实现（不触网）。
type testGoalService struct{}

func (s *testGoalService) Set(_ context.Context, _ coreapi.GoalSetRequest) (coreapi.ThreadGoal, error) {
	return coreapi.ThreadGoal{}, nil
}

func (s *testGoalService) Get(_ context.Context, _ coreapi.GoalRefRequest) (coreapi.GoalGetResponse, error) {
	return coreapi.GoalGetResponse{}, nil
}

func (s *testGoalService) Pause(_ context.Context, _ coreapi.GoalRefRequest) (coreapi.ThreadGoal, error) {
	return coreapi.ThreadGoal{}, nil
}

func (s *testGoalService) Resume(_ context.Context, _ coreapi.GoalRefRequest) (coreapi.ThreadGoal, error) {
	return coreapi.ThreadGoal{}, nil
}

func (s *testGoalService) Clear(_ context.Context, _ coreapi.GoalRefRequest) error { return nil }
