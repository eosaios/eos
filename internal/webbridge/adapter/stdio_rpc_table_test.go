package adapter

// 表驱动网关往返：scripted 服务器按 method 回放预设 result JSON，
// 批量点亮 StdioGateway 的薄委托方法（marshal→Call→unmarshal 全链）。

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
	"github.com/eosaios/eos/pkg/sandbox"
)

// newReplayGateway 起一个 net.Pipe 回放服务器：按 method 查表回 result。
func newReplayGateway(t *testing.T, replies map[string]any) *StdioGateway {
	t.Helper()
	client, serverConn := newPipeStdioClient(t)
	go func() {
		stream := protocoljsonrpc.NewStream(serverConn, serverConn)
		_ = serverConn
		for {
			msg, err := stream.ReadMessage()
			if err != nil {
				return
			}
			if msg.Request == nil {
				continue
			}
			result, ok := replies[msg.Request.Method]
			if !ok {
				result = map[string]any{}
			}
			raw, _ := json.Marshal(result)
			if err := stream.WriteMessage(protocoljsonrpc.Response{
				ID:     msg.Request.ID,
				Result: raw,
			}); err != nil {
				return
			}
		}
	}()
	return NewStdioGateway(client)
}

func TestGatewayBrowserRoundtrips(t *testing.T) {
	tabInfo := map[string]any{
		"index": 2, "url": "https://eos.example", "title": "EOS", "active": true,
	}
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodBrowserTabNew:        tabInfo,
		protocoljsonrpc.MethodBrowserTabSwitch:     tabInfo,
		protocoljsonrpc.MethodBrowserTabs:          []any{tabInfo},
		protocoljsonrpc.MethodBrowserProfiles:      []any{},
		protocoljsonrpc.MethodBrowserProfileUpsert: []any{},
	})
	ctx := context.Background()

	steps := []struct {
		name string
		fn   func() error
	}{
		{"takeover", func() error {
			return g.CoreBrowserControlTakeoverRPC(ctx, coreapi.BrowserControlTakeoverRequest{Reason: "test"})
		}},
		{"confirm", func() error { return g.CoreBrowserControlConfirmRPC(ctx) }},
		{"resume", func() error { return g.CoreBrowserControlResumeRPC(ctx) }},
		{"upload provide", func() error {
			return g.CoreBrowserUploadProvideRPC(ctx, "req-1", []string{"/tmp/a.png"})
		}},
		{"focus", func() error {
			return g.CoreBrowserFocusRPC(ctx, coreapi.BrowserFocusRequest{Profile: "default"})
		}},
		{"set default profile", func() error { return g.CoreBrowserSetDefaultProfileRPC(ctx, "default") }},
		{"navigate", func() error { return g.CoreBrowserNavigateRPC(ctx, "https://eos.example") }},
		{"tab close", func() error { return g.CoreBrowserTabCloseRPC(ctx, intPtr(3)) }},
		{"live start", func() error {
			return g.CoreBrowserLiveStartRPC(ctx, coreapi.BrowserLiveStartRequest{})
		}},
		{"live stop", func() error { return g.CoreBrowserLiveStopRPC(ctx) }},
		{"input", func() error {
			return g.CoreBrowserInputRPC(ctx, coreapi.BrowserInputRequest{Kind: "mouse", Action: "move"})
		}},
		{"history", func() error { return g.CoreBrowserHistoryRPC(ctx, "back") }},
		{"pick start", func() error { return g.CoreBrowserPickStartRPC(ctx) }},
		{"pick stop", func() error { return g.CoreBrowserPickStopRPC(ctx) }},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}

	if info, err := g.CoreBrowserTabNewRPC(ctx, "https://eos.example"); err != nil || info.URL != "https://eos.example" {
		t.Fatalf("tab new: %v %+v", err, info)
	}
	if info, err := g.CoreBrowserTabSwitchRPC(ctx, 2); err != nil || info.Index != 2 {
		t.Fatalf("tab switch: %v %+v", err, info)
	}
	if _, err := g.CoreBrowserCopySelectionRPC(ctx); err != nil {
		t.Fatalf("copy selection: %v", err)
	}
	if _, err := g.CoreBrowserProfilesRPC(ctx); err != nil {
		t.Fatalf("profiles: %v", err)
	}
	if _, err := g.CoreBrowserCredentialsImportRPC(ctx, coreapi.BrowserCredentialsImportRequest{
		Profile: "default", Domains: []string{"eosaios.com"},
	}); err != nil {
		t.Fatalf("credentials import: %v", err)
	}
	if _, err := g.CoreBrowserProfileUpsertRPC(ctx, map[string]any{
		"name": "default", "headless": false,
	}); err != nil {
		t.Fatalf("profile upsert: %v", err)
	}
}

