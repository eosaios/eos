package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
)

func newFakeAdapter(t *testing.T) (*CoreClientAdapter, *fakeEngine) {
	t.Helper()
	engine := &fakeEngine{}
	adapter := NewCoreClientAdapterFromEngine(engine)
	t.Cleanup(func() { _ = adapter.Close() })
	return adapter, engine
}

// TestCoreClientAdapterEngineHappyPaths 用 fake engine 把浅层透传方法全量
// 跑一遍：断言映射正确、错误不吞。
func TestCoreClientAdapterEngineHappyPaths(t *testing.T) {
	ctx := context.Background()
	adapter, engine := newFakeAdapter(t)

	engine.state = coreapi.StateSnapshot{ForegroundWorkspace: " /ws/fg "}
	if snap, err := adapter.StateSnapshot(ctx); err != nil || snap.ForegroundWorkspace != " /ws/fg " {
		t.Errorf("StateSnapshot = %+v, %v", snap, err)
	}
	if got := adapter.ActiveWorkspace(ctx); got != "/ws/fg" {
		t.Errorf("ActiveWorkspace = %q", got)
	}

	engine.workspaceList = []coreapi.Workspace{{Path: "/ws/a", Active: true}}
	if list, err := adapter.Workspaces(ctx); err != nil || len(list) != 1 {
		t.Errorf("Workspaces = %+v, %v", list, err)
	}
	for _, mutator := range []struct {
		name string
		call func() error
	}{
		{"add", func() error { return adapter.AddWorkspace(ctx, " /ws/b ") }},
		{"remove", func() error { return adapter.RemoveWorkspace(ctx, "/ws/b") }},
		{"use", func() error { return adapter.UseWorkspace(ctx, "/ws/a") }},
		{"trust", func() error { return adapter.TrustWorkspace(ctx, "/ws/a") }},
	} {
		if err := mutator.call(); err != nil {
			t.Errorf("%s workspace: %v", mutator.name, err)
		}
	}
	if err := adapter.StartContextEngine(ctx, "/ws/a"); err != nil {
		t.Errorf("StartContextEngine: %v", err)
	}

	if _, err := adapter.Settings(ctx); err != nil {
		t.Errorf("Settings: %v", err)
	}
	if err := adapter.SaveSettings(ctx, settingsZero()); err != nil {
		t.Errorf("SaveSettings: %v", err)
	}
	if _, err := adapter.RulesSnapshot(ctx); err != nil {
		t.Errorf("RulesSnapshot: %v", err)
	}
	if err := adapter.SaveRules(ctx, "global", "rules"); err != nil {
		t.Errorf("SaveRules: %v", err)
	}

	engine.permissionSnap = coreapi.PermissionSnapshot{AccessMode: "workspace-write"}
	if snap, err := adapter.PermissionSnapshot(ctx); err != nil || snap.AccessMode != "workspace-write" {
		t.Errorf("PermissionSnapshot = %+v, %v", snap, err)
	}
	if _, err := adapter.PendingReview(ctx); err != nil {
		t.Errorf("PendingReview: %v", err)
	}

	if goal, err := adapter.SetGoal(ctx, "obj", nil); err != nil || goal.Status != "active" {
		t.Errorf("SetGoal = %+v, %v", goal, err)
	}
	engine.goalGet = coreapi.GoalGetResponse{Goal: &coreapi.ThreadGoal{Objective: "obj"}}
	if got, err := adapter.GetGoal(ctx); err != nil || got.Goal == nil || got.Goal.Objective != "obj" {
		t.Errorf("GetGoal = %+v, %v", got, err)
	}
	if goal, err := adapter.PauseGoal(ctx); err != nil || goal.Status != "paused" {
		t.Errorf("PauseGoal = %+v, %v", goal, err)
	}
	if goal, err := adapter.ResumeGoal(ctx); err != nil || goal.Status != "active" {
		t.Errorf("ResumeGoal = %+v, %v", goal, err)
	}
	if err := adapter.ClearGoal(ctx); err != nil {
		t.Errorf("ClearGoal: %v", err)
	}

	engine.modeSnap = coreapi.ModeSnapshot{ExecutionMode: "auto", SandboxMode: "workspace-write", ReasoningLevel: "high"}
	if snap, err := adapter.ModeSnapshot(ctx); err != nil || snap.ReasoningLevel != "high" {
		t.Errorf("ModeSnapshot = %+v, %v", snap, err)
	}
	for _, setMode := range []struct {
		name string
		call func() error
	}{
		{"exec", func() error { return adapter.SetExecutionMode(ctx, "plan") }},
		{"access", func() error { return adapter.SetAccessMode(ctx, "read-only") }},
		{"apply", func() error { return adapter.ApplyAccessMode(ctx, "workspace-write") }},
		{"approval", func() error { return adapter.SetApprovalMode(ctx, "never") }},
		{"sync", func() error { return adapter.SyncModeSnapshots(ctx, "read-only", "workspace-write", "default") }},
	} {
		if err := setMode.call(); err != nil {
			t.Errorf("%s mode: %v", setMode.name, err)
		}
	}
	if len(engine.modeSets) == 0 {
		t.Error("mode mutations were not recorded on fake engine")
	}

	engine.models = []coreapi.ModelConfig{{Name: "m1"}, {Name: "m2"}}
	if models, err := adapter.Models(ctx); err != nil || len(models) != 2 {
		t.Errorf("Models = %+v, %v", models, err)
	}
	if entries, active, err := adapter.ModelEntries(ctx); err != nil || len(entries) != 2 || active != "" {
		t.Errorf("ModelEntries = %+v active=%q, %v", entries, active, err)
	}
	if err := adapter.ActivateModel(ctx, "m1"); err != nil {
		t.Errorf("ActivateModel: %v", err)
	}
	if _, err := adapter.ModelCatalog(ctx); err != nil {
		t.Errorf("ModelCatalog: %v", err)
	}
	engine.modelContext = coreapi.ModelContextSnapshot{ResolvedModelName: "m1", ResolvedScope: "global"}
	if snap, err := adapter.ModelContext(ctx); err != nil || snap.ResolvedModelName != "m1" {
		t.Errorf("ModelContext = %+v, %v", snap, err)
	}
	if err := adapter.SelectWorkspaceModel(ctx, "m2"); err != nil {
		t.Errorf("SelectWorkspaceModel: %v", err)
	}
	if err := adapter.DeleteModel(ctx, "m2"); err != nil {
		t.Errorf("DeleteModel: %v", err)
	}
	if err := adapter.SaveModel(ctx, modelSaveReqZero()); err != nil {
		t.Errorf("SaveModel: %v", err)
	}
	if snap := adapter.ModelCatalogSnapshot(ctx); snap == nil {
		t.Error("ModelCatalogSnapshot = nil")
	}
	if err := adapter.SyncEnvModel(ctx); err != nil {
		t.Errorf("SyncEnvModel: %v", err)
	}
	if name, base := adapter.GetModelInfo(); name != "" || base != "" {
		t.Errorf("GetModelInfo = %q, %q", name, base)
	}

	engine.mcpList = []coreapi.MCPServer{{Name: "mcp1"}}
	if servers, err := adapter.MCPServers(ctx); err != nil || len(servers) != 1 {
		t.Errorf("MCPServers = %+v, %v", servers, err)
	}
	if err := adapter.SetMCPEnabled(ctx, "mcp1", false); err != nil {
		t.Errorf("SetMCPEnabled: %v", err)
	}
	if err := adapter.DeleteMCPServer(ctx, "mcp1"); err != nil {
		t.Errorf("DeleteMCPServer: %v", err)
	}
	if err := adapter.ImportMCPJSON(ctx, "{}"); err != nil {
		t.Errorf("ImportMCPJSON: %v", err)
	}
	if err := adapter.AddMCPEntries(ctx, []config.MCPEntry{mcpEntryZero()}); err != nil {
		t.Errorf("AddMCPEntries: %v", err)
	}

	engine.lspList = []coreapi.LSPServer{{Language: "go", Status: "running"}}
	if servers, err := adapter.LSPServers(ctx); err != nil || len(servers) != 1 {
		t.Errorf("LSPServers = %+v, %v", servers, err)
	}
	if _, err := adapter.LSPDiagnostics(ctx); err != nil {
		t.Errorf("LSPDiagnostics: %v", err)
	}
	if _, err := adapter.LSPDiagnosticsSummary(ctx); err != nil {
		t.Errorf("LSPDiagnosticsSummary: %v", err)
	}
	if md := adapter.LSPDiagnosticsMarkdown(ctx); md != "" && !strings.Contains(md, "LSP") {
		t.Errorf("LSPDiagnosticsMarkdown = %q", md)
	}

	if _, err := adapter.ExecuteCoreTool(ctx, toolReqZero()); err != nil {
		t.Errorf("ExecuteCoreTool: %v", err)
	}
	if _, err := adapter.ToolTraces(ctx); err != nil {
		t.Errorf("ToolTraces: %v", err)
	}
	if _, err := adapter.ToolStats(ctx); err != nil {
		t.Errorf("ToolStats: %v", err)
	}

	engine.taskList = []coreapi.TaskSnapshot{{ID: "t1"}}
	if tasks, err := adapter.Tasks(ctx); err != nil || len(tasks) != 1 {
		t.Errorf("Tasks = %+v, %v", tasks, err)
	}
	if err := adapter.KillTask(ctx, "t1"); err != nil {
		t.Errorf("KillTask: %v", err)
	}
	if lines, err := adapter.TailTask(ctx, "t1"); err != nil || len(lines) != 1 {
		t.Errorf("TailTask = %v, %v", lines, err)
	}
	if n, err := adapter.CleanupTasks(ctx); err != nil || n != 3 {
		t.Errorf("CleanupTasks = %d, %v", n, err)
	}
	engine.todos = []coreapi.TodoItem{{Content: "todo"}}
	if todos, err := adapter.Todos(ctx); err != nil || len(todos) != 1 {
		t.Errorf("Todos = %+v, %v", todos, err)
	}
	if agents, err := adapter.Agents(ctx); err != nil || agents != nil {
		t.Errorf("Agents = %+v, %v", agents, err)
	}

	engine.contextPreview = []string{"ctx"}
	if preview, err := adapter.ContextPreview(ctx); err != nil || len(preview) != 1 {
		t.Errorf("ContextPreview = %v, %v", preview, err)
	}
	engine.contextStats = coreapi.ContextStats{MessageCount: 7, Estimated: 500}
	if stats, err := adapter.ContextStats(ctx); err != nil || stats.MessageCount != 7 {
		t.Errorf("ContextStats = %+v, %v", stats, err)
	}
	if tokens, err := adapter.ContextWindowTokens(ctx); err != nil || tokens != 1234 {
		t.Errorf("ContextWindowTokens = %d, %v", tokens, err)
	}
	used, ratio, err := adapter.CurrentContextUsage(ctx)
	if err != nil || used != 500 || ratio <= 0 {
		t.Errorf("CurrentContextUsage = %d, %f, %v", used, ratio, err)
	}
	if err := adapter.PinContextDocument(ctx, "id", "content", 100); err != nil {
		t.Errorf("PinContextDocument: %v", err)
	}
	if text, err := adapter.CompactContext(ctx); err != nil || text != "compacted" {
		t.Errorf("CompactContext = %q, %v", text, err)
	}
	if err := adapter.ClearContext(ctx); err != nil {
		t.Errorf("ClearContext: %v", err)
	}
	if err := adapter.ExportContext(ctx, t.TempDir()); err != nil {
		t.Errorf("ExportContext: %v", err)
	}

	engine.usageSummary = coreapi.UsageSummary{Rounds: 9}
	if summary, err := adapter.UsageSummary(ctx); err != nil || summary.Rounds != 9 {
		t.Errorf("UsageSummary = %+v, %v", summary, err)
	}
	if items, err := adapter.CostItems(ctx); err != nil || items != nil {
		t.Errorf("CostItems = %+v, %v", items, err)
	}

	engine.versions = []coreapi.VersionItem{{ID: "v1"}}
	if versions, err := adapter.Versions(ctx); err != nil || len(versions) != 1 {
		t.Errorf("Versions = %+v, %v", versions, err)
	}
	if err := adapter.RollbackVersion(ctx, "v1"); err != nil {
		t.Errorf("RollbackVersion: %v", err)
	}
	if err := adapter.DeleteVersion(ctx, "v1"); err != nil {
		t.Errorf("DeleteVersion: %v", err)
	}
	if n, err := adapter.DeleteFileVersions(ctx, "f"); err != nil || n != 1 {
		t.Errorf("DeleteFileVersions = %d, %v", n, err)
	}
	if n, err := adapter.ClearVersions(ctx); err != nil || n != 2 {
		t.Errorf("ClearVersions = %d, %v", n, err)
	}

	engine.memorySnap = coreapi.MemorySnapshot{Documents: []coreapi.MemoryDocument{{Scope: "global"}}}
	if snap, err := adapter.MemorySnapshot(ctx); err != nil || len(snap.Documents) != 1 {
		t.Errorf("MemorySnapshot = %+v, %v", snap, err)
	}
	if err := adapter.SaveMemory(ctx, "global", "note"); err != nil {
		t.Errorf("SaveMemory: %v", err)
	}

	engine.skills = []coreapi.SkillInfo{{Name: "s1"}}
	if skills, err := adapter.Skills(ctx); err != nil || len(skills) != 1 {
		t.Errorf("Skills = %+v, %v", skills, err)
	}
	if err := adapter.ReloadSkills(ctx); err != nil {
		t.Errorf("ReloadSkills: %v", err)
	}
	if ok, err := adapter.InvokeSkill(ctx, "s1", "args"); err != nil || ok {
		t.Errorf("InvokeSkill = %v, %v", ok, err)
	}

	engine.plugins = []coreapi.PluginInfo{{Name: "p1"}}
	if plugins, err := adapter.Plugins(ctx); err != nil || len(plugins) != 1 {
		t.Errorf("Plugins = %+v, %v", plugins, err)
	}

	engine.browserStatus = coreapi.BrowserRuntimeStatus{Running: true}
	if status, err := adapter.BrowserStatus(ctx); err != nil || !status.Running {
		t.Errorf("BrowserStatus = %+v, %v", status, err)
	}
	for _, browser := range []struct {
		name string
		call func() error
	}{
		{"launch", func() error { return adapter.BrowserLaunch(ctx, browserLaunchReqZero()) }},
		{"close", func() error { return adapter.BrowserClose(ctx, browserCloseReqZero()) }},
		{"takeover", func() error { return adapter.BrowserControlTakeover(ctx, takeoverReqZero()) }},
		{"confirm", func() error { return adapter.BrowserControlConfirm(ctx) }},
		{"resume", func() error { return adapter.BrowserControlResume(ctx) }},
	} {
		if err := browser.call(); err != nil {
			t.Errorf("browser %s: %v", browser.name, err)
		}
	}
	engine.browserTabs = []coreapi.BrowserTabInfo{{Index: 1}}
	if tabs, err := adapter.BrowserTabs(ctx); err != nil || len(tabs) != 1 {
		t.Errorf("BrowserTabs = %+v, %v", tabs, err)
	}
	engine.browserProfiles = []coreapi.BrowserProfileRecord{{Name: "default"}}
	if profiles, err := adapter.BrowserProfiles(ctx); err != nil || len(profiles) != 1 {
		t.Errorf("BrowserProfiles = %+v, %v", profiles, err)
	}

	engine.remoteRepo = coreapi.RemoteRepoState{Platform: "github", Repo: "eos"}
	engine.remoteRepoOK = true
	if state, ok, err := adapter.CurrentRemoteRepo(ctx); err != nil || !ok || state.Repo != "eos" {
		t.Errorf("CurrentRemoteRepo = %+v, %v, %v", state, ok, err)
	}
	if next, err := adapter.PredictNextUserMessage(ctx, "d"); err != nil || next != "next" {
		t.Errorf("PredictNextUserMessage = %q, %v", next, err)
	}

	engine.gitBranches = coreapi.GitBranchesResult{Current: "main"}
	if branches, err := adapter.GitBranches(ctx, "/ws"); err != nil || branches.Current != "main" {
		t.Errorf("GitBranches = %+v, %v", branches, err)
	}
	engine.gitSummary = coreapi.GitSummaryResult{Branch: "main"}
	if summary, err := adapter.GitSummary(ctx, "/ws"); err != nil || summary.Branch != "main" {
		t.Errorf("GitSummary = %+v, %v", summary, err)
	}
	if changes, err := adapter.GitStatus(ctx); err != nil || changes != nil {
		t.Errorf("GitStatus = %+v, %v", changes, err)
	}
	if diff, err := adapter.GitDiff(ctx, "p"); err != nil || diff != "diff" {
		t.Errorf("GitDiff = %q, %v", diff, err)
	}
	if _, err := adapter.GitLog(ctx, gitLogReqZero()); err != nil {
		t.Errorf("GitLog: %v", err)
	}
	if _, err := adapter.GitShow(ctx, gitShowReqZero()); err != nil {
		t.Errorf("GitShow: %v", err)
	}
}

