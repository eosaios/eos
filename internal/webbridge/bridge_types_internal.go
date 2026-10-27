package webbridge

import (
	"context"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// Bridge 运行期内部状态类型（非前端 DTO，不跨进程传输）。
// 仅在 Go 侧用于维护会话、对话流、提示、bootstrap 局部装配与工作区持久化。

type sessionState struct {
	ID             string
	Title          string
	WorkspacePath  string
	Messages       []ChatMessage
	Running        bool
	Persisted      bool
	NeedsAttention bool
	UpdatedAt      time.Time
}

type runningConversationState struct {
	AssistantMessageID string
	WorkspacePath      string
	Cancel             context.CancelFunc
	TurnID             string
	Interrupt          func(context.Context) error
	Cancelled          bool
}

type conversationStreamHandle struct {
	Events    <-chan adapter.Event
	TurnID    string
	SessionID string
	Interrupt func(context.Context) error
}

type promptState struct {
	PromptCard
	AssistantMessageID string
	Source             string
	Input              string
}

type localBootstrapSession struct {
	Session       *sessionState
	PromptCount   int
	ActiveSession string
	WorkspacePath string
}

type workspacePersistenceState struct {
	LastWorkspace string `json:"last_workspace"`
	LastSession   string `json:"last_session"`
}

// SessionCreateNotify controls whether creating a workspace session pushes a
// user-visible notification. Replaces the previous bool parameter, which read
// ambiguously at call sites (createWorkspaceSessionLocked(ws, title, true)).
type SessionCreateNotify string

const (
	// SessionCreateNotifyUser pushes a "Session Created" notification.
	SessionCreateNotifyUser SessionCreateNotify = "notify"
	// SessionCreateSilent creates the session without disturbing the user
	// (used during bootstrap / restore).
	SessionCreateSilent SessionCreateNotify = "silent"
)

// BootstrapLoadScope selects whether LoadBootstrap hydrates only the immediate
// state or also the deferred snapshots (capability lists, usage, diagnostics).
// Replaces the previous includeDeferred bool.
type BootstrapLoadScope string

const (
	// BootstrapLoadImmediate skips deferred hydration.
	BootstrapLoadImmediate BootstrapLoadScope = "immediate"
	// BootstrapLoadIncludeDeferred hydrates capability/usage/diagnostic snapshots.
	BootstrapLoadIncludeDeferred BootstrapLoadScope = "include-deferred"
)

// WorkspaceActivation expresses whether a workspace is brought to the
// foreground (active, persisted as last) or kept in the background. Replaces
// the previous foreground bool on remember/ensure/resolve helpers.
type WorkspaceActivation string

const (
	// WorkspaceActivationForeground activates and persists the workspace as last-used.
	WorkspaceActivationForeground WorkspaceActivation = "foreground"
	// WorkspaceActivationBackground records the workspace without promoting it.
	WorkspaceActivationBackground WorkspaceActivation = "background"
)

// RuleDocumentScope selects whether rule document reads are limited to the
// active rule set or include inactive ones. Replaces the previous active bool.
type RuleDocumentScope string

const (
	// RuleDocumentScopeActiveOnly reads only the active rule document.
	RuleDocumentScopeActiveOnly RuleDocumentScope = "active-only"
	// RuleDocumentScopeAll reads regardless of active state.
	RuleDocumentScopeAll RuleDocumentScope = "all"
)
