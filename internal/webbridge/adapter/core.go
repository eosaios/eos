package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"strings"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

type Event struct {
	Type          string
	EventType     string
	Version       string
	EventID       string
	RequestID     string
	SessionID     string
	ThreadID      string
	TurnID        string
	AgentID       string
	CorrelationID string
	Source        string
	Message       string
	Data          map[string]any
	Payload       map[string]any
}

type BackgroundTask struct {
	ID        string
	Status    string
	StartedAt time.Time
	Label     string
	CanKill   bool
	Logs      []string
	Workspace string
}

// Format 协议格式常量（对齐核心 FormatKind，封闭三值）。
const (
	FormatOpenaiChat      = "openai_chat"
	FormatOpenaiResponses = "openai_responses"
	FormatAnthropic       = "anthropic"
)

// ProviderEndpoint 服务商一个接入端点 = (plan, format) 组合。
type ProviderEndpoint struct {
	Plan    string
	Format  string
	APIBase string
}

// PlanModel 套餐类 preset 内可选的模型项。
type PlanModel struct {
	ModelID       string
	Label         string
	ContextWindow int64
	// 模型级能力标注（nil = 未标注，回落 preset 级）。
	SupportsReasoningEffort *bool
	SupportsVision          *bool
	SupportsTools           *bool
	// 模型级思考档位（nil = 未标注，回落 preset 级；wire 档位词汇见内核 protocol）。
	ReasoningLevels []string
}

type ModelConfig struct {
	Name                    string
	APIBase                 string
	APIKeyMasked            string
	Model                   string
	Source                  string
	Active                  bool
	SupportsReasoningEffort bool
	// 思考档位（空 = 未标注，前端回落通用四档）。
	ReasoningLevels []string
	SupportsVision  bool
	SupportsTools   bool
	ProviderID      string
	Format          string
	PresetID        string
	ContextWindow   int64
	EditKind        string
	CanEdit         bool
	CanDelete       bool
}

type ModelProviderOption struct {
	ID            string
	Name          string
	Website       string
	APIKeyEnv     string
	Endpoints     []ProviderEndpoint
	DefaultModels []string
}

type ModelPresetOption struct {
	ID                      string
	Name                    string
	ProviderID              string
	ModelName               string
	Plan                    string
	Format                  string
	ContextWindow           int
	Tags                    []string
	Description             string
	SupportsReasoningEffort bool
	// 思考档位（空 = 不支持思考强度；wire 档位词汇见内核 protocol）。
	ReasoningLevels []string
	SupportsVision  bool
	SupportsTools   bool
	PlanModels      []PlanModel
}

type ModelCatalogState struct {
	Providers           []ModelProviderOption
	Presets             []ModelPresetOption
	AllowCustomProvider bool
	AllowCustomModel    bool
}

type ModelSaveRequest struct {
	OriginalName            string
	Mode                    string
	ProviderID              string
	PresetID                string
	Name                    string
	APIKey                  string
	APIBase                 string
	Model                   string
	SupportsReasoningEffort *bool
	SupportsVision          *bool
	SupportsTools           *bool
}

type ModelContextRequest struct {
	WorkspaceRoot string
	SessionID     string
}

type ModelContextSnapshot struct {
	WorkspaceRoot      string
	SessionID          string
	GlobalDefaultName  string
	WorkspaceModelName string
	SessionModelName   string
	ResolvedModelName  string
	ResolvedScope      string
}

type Workspace struct {
	Path    string
	Trusted bool
	Active  bool
}

type Worktree struct {
	Name      string
	Path      string
	Branch    string
	Head      string
	Active    bool
	Removable bool
}

type WorkspaceSnapshot struct {
	Path             string
	Name             string
	Trusted          bool
	Active           bool
	SessionCount     int
	CurrentSessionID string
}

type SessionMeta struct {
	ID          string
	SavedAt     time.Time
	Model       string
	Summary     string
	Preview     string
	Title       string
	Rounds      int
	Tokens      int
	SandboxMode string
}

type SessionSnapshot struct {
	ID             string
	WorkspacePath  string
	Title          string
	Preview        string
	Source         string
	Archived       bool
	UpdatedAt      time.Time
	Running        bool
	NeedsAttention bool
	MessageCount   int
	PendingPrompts int
	Active         bool
}

type SessionMessage struct {
	Role       string
	Type       string
	Content    string
	Time       time.Time
	ImagePaths []string
	Metadata   map[string]any            `json:"metadata,omitempty"`
	ChangeSet  *coreapi.MessageChangeSet `json:"-"`
	Rollback   *coreapi.TurnRollback     `json:"-"`
}

type RuntimeSnapshot struct {
	ForegroundWorkspace string
	Workspaces          []WorkspaceSnapshot
	Sessions            []SessionSnapshot
	CurrentSession      *SessionSnapshot
	Messages            []SessionMessage
	Tasks               []BackgroundTask
	Agents              []AgentSnapshot
}