// TestCoreClientAdapterSessionFlows 覆盖会话辅助逻辑：
// ensureSessionID 的 Current→Create 回落、SessionsDir 派生、消息存取。
func TestCoreClientAdapterSessionFlows(t *testing.T) {
	ctx := context.Background()
	adapter, engine := newFakeAdapter(t)

	engine.state = coreapi.StateSnapshot{ForegroundWorkspace: "/ws/cur"}
	// fakeSessions.Current 返回空 ID → ensureSessionID 走 Create。
	sid, err := adapter.ensureSessionID(ctx)
	if err != nil || sid != "s-created" {
		t.Errorf("ensureSessionID = %q, %v", sid, err)
	}
	if got, err := adapter.CurrentSessionID(ctx); err != nil || got != "" {
		t.Errorf("CurrentSessionID = %q, %v", got, err)
	}
	if err := adapter.ResumeSession(ctx, "s-created"); err != nil {
		t.Errorf("ResumeSession: %v", err)
	}
	if got := engine.resumeCalls; len(got) != 1 || got[0] != "s-created" {
		t.Errorf("resume calls = %v", got)
	}
	if err := adapter.RenameSession(ctx, "s-created", "title"); err != nil {
		t.Errorf("RenameSession: %v", err)
	}
	if id, err := adapter.SaveSessionMessages(ctx, "s-created", nil); err != nil || id != "s-created" {
		t.Errorf("SaveSessionMessages = %q, %v", id, err)
	}
	if _, err := adapter.LoadSessionMessages(ctx, "s-created"); err != nil {
		t.Errorf("LoadSessionMessages: %v", err)
	}
	if _, err := adapter.ListSessions(ctx); err != nil {
		t.Errorf("ListSessions: %v", err)
	}
	if got := adapter.SessionsDir(ctx); !strings.HasSuffix(got, filepath.Join(".eos", "sessions")) {
		t.Errorf("SessionsDir = %q", got)
	}
}

