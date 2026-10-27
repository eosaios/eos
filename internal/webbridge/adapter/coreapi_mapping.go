//go:build !legacy

package adapter

import (
	"math"
	"strings"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

func runtimeSnapshotFromCoreAPI(snapshot coreapi.StateSnapshot) RuntimeSnapshot {
	out := RuntimeSnapshot{
		ForegroundWorkspace: snapshot.ForegroundWorkspace,
		Workspaces:          make([]WorkspaceSnapshot, 0, len(snapshot.Workspaces)),
		Sessions:            make([]SessionSnapshot, 0, len(snapshot.Sessions)),
		Messages:            make([]SessionMessage, 0, len(snapshot.Messages)),
		Tasks:               make([]BackgroundTask, 0, len(snapshot.Tasks)),
		Agents:              make([]AgentSnapshot, 0, len(snapshot.Agents)),
	}
	for _, workspace := range snapshot.Workspaces {
		out.Workspaces = append(out.Workspaces, WorkspaceSnapshot{
			Path:             workspace.Path,
			Name:             workspace.Name,
			Trusted:          workspace.Trusted,
			Active:           workspace.Active,
			SessionCount:     workspace.SessionCount,
			CurrentSessionID: workspace.CurrentSessionID,
		})
	}
	for _, session := range snapshot.Sessions {
		out.Sessions = append(out.Sessions, sessionSnapshotFromCoreAPI(session))
	}
	if snapshot.CurrentSession != nil {
		current := sessionSnapshotFromCoreAPI(*snapshot.CurrentSession)
		out.CurrentSession = &current
	}
	for _, msg := range snapshot.Messages {
		out.Messages = append(out.Messages, SessionMessage{
			Role:       msg.Role,
			Type:       msg.Type,
			Content:    msg.Content,
			Time:       msg.Time,
			ImagePaths: append([]string(nil), msg.ImagePaths...),
			Metadata:   cloneSessionMessageMetadata(msg.Metadata),
			ChangeSet:  msg.ChangeSet,
			Rollback:   msg.Rollback,
		})
	}
	for _, task := range snapshot.Tasks {
		out.Tasks = append(out.Tasks, BackgroundTask{
			ID:        task.ID,
			Status:    task.Status,
			StartedAt: task.StartedAt,
			Label:     task.Label,
			CanKill:   task.CanKill,
			Workspace: task.Workspace,
		})
	}
	for _, agent := range snapshot.Agents {
		out.Agents = append(out.Agents, AgentSnapshot{
			ID:            strings.TrimSpace(agent.ID),
			ParentAgentID: strings.TrimSpace(agent.ParentAgentID),
			RoleID:        strings.TrimSpace(agent.RoleID),
			Task:          strings.TrimSpace(agent.Task),
			Status:        strings.TrimSpace(agent.Status),
			CreatedAt:     agent.CreatedAt,
			UpdatedAt:     agent.UpdatedAt,
		})
	}
	return out
}

func sessionSnapshotFromCoreAPI(session coreapi.SessionSnapshot) SessionSnapshot {
	return SessionSnapshot{
		ID:             session.ID,
		WorkspacePath:  session.WorkspacePath,
		Title:          session.Title,
		Preview:        session.Preview,
		Source:         session.Source,
		Archived:       session.Archived,
		UpdatedAt:      session.UpdatedAt,
		Running:        session.Running,
		NeedsAttention: session.NeedsAttention,
		MessageCount:   session.MessageCount,
		PendingPrompts: session.PendingPrompts,
		Active:         session.Active,
	}
}

func workspacesFromCoreAPI(items []coreapi.Workspace) []Workspace {
	out := make([]Workspace, 0, len(items))
	for _, item := range items {
		out = append(out, Workspace{Path: strings.TrimSpace(item.Path), Trusted: item.Trusted, Active: item.Active})
	}
	return out
}

func worktreesFromCoreAPI(items []coreapi.Worktree) []Worktree {
	out := make([]Worktree, 0, len(items))
	for _, item := range items {
		out = append(out, worktreeFromCoreAPI(item))
	}
	return out
}

func worktreeFromCoreAPI(item coreapi.Worktree) Worktree {
	return Worktree{
		Name:      strings.TrimSpace(item.Name),
		Path:      strings.TrimSpace(item.Path),
		Branch:    strings.TrimSpace(item.Branch),
		Head:      strings.TrimSpace(item.Head),
		Active:    item.Active,
		Removable: item.Removable,
	}
}

func mcpServersFromCoreAPI(items []coreapi.MCPServer) []MCPServer {
	out := make([]MCPServer, 0, len(items))
	for _, item := range items {
		out = append(out, MCPServer{
			Name:    strings.TrimSpace(item.Name),
			Type:    strings.TrimSpace(item.Type),
			Target:  strings.TrimSpace(item.Target),
			Enabled: item.Enabled,
		})
	}
	return out
}

func lspServersFromCoreAPI(items []coreapi.LSPServer) []LSPServer {
	out := make([]LSPServer, 0, len(items))
	for _, item := range items {
		out = append(out, LSPServer{
			Language: strings.TrimSpace(item.Language),
			Status:   strings.TrimSpace(item.Status),
			Command:  strings.TrimSpace(item.Command),
		})
	}
	return out
}

func guiSettingsFromCoreAPI(settings coreapi.Settings) GUISettings {
	return GUISettings{
		Language:       strings.TrimSpace(settings.Language),
		Theme:          strings.TrimSpace(settings.Theme),
		MidRiskConfirm: settings.MidRiskConfirm,
	}
}

func permissionSnapshotFromCoreAPI(snapshot coreapi.PermissionSnapshot) PermissionSnapshot {
	return PermissionSnapshot{
		ExecutionMode: normalizeExecutionMode(snapshot.ExecutionMode),
		SandboxMode:   normalizeSandboxMode(snapshot.SandboxMode),
		// 内核 ApprovalMode serde 只输出标准 kebab-case，读取侧无需别名映射。
		ApprovalMode:      strings.TrimSpace(snapshot.ApprovalMode),
		AllowAll:          snapshot.AllowAll,
		AllowedCategories: append([]string(nil), snapshot.AllowedCategories...),
		HasPendingDiff:    snapshot.HasPendingDiff,
		PendingDiffPath:   strings.TrimSpace(snapshot.PendingDiffPath),
	}
}

func skillInfosFromCoreAPI(items []coreapi.SkillInfo) []SkillInfo {
	out := make([]SkillInfo, 0, len(items))
	for _, item := range items {
		out = append(out, SkillInfo{
			Name:                   strings.TrimSpace(item.Name),
			Description:            strings.TrimSpace(item.Description),
			Source:                 strings.TrimSpace(item.Source),
			ArgumentHint:           strings.TrimSpace(item.ArgumentHint),
			Location:               strings.TrimSpace(item.Location),
			BaseDir:                strings.TrimSpace(item.BaseDir),
			AllowedTools:           append([]string(nil), item.AllowedTools...),
			Enabled:                item.Enabled,
			Active:                 item.Active,
			DisableModelInvocation: item.DisableModelInvocation,
			UserInvocable:          item.UserInvocable,
			UserInvocableDefined:   item.UserInvocableDefined,
		})
	}
	return out
}

func pluginInfosFromCoreAPI(items []coreapi.PluginInfo) []PluginInfo {
	out := make([]PluginInfo, 0, len(items))
	for _, item := range items {
		out = append(out, PluginInfo{
			Name:        strings.TrimSpace(item.Name),
			Description: strings.TrimSpace(item.Description),
			Source:      strings.TrimSpace(item.Source),
			Command:     strings.TrimSpace(item.Command),
			Enabled:     item.Enabled,
		})
	}
	return out
}

func contextStatsFromCoreAPI(stats coreapi.ContextStats) ContextStats {
	return ContextStats{MessageCount: stats.MessageCount, Estimated: stats.Estimated}
}

func usageSummaryFromCoreAPI(summary coreapi.UsageSummary) UsageSummary {
	return UsageSummary{
		Rounds:             summary.Rounds,
		InputTokens:        cloneIntPtr(summary.InputTokens),
		ReplyTokens:        cloneIntPtr(summary.ReplyTokens),
		CachedInputTokens:  cloneIntPtr(summary.CachedInputTokens),
		TotalTokens:        cloneIntPtr(summary.TotalTokens),
		CostUSD:            cloneFloatPtr(summary.CostUSD),
		UnknownUsageRounds: summary.UnknownUsageRounds,
		UnknownCostRounds:  summary.UnknownCostRounds,
	}
}

func costItemsFromCoreAPI(items []coreapi.CostItem) []CostItem {
	out := make([]CostItem, 0, len(items))
	for _, item := range items {
		out = append(out, CostItem{
			Time:               item.Time,
			Model:              strings.TrimSpace(item.Model),
			InputTokens:        cloneIntPtr(item.InputTokens),
			ReplyTokens:        cloneIntPtr(item.ReplyTokens),
			CachedInputTokens:  cloneIntPtr(item.CachedInputTokens),
			TotalTokens:        cloneIntPtr(item.TotalTokens),
			ContextInputTokens: cloneIntPtr(item.ContextInputTokens),
			CostUSD:            cloneFloatPtr(item.CostUSD),
			UsageKnown:         item.UsageKnown,
			CostKnown:          item.CostKnown,
		})
	}
	return out
}

func versionItemsFromCoreAPI(items []coreapi.VersionItem) []VersionItem {
	out := make([]VersionItem, 0, len(items))
	for _, item := range items {
		out = append(out, VersionItem{
			ID:        strings.TrimSpace(item.ID),
			File:      strings.TrimSpace(item.File),
			CreatedAt: item.CreatedAt,
			Summary:   strings.TrimSpace(item.Summary),
		})
	}
	return out
}

// BackgroundTasksFromCoreAPI 把内核 task/list 快照映射为壳层的后台任务。
// 导出：bridge 的任务监听协程直接拉 task/list 增量刷新任务页。
func BackgroundTasksFromCoreAPI(items []coreapi.TaskSnapshot) []BackgroundTask {
	out := make([]BackgroundTask, 0, len(items))
	for _, item := range items {
		out = append(out, BackgroundTask{
			ID:        strings.TrimSpace(item.ID),
			Status:    strings.TrimSpace(item.Status),
			StartedAt: item.StartedAt,
			Label:     strings.TrimSpace(item.Label),
			CanKill:   item.CanKill,
			Workspace: strings.TrimSpace(item.Workspace),
		})
	}
	return out
}

func modelConfigsFromCoreAPI(items []coreapi.ModelConfig) []ModelConfig {
	out := make([]ModelConfig, 0, len(items))
	for _, item := range items {
		out = append(out, ModelConfig{
			Name:                    strings.TrimSpace(item.Name),
			APIBase:                 strings.TrimSpace(item.APIBase),
			APIKeyMasked:            strings.TrimSpace(item.APIKeyMasked),
			Model:                   strings.TrimSpace(item.Model),
			Source:                  strings.TrimSpace(item.Source),
			Active:                  item.Active,
			SupportsReasoningEffort: item.SupportsReasoningEffort,
			ReasoningLevels:         append([]string(nil), item.ReasoningLevels...),
			SupportsVision:          item.SupportsVision,
			SupportsTools:           item.SupportsTools,
			ProviderID:              strings.TrimSpace(item.ProviderID),
			Format:                  strings.TrimSpace(item.Format),
			PresetID:                strings.TrimSpace(item.PresetID),
			ContextWindow:           item.ContextWindow,
			EditKind:                strings.TrimSpace(item.EditKind),
			CanEdit:                 item.CanEdit,
			CanDelete:               item.CanDelete,
		})
	}
	return out
}

func modelCatalogFromCoreAPI(catalog coreapi.ModelCatalogState) ModelCatalogState {
	out := ModelCatalogState{
		Providers:           make([]ModelProviderOption, 0, len(catalog.Providers)),
		Presets:             make([]ModelPresetOption, 0, len(catalog.Presets)),
		AllowCustomProvider: catalog.AllowCustomProvider,
		AllowCustomModel:    catalog.AllowCustomModel,
	}
	for _, provider := range catalog.Providers {
		endpoints := make([]ProviderEndpoint, 0, len(provider.Endpoints))
		for _, ep := range provider.Endpoints {
			endpoints = append(endpoints, ProviderEndpoint{
				Plan:    strings.TrimSpace(ep.Plan),
				Format:  strings.TrimSpace(ep.Format),
				APIBase: strings.TrimSpace(ep.APIBase),
			})
		}
		out.Providers = append(out.Providers, ModelProviderOption{
			ID:            strings.TrimSpace(provider.ID),
			Name:          strings.TrimSpace(provider.Name),
			Website:       strings.TrimSpace(provider.Website),
			APIKeyEnv:     strings.TrimSpace(provider.APIKeyEnv),
			Endpoints:     endpoints,
			DefaultModels: append([]string(nil), provider.DefaultModels...),
		})
	}
	for _, preset := range catalog.Presets {
		planModels := make([]PlanModel, 0, len(preset.PlanModels))
		for _, pm := range preset.PlanModels {
			planModels = append(planModels, PlanModel{
				ModelID:                 strings.TrimSpace(pm.ModelID),
				Label:                   strings.TrimSpace(pm.Label),
				ContextWindow:           pm.ContextWindow,
				SupportsReasoningEffort: pm.SupportsReasoningEffort,
				SupportsVision:          pm.SupportsVision,
				SupportsTools:           pm.SupportsTools,
				ReasoningLevels:         append([]string(nil), pm.ReasoningLevels...),
			})
		}
		out.Presets = append(out.Presets, ModelPresetOption{
			ID:                      strings.TrimSpace(preset.ID),
			Name:                    strings.TrimSpace(preset.Name),
			ProviderID:              strings.TrimSpace(preset.ProviderID),
			ModelName:               strings.TrimSpace(preset.ModelName),
			Plan:                    strings.TrimSpace(preset.Plan),
			Format:                  strings.TrimSpace(preset.Format),
			ContextWindow:           preset.ContextWindow,
			Tags:                    append([]string(nil), preset.Tags...),
			Description:             strings.TrimSpace(preset.Description),
			SupportsReasoningEffort: preset.SupportsReasoningEffort,
			ReasoningLevels:         append([]string(nil), preset.ReasoningLevels...),
			SupportsVision:          preset.SupportsVision,
			SupportsTools:           preset.SupportsTools,
			PlanModels:              planModels,
		})
	}
	return out
}

func remoteWorkspacesFromCoreAPI(items []coreapi.RemoteWorkspace) []RemoteWorkspace {
	out := make([]RemoteWorkspace, 0, len(items))
	for _, item := range items {
		out = append(out, remoteWorkspaceFromCoreAPI(item))
	}
	return out
}

func remoteWorkspaceFromCoreAPI(item coreapi.RemoteWorkspace) RemoteWorkspace {
	return RemoteWorkspace{
		ID:            strings.TrimSpace(item.ID),
		Kind:          strings.TrimSpace(item.Kind),
		Platform:      strings.TrimSpace(item.Platform),
		RepoURL:       strings.TrimSpace(item.RepoURL),
		Owner:         strings.TrimSpace(item.Owner),
		Repo:          strings.TrimSpace(item.Repo),
		DefaultBranch: strings.TrimSpace(item.DefaultBranch),
		Branch:        strings.TrimSpace(item.Branch),
		Account:       strings.TrimSpace(item.Account),
		LocalPath:     strings.TrimSpace(item.LocalPath),
		Active:        item.Active,
		Exists:        item.Exists,
		LastUsedAt:    item.LastUsedAt,
	}
}

func remoteRepoStateFromCoreAPI(state coreapi.RemoteRepoState) RemoteRepoState {
	return RemoteRepoState{
		Mode:          strings.TrimSpace(state.Mode),
		Platform:      strings.TrimSpace(state.Platform),
		RepoURL:       strings.TrimSpace(state.RepoURL),
		Owner:         strings.TrimSpace(state.Owner),
		Repo:          strings.TrimSpace(state.Repo),
		DefaultBranch: strings.TrimSpace(state.DefaultBranch),
		WorkingBranch: strings.TrimSpace(state.WorkingBranch),
		LocalPath:     strings.TrimSpace(state.LocalPath),
		AccountLogin:  strings.TrimSpace(state.AccountLogin),
		AccountName:   strings.TrimSpace(state.AccountName),
		UpdatedAt:     state.UpdatedAt,
	}
}

func planSnapshotFromCoreAPI(snapshot coreapi.PlanSnapshot) PlanSnapshot {
	return PlanSnapshot{
		HasPlan:          snapshot.HasPlan,
		Content:          strings.TrimSpace(snapshot.Content),
		WorkspaceCurrent: strings.TrimSpace(snapshot.WorkspaceCurrent),
		UserLatest:       strings.TrimSpace(snapshot.UserLatest),
		UserSnapshot:     strings.TrimSpace(snapshot.UserSnapshot),
		UpdatedAt:        snapshot.UpdatedAt,
	}
}

func memorySnapshotFromCoreAPI(snapshot coreapi.MemorySnapshot) MemorySnapshot {
	out := MemorySnapshot{Documents: make([]MemoryDocument, 0, len(snapshot.Documents))}
	for _, doc := range snapshot.Documents {
		out.Documents = append(out.Documents, MemoryDocument{
			Scope:     strings.TrimSpace(doc.Scope),
			Path:      strings.TrimSpace(doc.Path),
			Exists:    doc.Exists,
			Content:   doc.Content,
			Summary:   strings.TrimSpace(doc.Summary),
			UpdatedAt: doc.UpdatedAt,
		})
	}
	return out
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloatPtr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func coreAPISessionMessages(items []SessionMessage) []coreapi.SessionMessage {
	out := make([]coreapi.SessionMessage, 0, len(items))
	for _, item := range items {
		out = append(out, coreapi.SessionMessage{
			Role:       item.Role,
			Type:       item.Type,
			Content:    item.Content,
			Time:       item.Time,
			ImagePaths: append([]string(nil), item.ImagePaths...),
			Metadata:   cloneSessionMessageMetadata(item.Metadata),
			ChangeSet:  item.ChangeSet,
			Rollback:   item.Rollback,
		})
	}
	return out
}

func sessionMessagesFromCoreAPI(items []coreapi.SessionMessage) []SessionMessage {
	out := make([]SessionMessage, 0, len(items))
	for _, item := range items {
		out = append(out, SessionMessage{
			Role:       item.Role,
			Type:       item.Type,
			Content:    item.Content,
			Time:       item.Time,
			ImagePaths: append([]string(nil), item.ImagePaths...),
			Metadata:   cloneSessionMessageMetadata(item.Metadata),
			ChangeSet:  item.ChangeSet,
			Rollback:   item.Rollback,
		})
	}
	return out
}

func sessionMetasFromCoreAPI(items []coreapi.Session) []SessionMeta {
	out := make([]SessionMeta, 0, len(items))
	for _, item := range items {
		out = append(out, sessionMetaFromCoreAPI(item))
	}
	return out
}

func sessionMetaFromCoreAPI(item coreapi.Session) SessionMeta {
	meta := SessionMeta{
		ID:          strings.TrimSpace(item.ID),
		SavedAt:     firstNonZeroCoreAPITime(item.UpdatedAt, item.CreatedAt),
		Model:       coreAPIMetadataString(item.Metadata, "model"),
		Summary:     coreAPIMetadataString(item.Metadata, "summary"),
		Preview:     coreAPIMetadataString(item.Metadata, "preview"),
		Title:       coreAPIMetadataString(item.Metadata, "title"),
		Rounds:      coreAPIMetadataInt(item.Metadata, "rounds"),
		Tokens:      coreAPIMetadataInt(item.Metadata, "tokens"),
		SandboxMode: normalizeSessionSandboxMode(coreAPIMetadataString(item.Metadata, "sandbox_mode")),
	}
	if meta.Title == "" {
		meta.Title = meta.ID
	}
	return meta
}

// normalizeSessionSandboxMode 会话 metadata 读取侧归一化：空值保持空（表示未
// 记录，由调用方默认逻辑接管），非空则折叠到内核 kebab-case 规范词表
// （历史 workspace/full_access 持久化值兼容）。
func normalizeSessionSandboxMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return ""
	}
	return normalizeSandboxMode(mode)
}

func firstNonZeroCoreAPITime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func coreAPIMetadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	switch value := metadata[key].(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func coreAPIMetadataInt(metadata map[string]any, key string) int {
	if len(metadata) == 0 {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0
		}
		return int(value)
	case interface{ Int64() (int64, error) }:
		n, _ := value.Int64()
		return int(n)
	default:
		return 0
	}
}
