package settings

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// Permissions defines tool permission rules
type Permissions struct {
	AllowedTools []string         `json:"allowed_tools,omitempty"`
	DeniedTools  []string         `json:"denied_tools,omitempty"`
	Rules        []PermissionRule `json:"rules,omitempty"` // Pattern-based permission rules
}

// PermissionRule defines a glob-pattern-based permission rule
type PermissionRule struct {
	Pattern  string `json:"pattern"`  // Glob pattern matching tool name (e.g., "bash:*rm*", "edit:*")
	Decision string `json:"decision"` // "allow" | "deny" | "ask"
}

// Settings 表示用户配置
type Settings struct {
	PlanPromptStyle      string   `json:"plan_prompt_style"`
	PlanBubbleColor      string   `json:"plan_bubble"`
	AutoContext          bool     `json:"auto_context"`
	DesktopNotifications *bool    `json:"desktop_notifications,omitempty"`
	GitCommitReminder    *bool    `json:"git_commit_reminder,omitempty"`
	MaxInjectKB          int      `json:"max_inject_kb"`
	WatchMode            string   `json:"watch_mode"`
	WatchDebounceMs      int      `json:"watch_debounce_ms"`
	PollIntervalSec      int      `json:"poll_interval_sec"`
	Language             string   `json:"language"`
	Workspaces           []string `json:"workspaces"`
	ActiveWorkspace      string   `json:"active_workspace"`
	Theme                string   `json:"theme"`
	Trusted              bool     `json:"trusted"`
	TrustedAt            string   `json:"trusted_at,omitempty"`

	// Token budget configuration
	MaxTurnTokens    int `json:"max_turn_tokens,omitempty"`
	MaxSessionTokens int `json:"max_session_tokens,omitempty"`

	// Auto mode classifier custom rules
	AutoRules []AutoRule `json:"auto_rules,omitempty"`

	// Tool permissions
	Permissions *Permissions `json:"permissions,omitempty"`
}

// AutoRule defines a custom classification rule for auto mode
type AutoRule struct {
	Pattern     string `json:"pattern"`     // regex or glob pattern
	Action      string `json:"action"`      // "allow", "deny", "ask"
	Category    string `json:"category"`    // tool/command category
	Description string `json:"description"` // human-readable description
}