// TestCoreClientAdapterInvokeFlow 覆盖 turn 流：delta 累计、final 覆盖、
// request.completed 收尾与失败分支。
func TestCoreClientAdapterInvokeFlow(t *testing.T) {
	ctx := context.Background()
	adapter, engine := newFakeAdapter(t)

	events := make(chan protocol.Envelope, 64)
	engine.eventsSub = &fakeEvents{ch: events}
	// 事件必须在 Invoke 内部订阅建立后送达；订阅注册先于 turn/start
	// （subscribeEvents → setActiveTurn → Turns.Start），因此以轮询补发
	// 直到 Invoke 返回，规避 dispatcher 在无订阅者时丢事件。
	pumpEvents := func(ets ...protocol.Envelope) (stop chan struct{}) {
		stop = make(chan struct{})
		go func() {
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					for _, ev := range ets {
						select {
						case events <- ev:
						case <-stop:
							return
						}
					}
				}
			}
		}()
		return stop
	}
	stop1 := pumpEvents(
		protocol.Envelope{EventType: protocol.EventTypeItemDelta, Payload: map[string]any{"delta": "你好"}},
		protocol.Envelope{EventType: protocol.EventTypeTextFinal, Payload: map[string]any{"text": "最终回答"}},
		protocol.Envelope{EventType: protocol.EventTypeRequestDone},
	)

	out, err := adapter.Invoke(ctx, "查询", "auto", nil, false)
	close(stop1)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out != "最终回答" {
		t.Errorf("Invoke output = %q, want 最终回答", out)
	}
	if len(engine.turnStarts) != 1 {
		t.Fatalf("turn starts = %d", len(engine.turnStarts))
	}
	started := engine.turnStarts[0]
	if started.Input != "查询" || started.SessionID == "" {
		t.Errorf("turn start req = %+v", started)
	}
	if started.UseMemory == nil || *started.UseMemory {
		t.Errorf("use_memory = %v, want false", started.UseMemory)
	}

	// plan 模式透传 collaboration_mode=plan；失败事件同样轮询补发。
	stop2 := pumpEvents(protocol.Envelope{
		EventType: protocol.EventTypeRequestFailed,
		Payload:   map[string]any{"error": "模型错误"},
	})
	_, err2 := adapter.Invoke(ctx, "q2", "PLAN", nil, false)
	close(stop2)
	if err2 == nil || !strings.Contains(err2.Error(), "模型错误") {
		t.Errorf("Invoke failure = %v, want 模型错误", err)
	}
	if len(engine.turnStarts) != 2 {
		t.Fatalf("turn starts after plan = %d", len(engine.turnStarts))
	}
	if cm := engine.turnStarts[1].CollaborationMode; cm == nil || cm.Mode != coreapi.ModePlan {
		t.Errorf("plan collaboration mode = %+v", engine.turnStarts[1].CollaborationMode)
	}

	// ctx 取消分支。
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = adapter.Invoke(canceled, "q3", "auto", nil, false) // 只要求不挂死
}

