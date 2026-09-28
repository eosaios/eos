package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"testing"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/pkg/settings"
	"github.com/eosaios/eos/pkg/coreapi"
)

func settingsZero() settings.Settings              { return settings.Settings{} }
func modelSaveReqZero() coreapi.ModelSaveRequest   { return coreapi.ModelSaveRequest{} }
func toolReqZero() coreapi.ToolRequest             { return coreapi.ToolRequest{} }
func browserLaunchReqZero() coreapi.BrowserLaunchRequest {
	return coreapi.BrowserLaunchRequest{}
}
func browserCloseReqZero() coreapi.BrowserCloseRequest           { return coreapi.BrowserCloseRequest{} }
func takeoverReqZero() coreapi.BrowserControlTakeoverRequest    { return coreapi.BrowserControlTakeoverRequest{} }
func gitLogReqZero() coreapi.GitLogRequest                       { return coreapi.GitLogRequest{} }
func gitShowReqZero() coreapi.GitShowRequest                     { return coreapi.GitShowRequest{} }
func mcpEntryZero() config.MCPEntry                              { return config.MCPEntry{} }

// TestCoreClientAdapterNotAvailableGuards 覆盖全部方法的「引擎不可用」
// 防御分支：nil adapter 与未注入 engine 的 adapter 一律返回错误/零值，
// 绝不 panic。
func TestCoreClientAdapterNotAvailableGuards(t *testing.T) {
	var nilAdapter *CoreClientAdapter
	noEngine := NewCoreClientAdapterFromEngine(nil)
	ctx := context.Background()

	type errCase struct {
		name string
		call func() error
	}
	errCases := []errCase{
		{"Invoke", func() error { _, err := nilAdapter.Invoke(ctx, "q", "auto", nil, false); return err }},
		{"ExecuteBash", func() error { _, err := nilAdapter.ExecuteBash(ctx, "ls"); return err }},
		{"StateSnapshot", func() error { _, err := nilAdapter.StateSnapshot(ctx); return err }},
		{"Workspaces", func() error { _, err := nilAdapter.Workspaces(ctx); return err }},
		{"AddWorkspace", func() error { return nilAdapter.AddWorkspace(ctx, "/w") }},
		{"RemoveWorkspace", func() error { return nilAdapter.RemoveWorkspace(ctx, "/w") }},
		{"UseWorkspace", func() error { return nilAdapter.UseWorkspace(ctx, "/w") }},
		{"TrustWorkspace", func() error { return nilAdapter.TrustWorkspace(ctx, "/w") }},
		{"Settings", func() error { _, err := nilAdapter.Settings(ctx); return err }},
		{"SaveSettings", func() error { return nilAdapter.SaveSettings(ctx, settingsZero()) }},
		{"RulesSnapshot", func() error { _, err := nilAdapter.RulesSnapshot(ctx); return err }},
		{"SaveRules", func() error { return nilAdapter.SaveRules(ctx, "global", "c") }},
		{"PermissionSnapshot", func() error { _, err := nilAdapter.PermissionSnapshot(ctx); return err }},
		{"SetGoal", func() error { _, err := nilAdapter.SetGoal(ctx, "obj", nil); return err }},
		{"GetGoal", func() error { _, err := nilAdapter.GetGoal(ctx); return err }},
		{"PauseGoal", func() error { _, err := nilAdapter.PauseGoal(ctx); return err }},
		{"ResumeGoal", func() error { _, err := nilAdapter.ResumeGoal(ctx); return err }},
		{"ClearGoal", func() error { return nilAdapter.ClearGoal(ctx) }},
		{"ModeSnapshot", func() error { _, err := nilAdapter.ModeSnapshot(ctx); return err }},
		{"SetExecutionMode", func() error { return nilAdapter.SetExecutionMode(ctx, "auto") }},
		{"SetAccessMode", func() error { return nilAdapter.SetAccessMode(ctx, "workspace-write") }},
		{"ApplyAccessMode", func() error { return nilAdapter.ApplyAccessMode(ctx, "workspace-write") }},
		{"SetApprovalMode", func() error { return nilAdapter.SetApprovalMode(ctx, "default") }},
		{"SyncModeSnapshots", func() error { return nilAdapter.SyncModeSnapshots(ctx, "read-only", "workspace-write", "default") }},
		{"PendingReview", func() error { _, err := nilAdapter.PendingReview(ctx); return err }},
		{"ListSessions", func() error { _, err := nilAdapter.ListSessions(ctx); return err }},
		{"SaveSessionMessages", func() error { _, err := nilAdapter.SaveSessionMessages(ctx, "s", nil); return err }},
		{"LoadSessionMessages", func() error { _, err := nilAdapter.LoadSessionMessages(ctx, "s"); return err }},
		{"RenameSession", func() error { return nilAdapter.RenameSession(ctx, "s", "t") }},
		{"ResumeSession", func() error { return nilAdapter.ResumeSession(ctx, "s") }},
		{"Models", func() error { _, err := nilAdapter.Models(ctx); return err }},
		{"ModelEntries", func() error { _, _, err := nilAdapter.ModelEntries(ctx); return err }},
		{"ActivateModel", func() error { return nilAdapter.ActivateModel(ctx, "m") }},
		{"ModelCatalog", func() error { _, err := nilAdapter.ModelCatalog(ctx); return err }},
		{"ModelContext", func() error { _, err := nilAdapter.ModelContext(ctx); return err }},
		{"SelectModelForCurrentContext", func() error { _, err := nilAdapter.SelectModelForCurrentContext(ctx, "m"); return err }},
		{"SelectWorkspaceModel", func() error { return nilAdapter.SelectWorkspaceModel(ctx, "m") }},
		{"DeleteModel", func() error { return nilAdapter.DeleteModel(ctx, "m") }},
		{"SaveModel", func() error { return nilAdapter.SaveModel(ctx, modelSaveReqZero()) }},
		{"ResolveModelInput", func() error { _, err := nilAdapter.ResolveModelInput(ctx, "m"); return err }},
		{"SwitchPlanModel", func() error { return nilAdapter.SwitchPlanModel(ctx, "e", "p") }},
		{"SyncEnvModel", func() error { return nilAdapter.SyncEnvModel(ctx) }},
		{"MCPServers", func() error { _, err := nilAdapter.MCPServers(ctx); return err }},
		{"SetMCPEnabled", func() error { return nilAdapter.SetMCPEnabled(ctx, "n", true) }},
		{"DeleteMCPServer", func() error { return nilAdapter.DeleteMCPServer(ctx, "n") }},
		{"ImportMCPJSON", func() error { return nilAdapter.ImportMCPJSON(ctx, "{}") }},
		{"AddMCPEntries", func() error { return nilAdapter.AddMCPEntries(ctx, nil) }},
		{"LSPServers", func() error { _, err := nilAdapter.LSPServers(ctx); return err }},
		{"LSPDiagnostics", func() error { _, err := nilAdapter.LSPDiagnostics(ctx); return err }},
		{"LSPDiagnosticsSummary", func() error { _, err := nilAdapter.LSPDiagnosticsSummary(ctx); return err }},
		{"ExecuteCoreTool", func() error { _, err := nilAdapter.ExecuteCoreTool(ctx, toolReqZero()); return err }},
		{"ToolTraces", func() error { _, err := nilAdapter.ToolTraces(ctx); return err }},
		{"ToolStats", func() error { _, err := nilAdapter.ToolStats(ctx); return err }},
		{"Tasks", func() error { _, err := nilAdapter.Tasks(ctx); return err }},
		{"KillTask", func() error { return nilAdapter.KillTask(ctx, "t") }},
		{"TailTask", func() error { _, err := nilAdapter.TailTask(ctx, "t"); return err }},
		{"CleanupTasks", func() error { _, err := nilAdapter.CleanupTasks(ctx); return err }},
		{"Todos", func() error { _, err := nilAdapter.Todos(ctx); return err }},
		{"ContextPreview", func() error { _, err := nilAdapter.ContextPreview(ctx); return err }},
		{"ContextStats", func() error { _, err := nilAdapter.ContextStats(ctx); return err }},
		{"ContextWindowTokens", func() error { _, err := nilAdapter.ContextWindowTokens(ctx); return err }},
		{"PinContextDocument", func() error { return nilAdapter.PinContextDocument(ctx, "id", "c", 10) }},
		{"CompactContext", func() error { _, err := nilAdapter.CompactContext(ctx); return err }},
		{"ClearContext", func() error { return nilAdapter.ClearContext(ctx) }},
		{"ExportContext", func() error { return nilAdapter.ExportContext(ctx, "/tmp/x") }},
		{"UsageSummary", func() error { _, err := nilAdapter.UsageSummary(ctx); return err }},
		{"CostItems", func() error { _, err := nilAdapter.CostItems(ctx); return err }},
		{"Versions", func() error { _, err := nilAdapter.Versions(ctx); return err }},
		{"RollbackVersion", func() error { return nilAdapter.RollbackVersion(ctx, "v") }},
		{"DeleteVersion", func() error { return nilAdapter.DeleteVersion(ctx, "v") }},
		{"ClearVersions", func() error { _, err := nilAdapter.ClearVersions(ctx); return err }},
		{"MemorySnapshot", func() error { _, err := nilAdapter.MemorySnapshot(ctx); return err }},
		{"SaveMemory", func() error { return nilAdapter.SaveMemory(ctx, "global", "n") }},
		{"Skills", func() error { _, err := nilAdapter.Skills(ctx); return err }},
		{"ReloadSkills", func() error { return nilAdapter.ReloadSkills(ctx) }},
		{"InvokeSkill", func() error { _, err := nilAdapter.InvokeSkill(ctx, "s", "a"); return err }},
		{"Plugins", func() error { _, err := nilAdapter.Plugins(ctx); return err }},
		{"BrowserStatus", func() error { _, err := nilAdapter.BrowserStatus(ctx); return err }},
		{"BrowserLaunch", func() error { return nilAdapter.BrowserLaunch(ctx, browserLaunchReqZero()) }},
		{"BrowserClose", func() error { return nilAdapter.BrowserClose(ctx, browserCloseReqZero()) }},
		{"BrowserControlTakeover", func() error { return nilAdapter.BrowserControlTakeover(ctx, takeoverReqZero()) }},
		{"BrowserControlConfirm", func() error { return nilAdapter.BrowserControlConfirm(ctx) }},
		{"BrowserControlResume", func() error { return nilAdapter.BrowserControlResume(ctx) }},
		{"BrowserTabs", func() error { _, err := nilAdapter.BrowserTabs(ctx); return err }},
		{"BrowserProfiles", func() error { _, err := nilAdapter.BrowserProfiles(ctx); return err }},
		{"CurrentRemoteRepo", func() error { _, _, err := nilAdapter.CurrentRemoteRepo(ctx); return err }},
		{"PredictNextUserMessage", func() error { _, err := nilAdapter.PredictNextUserMessage(ctx, "d"); return err }},
		{"GitStatus", func() error { _, err := nilAdapter.GitStatus(ctx); return err }},
		{"GitDiff", func() error { _, err := nilAdapter.GitDiff(ctx, "p"); return err }},
		{"GitBranches", func() error { _, err := nilAdapter.GitBranches(ctx, "/ws"); return err }},
		{"GitSummary", func() error { _, err := nilAdapter.GitSummary(ctx, "/ws"); return err }},
		{"GitLog", func() error { _, err := nilAdapter.GitLog(ctx, gitLogReqZero()); return err }},
		{"GitShow", func() error { _, err := nilAdapter.GitShow(ctx, gitShowReqZero()); return err }},
	}
	for _, c := range errCases {
		if err := c.call(); err == nil {
			t.Errorf("%s: nil adapter should error", c.name)
		}
	}

	// 零值返回族。
	if got := nilAdapter.ActiveWorkspace(ctx); got != "" {
		t.Errorf("ActiveWorkspace = %q", got)
	}
	if _, err := nilAdapter.CurrentSessionID(ctx); err == nil {
		t.Error("nil adapter CurrentSessionID should error")
	}
	if got := nilAdapter.SessionsDir(ctx); got == "" {
		t.Error("SessionsDir should fall back to cwd")
	}
	if got := nilAdapter.HealCurrentSessionModel(ctx); got != "" {
		t.Errorf("HealCurrentSessionModel = %q, want empty", got)
	}
	if name, base := nilAdapter.GetModelInfo(); name != "" || base != "" {
		t.Errorf("GetModelInfo = %q, %q, want empty", name, base)
	}
	if err := nilAdapter.Close(); err != nil {
		t.Errorf("nil Close = %v", err)
	}
	if got := noEngine.Engine(); got != nil {
		t.Errorf("no-engine Engine() = %v", got)
	}

	// CallCore 需要 client；无 engine/client 时报错而非 panic。
	if err := noEngine.CallCore(ctx, "x", nil, nil); err == nil {
		t.Error("CallCore without client should error")
	}
}
