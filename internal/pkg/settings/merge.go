package settings

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// Layer represents the origin of a settings layer
type Layer string

const (
	LayerManaged      Layer = "managed"       // Highest priority (admin/org policy)
	LayerProject      Layer = "project"       // .eos/settings.json (shared, committable)
	LayerProjectLocal Layer = "project_local" // .eos/settings.local.json (gitignored)
	LayerUser         Layer = "user"          // ~/.eos.json (lowest priority)
)

// LoadMerged loads and merges settings from all layers.
// Priority: managed > project > project_local > user
func LoadMerged(userPath, projectRoot string) *Settings {
	user := loadLayer(userPath)
	projectLocal := loadLayer(filepath.Join(projectRoot, ".eos", "settings.local.json"))
	project := loadLayer(filepath.Join(projectRoot, ".eos", "settings.json"))
	managed := loadLayer(filepath.Join(projectRoot, ".eos", "settings.managed.json"))

	// Merge: start with lowest priority, overlay higher
	result := user
	mergeInto(&result, projectLocal)
	mergeInto(&result, project)
	mergeInto(&result, managed)

	return &result
}

// loadLayer loads a single settings file, returning defaults if not found
func loadLayer(path string) Settings {
	var s Settings
	if path == "" {
		return s
	}
	bs, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(bs, &s); err != nil {
		slog.Debug("settings.load_layer.error", "path", path, "error", err)
		return s
	}
	return s
}

// mergeInto overlays src onto dst. Non-zero values in src override dst.
// Slices are concatenated (union), not replaced.
func mergeInto(dst *Settings, src Settings) {
	if src.PlanPromptStyle != "" {
		dst.PlanPromptStyle = src.PlanPromptStyle
	}
	if src.PlanBubbleColor != "" {
		dst.PlanBubbleColor = src.PlanBubbleColor
	}
	if src.AutoContext {
		dst.AutoContext = src.AutoContext
	}
	if src.DesktopNotifications != nil {
		dst.DesktopNotifications = src.DesktopNotifications
	}
	if src.GitCommitReminder != nil {
		dst.GitCommitReminder = src.GitCommitReminder
	}
	if src.MaxInjectKB > 0 {
		dst.MaxInjectKB = src.MaxInjectKB
	}
	if src.WatchMode != "" {
		dst.WatchMode = src.WatchMode
	}
	if src.WatchDebounceMs > 0 {
		dst.WatchDebounceMs = src.WatchDebounceMs
	}
	if src.PollIntervalSec > 0 {
		dst.PollIntervalSec = src.PollIntervalSec
	}
	if src.Language != "" {
		dst.Language = src.Language
	}
	if src.ActiveWorkspace != "" {
		dst.ActiveWorkspace = src.ActiveWorkspace
	}
	if src.Theme != "" {
		dst.Theme = src.Theme
	}
	if src.Trusted {
		dst.Trusted = src.Trusted
	}
	if src.TrustedAt != "" {
		dst.TrustedAt = src.TrustedAt
	}
	if src.MaxTurnTokens > 0 {
		dst.MaxTurnTokens = src.MaxTurnTokens
	}
	if src.MaxSessionTokens > 0 {
		dst.MaxSessionTokens = src.MaxSessionTokens
	}

	// Merge workspaces (union)
	if len(src.Workspaces) > 0 {
		existing := make(map[string]bool)
		for _, w := range dst.Workspaces {
			existing[w] = true
		}
		for _, w := range src.Workspaces {
			if !existing[w] {
				dst.Workspaces = append(dst.Workspaces, w)
			}
		}
	}

	// Merge auto rules (union by pattern)
	if len(src.AutoRules) > 0 {
		existingPatterns := make(map[string]bool)
		for _, r := range dst.AutoRules {
			existingPatterns[r.Pattern] = true
		}
		for _, r := range src.AutoRules {
			if !existingPatterns[r.Pattern] {
				dst.AutoRules = append(dst.AutoRules, r)
			}
		}
	}

	// Merge permissions (deep merge with rules)
	if src.Permissions != nil {
		if dst.Permissions == nil {
			dst.Permissions = &Permissions{}
		}
		if len(src.Permissions.AllowedTools) > 0 {
			dst.Permissions.AllowedTools = src.Permissions.AllowedTools
		}
		if len(src.Permissions.DeniedTools) > 0 {
			dst.Permissions.DeniedTools = src.Permissions.DeniedTools
		}
		if len(src.Permissions.Rules) > 0 {
			dst.Permissions.Rules = mergePermissionRules(dst.Permissions.Rules, src.Permissions.Rules)
		}
	}
}

// mergePermissionRules merges two slices of permission rules.
// Later rules override earlier rules with the same pattern.
func mergePermissionRules(base, overlay []PermissionRule) []PermissionRule {
	patternIndex := make(map[string]int)
	for i, r := range base {
		patternIndex[r.Pattern] = i
	}
	result := make([]PermissionRule, len(base))
	copy(result, base)
	for _, r := range overlay {
		if idx, exists := patternIndex[r.Pattern]; exists {
			result[idx] = r // override existing rule
		} else {
			result = append(result, r)
		}
	}
	return result
}