type AgentSnapshot struct {
	ID            string
	ParentAgentID string
	RoleID        string
	Task          string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type MCPServer struct {
	Name    string
	Type    string
	Target  string
	Enabled bool
}

type LSPServer struct {
	Language string
	Status   string
	Command  string
}

type PermissionSnapshot struct {
	ExecutionMode     string
	SandboxMode       string
	ApprovalMode      string
	AllowAll          bool
	AllowedCategories []string
	HasPendingDiff    bool
	PendingDiffPath   string
}

type PendingReview struct {
	Path    string
	Diff    string
	HasDiff bool
}

type SkillInfo struct {
	Name                   string
	Description            string
	Source                 string
	ArgumentHint           string
	Location               string
	BaseDir                string
	AllowedTools           []string
	Enabled                bool
	Active                 bool
	DisableModelInvocation bool
	UserInvocable          bool
	UserInvocableDefined   bool
}

type PluginInfo struct {
	Name        string
	Description string
	Source      string
	Command     string
	Enabled     bool
}

type ContextStats struct {
	MessageCount int
	Estimated    int
}

type CostItem struct {
	Time              time.Time
	Model             string
	InputTokens       *int
	ReplyTokens       *int
	CachedInputTokens *int
	TotalTokens       *int
	// ContextInputTokens 是该 turn 最近一次请求的真实 prompt tokens ≈ 上下文规模
	// （InputTokens 是各步累加的计费口径，不能当上下文占用展示）。
	ContextInputTokens *int
	CostUSD            *float64
	UsageKnown         bool
	CostKnown          bool
}

type UsageSummary struct {
	Rounds             int
	InputTokens        *int
	ReplyTokens        *int
	CachedInputTokens  *int
	TotalTokens        *int
	CostUSD            *float64
	UnknownUsageRounds int
	UnknownCostRounds  int
}

type VersionItem struct {
	ID        string
	File      string
	CreatedAt time.Time
	Summary   string
}

type StartupDiagnosticsResult struct {
	BinaryPath      string `json:"binary_path"`
	ManifestVersion string `json:"manifest_version"`
	ProtocolVersion string `json:"protocol_version"`
	StoreDir        string `json:"store_dir"`
	SandboxBackend  string `json:"sandbox_backend"`
	MigrationMarker string `json:"migration_marker"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
}

type GUISettings struct {
	Language       string
	Theme          string
	MidRiskConfirm bool
}

type Attachment struct {
	Name string
	Path string
	MIME string
	Kind string
}

type RemoteWorkspace struct {
	ID            string
	Kind          string
	Platform      string
	RepoURL       string
	Owner         string
	Repo          string
	DefaultBranch string
	Branch        string
	Account       string
	LocalPath     string
	Active        bool
	Exists        bool
	LastUsedAt    time.Time
}

type RemoteRepoState struct {
	Mode          string
	Platform      string
	RepoURL       string
	Owner         string
	Repo          string
	DefaultBranch string
	WorkingBranch string
	LocalPath     string
	AccountLogin  string
	AccountName   string
	UpdatedAt     time.Time
}

type PlanSnapshot struct {
	HasPlan          bool
	Content          string
	WorkspaceCurrent string
	UserLatest       string
	UserSnapshot     string
	UpdatedAt        time.Time
}

type MemoryDocument struct {
	Scope     string
	Path      string
	Exists    bool
	Content   string
	Summary   string
	UpdatedAt time.Time
}

type MemorySnapshot struct {
	Documents []MemoryDocument
}

type Core interface {
	Invoke(context.Context, string) (<-chan Event, error)
	InvokeWithImages(context.Context, string, []string) (<-chan Event, error)
	InvokeWithAttachments(context.Context, string, []Attachment) (<-chan Event, error)
	RunBash(context.Context, string) (<-chan Event, error)
	SetExecutionMode(string)
	ExecutionMode() string
	SetSandboxMode(string)
	SandboxMode() string
	SetReasoningLevel(string) error
	ReasoningLevel() string
	ResolveConfirmation(string, bool)
	ListTasks() []BackgroundTask
	TailTask(string) ([]string, error)
	KillTask(string) error
	CleanupTasks() int
	CoreConfigPath() string
	ListModels() []ModelConfig
	UpsertModel(string, string, string, string) error
	ModelCatalog() ModelCatalogState
	SaveModel(ModelSaveRequest) error
	DeleteModel(string) error
	ActivateModel(string) error
	ListWorkspaces() []Workspace
	DefaultWorkspacePath() string
	LastWorkspace() string
	ResolveForegroundWorkspace(string) (string, error)
	RememberWorkspace(string, bool) error
	ForgetWorkspace(string) error
	AddWorkspace(string) error
	RemoveWorkspace(string) error
	UseWorkspace(string) error
	SetForegroundWorkspace(string) error
	TrustWorkspace(string) error
	ListWorktrees() []Worktree
	CreateWorktree(string) (Worktree, error)
	RemoveWorktree(string, bool) error
	ListSessions() []SessionMeta
	ListWorkspaceSessions(string) ([]SessionMeta, error)
	CreateWorkspaceSession(string, string, []SessionMessage) (SessionMeta, error)
	SaveSession(string) (string, error)
	SaveWorkspaceSessionMessages(string, string, []SessionMessage) (string, error)
	SaveSessionMessages(string, []SessionMessage) (string, error)
	LoadWorkspaceSessionMessages(string, string) ([]SessionMessage, error)
	LoadSessionMessages(string) ([]SessionMessage, error)
	GetWorkspaceCurrentSession(string) (string, error)
	CurrentSessionID() (string, error)
	SetWorkspaceCurrentSession(string, string) error
	SetCurrentSession(string) error
	UpdateWorkspaceSessionTitle(string, string, string) error
	UpdateSessionTitle(string, string) error
	ResumeWorkspaceSession(string, string) error
	ResumeSession(string) error
	DeleteWorkspaceSession(string, string) error
	DeleteSession(string) error
	ResolveSessionWorkspace(string) (string, error)
	RuntimeSnapshot() RuntimeSnapshot
	ListMCP() []MCPServer
	UpsertMCP(string, string, string, bool) error
	ImportMCPJSON(string) error
	DeleteMCP(string) error
	SetMCPEnabled(string, bool) error
	ListLSP() []LSPServer
	DetectLSP(string) string
	StartLSP(string) string
	InstallLSP(string) string
	LSPDiagnostics() []string
	PermissionSnapshot() PermissionSnapshot
	PendingReview() PendingReview
	ClearPendingReview()
	ListSkills() []SkillInfo
	ReloadSkills() error
	SetSkillEnabled(string, bool) error
	ListPlugins() []PluginInfo
	SetPluginEnabled(string, bool) error
	ContextPreview() []string
	ContextStats() ContextStats
	CompactContext() string
	ClearContext()
	ExportContext(string) error
	CostSummary() string
	UsageSummary() UsageSummary
	CostItems() []CostItem
	GetRules() string
	SaveRules(string)
	ResetRules()
	GetSettings() GUISettings
	SaveSettings(GUISettings)
	ListVersions() []VersionItem
	RollbackVersion(string) error
	DeleteVersion(string) error
	DeleteFileVersions(string) int
	ClearVersions() int
	InitEOS() error
	StartupDiagnostics() StartupDiagnosticsResult
}

type ThreadHandleProvider interface {
	ThreadCore(string) Core
	ReleaseThreadCore(string)
}

type savedSession struct {
	meta      SessionMeta
	messages  []SessionMessage
	workspace string
}

// ─── 辅助函数（被 coreapi_mapping / stdio_rpc 等活跃代码使用）────────────────

func normalizeExecutionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "plan", "计划优先", "先出计划":
		return "plan"
	default:
		return "auto"
	}
}

// normalizeSandboxMode 与 app.NormalizeSandboxMode 同一张别名表：规范词表是内核
// SandboxMode 的 kebab-case 三值，历史 GUI 值（workspace/full_access）仅作读取别名。
func normalizeSandboxMode(mode string) string {
	key := strings.ToLower(strings.TrimSpace(mode))
	key = strings.ReplaceAll(key, "_", "-")
	switch key {
	case "read-only", "readonly", "ro":
		return "read-only"
	case "danger-full-access", "dangerfullaccess", "full-access", "fullaccess", "full", "danger", "allow-all", "完全访问", "完全访问权限":
		return "danger-full-access"
	case "workspace-write", "workspacewrite", "workspace", "ww":
		return "workspace-write"
	default:
		return "workspace-write"
	}
}

// 注：approval_mode 的别名归一化已删除——内核 ApprovalMode::parse 是单一真相源
// （eos-core-tools/src/lib.rs），壳层读取侧只 trim 空白，写入侧直接透传，让内核
// 拒绝未知值（yolo/always-allow 等）。见 AGENTS.md §3 + 核心原则 1。

func cloneSessionMessageMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = cloneSessionMessageMetadataValue(value)
	}
	return out
}

func cloneSessionMessageMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSessionMessageMetadata(typed)
	case []any:
		out := make([]any, len(typed))
		for index := range typed {
			out[index] = cloneSessionMessageMetadataValue(typed[index])
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for index := range typed {
			out[index] = cloneSessionMessageMetadata(typed[index])
		}
		return out
	case []string:
		return append([]string{}, typed...)
	case []int:
		return append([]int{}, typed...)
	case []int64:
		return append([]int64{}, typed...)
	case []float64:
		return append([]float64{}, typed...)
	case []bool:
		return append([]bool{}, typed...)
	default:
		return value
	}
}
