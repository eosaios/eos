// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

package ui

import (
	"context"
	"encoding/json"
	"errors"
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
	mcpList          []coreapi.MCPServer

	// Session 行为可观测/可注入字段（供启动期 resume 测试用，零值保持原行为）：
	//   resumeCalls      —— 记录 Resume 收到的 SessionID（"latest" 透传）
	//   messages         —— LoadMessages 返回的消息（按 turn_id 分组回填 history）
	//   currentSessionID —— Current 返回的 ID（默认 "test-session"）
	resumeCalls      []string
	messages         []coreapi.SessionMessage
	currentSessionID string

	// currentSessionMeta 注入 Sessions.Current 的 Metadata（HealCurrentSessionModel
	// 读 model_name 覆盖用）。
	currentSessionMeta map[string]any

	// 失败臂批测注入口（零值=原成功行为）：config/context/memory 服务
	// 经共享状态读取（服务每次取用时新建实例，注入字段必须挂 engine 上）。
	saveRulesErr   error
	saveMemoryErr  error
	compactErr     error
	compactMessage string
	clearCtxErr    error
	exportCtxErr   error

	// foregroundWS 写入 StateSnapshot.ForegroundWorkspace：/init 等按工作区
	// 根写文件的路径必须指向 t.TempDir()，避免往测试进程 cwd 落 EOS.md。
	foregroundWS string

	// git 注入（供 /git /diff /review 分支批测）。
	gitStatus    []coreapi.GitChange
	gitStatusErr error
	gitBranches  coreapi.GitBranchesResult
	gitLog       coreapi.GitLogResult
	gitShow      coreapi.GitShowResult
	gitDiff      coreapi.GitTextResult
	gitDiffErr   error

	// pendingReview 注入（/diff 无参、/review 无参的待审批分支）。
	pendingReview coreapi.PendingReview

	// caller 注入（plugin/install|search|remove 的 CallCore 路径）。
	caller coreapi.Caller

	// slash_runtime 批测注入口（零值=原行为）。
	loadMessagesErr     error                        // LoadSessionMessages 失败臂
	sessionList         []coreapi.Session            // ListSessions 返回
	listSessionsErr     error                        // ListSessions 失败臂
	saveSessionMsgsErr  error                        // SaveSessionMessages 失败臂
	usageSummaryErr     error                        // UsageSummary 失败臂（/stats）
	usageSummary        coreapi.UsageSummary         // UsageSummary 返回
	toolStats           []coreapi.ToolStat           // ToolStats/doctor 统计
	toolStatsErr        error                        // ToolStats 失败臂
	toolTraces          []coreapi.ToolTrace          // ToolTraces/doctor 时间线
	toolTracesErr       error                        // ToolTraces 失败臂
	taskList            []coreapi.TaskSnapshot       // Tasks.List（doctor 后台任务）
	agentList           []coreapi.Agent              // Agents.List（doctor 代理任务）
	todoList            []coreapi.TodoItem           // Todos（doctor 待办）
	skills              []coreapi.SkillInfo          // ListSkills（doctor/skills 面板）
	plugins             []coreapi.PluginInfo         // ListPlugins（doctor/插件面板）
	browserStatus       coreapi.BrowserRuntimeStatus // BrowserStatus（doctor 浏览器行）
	browserStatusErr    error                        // BrowserStatus 失败臂
	lspDiagnostics      []string                     // LSP Diagnostics（doctor 摘要）
	remoteRepo          coreapi.RemoteRepoState      // CurrentRemoteRepo 返回（remoteOK=true 时）
	remoteOK            bool                         // CurrentRemoteRepo 第二返回值
	remoteErr           error                        // CurrentRemoteRepo 失败臂
	goalGetResp         coreapi.GoalGetResponse      // Goals.Get 返回
	goalGetErr          error                        // Goals.Get 失败臂
	goalSetResp         coreapi.ThreadGoal           // Goals.Set 返回
	goalSetErr          error                        // Goals.Set 失败臂
	goalPauseResumeResp coreapi.ThreadGoal           // Goals.Pause/Resume 返回
	goalPauseErr        error                        // Goals.Pause 失败臂
	goalResumeErr       error                        // Goals.Resume 失败臂
	goalClearErr        error                        // Goals.Clear 失败臂
	modelCtxSnapshot    coreapi.ModelContextSnapshot // Models.Context 返回（零值回退 activeModel）
	modelCtxErr         error                        // Models.Context 失败臂
	selectModelErr      error                        // SelectModelForCurrentContext 失败臂
	resumeSessionErr    error                        // ResumeSession 失败臂（/resume）
	reloadSkillsErr     error                        // ReloadSkills 失败臂（/skills reload）

	// app_send / startup 批测注入口（零值=原行为）。
	toolExecResult   coreapi.ToolResult // Tools.Execute 返回
	toolExecErr      error              // Tools.Execute 失败臂
	predictText      string             // Insights.PredictNextUserMessage 返回
	predictErr       error              // Insights.Predict 失败臂
	modelsSaveErr    error              // Models.Save 失败臂（SwitchPlanModel）
	activateModelErr error              // Models.Activate 失败臂
	setWorkspaceErr  error              // Models.SetWorkspace 失败臂
	healNote         string             // HealCurrentSessionModel 提示文案（经 Sessions.Current 驱动）
	invokeSkillInvoked bool             // Extensions.InvokeSkill 返回 Invoked
	invokeSkillErr     error            // Extensions.InvokeSkill 失败臂
	modelCatalog       *coreapi.ModelCatalogState // Models.Catalog 覆盖（nil=默认）
	turnStartErr       error // Turns.Start 失败臂（Invoke 异步回包）
	// 面板刷新批测注入口（零值=原行为）。
	lspServers        []coreapi.LSPServer
	lspServersErr     error
	rulesSnapshot     coreapi.RulesSnapshot
	rulesSnapshotErr  error
	contextPreview    []string
	contextPreviewErr error
	contextStats      coreapi.ContextStats
	contextStatsErr   error
	costItems         []coreapi.CostItem
	costItemsErr      error
	memorySnapshot    coreapi.MemorySnapshot
	memorySnapshotErr error

	// slash_runtime 第四轮收尾注入口（零值=原行为）。
	workspaceAddErr     error // Workspaces.Add 失败臂（/workspace add）
	workspaceRemoveErr  error // Workspaces.Remove 失败臂（/workspace remove）
	setExecModeErr      error // Modes.SetExecutionMode 失败臂（/permissions auto）
	setSandboxModeErr   error // Modes.SetSandboxMode 失败臂（/permissions access）
	setAccessModeErr    error // Permissions.SetAccessMode 失败臂
	setApprovalErr      error // Permissions.SetApprovalMode 失败臂（/permissions approval）
	enterFullAccessErr  error // Permissions.EnterFullAccess 失败臂（danger 档）
	permissionSnapErr   error // Permissions.Snapshot 失败臂（/permissions 回显）
	skillsListErr       error // Extensions.ListSkills 失败臂（/skills）
	pluginsListErr      error // Extensions.ListPlugins 失败臂（/plugin）
	getSettingsErr      error // Config.GetSettings 失败臂（/theme /plan-style）
	saveSettingsErr     error // Config.SaveSettings 失败臂（/theme /plan-style）
	renameSessionErr    error // Sessions.Rename 失败臂（/rename）
	gitBranchesErr      error // Git.Branches 失败臂（/git branches）
	gitLogErr           error // Git.Log 失败臂（/git log）
	gitShowErr          error // Git.Show 失败臂（/git show）
	currentSessionEmpty bool  // Current 返回空 ID（/export /rename /share 无会话臂）
	windowTokens        int   // Context.WindowTokens 返回（/status 上下文窗口行）
	savedMessages       []coreapi.SessionMessage // SaveMessages 请求录制（/session save）
}