func intPtr(i int) *int { return &i }

func TestGatewayWorkspaceSessionModelRoundtrips(t *testing.T) {
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodSessionCreate:           map[string]any{"id": "s-1", "title": "T"},
		protocoljsonrpc.MethodSessionMessagesSave:     map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionMessagesLoad:     []any{},
		protocoljsonrpc.MethodSessionRename:           map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionCurrent:          map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionResume:           map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionList:             []any{map[string]any{"id": "s-1", "workspace_root": "/tmp/ws"}},
		protocoljsonrpc.MethodModelList:               []any{},
		protocoljsonrpc.MethodModelCatalog:            map[string]any{"providers": []any{}, "presets": []any{}},
		protocoljsonrpc.MethodModelContext:            map[string]any{},
		protocoljsonrpc.MethodStateSnapshot:           map[string]any{},
		protocoljsonrpc.MethodRuntimeModesGet:         map[string]any{},
		protocoljsonrpc.MethodWorkspaceList:           []any{},
		protocoljsonrpc.MethodMCPList:                 []any{},
		protocoljsonrpc.MethodWorkspaceWorktreeList:   []any{},
		protocoljsonrpc.MethodRemoteWorkspaceList:     []any{},
		protocoljsonrpc.MethodWorkspaceWorktreeCreate: map[string]any{"name": "wt"},
		protocoljsonrpc.MethodUsageSummary:            map[string]any{"rounds": 0},
		protocoljsonrpc.MethodModelVerify:             map[string]any{"ok": true},
	})
	ctx := context.Background()

	simple := []struct {
		name string
		fn   func() error
	}{
		{"add workspace", func() error { return g.CoreAddWorkspaceRPC(ctx, "/tmp/ws") }},
		{"remove workspace", func() error { return g.CoreRemoveWorkspaceRPC(ctx, "/tmp/ws") }},
		{"use workspace", func() error { return g.CoreUseWorkspaceRPC(ctx, "/tmp/ws") }},
		{"trust workspace", func() error { return g.CoreTrustWorkspaceRPC(ctx, "/tmp/ws") }},
		{"remember workspace", func() error { return g.CoreRememberWorkspaceRPC(ctx, "/tmp/ws", false) }},
		{"set execution mode", func() error { return g.CoreSetExecutionModeRPC(ctx, "auto") }},
		{"set reasoning level", func() error { return g.CoreSetReasoningLevelRPC(ctx, "high") }},
		{"set session model", func() error { return g.CoreSetSessionModelRPC(ctx, "s-1", "glm-5.3") }},
		{"clear session model", func() error { return g.CoreClearSessionModelRPC(ctx, "s-1") }},
		{"set workspace model", func() error { return g.CoreSetWorkspaceModelRPC(ctx, "/tmp/ws", "glm-5.3") }},
		{"clear workspace model", func() error { return g.CoreClearWorkspaceModelRPC(ctx, "/tmp/ws") }},
		{"upsert model", func() error { return g.CoreUpsertModelRPC(ctx, "m1", "https://api", "sk", "glm") }},
		{"upsert mcp", func() error { return g.CoreUpsertMCPRPC(ctx, "srv", "stdio", "/bin/x", true) }},
		{"set session sandbox", func() error { return g.CoreSetSessionSandboxModeRPC(ctx, "s-1", "workspace-write") }},
	}
	for _, s := range simple {
		if err := s.fn(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}

	if _, err := g.CoreCreateSessionRPC(ctx, "/tmp/ws", "T", "chat", nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := g.CoreSaveSessionMessagesRPC(ctx, "/tmp/ws", "s-1", nil); err != nil {
		t.Fatalf("save messages: %v", err)
	}
	if _, err := g.CoreRenameSessionRPC(ctx, "/tmp/ws", "s-1", "新标题"); err != nil {
		t.Fatalf("rename session: %v", err)
	}
	if _, err := g.CoreListModelsRPC(ctx); err != nil {
		t.Fatalf("list models: %v", err)
	}
	if _, err := g.CoreModelCatalogRPC(ctx); err != nil {
		t.Fatalf("model catalog: %v", err)
	}
	if _, err := g.CoreCreateWorktreeRPC(ctx, "wt"); err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if _, err := g.CoreUsageSummaryRPC(ctx); err != nil {
		t.Fatalf("usage summary: %v", err)
	}
	if _, err := g.CoreListMCPRPC(ctx); err != nil {
		t.Fatalf("list mcp: %v", err)
	}
}

func TestGatewayEngineWrapperSweep(t *testing.T) {
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodSessionList:               []any{map[string]any{"id": "s-1", "workspace_root": "/tmp/ws"}},
		protocoljsonrpc.MethodWorkspaceResolve:          "/tmp/ws",
		protocoljsonrpc.MethodWorkspaceDefault:          map[string]any{"path": "/tmp/ws"},
		protocoljsonrpc.MethodWorkspaceLast:             map[string]any{"path": "/tmp/ws"},
		protocoljsonrpc.MethodWorkspaceList:             []any{},
		protocoljsonrpc.MethodRuntimeModesGet:           map[string]any{},
		protocoljsonrpc.MethodModelList:                 []any{},
		protocoljsonrpc.MethodModelCatalog:              map[string]any{"providers": []any{}, "presets": []any{}},
		protocoljsonrpc.MethodModelContext:              map[string]any{},
		protocoljsonrpc.MethodRemoteWorkspaceList:       []any{},
		protocoljsonrpc.MethodInsightPredictNextUser:    map[string]any{"prediction": "下一句"},
		protocoljsonrpc.MethodInsightRefineInput:        map[string]any{"text": "润色后"},
		protocoljsonrpc.MethodMCPList:                   []any{},
		protocoljsonrpc.MethodLSPList:                   []any{},
		protocoljsonrpc.MethodExtensionsSkillsList:      []any{},
		protocoljsonrpc.MethodExtensionsPluginsList:     []any{},
		protocoljsonrpc.MethodLSPDetect:                 map[string]any{"status": "installed"},
		protocoljsonrpc.MethodLSPStart:                  map[string]any{"status": "started"},
		protocoljsonrpc.MethodLSPInstall:                map[string]any{"status": "installed"},
		protocoljsonrpc.MethodSessionCreate:             map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionMessagesSave:       map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionMessagesLoad:       []any{},
		protocoljsonrpc.MethodSessionRename:             map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionCurrent:            map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionResume:             map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionSetCurrent:         map[string]any{},
		protocoljsonrpc.MethodSessionSetMeta:            map[string]any{},
		protocoljsonrpc.MethodSessionDelete:             map[string]any{},
		protocoljsonrpc.MethodWorkspaceWorktreeCreate:   map[string]any{"name": "wt"},
		protocoljsonrpc.MethodWorkspaceWorktreeList:     []any{},
		protocoljsonrpc.MethodWorkspaceWorktreeRemove:   map[string]any{},
		protocoljsonrpc.MethodRemoteWorkspaceOpen:       map[string]any{"id": "r-1", "path": "/tmp/r"},
		protocoljsonrpc.MethodRemoteWorkspaceForget:     map[string]any{},
		protocoljsonrpc.MethodRemoteWorkspaceClearCache: map[string]any{},
		protocoljsonrpc.MethodRuntimeExecutionModeSet:   map[string]any{},
		protocoljsonrpc.MethodRuntimeSandboxModeSet:     map[string]any{},
		protocoljsonrpc.MethodRuntimeReasoningLevelSet:  map[string]any{},
		protocoljsonrpc.MethodPermissionApprovalModeSet: map[string]any{},
		protocoljsonrpc.MethodModelSave:                 map[string]any{},
		protocoljsonrpc.MethodModelActivate:             map[string]any{},
		protocoljsonrpc.MethodModelDelete:               map[string]any{},
		protocoljsonrpc.MethodMCPImportJSON:             map[string]any{},
		protocoljsonrpc.MethodMCPDelete:                 map[string]any{},
		protocoljsonrpc.MethodMCPSetEnabled:             map[string]any{},
		protocoljsonrpc.MethodExtensionsSkillsReload:    nil,
		protocoljsonrpc.MethodExtensionsSkillSetEnabled: map[string]any{},
		protocoljsonrpc.MethodStateSnapshot:             map[string]any{},
		protocoljsonrpc.MethodUsageSummary:              map[string]any{"rounds": 0},
	})

	// —— 无 ctx 的 Engine 包装层 ——
	_ = g.ListSessions()
	if _, err := g.ListWorkspaceSessions("/tmp/ws"); err != nil {
		t.Fatalf("list workspace sessions: %v", err)
	}
	if _, err := g.ResolveForegroundWorkspace(""); err != nil {
		t.Fatalf("resolve foreground: %v", err)
	}
	_ = g.DefaultWorkspacePath()
	_ = g.LastWorkspace()
	if _, err := g.RunBash(context.Background(), "ls"); err == nil {
		t.Fatal("RunBash should be not-implemented over stdio")
	}
	if _, err := g.Invoke(context.Background(), "probe"); err == nil {
		t.Fatal("Invoke should be not-implemented over stdio")
	}
	for _, fn := range []struct {
		name string
		fn   func() error
	}{
		{"add ws", func() error { return g.AddWorkspace("/tmp/ws") }},
		{"remove ws", func() error { return g.RemoveWorkspace("/tmp/ws") }},
		{"use ws", func() error { return g.UseWorkspace("/tmp/ws") }},
		{"trust ws", func() error { return g.TrustWorkspace("/tmp/ws") }},
		{"remember ws", func() error { return g.RememberWorkspace("/tmp/ws", true) }},
		{"remove worktree", func() error { return g.RemoveWorktree("/tmp/ws/.eos/worktrees/wt", true) }},
		{"forget remote", func() error { return g.ForgetRemoteWorkspace("r-1") }},
		{"clear remote cache", func() error { return g.ClearRemoteWorkspaceCache("r-1") }},
		{"set reasoning level", func() error { return g.SetReasoningLevel("high") }},
		{"delete session", func() error { return g.DeleteWorkspaceSession("/tmp/ws", "s-1") }},
		{"rename session", func() error { return g.UpdateWorkspaceSessionTitle("/tmp/ws", "s-1", "T") }},
		{"set current session", func() error { return g.SetWorkspaceCurrentSession("/tmp/ws", "s-1") }},
		{"resume session", func() error { return g.ResumeWorkspaceSession("/tmp/ws", "s-1") }},
		{"upsert model", func() error { return g.UpsertModel("m1", "https://a", "sk", "glm") }},
		{"activate model", func() error { return g.ActivateModel("m1") }},
		{"delete model", func() error { return g.DeleteModel("m1") }},
		{"upsert mcp", func() error { return g.UpsertMCP("srv", "stdio", "/bin/x", true) }},
		{"import mcp json", func() error { return g.ImportMCPJSON("{}") }},
		{"delete mcp", func() error { return g.DeleteMCP("srv") }},
		{"set mcp enabled", func() error { return g.SetMCPEnabled("srv", false) }},
		{"reload skills", func() error { return g.ReloadSkills() }},
		{"set skill enabled", func() error { return g.SetSkillEnabled("ffmpeg", false) }},
	} {
		if err := fn.fn(); err != nil {
			t.Fatalf("%s: %v", fn.name, err)
		}
	}

	g.SetExecutionMode("auto")
	_ = g.ExecutionMode()
	g.SetSandboxMode("workspace-write")
	_ = g.SandboxMode()
	_ = g.ReasoningLevel()
	if _, err := g.CreateWorkspaceSession("/tmp/ws", "T", nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := g.LoadWorkspaceSessionMessages("/tmp/ws", "s-1"); err != nil {
		t.Fatalf("load messages: %v", err)
	}
	if _, err := g.SaveWorkspaceSessionMessages("/tmp/ws", "s-1", nil); err != nil {
		t.Fatalf("save messages: %v", err)
	}
	_ = g.RuntimeSnapshot()
	if _, err := g.GetWorkspaceCurrentSession("/tmp/ws"); err != nil {
		t.Fatalf("get current session: %v", err)
	}
	if _, err := g.ResolveSessionWorkspace("s-1"); err != nil {
		t.Fatalf("resolve session workspace: %v", err)
	}
	_ = g.ListModels()
	_ = g.ModelCatalog()
	_ = g.ListRemoteWorkspaces()
	_, _ = g.CurrentRemoteRepo()
	if _, err := g.PredictNextUserMessage(context.Background(), "草稿"); err != nil {
		t.Fatalf("predict: %v", err)
	}
	_ = g.ListMCP()
	_ = g.ListLSP()
	_ = g.DetectLSP("rust")
	_ = g.StartLSP("rust")
	_ = g.InstallLSP("rust")
	_ = g.ListSkills()
	_ = g.ListPlugins()
	if _, err := g.CreateWorktree("wt"); err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if _, err := g.OpenRemoteWorkspace("r-1"); err != nil {
		t.Fatalf("open remote: %v", err)
	}
}