// TestCoreClientAdapterExecuteBash 覆盖 bash 工具路径与空命令校验。
func TestCoreClientAdapterExecuteBash(t *testing.T) {
	ctx := context.Background()
	adapter, _ := newFakeAdapter(t)

	if _, err := adapter.ExecuteBash(ctx, "   "); err == nil {
		t.Error("empty command should be rejected")
	}
	engine := &fakeEngine{}
	engine.toolExecResult = coreapi.ToolResult{Status: "ok", Display: "done\n"}
	a2 := NewCoreClientAdapterFromEngine(engine)
	defer a2.Close()
	out, err := a2.ExecuteBash(ctx, " echo hi ")
	if err != nil || out != "done" {
		t.Errorf("ExecuteBash = %q, %v", out, err)
	}

	// 工具返回 error 状态。
	engine2 := &fakeEngine{}
	engine2.toolExecResult = coreapi.ToolResult{Status: "error", Error: "boom"}
	a3 := NewCoreClientAdapterFromEngine(engine2)
	defer a3.Close()
	if out, err := a3.ExecuteBash(ctx, "bad"); err == nil || out != "boom" {
		t.Errorf("ExecuteBash error = %q, %v", out, err)
	}
}

// TestCoreClientAdapterEventPump 覆盖事件订阅分发与活跃 turn 中断。
func TestCoreClientAdapterEventPump(t *testing.T) {
	adapter, engine := newFakeAdapter(t)

	events := make(chan protocol.Envelope, 4)
	engine.eventsSub = &fakeEvents{ch: events}

	sub := adapter.Events()
	events <- protocol.Envelope{EventType: protocol.EventTypeItemDelta, Payload: map[string]any{"delta": "x"}}
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-sub:
			if ev.Type != string(protocol.EventTypeItemDelta) {
				t.Errorf("pumped event = %+v", ev)
			}
			goto pumped
		case <-deadline:
			t.Fatal("event not pumped")
		}
	}