func (e *testEngine) Caller() coreapi.Caller {
	if e.caller != nil {
		return e.caller
	}
	return nil
}
func (e *testEngine) State() coreapi.StateService            { return &testStateServiceWithEngine{e: e} }
func (e *testEngine) Workspaces() coreapi.WorkspaceService   { return &testWorkspaceService{e: e} }
func (e *testEngine) Sessions() coreapi.SessionService       { return &testSessionService{e: e} }
func (e *testEngine) MCP() coreapi.MCPService                { return &testMCPService{e: e} }
func (e *testEngine) LSP() coreapi.LSPService                { return &testLSPService{e: e} }
func (e *testEngine) Config() coreapi.ConfigService          { return &testConfigService{e: e} }
func (e *testEngine) Permissions() coreapi.PermissionService { return &testPermissionService{e: e} }
func (e *testEngine) Extensions() coreapi.ExtensionService   { return &testExtensionService{e: e} }
func (e *testEngine) Context() coreapi.ContextService {
	return &testContextService{e: e}
}
func (e *testEngine) Usage() coreapi.UsageService {
	return &testUsageService{e: e}
}
func (e *testEngine) Versions() coreapi.VersionService { return &testVersionService{} }
func (e *testEngine) Tasks() coreapi.TaskService       { return &testTaskService{e: e} }
func (e *testEngine) Goals() coreapi.GoalService       { return &testGoalService{e: e} }
func (e *testEngine) Modes() coreapi.ModeService       { return &testModeService{e: e} }
func (e *testEngine) Models() coreapi.ModelService     { return &testModelService{e: e} }
func (e *testEngine) RemoteWorkspaces() coreapi.RemoteWorkspaceService {
	return &testRemoteWorkspaceService{e: e}
}
func (e *testEngine) Git() coreapi.GitService                 { return &testGitService{e: e} }
func (e *testEngine) Insights() coreapi.InsightService        { return &testInsightsService{e: e} }
func (e *testEngine) Memory() coreapi.MemoryService           { return &testMemoryService{e: e} }
func (e *testEngine) Roles() coreapi.RoleService              { return nil }
func (e *testEngine) Turns() coreapi.TurnService              { return &testTurnService{e: e} }
func (e *testEngine) Approvals() coreapi.ApprovalService      { return &testApprovalsService{} }
func (e *testEngine) Inquiries() coreapi.InquiryService       { return &testInquiryService{} }
func (e *testEngine) Agents() coreapi.AgentService            { return &testAgentsService{e: e} }
func (e *testEngine) Tools() coreapi.ToolExecutor             { return &testToolExecutor{e: e} }
func (e *testEngine) ToolCatalog() coreapi.ToolCatalogService { return nil }
func (e *testEngine) ToolTelemetry() coreapi.ToolTelemetryService {
	return &testToolTelemetryService{e: e}
}
func (e *testEngine) Events() coreapi.EventSubscriber         { return &testEventSubscriber{} }
func (e *testEngine) Sandbox() coreapi.SandboxService         { return &testSandboxService{} }
func (e *testEngine) Diagnostics() coreapi.DiagnosticsService { return &testDiagnosticsService{} }

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
	if s.e != nil && s.e.workspaceAddErr != nil {
		return s.e.workspaceAddErr
	}
	s.e.workspaceList = append(s.e.workspaceList, coreapi.Workspace{Path: req.Path, Active: true})
	return nil
}
func (s *testWorkspaceService) Remove(_ context.Context, req coreapi.WorkspacePathRequest) error {
	if s.e != nil && s.e.workspaceRemoveErr != nil {
		return s.e.workspaceRemoveErr
	}
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
	if s.e.resumeSessionErr != nil {
		return coreapi.Session{}, s.e.resumeSessionErr
	}
	s.e.resumeCalls = append(s.e.resumeCalls, req.SessionID)
	return coreapi.Session{ID: req.SessionID}, nil
}
func (s *testSessionService) List(context.Context, coreapi.ListSessionsRequest) ([]coreapi.Session, error) {
	if s.e != nil {
		return s.e.sessionList, s.e.listSessionsErr
	}
	return nil, nil
}
func (s *testSessionService) Current(context.Context, coreapi.CurrentSessionRequest) (coreapi.Session, error) {
	if s.e != nil && s.e.currentSessionEmpty {
		return coreapi.Session{ID: ""}, nil
	}
	id := strings.TrimSpace(s.e.currentSessionID)
	if id == "" {
		id = "test-session"
	}
	return coreapi.Session{ID: id, Metadata: s.e.currentSessionMeta}, nil
}
func (s *testSessionService) SetCurrent(context.Context, coreapi.SetCurrentSessionRequest) error {
	return nil
}
func (s *testSessionService) Delete(context.Context, coreapi.DeleteSessionRequest) error { return nil }
func (s *testSessionService) Rename(context.Context, coreapi.RenameSessionRequest) (coreapi.Session, error) {
	if s.e != nil && s.e.renameSessionErr != nil {
		return coreapi.Session{}, s.e.renameSessionErr
	}
	return coreapi.Session{}, nil
}
func (s *testSessionService) SetMeta(context.Context, coreapi.SetSessionMetaRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s *testSessionService) LoadMessages(context.Context, coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	if s.e.loadMessagesErr != nil {
		return nil, s.e.loadMessagesErr
	}
	if s.e.messages != nil {
		return s.e.messages, nil
	}
	return nil, nil
}
func (s *testSessionService) SaveMessages(_ context.Context, req coreapi.SaveSessionMessagesRequest) (coreapi.Session, error) {
	if s.e.saveSessionMsgsErr != nil {
		return coreapi.Session{}, s.e.saveSessionMsgsErr
	}
	if s.e != nil {
		s.e.savedMessages = req.Messages
	}
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
	if s.e != nil && s.e.rulesSnapshotErr != nil {
		return coreapi.RulesSnapshot{}, s.e.rulesSnapshotErr
	}
	if s.e != nil {
		return s.e.rulesSnapshot, nil
	}
	return coreapi.RulesSnapshot{}, nil
}
func (s *testConfigService) SaveRules(context.Context, coreapi.SaveRulesRequest) error {
	if s.e != nil && s.e.saveRulesErr != nil {
		return s.e.saveRulesErr
	}
	return nil
}
func (s *testConfigService) ResetRules(context.Context) error { return nil }
func (s *testConfigService) GetSettings(context.Context) (coreapi.Settings, error) {
	if s.e != nil && s.e.getSettingsErr != nil {
		return coreapi.Settings{}, s.e.getSettingsErr
	}
	return s.e.settings, nil
}
func (s *testConfigService) SaveSettings(_ context.Context, settings coreapi.Settings) error {
	if s.e != nil && s.e.saveSettingsErr != nil {
		return s.e.saveSettingsErr
	}
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
	if s.e != nil && s.e.permissionSnapErr != nil {
		return coreapi.PermissionSnapshot{}, s.e.permissionSnapErr
	}
	return s.e.permissionSnap, nil
}
func (s *testPermissionService) PendingReview(context.Context) (coreapi.PendingReview, error) {
	if s.e != nil {
		return s.e.pendingReview, nil
	}
	return coreapi.PendingReview{}, nil
}
func (s *testPermissionService) ClearPendingReview(context.Context) error { return nil }
func (s *testPermissionService) SetAccessMode(_ context.Context, req coreapi.SetModeRequest) error {
	if s.e != nil && s.e.setAccessModeErr != nil {
		return s.e.setAccessModeErr
	}
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
	if s.e != nil && s.e.setApprovalErr != nil {
		return s.e.setApprovalErr
	}
	s.e.permissionSnap.ApprovalMode = req.Mode
	return nil
}
func (s *testPermissionService) EnterFullAccess(_ context.Context, req coreapi.EnterFullAccessRequest) error {
	if s.e != nil && s.e.enterFullAccessErr != nil {
		return s.e.enterFullAccessErr
	}
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
	if s.e.modelCatalog != nil {
		return *s.e.modelCatalog, nil
	}
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
func (s *testModelService) Save(context.Context, coreapi.ModelSaveRequest) error {
	return s.e.modelsSaveErr
}
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
	if s.e.activateModelErr != nil {
		return s.e.activateModelErr
	}
	s.e.activeModel = req.Name
	return nil
}
func (s *testModelService) SyncEnv(context.Context) error { return nil }
func (s *testModelService) Context(context.Context, coreapi.ModelContextRequest) (coreapi.ModelContextSnapshot, error) {
	if s.e.modelCtxErr != nil {
		return coreapi.ModelContextSnapshot{}, s.e.modelCtxErr
	}
	if s.e.modelCtxSnapshot.ResolvedModelName != "" || s.e.modelCtxSnapshot.GlobalDefaultName != "" ||
		s.e.modelCtxSnapshot.WorkspaceModelName != "" {
		return s.e.modelCtxSnapshot, nil
	}
	return coreapi.ModelContextSnapshot{
		ResolvedModelName: s.e.activeModel,
		ResolvedScope:     "global",
	}, nil
}
func (s *testModelService) SetWorkspace(_ context.Context, req coreapi.SetWorkspaceModelRequest) error {
	if s.e.setWorkspaceErr != nil {
		return s.e.setWorkspaceErr
	}
	s.e.activeModel = req.ModelName
	return nil
}
func (s *testModelService) ClearWorkspace(context.Context, coreapi.ClearWorkspaceModelRequest) error {
	return nil
}
func (s *testModelService) SetSession(_ context.Context, req coreapi.SetSessionModelRequest) error {
	if s.e.selectModelErr != nil {
		return s.e.selectModelErr
	}
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
	if s.e != nil && s.e.setExecModeErr != nil {
		return s.e.setExecModeErr
	}
	return nil
}