func TestGatewayGitVersionsSettingsInsightSweep(t *testing.T) {
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodGitBranches:                map[string]any{"current": "main", "branches": []any{}},
		protocoljsonrpc.MethodGitSummary:                 map[string]any{"changes": []any{}},
		protocoljsonrpc.MethodGitRepos:                   map[string]any{"repos": []any{}},
		protocoljsonrpc.MethodGitStage:                   map[string]any{},
		protocoljsonrpc.MethodGitCommit:                  map[string]any{"commit_id": "abc"},
		protocoljsonrpc.MethodGitPush:                    map[string]any{"status": "pushed", "branch": "main"},
		protocoljsonrpc.MethodGitSuggestMessage:          map[string]any{"message": "提交"},
		protocoljsonrpc.MethodVersionsList:               []any{},
		protocoljsonrpc.MethodVersionsRollback:           map[string]any{},
		protocoljsonrpc.MethodVersionsDelete:             map[string]any{},
		protocoljsonrpc.MethodVersionsClear:              map[string]any{"count": 2},
		protocoljsonrpc.MethodConfigSettingsGet:          map[string]any{},
		protocoljsonrpc.MethodConfigSettingsSave:         map[string]any{},
		protocoljsonrpc.MethodApprovalList:               map[string]any{"approvals": []any{}},
		protocoljsonrpc.MethodApprovalRespond:            map[string]any{},
		protocoljsonrpc.MethodPermissionSnapshot:         map[string]any{},
		protocoljsonrpc.MethodWorkspaceRollbackApply:     map[string]any{},
		protocoljsonrpc.MethodInsightPlanSnapshot:        map[string]any{},
		protocoljsonrpc.MethodMemorySnapshot:             map[string]any{},
		protocoljsonrpc.MethodPermissionPendingReview:    map[string]any{},
		protocoljsonrpc.MethodLSPDiagnostics:             []any{},
		protocoljsonrpc.MethodContextPreview:             []any{},
		protocoljsonrpc.MethodContextStats:               map[string]any{},
		protocoljsonrpc.MethodUsageCostSummary:           map[string]any{"text": "费用摘要"},
		protocoljsonrpc.MethodUsageCostItems:             []any{},
		protocoljsonrpc.MethodExtensionsPluginSetEnabled: map[string]any{},
		protocoljsonrpc.MethodWorkspaceWorktreeList:      []any{},
		protocoljsonrpc.MethodWorkspaceList:              []any{},
		protocoljsonrpc.MethodStateSnapshot:              map[string]any{},
	})
	ctx := context.Background()

	simple := []struct {
		name string
		fn   func() error
	}{
		{"git stage", func() error { return g.CoreGitStageRPC(ctx, "/tmp/ws", nil, true, false) }},
		{"git merge abort", func() error { return g.CoreGitMergeAbortRPC(ctx, "/tmp/ws") }},
		{"version rollback", func() error { return g.CoreRollbackVersionRPC(ctx, "v1") }},
		{"version delete", func() error { return g.CoreDeleteVersionRPC(ctx, "v1") }},
		{"memory save", func() error { return g.CoreMemorySaveRPC(ctx, "记忆") }},
		{"plugin set enabled", func() error { return g.CoreSetPluginEnabledRPC(ctx, "p1", true) }},
		{"rollback apply", func() error {
			return g.CoreWorkspaceRollbackApplyRPC(ctx, "/tmp/ws", nil)
		}},
		{"respond approval", func() error {
			return g.CoreRespondApprovalRPC(ctx, "ap-1", coreapi.ApprovalAccept)
		}},
		{"respond approval with reason", func() error {
			return g.CoreRespondApprovalWithReasonRPC(ctx, "ap-1", "approve", "原因")
		}},
		{"set plugin enabled", func() error { return g.SetPluginEnabled("p1", false) }},
		{"rollback version", func() error { return g.RollbackVersion("v1") }},
		{"delete version", func() error { return g.DeleteVersion("v1") }},
	}
	for _, s := range simple {
		if err := s.fn(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}

	if _, err := g.CoreGitBranchesRPC(ctx, "/tmp/ws"); err != nil {
		t.Fatalf("git branches: %v", err)
	}
	if _, err := g.CoreGitSummaryRPC(ctx, "/tmp/ws"); err != nil {
		t.Fatalf("git summary: %v", err)
	}
	if _, err := g.CoreGitReposRPC(ctx, "/tmp/ws"); err != nil {
		t.Fatalf("git repos: %v", err)
	}
	if _, err := g.CoreGitCommitRPC(ctx, "/tmp/ws", "msg"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := g.CoreGitPushRPC(ctx, "/tmp/ws"); err != nil {
		t.Fatalf("git push: %v", err)
	}
	if _, err := g.CoreGitSuggestMessageRPC(ctx, "/tmp/ws"); err != nil {
		t.Fatalf("git suggest: %v", err)
	}
	if _, err := g.CoreListVersionsRPC(ctx); err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if n, err := g.CoreClearVersionsRPC(ctx); err != nil || n != 2 {
		t.Fatalf("clear versions: %v %d", err, n)
	}
	if _, err := g.CoreGetSettingsRPC(ctx); err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if _, err := g.CoreGetFullSettingsRPC(ctx); err != nil {
		t.Fatalf("get full settings: %v", err)
	}
	if err := g.CoreSaveSettingsRPC(ctx, coreapi.Settings{}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if _, err := g.CoreApprovalListRPC(ctx, coreapi.PendingApprovalListRequest{}); err != nil {
		t.Fatalf("approval list: %v", err)
	}
	if _, err := g.CorePermissionSnapshotRPC(ctx); err != nil {
		t.Fatalf("permission snapshot: %v", err)
	}
	if _, err := g.CorePlanSnapshotRPC(ctx); err != nil {
		t.Fatalf("plan snapshot: %v", err)
	}
	if _, err := g.CoreMemorySnapshotRPC(ctx); err != nil {
		t.Fatalf("memory snapshot: %v", err)
	}
	if _, err := g.CorePendingReviewRPC(ctx); err != nil {
		t.Fatalf("pending review: %v", err)
	}
	if _, err := g.CoreLSPDiagnosticsRPC(ctx); err != nil {
		t.Fatalf("lsp diagnostics: %v", err)
	}
	if _, err := g.CoreContextPreviewRPC(ctx); err != nil {
		t.Fatalf("context preview: %v", err)
	}
	if _, err := g.CoreContextStatsRPC(ctx); err != nil {
		t.Fatalf("context stats: %v", err)
	}
	if s, err := g.CoreCostSummaryRPC(ctx); err != nil || s != "费用摘要" {
		t.Fatalf("cost summary: %v %q", err, s)
	}
	if _, err := g.CoreCostItemsRPC(ctx); err != nil {
		t.Fatalf("cost items: %v", err)
	}
	if _, err := g.CoreListWorktreesRPC(ctx); err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	if _, err := g.CoreListWorkspacesRPC(ctx); err != nil {
		t.Fatalf("list workspaces: %v", err)
	}

	// Engine 包装层
	_ = g.ListWorktrees()
	_ = g.UsageSummary()
	_ = g.CostItems()
	_ = g.ListVersions()
	_ = g.ClearVersions()
	_ = g.GetSettings()
	_ = g.PermissionSnapshot()
	_ = g.PlanSnapshot()
	_ = g.MemorySnapshot()
	_ = g.PendingReview()
	_ = g.LSPDiagnostics()
	_ = g.ContextPreview()
	_ = g.ContextStats()
	_ = g.CostSummary()
}

func TestGatewayMiscCompositeSweep(t *testing.T) {
	policy := map[string]any{"mode": "workspace-write", "workspace_root": "/tmp/ws"}
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodInitialize:                map[string]any{"server_name": "eos-core"},
		protocoljsonrpc.MethodSessionList:               []any{},
		protocoljsonrpc.MethodSandboxBackend:            map[string]any{"goos": "darwin", "backend": "sandbox-exec"},
		protocoljsonrpc.MethodSandboxPolicy:             policy,
		protocoljsonrpc.MethodSandboxSetPolicy:          policy,
		protocoljsonrpc.MethodSandboxDerivePolicy:       policy,
		protocoljsonrpc.MethodPermissionEnterFullAccess: policy,
		protocoljsonrpc.MethodApprovalPreview:           map[string]any{"lines": []any{}},
		protocoljsonrpc.MethodGoalSet:                   map[string]any{"objective": "目标", "status": "active"},
		protocoljsonrpc.MethodGoalGet:                   map[string]any{"goal": map[string]any{"objective": "目标"}},
		protocoljsonrpc.MethodGoalPause:                 map[string]any{"status": "paused"},
		protocoljsonrpc.MethodGoalResume:                map[string]any{"status": "active"},
		protocoljsonrpc.MethodGoalClear:                 map[string]any{},
		protocoljsonrpc.MethodTaskList:                  []any{},
		protocoljsonrpc.MethodTaskKill:                  map[string]any{},
		protocoljsonrpc.MethodTaskCleanup:               map[string]any{"count": 2},
		protocoljsonrpc.MethodNetworkList:               map[string]any{"enabled": false, "records": []any{}},
		protocoljsonrpc.MethodNetworkClear:              map[string]any{"removed": 3},
		protocoljsonrpc.MethodModelVerify:               map[string]any{"ok": true},
		protocoljsonrpc.MethodModelSave:                 map[string]any{},
		protocoljsonrpc.MethodInsightRefineInput:        map[string]any{"text": "润色"},
		protocoljsonrpc.MethodSessionCurrent:            map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionSetMeta:            map[string]any{},
		protocoljsonrpc.MethodPermissionApprovalModeSet: map[string]any{},
	})
	ctx := context.Background()

	if _, err := g.CoreInitializeRPC(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := g.CoreListArchivedSessionsRPC(ctx); err != nil {
		t.Fatalf("archived sessions: %v", err)
	}
	if err := g.CoreSetApprovalModeRPC(ctx, "auto"); err != nil {
		t.Fatalf("set approval mode: %v", err)
	}
	if err := g.CoreArchiveSessionRPC(ctx, "s-1", true); err != nil {
		t.Fatalf("archive session: %v", err)
	}
	if _, err := g.CoreCurrentSessionRPC(ctx, "/tmp/ws"); err != nil {
		t.Fatalf("current session: %v", err)
	}
	if _, err := g.CoreModelContextRPC(ctx, ModelContextRequest{}); err != nil {
		t.Fatalf("model context: %v", err)
	}
	if err := g.CoreSaveModelRPC(ctx, ModelSaveRequest{Name: "m1"}); err != nil {
		t.Fatalf("save model: %v", err)
	}
	if _, err := g.CoreVerifyModelRPC(ctx, ModelSaveRequest{Name: "m1"}); err != nil {
		t.Fatalf("verify model: %v", err)
	}
	if s, err := g.CoreRefineInputRPC(ctx, "草稿"); err != nil || s != "润色" {
		t.Fatalf("refine: %v %q", err, s)
	}
	if _, err := g.CoreCallRPC(ctx, "session/list", json.RawMessage("{}")); err != nil {
		t.Fatalf("call rpc: %v", err)
	}
	if _, err := g.CoreToolExecuteRPC(ctx, json.RawMessage(`{"name":"bash"}`)); err != nil {
		t.Fatalf("tool execute: %v", err)
	}
	if _, err := g.CoreNetworkListRPC(ctx, 10); err != nil {
		t.Fatalf("network list: %v", err)
	}
	if n, err := g.CoreNetworkClearRPC(ctx); err != nil || n != 3 {
		t.Fatalf("network clear: %v %d", err, n)
	}
	if err := g.CoreKillTaskRPC(ctx, "t-1"); err != nil {
		t.Fatalf("kill task: %v", err)
	}
	if _, err := g.CoreTaskListRPC(ctx); err != nil {
		t.Fatalf("task list: %v", err)
	}
	if n, err := g.CoreCleanupTasksRPC(ctx); err != nil || n != 2 {
		t.Fatalf("cleanup tasks: %v %d", err, n)
	}
	if _, err := g.CoreSandboxPolicyRPC(ctx, "s-1"); err != nil {
		t.Fatalf("sandbox policy: %v", err)
	}
	if err := g.CoreSetSandboxPolicyRPC(ctx, "s-1", sandbox.Policy{Mode: "workspace-write"}); err != nil {
		t.Fatalf("set sandbox policy: %v", err)
	}
	if _, err := g.CoreDeriveSandboxPolicyRPC(ctx, "workspace-write", "/tmp/ws"); err != nil {
		t.Fatalf("derive policy: %v", err)
	}
	if _, err := g.CoreEnterFullAccessRPC(ctx, "/tmp/ws"); err != nil {
		t.Fatalf("enter full access: %v", err)
	}
	if _, err := g.CoreApprovalPreviewRPC(ctx, coreapi.ApprovalPreviewRequest{}); err != nil {
		t.Fatalf("approval preview: %v", err)
	}
	if _, err := g.CoreSandboxBackendStatusRPC(ctx); err != nil {
		t.Fatalf("sandbox backend: %v", err)
	}
	goal, err := g.CoreGoalSetRPC(ctx, coreapi.GoalSetRequest{Objective: "目标"})
	if err != nil || goal.Status != "active" {
		t.Fatalf("goal set: %v %+v", err, goal)
	}
	if _, err := g.CoreGoalGetRPC(ctx, "s-1"); err != nil {
		t.Fatalf("goal get: %v", err)
	}
	if _, err := g.CoreGoalPauseRPC(ctx, "s-1"); err != nil {
		t.Fatalf("goal pause: %v", err)
	}
	if _, err := g.CoreGoalResumeRPC(ctx, "s-1"); err != nil {
		t.Fatalf("goal resume: %v", err)
	}
	if err := g.CoreGoalClearRPC(ctx, "s-1"); err != nil {
		t.Fatalf("goal clear: %v", err)
	}
	_ = g.KillTask("t-1")
	_ = g.CleanupTasks()
}

func TestGatewayTurnStreamAndPureHelpers(t *testing.T) {
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodEventSubscribe:   map[string]any{"subscription_id": "sub-1"},
		protocoljsonrpc.MethodEventUnsubscribe: map[string]any{},
		protocoljsonrpc.MethodTurnStart:        map[string]any{"id": "t-1", "session_id": "s-1"},
		protocoljsonrpc.MethodTurnResume:       map[string]any{"id": "t-1"},
		protocoljsonrpc.MethodTurnInterrupt:    map[string]any{},
	})
	ctx := context.Background()

	// 订阅/退订往返
	_, unsub, err := g.CoreSubscribeEventsRPC(ctx, "s-1", "t-1", "", 8)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	unsub()

	// turn 流启动（订阅+start 调用后立即中断消费）
	ch, turn, err := g.CoreStartTurnStreamWithRequestRPC(ctx, coreapi.StartTurnRequest{
		SessionID: "s-1", Input: "你好",
		ImagePaths:  []string{" a.png ", "", "a.png"},
		Attachments: []coreapi.Attachment{{Name: " n ", Path: " p.png "}, {Path: "  "}},
	})
	if err != nil {
		t.Fatalf("start turn stream: %v", err)
	}
	if !strings.HasPrefix(turn.ID, "turn_") {
		t.Fatalf("turn id 应为本地生成的 turn_ 前缀: %+v", turn)
	}
	// 事件通道由后台桥接 goroutine 驱动——等一拍后中断 ctx 结束
	_ = ch

	// resume 流
	if _, _, err := g.CoreResumeTurnStreamRPC(ctx, "s-1", "t-1"); err != nil {
		t.Fatalf("resume stream: %v", err)
	}
	if err := g.CoreInterruptTurnRPC(ctx, "s-1", "t-1"); err != nil {
		t.Fatalf("interrupt: %v", err)
	}

	// 纯 helper 域
	if _, err := g.CoreRunBashStreamRPC(ctx, "ls"); err == nil {
		t.Fatal("bash stream should be not-implemented")
	}
	// stdio 网关不落配置文件——CoreConfigPath 恒空（行为固化）
	if p := g.CoreConfigPath(); p != "" {
		t.Fatalf("config path should be empty over stdio, got %q", p)
	}
	if err := g.SaveModel(ModelSaveRequest{Name: "m1"}); err != nil {
		t.Fatalf("save model: %v", err)
	}
	g.ResolveConfirmation("ap-1", coreapi.ApprovalAccept)
	if c := g.ThreadCoreIfExists("s-1"); c != nil {
		t.Fatalf("stdio gateway has no thread core, got %v", c)
	}
	d := g.StartupDiagnostics()
	_ = d

	// compact helpers 语义
	atts := stdioCompactCoreAPIAttachments([]coreapi.Attachment{
		{Name: " n ", Path: " p.png "}, {Path: "  "}, {Path: "k.png"},
	})
	if len(atts) != 2 || atts[0].Name != "n" || atts[0].Path != "p.png" {
		t.Fatalf("attachments compact: %+v", atts)
	}
	strs := stdioCompactStringSlice([]string{" a ", "", "a", "b ", "b"})
	if len(strs) != 2 || strs[0] != "a" || strs[1] != "b" {
		t.Fatalf("string compact（trim+去重）: %v", strs)
	}
	if id := stdioNewTurnID(); id == "" {
		t.Fatal("turn id should be non-empty")
	}

	// not-implemented 错误文案
	var nie *stdioNotImplementedError
	_ = nie.Error()
}
