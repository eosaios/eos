package webbridge

// bridge_types.go 是 Bridge 层 DTO 的顶层入口，只承载启动/重连时下发给前端的
// 顶层聚合 DTO（BootstrapState）及其专属的 Plan / Memory 快照。
//
// 其余 DTO 按领域拆分到 bridge_types_*.go：
//   - bridge_types_rules.go        Rules 域
//   - bridge_types_workspace.go    Workspace / Worktree / RemoteRepo / Migration
//   - bridge_types_automation.go   Automation 域
//   - bridge_types_model.go        Model 域
//   - bridge_types_conversation.go Session / Message / Attachment / Prompt / ChangeSet / Rollback / Task
//   - bridge_types_system.go       Notification / Command / Diagnostics / Clipboard / Window / FileDialog / Export / Resource
//   - bridge_types_probe.go        Probe / Heartbeat
//   - bridge_types_internal.go     运行期内部状态类型（非 DTO）
//
// 所有 JSON 字段语义与前端契约一致，本文件只做类型归属拆分。

// bootstrapRuntimeShell 标识桌面壳层技术栈，下发到 BootstrapState.Runtime，
// 由前端环境信息面板只读展示（workbench-settings.tsx）。不参与任何分支判断，
// 仅作展示，所以改值不会破坏前端逻辑。
const bootstrapRuntimeShell = "wails-v3-solid-ts-shell"

// BootstrapState 是启动/重连时由 Bridge 层一次性下发给前端的顶层聚合 DTO，
// 聚合各领域卡片。字段语义必须与前端契约保持一致。
type BootstrapState struct {
	Runtime             string                   `json:"runtime"`
	BridgeMode          string                   `json:"bridgeMode"`
	ExecutionMode       string                   `json:"executionMode"`
	ReasoningLevel      string                   `json:"reasoningLevel"`
	ConfigPath          string                   `json:"configPath"`
	ActiveWorkspace     string                   `json:"activeWorkspace"`
	WorkspaceCount      int                      `json:"workspaceCount"`
	ModelCount          int                      `json:"modelCount"`
	HasConfiguredModel  bool                     `json:"hasConfiguredModel"`
	TaskCount           int                      `json:"taskCount"`
	CurrentSessionID    string                   `json:"currentSessionId"`
	Workspaces          []WorkspaceCard          `json:"workspaces"`
	RemoteWorkspaces    []WorkspaceCard          `json:"remoteWorkspaces"`
	CurrentRemoteRepo   *RemoteRepoState         `json:"currentRemoteRepo,omitempty"`
	Worktrees           []WorktreeCard           `json:"worktrees"`
	Models              []ModelCard              `json:"models"`
	ModelContext        ModelContextSnapshot     `json:"modelContext"`
	ModelCatalog        ModelCatalogState        `json:"modelCatalog"`
	MCPServers          []MCPServerCard          `json:"mcpServers"`
	LSPServers          []LSPServerCard          `json:"lspServers"`
	Skills              []SkillCard              `json:"skills"`
	Plugins             []PluginCard             `json:"plugins"`
	UsageSummary        UsageSummaryCard         `json:"usageSummary"`
	CostItems           []CostItemCard           `json:"costItems"`
	Versions            []VersionCard            `json:"versions"`
	Permission          PermissionState          `json:"permission"`
	Settings            GUISettingsState         `json:"settings"`
	RulesState          RulesState               `json:"rulesState"`
	Rules               string                   `json:"rules"`
	Bash                BashState                `json:"bash"`
	Terminal            TerminalState            `json:"terminal"`
	Tasks               []TaskCard               `json:"tasks"`
	Sessions            []SessionCard            `json:"sessions"`
	Messages            []ChatMessage            `json:"messages"`
	Prompts             []PromptCard             `json:"prompts"`
	Notifications       []NotificationItem       `json:"notifications"`
	AutomationTemplates []AutomationTemplateCard `json:"automationTemplates"`
	AutomationRuns      []AutomationRunCard      `json:"automationRuns"`
	CommandPalette      []CommandAction          `json:"commandPalette"`
	Diagnostics         DiagnosticsState         `json:"diagnostics"`
	Clipboard           ClipboardState           `json:"clipboard"`
	Window              WindowSnapshot           `json:"window"`
	InputSuggestions    []string                 `json:"inputSuggestions"`
	MigrationBoundaries []MigrationBoundary      `json:"migrationBoundaries"`
	ResourceChecks      []ResourceCheck          `json:"resourceChecks"`
	AppVersion          string                   `json:"appVersion"`
	UpdateInfo          *UpdateCheckResult       `json:"updateInfo"`
	UpdateInstall       UpdateInstallState       `json:"updateInstall"`
	Plan                PlanSnapshot             `json:"plan"`
	Goal                GoalSnapshot             `json:"goal"`
	Memory              MemorySnapshot           `json:"memory"`
	// GitBranch 是当前工作区所在 git 仓库的分支（非 git 工作区为空，前端隐藏）。
	GitBranch string `json:"gitBranch"`
	UpdatedAt string `json:"updatedAt"`
}

type PlanSnapshot struct {
	HasPlan          bool   `json:"hasPlan"`
	Content          string `json:"content"`
	WorkspaceCurrent string `json:"workspaceCurrent"`
	UserLatest       string `json:"userLatest"`
	UserSnapshot     string `json:"userSnapshot"`
	UpdatedAt        string `json:"updatedAt"`
}

type MemoryDocument struct {
	Scope     string `json:"scope"`
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	Content   string `json:"content"`
	Summary   string `json:"summary"`
	UpdatedAt string `json:"updatedAt"`
}

type MemorySnapshot struct {
	Documents []MemoryDocument `json:"documents"`
}