// SetSandboxMode 模拟内核 runtime/sandbox_mode/set：同步 mode 快照与
// permission_snapshot.sandbox_mode（内核行为见 eos-core-runtime 的
// runtime_sandbox_mode_sync 回归测试）。
func (s *testModeService) SetSandboxMode(_ context.Context, req coreapi.SetModeRequest) error {
	if s.e != nil && s.e.setSandboxModeErr != nil {
		return s.e.setSandboxModeErr
	}
	s.e.permissionSnap.SandboxMode = req.Mode
	return nil
}
func (s *testModeService) SetReasoningLevel(context.Context, coreapi.SetModeRequest) error {
	return nil
}

// === State ===
// testStateServiceWithEngine 共享 engine 状态以回填 ForegroundWorkspace。
// /init 等按工作区根写文件的路径必须指向 t.TempDir()，避免污染测试进程 cwd。
type testStateServiceWithEngine struct{ e *testEngine }

func (s *testStateServiceWithEngine) Snapshot(context.Context, coreapi.StateSnapshotRequest) (coreapi.StateSnapshot, error) {
	return coreapi.StateSnapshot{ForegroundWorkspace: s.e.foregroundWS}, nil
}

// === Usage ===
// 补全 Usage 空实现，让依赖 refreshCostPanel/UsageSummary 的路径（如启动期 resume）
// 在测试里不再 nil 解引用。零值无副作用，不改变既有测试行为。
type testUsageService struct{ e *testEngine }