pumped:

	// 活跃 turn 中断：CancelForegroundRequest 无活跃 turn 时 false。
	if adapter.CancelForegroundRequest() {
		t.Error("no active turn should not cancel")
	}
	// 中断失败（Turns.Interrupt 成功路径）由 fake 返回 nil → true。
	adapter.setActiveTurn(coreapi.TurnRef{SessionID: "s", TurnID: "t"})
	if ref, ok := adapter.currentActiveTurn(); !ok || ref.TurnID != "t" {
		t.Errorf("currentActiveTurn = %+v, %v", ref, ok)
	}
	if !adapter.CancelForegroundRequest() {
		t.Error("active turn should be interrupted")
	}
	if _, ok := adapter.currentActiveTurn(); ok {
		t.Error("active turn should be cleared after cancel")
	}
}

// TestCoreClientAdapterReloadAndExport 覆盖维护入口。
//
// 豁免说明：Reload 的成功路径要求 a.client.Process() 非 nil（重启真实
// sidecar 子进程），fake engine 注入的 adapter 无 client——该分支只能由
// vendored-sidecar 集成测试覆盖；此处验证无进程时的防御语义。
func TestCoreClientAdapterReloadAndExport(t *testing.T) {
	adapter, _ := newFakeAdapter(t)
	if err := adapter.Reload(); err == nil {
		t.Error("Reload without sidecar process should report unavailable")
	}
	adapter.ClearTokenHistory() // 只要求不 panic
}