func (s *testUsageService) Summary(context.Context) (coreapi.UsageSummary, error) {
	if s.e != nil {
		return s.e.usageSummary, s.e.usageSummaryErr
	}
	return coreapi.UsageSummary{}, nil
}
func (s *testUsageService) CostSummary(context.Context) (string, error) { return "", nil }
func (s *testUsageService) CostItems(context.Context) ([]coreapi.CostItem, error) {
	if s.e != nil && s.e.costItemsErr != nil {
		return nil, s.e.costItemsErr
	}
	if s.e != nil {
		return s.e.costItems, nil
	}
	return nil, nil
}

// === Context ===
type testContextService struct{ e *testEngine }

func (s *testContextService) Preview(context.Context) ([]string, error) {
	if s.e != nil && s.e.contextPreviewErr != nil {
		return nil, s.e.contextPreviewErr
	}
	if s.e != nil {
		return s.e.contextPreview, nil
	}
	return nil, nil
}
func (s *testContextService) Stats(context.Context) (coreapi.ContextStats, error) {
	if s.e != nil && s.e.contextStatsErr != nil {
		return coreapi.ContextStats{}, s.e.contextStatsErr
	}
	if s.e != nil {
		return s.e.contextStats, nil
	}
	return coreapi.ContextStats{}, nil
}
func (s *testContextService) WindowTokens(context.Context) (int, error) {
	if s.e != nil {
		return s.e.windowTokens, nil
	}
	return 0, nil
}
func (s *testContextService) PinDocument(context.Context, coreapi.PinDocumentRequest) error {
	return nil
}
func (s *testContextService) Compact(context.Context) (string, error) {
	if s.e != nil {
		if s.e.compactErr != nil {
			return "", s.e.compactErr
		}
		return s.e.compactMessage, nil
	}
	return "", nil
}
func (s *testContextService) Clear(context.Context) error {
	if s.e != nil && s.e.clearCtxErr != nil {
		return s.e.clearCtxErr
	}
	return nil
}
func (s *testContextService) Export(context.Context, coreapi.ExportContextRequest) error {
	if s.e != nil && s.e.exportCtxErr != nil {
		return s.e.exportCtxErr
	}
	return nil
}

// === RemoteWorkspace ===
type testRemoteWorkspaceService struct{ e *testEngine }

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
	if s.e != nil {
		return s.e.remoteRepo, s.e.remoteOK, s.e.remoteErr
	}
	return coreapi.RemoteRepoState{}, false, nil
}

// === Extension ===
type testExtensionService struct{ e *testEngine }

func (s *testExtensionService) ListSkills(context.Context) ([]coreapi.SkillInfo, error) {
	if s.e != nil {
		if s.e.skillsListErr != nil {
			return nil, s.e.skillsListErr
		}
		return s.e.skills, nil
	}
	return nil, nil
}
func (s *testExtensionService) ReloadSkills(context.Context) error {
	if s.e != nil {
		return s.e.reloadSkillsErr
	}
	return nil
}
func (s *testExtensionService) SetSkillEnabled(context.Context, coreapi.SetExtensionEnabledRequest) error {
	return nil
}
func (s *testExtensionService) InvokeSkill(context.Context, coreapi.InvokeSkillRequest) (coreapi.InvokeSkillResult, error) {
	if s.e != nil && s.e.invokeSkillErr != nil {
		return coreapi.InvokeSkillResult{}, s.e.invokeSkillErr
	}
	if s.e != nil {
		return coreapi.InvokeSkillResult{Invoked: s.e.invokeSkillInvoked}, nil
	}
	return coreapi.InvokeSkillResult{}, nil
}
func (s *testExtensionService) ListPlugins(context.Context) ([]coreapi.PluginInfo, error) {
	if s.e != nil {
		if s.e.pluginsListErr != nil {
			return nil, s.e.pluginsListErr
		}
		return s.e.plugins, nil
	}
	return nil, nil
}
func (s *testExtensionService) SetPluginEnabled(context.Context, coreapi.SetExtensionEnabledRequest) error {
	return nil
}
func (s *testExtensionService) BrowserStatus(context.Context) (coreapi.BrowserRuntimeStatus, error) {
	if s.e != nil {
		return s.e.browserStatus, s.e.browserStatusErr
	}
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
type testMCPService struct{ e *testEngine }

func (s *testMCPService) List(context.Context) ([]coreapi.MCPServer, error) {
	return s.e.mcpList, nil
}
func (s *testMCPService) Upsert(context.Context, coreapi.UpsertMCPRequest) error { return nil }
func (s *testMCPService) ImportJSON(context.Context, coreapi.ImportMCPJSONRequest) error {
	return nil
}
func (s *testMCPService) Delete(context.Context, coreapi.MCPNameRequest) error { return nil }
func (s *testMCPService) SetEnabled(context.Context, coreapi.SetMCPEnabledRequest) error {
	return nil
}

// === Task ===
type testTaskService struct{ e *testEngine }

func (s *testTaskService) List(context.Context) ([]coreapi.TaskSnapshot, error) {
	if s.e != nil {
		return s.e.taskList, nil
	}
	return nil, nil
}
func (s *testTaskService) Todos(context.Context) ([]coreapi.TodoItem, error) {
	if s.e != nil {
		return s.e.todoList, nil
	}
	return nil, nil
}
func (s *testTaskService) Tail(context.Context, coreapi.TaskIDRequest) ([]string, error) {
	return nil, nil
}
func (s *testTaskService) Kill(context.Context, coreapi.TaskIDRequest) error { return nil }
func (s *testTaskService) Cleanup(context.Context) (int, error)              { return 0, nil }

// testGoalService 是 UI 测试用的 goal service 假实现（不触网）。
type testGoalService struct{ e *testEngine }

func (s *testGoalService) Set(_ context.Context, _ coreapi.GoalSetRequest) (coreapi.ThreadGoal, error) {
	if s.e != nil {
		return s.e.goalSetResp, s.e.goalSetErr
	}
	return coreapi.ThreadGoal{}, nil
}

func (s *testGoalService) Get(_ context.Context, _ coreapi.GoalRefRequest) (coreapi.GoalGetResponse, error) {
	if s.e != nil {
		return s.e.goalGetResp, s.e.goalGetErr
	}
	return coreapi.GoalGetResponse{}, nil
}

func (s *testGoalService) Pause(_ context.Context, _ coreapi.GoalRefRequest) (coreapi.ThreadGoal, error) {
	if s.e != nil {
		return s.e.goalPauseResumeResp, s.e.goalPauseErr
	}
	return coreapi.ThreadGoal{}, nil
}

func (s *testGoalService) Resume(_ context.Context, _ coreapi.GoalRefRequest) (coreapi.ThreadGoal, error) {
	if s.e != nil {
		return s.e.goalPauseResumeResp, s.e.goalResumeErr
	}
	return coreapi.ThreadGoal{}, nil
}

func (s *testGoalService) Clear(_ context.Context, _ coreapi.GoalRefRequest) error {
	if s.e != nil {
		return s.e.goalClearErr
	}
	return nil
}

// testApprovalsService / testInquiryService / testToolTelemetryService：
// UI 测试用假实现，避免 engine.X() 返回 nil 后 adapter 解引用 panic。
// testTurnService：Invoke 异步 Start 注入点，零值返回通用错误避免 nil panic。
type testTurnService struct{ e *testEngine }

func (s *testTurnService) Start(context.Context, coreapi.StartTurnRequest) (coreapi.Turn, error) {
	if s.e != nil && s.e.turnStartErr != nil {
		return coreapi.Turn{}, s.e.turnStartErr
	}
	return coreapi.Turn{}, errors.New("turn start not configured in tests")
}
func (s *testTurnService) Interrupt(context.Context, coreapi.TurnRef) error { return nil }
func (s *testTurnService) Resume(context.Context, coreapi.TurnRef) (coreapi.Turn, error) {
	return coreapi.Turn{}, errors.New("turn resume not configured in tests")
}

type testApprovalsService struct{}

func (s *testApprovalsService) Respond(context.Context, coreapi.ApprovalResponse) error { return nil }

type testInquiryService struct{}

func (s *testInquiryService) Respond(context.Context, coreapi.InquiryResponse) error { return nil }

type testToolTelemetryService struct{ e *testEngine }

func (s *testToolTelemetryService) Traces(context.Context) ([]coreapi.ToolTrace, error) {
	if s.e != nil {
		return s.e.toolTraces, s.e.toolTracesErr
	}
	return nil, nil
}
func (s *testToolTelemetryService) Stats(context.Context) ([]coreapi.ToolStat, error) {
	if s.e != nil {
		return s.e.toolStats, s.e.toolStatsErr
	}
	return nil, nil
}

// testGitService：UI 测试用假 git service，结果可经 testEngine 注入。
type testGitService struct{ e *testEngine }

func (s *testGitService) Status(context.Context, coreapi.GitStatusRequest) ([]coreapi.GitChange, error) {
	if s.e != nil && s.e.gitStatusErr != nil {
		return nil, s.e.gitStatusErr
	}
	if s.e != nil {
		return s.e.gitStatus, nil
	}
	return nil, nil
}
func (s *testGitService) Summary(context.Context, coreapi.GitSummaryRequest) (coreapi.GitSummaryResult, error) {
	return coreapi.GitSummaryResult{Branch: "main"}, nil
}
func (s *testGitService) Diff(context.Context, coreapi.GitDiffRequest) (coreapi.GitTextResult, error) {
	if s.e != nil && s.e.gitDiffErr != nil {
		return coreapi.GitTextResult{}, s.e.gitDiffErr
	}
	if s.e != nil {
		return s.e.gitDiff, nil
	}
	return coreapi.GitTextResult{}, nil
}
func (s *testGitService) Branches(context.Context, coreapi.GitBranchesRequest) (coreapi.GitBranchesResult, error) {
	if s.e != nil {
		if s.e.gitBranchesErr != nil {
			return coreapi.GitBranchesResult{}, s.e.gitBranchesErr
		}
		return s.e.gitBranches, nil
	}
	return coreapi.GitBranchesResult{}, nil
}
func (s *testGitService) Log(context.Context, coreapi.GitLogRequest) (coreapi.GitLogResult, error) {
	if s.e != nil {
		if s.e.gitLogErr != nil {
			return coreapi.GitLogResult{}, s.e.gitLogErr
		}
		return s.e.gitLog, nil
	}
	return coreapi.GitLogResult{}, nil
}
func (s *testGitService) Show(context.Context, coreapi.GitShowRequest) (coreapi.GitShowResult, error) {
	if s.e != nil {
		if s.e.gitShowErr != nil {
			return coreapi.GitShowResult{}, s.e.gitShowErr
		}
		return s.e.gitShow, nil
	}
	return coreapi.GitShowResult{}, nil
}

// testMemoryService：UI 测试用假 memory service。
type testMemoryService struct{ e *testEngine }

func (s *testMemoryService) Snapshot(context.Context) (coreapi.MemorySnapshot, error) {
	if s.e != nil && s.e.memorySnapshotErr != nil {
		return coreapi.MemorySnapshot{}, s.e.memorySnapshotErr
	}
	if s.e != nil {
		return s.e.memorySnapshot, nil
	}
	return coreapi.MemorySnapshot{}, nil
}
func (s *testMemoryService) Save(context.Context, coreapi.SaveMemoryRequest) error {
	if s.e != nil && s.e.saveMemoryErr != nil {
		return s.e.saveMemoryErr
	}
	return nil
}
func (s *testMemoryService) RebuildIndex(context.Context) error { return nil }
func (s *testMemoryService) RecordAdd(context.Context, coreapi.AddMemoryRecordRequest) (coreapi.MemoryRecord, error) {
	return coreapi.MemoryRecord{}, nil
}
func (s *testMemoryService) RecordList(context.Context, coreapi.ListMemoryRecordsRequest) ([]coreapi.MemoryRecord, error) {
	return nil, nil
}
func (s *testMemoryService) RecordSearch(context.Context, coreapi.SearchMemoryRecordsRequest) ([]coreapi.MemoryRecord, error) {
	return nil, nil
}
func (s *testMemoryService) RecordDelete(context.Context, coreapi.DeleteMemoryRecordRequest) error {
	return nil
}

// testLSPService / testVersionService：UI 测试用假实现。
type testLSPService struct{ e *testEngine }

func (s *testLSPService) List(context.Context) ([]coreapi.LSPServer, error) {
	if s.e != nil && s.e.lspServersErr != nil {
		return nil, s.e.lspServersErr
	}
	if s.e != nil {
		return s.e.lspServers, nil
	}
	return nil, nil
}
func (s *testLSPService) Detect(context.Context, coreapi.LSPLanguageRequest) (string, error) {
	return "go", nil
}
func (s *testLSPService) Start(context.Context, coreapi.LSPLanguageRequest) (string, error) {
	return "", nil
}
func (s *testLSPService) Install(context.Context, coreapi.LSPLanguageRequest) (string, error) {
	return "", nil
}
func (s *testLSPService) Diagnostics(context.Context) ([]string, error) {
	if s.e != nil {
		return s.e.lspDiagnostics, nil
	}
	return nil, nil
}
func (s *testLSPService) DiagnosticsSummary(context.Context) (coreapi.LSPDiagnosticsSummary, error) {
	return coreapi.LSPDiagnosticsSummary{}, nil
}

type testVersionService struct{}

func (s *testVersionService) List(context.Context) ([]coreapi.VersionItem, error) { return nil, nil }
func (s *testVersionService) Rollback(context.Context, coreapi.VersionIDRequest) error {
	return nil
}
func (s *testVersionService) Delete(context.Context, coreapi.VersionIDRequest) error { return nil }
func (s *testVersionService) DeleteFile(context.Context, coreapi.VersionFileRequest) (int, error) {
	return 0, nil
}
func (s *testVersionService) Clear(context.Context) (int, error) { return 0, nil }

// testAgentsService：UI 测试用假 agent service。
type testAgentsService struct{ e *testEngine }

func (s *testAgentsService) Spawn(context.Context, coreapi.SpawnAgentRequest) (coreapi.Agent, error) {
	return coreapi.Agent{ID: "a1", Status: "running"}, nil
}
func (s *testAgentsService) SendInput(context.Context, coreapi.AgentInput) error { return nil }
func (s *testAgentsService) Wait(context.Context, coreapi.AgentRef) (coreapi.Agent, error) {
	return coreapi.Agent{ID: "a1", Status: "done"}, nil
}
func (s *testAgentsService) Run(context.Context, coreapi.RunAgentRequest) (coreapi.AgentRunResult, error) {
	return coreapi.AgentRunResult{}, nil
}
func (s *testAgentsService) RunTool(context.Context, coreapi.AgentToolRequest) (coreapi.AgentToolResult, error) {
	return coreapi.AgentToolResult{}, nil
}
func (s *testAgentsService) List(context.Context, coreapi.ListAgentsRequest) ([]coreapi.Agent, error) {
	if s.e != nil {
		return s.e.agentList, nil
	}
	return nil, nil
}
func (s *testAgentsService) Close(context.Context, coreapi.AgentRef) error { return nil }

// testToolExecutor：UI 测试用假工具执行器。
type testToolExecutor struct{ e *testEngine }

func (s *testToolExecutor) Execute(context.Context, coreapi.ToolRequest) (coreapi.ToolResult, error) {
	if s.e != nil && s.e.toolExecErr != nil {
		return coreapi.ToolResult{}, s.e.toolExecErr
	}
	if s.e != nil && (s.e.toolExecResult.Status != "" || len(s.e.toolExecResult.Output) > 0 || s.e.toolExecResult.Error != "" || s.e.toolExecResult.Display != "") {
		return s.e.toolExecResult, nil
	}
	return coreapi.ToolResult{Status: "success"}, nil
}

// testInsightsService：UI 测试用假 insight service。
type testInsightsService struct{ e *testEngine }

func (s *testInsightsService) PredictNextUserMessage(context.Context, coreapi.PredictNextUserMessageRequest) (string, error) {
	if s.e != nil && s.e.predictErr != nil {
		return "", s.e.predictErr
	}
	if s.e != nil && s.e.predictText != "" {
		return s.e.predictText, nil
	}
	return "predicted text", nil
}
func (s *testInsightsService) RefineInput(context.Context, coreapi.RefineInputRequest) (string, error) {
	return "", nil
}
func (s *testInsightsService) PlanSnapshot(context.Context) (coreapi.PlanSnapshot, error) {
	return coreapi.PlanSnapshot{}, nil
}
