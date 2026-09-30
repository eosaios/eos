// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

package ui

// slash_runtime 第四轮收尾：/workspace add/remove 失败臂、/model 套餐内
// 切换（NeedsPlanSwitch 两臂）、/permissions 三轴失败臂、/plan 待办列表、
// /skills /plugin 失败臂与 install/remove 分发、/doctor 少量 traces、
// /diff 三臂、/review 富摘要、/git branches|log|show 失败臂、/status 富行、
// /fast 切回失败、/export /rename /share 少会话与 IO 失败臂、/theme
// /plan-style 读写失败臂、/session save transcript 过滤、会话列表 label 行、
// /goal get 分发、内存面板编辑态取消、approval 别名词表。

// 不可达清单（本文件无法覆盖的分支及原因）：
//   - currentWorkspaceRoot os.Getwd 失败臂（L42-44）：测试进程 cwd 始终有效，
//     Windows 下删除 cwd 后 Getwd 常仍返回缓存值，无法稳定构造。
//   - /model 切换成功后 scopeLabel=="" 回退（L268-270）：SelectModelForCurrentContext
//     成功时只返回 session|workspace|global 三值，防御分支不可达。
//   - handleReloadPluginsSlash 成功臂（L577-584）：Reload 走 sidecar 客户端
//     (a.client.Process())，engine-only 测试环境 client 恒 nil。
//   - /plan-style 无工作区臂（L1350-1353）：currentWorkspaceRoot 在 Getwd 成功时
//     恒非空，同 currentWorkspaceRoot 不可达。
//   - /share role=="" 回退（L1432-1434）：sessionTranscript 只产出
//     user/assistant/system 三种非空 Role，防御分支不可达。
//   - copyToClipboard darwin/linux 分支（L1463-1466）：平台专属，Windows 口径豁免。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/pkg/coreapi"
	tea "charm.land/bubbletea/v2"
)

func TestWorkspaceSlashAddRemoveFailures(t *testing.T) {
	setTestHome(t)
	dir := t.TempDir()

	t.Run("add service error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.workspaceAddErr = os.ErrPermission
		app.handleWorkspaceSlash([]string{"add", dir})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want add error surfaced, got %q", got)
		}
	})

	t.Run("add resolve error on blank path", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handleWorkspaceSlash([]string{"add", " "})
		if got := lastSystemText(app); !strings.Contains(got, "路径") && !strings.Contains(got, "path") {
			t.Fatalf("want resolve error surfaced, got %q", got)
		}
	})

	t.Run("remove service error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.workspaceRemoveErr = os.ErrPermission
		app.handleWorkspaceSlash([]string{"remove", dir})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want remove error surfaced, got %q", got)
		}
	})
}

func TestModelSlashPlanSwitchArms(t *testing.T) {
	setTestHome(t)
	planEntry := coreapi.ModelConfig{
		Name:       "MiniMax Plan",
		APIBase:    "https://api.minimax.chat/v1",
		Model:      "MiniMax-M2.7",
		ProviderID: "minimax",
		PresetID:   "minimax-token-plan",
		Active:     true,
	}
	catalog := &coreapi.ModelCatalogState{
		Presets: []coreapi.ModelPresetOption{{
			ID:         "minimax-token-plan",
			ProviderID: "minimax",
			PlanModels: []coreapi.PlanModel{{ModelID: "MiniMax-M3", Label: "MiniMax M3"}},
		}},
	}

	t.Run("switch error arm", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.models = []coreapi.ModelConfig{planEntry}
		eng.modelCatalog = catalog
		eng.modelsSaveErr = os.ErrPermission
		app.handleModelSlash([]string{"use", "MiniMax", "M3"})
		if got := lastSystemText(app); !strings.Contains(got, "切换套餐内模型失败") {
			t.Fatalf("want plan switch failure, got %q", got)
		}
	})

	t.Run("switch success arm", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.models = []coreapi.ModelConfig{planEntry}
		eng.modelCatalog = catalog
		app.handleModelSlash([]string{"use", "MiniMax", "M3"})
		got := lastSystemText(app)
		if !strings.Contains(got, "已切换模型") || !strings.Contains(got, "MiniMax-M3") {
			t.Fatalf("want switched message with plan model id, got %q", got)
		}
	})

	t.Run("empty name usage", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handleModelSlash([]string{" "})
		if got := lastSystemText(app); !strings.Contains(got, "用法") {
			t.Fatalf("want usage warning, got %q", got)
		}
	})
}

func TestPermissionsSlashFailureArms(t *testing.T) {
	setTestHome(t)

	t.Run("execution mode error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.setExecModeErr = os.ErrPermission
		app.handlePermissionsSlash([]string{"auto"})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want exec mode error, got %q", got)
		}
	})

	t.Run("access mode sandbox error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.setSandboxModeErr = os.ErrPermission
		app.handlePermissionsSlash([]string{"access", "read-only"})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want access mode error, got %q", got)
		}
	})

	t.Run("access mode danger error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.enterFullAccessErr = os.ErrPermission
		app.handlePermissionsSlash([]string{"access", "full"})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want danger access error, got %q", got)
		}
	})

	t.Run("access mode apply success", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		app.handlePermissionsSlash([]string{"access", "read-only"})
		// 产品行为：切换成功后 fall-through 渲染最新快照（toast 不是最后一条）。
		if eng.permissionSnap.SandboxMode != "read-only" {
			t.Fatalf("sandbox mode = %q, want read-only", eng.permissionSnap.SandboxMode)
		}
		found := false
		for _, h := range app.history {
			if strings.Contains(h.content, "访问模式已切换为") {
				found = true
			}
		}
		if !found {
			t.Fatalf("want switch toast in history, last = %q", lastSystemText(app))
		}
	})

	t.Run("approval mode error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.setApprovalErr = os.ErrPermission
		app.handlePermissionsSlash([]string{"approval", "never"})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want approval error, got %q", got)
		}
	})

	t.Run("snapshot error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.permissionSnapErr = os.ErrPermission
		app.handlePermissionsSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want snapshot error, got %q", got)
		}
	})
}

func TestPlanSlashTodoListArms(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.todoList = []coreapi.TodoItem{
		{ID: "t1", Status: "done", Content: "write tests"},
		{Status: "pending", Content: "ship it"},
	}
	app.handlePlanSlash(nil)
	got := lastSystemText(app)
	if !strings.Contains(got, "1. [done] write tests (t1)") {
		t.Fatalf("want numbered todo with id, got %q", got)
	}
	if !strings.Contains(got, "2. [pending] ship it") {
		t.Fatalf("want second todo without id, got %q", got)
	}
}

func TestSkillsAndPluginFailureArms(t *testing.T) {
	setTestHome(t)

	t.Run("skills list error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.skillsListErr = os.ErrPermission
		app.handleSkillsSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "读取 skills 失败") {
			t.Fatalf("want skills error, got %q", got)
		}
	})

	t.Run("plugin list error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.pluginsListErr = os.ErrPermission
		app.handlePluginSlash()
		if got := lastSystemText(app); !strings.Contains(got, "读取插件失败") {
			t.Fatalf("want plugins error, got %q", got)
		}
	})

	t.Run("plugin install dispatch", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handlePluginSlash("install", "./local-plugin")
		// 无 caller 时 CallCore 报错，goroutine 落「安装失败」——作为分发同步点。
		if !waitForHistory(t, app, "安装失败") {
			t.Fatal("install dispatch should reach async call core")
		}
	})

	t.Run("plugin remove dispatch", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handlePluginSlash("remove", "demo")
		if !waitForHistory(t, app, "失败") {
			t.Fatal("remove dispatch should reach async call core")
		}
	})
}

func TestDoctorSlashFewTracesClamp(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.toolTraces = []coreapi.ToolTrace{
		{Tool: "bash", Success: true},
		{Tool: "read", Success: false},
	}
	app.handleDoctorSlash()
	got := lastSystemText(app)
	if !strings.Contains(got, "bash [ok]") || !strings.Contains(got, "read [error]") {
		t.Fatalf("want both traces rendered with status, got %q", got)
	}
}

func TestDiffSlashThreeArms(t *testing.T) {
	setTestHome(t)

	t.Run("pending review without path label", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.pendingReview = coreapi.PendingReview{Diff: "diff --git a/x\n+pending\n"}
		app.handleDiffSlash(nil)
		got := lastSystemText(app)
		if !strings.Contains(got, "当前待审批改动") {
			t.Fatalf("want fallback pending label, got %q", got)
		}
	})

	t.Run("git change file list", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.gitStatus = []coreapi.GitChange{{State: "M", Path: "a.go"}, {State: "??", Path: "b.go"}}
		app.handleDiffSlash(nil)
		got := lastSystemText(app)
		if !strings.Contains(got, "- [M] a.go") || !strings.Contains(got, "/diff <path>") {
			t.Fatalf("want changed files list, got %q", got)
		}
	})

	t.Run("git status error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.gitStatusErr = os.ErrPermission
		app.handleDiffSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want git status error, got %q", got)
		}
	})

	t.Run("explicit path styled diff", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.gitDiff = coreapi.GitTextResult{Text: "diff --git a/a.go\n+line\n"}
		app.handleDiffSlash([]string{"a.go"})
		got := lastSystemText(app)
		if !strings.Contains(got, "文件 diff") || !strings.Contains(got, "a.go") {
			t.Fatalf("want styled file diff, got %q", got)
		}
	})
}

func TestReviewSlashRichSummary(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.gitStatus = []coreapi.GitChange{{State: "M", Path: "a.go"}}
	eng.lspDiagnostics = []string{"a.go:3:1 unused variable"}
	eng.toolTraces = []coreapi.ToolTrace{
		{Tool: "bash", Success: true},
		{Tool: "edit", Success: false},
	}
	app.handleReviewSlash(nil)
	got := lastSystemText(app)
	if !strings.Contains(got, "- [M] a.go") {
		t.Fatalf("want git changes in review, got %q", got)
	}
	if !strings.Contains(got, "unused variable") {
		t.Fatalf("want diagnostics in review, got %q", got)
	}
	if !strings.Contains(got, "edit [error]") {
		t.Fatalf("want tool traces in review, got %q", got)
	}
}

func TestGitSlashSubCommandErrors(t *testing.T) {
	setTestHome(t)

	t.Run("branches error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.gitBranchesErr = os.ErrPermission
		app.handleGitSlash([]string{"branches"})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want branches error, got %q", got)
		}
	})

	t.Run("log error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.gitLogErr = os.ErrPermission
		app.handleGitSlash([]string{"log"})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want log error, got %q", got)
		}
	})

	t.Run("show error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.gitShowErr = os.ErrPermission
		app.handleGitSlash([]string{"show", "HEAD"})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want show error, got %q", got)
		}
	})
}

func TestSessionSlashLabelAndRounds(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.sessionList = []coreapi.Session{
		{ID: "s1", Metadata: map[string]any{"title": "调试会话", "rounds": int64(3)}},
		{ID: "s2"},
	}
	app.handleSessionSlash(nil)
	got := lastSystemText(app)
	if !strings.Contains(got, "调试会话") {
		t.Fatalf("want session label rendered, got %q", got)
	}
	if !strings.Contains(got, "rounds=0") {
		t.Fatalf("want zero rounds for metadata-less session, got %q", got)
	}
}

func TestSessionSaveTranscriptFiltering(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	app.appendHistory(historyEntry{kind: "user", content: "hello"})
	app.appendHistory(historyEntry{kind: "ai", content: "world"})
	app.appendHistory(historyEntry{kind: "system", content: "sys"})
	app.appendHistory(historyEntry{kind: "tool", content: "tool output"})
	app.appendHistory(historyEntry{kind: "reasoning", content: "thinking"})
	app.appendHistory(historyEntry{kind: "user", content: "   "})
	app.handleSessionSlash([]string{"save"})
	saved := eng.savedMessages
	if len(saved) != 3 {
		t.Fatalf("saved messages = %d, want 3 (user/ai/system only)", len(saved))
	}
	roles := []string{saved[0].Role, saved[1].Role, saved[2].Role}
	if strings.Join(roles, ",") != "user,assistant,system" {
		t.Fatalf("roles = %v, want user,assistant,system", roles)
	}
}

func TestRestoreSessionHistoryStatusEmptyContent(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.messages = []coreapi.SessionMessage{
		{Role: "assistant", Content: "", Metadata: map[string]any{
			"turn_id": "t1", "kind": "status",
		}},
		{Role: "assistant", Content: "done", Metadata: map[string]any{
			"turn_id": "t1", "kind": "status",
		}},
	}
	app.restoreSessionHistory("s1")
	found := false
	for _, h := range app.history {
		if h.content == "done" {
			found = true
		}
	}
	if !found {
		t.Fatalf("non-empty status should render; history = %+v", app.history)
	}
}

func TestRestoreSessionHistoryToolNameNotString(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	// tool_call.name 非字符串（脏数据）：sessionMessageToolName 走 return "" 臂。
	eng.messages = []coreapi.SessionMessage{
		{Role: "tool", Content: "out", Metadata: map[string]any{
			"turn_id": "t1", "tool_call": map[string]any{"name": 123},
		}},
	}
	app.restoreSessionHistory("s1")
	found := false
	for _, h := range app.history {
		if h.kind == "tool" && h.toolName == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tool entry with blank name should render; history = %+v", app.history)
	}
}

func TestStatusSlashRichLines(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.permissionSnap = coreapi.PermissionSnapshot{
		LastAuthorization:       "approved",
		LastAuthorizationTarget: "bash",
		LastAuthorizationNote:   "one-off",
	}
	eng.remoteOK = true
	eng.remoteRepo = coreapi.RemoteRepoState{
		Owner: "eosaios", Repo: "eos-cli",
		WorkingBranch: "main", LocalPath: "/repo",
	}
	eng.browserStatus = coreapi.BrowserRuntimeStatus{LastError: "not launched"}
	eng.windowTokens = 128000
	app.handleStatusSlash()
	got := lastSystemText(app)
	for _, want := range []string{"approved", "bash", "one-off", "eosaios/eos-cli", "not launched", "128000 tokens"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status missing %q:\n%s", want, got)
		}
	}
}

func TestFastSlashSwitchBackSelectError(t *testing.T) {
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	cfgPath := filepath.Join(home, ".eos.json")
	if err := config.Save(config.Config{FastModel: "fast-model", Active: "default-model"}, cfgPath); err != nil {
		t.Fatal(err)
	}
	app, eng := newTestAppModelWithEngine(t)
	eng.models = []coreapi.ModelConfig{
		{Name: "default-model", APIBase: "https://a/v1", Model: "std"},
		{Name: "fast-model", APIBase: "https://b/v1", Model: "fast"},
	}
	eng.activeModel = "fast-model"
	eng.modelCtxSnapshot = coreapi.ModelContextSnapshot{
		ResolvedModelName:   "fast-model",
		WorkspaceModelName:  "default-model",
		GlobalDefaultName:   "default-model",
	}
	eng.selectModelErr = os.ErrPermission
	app.handleFastSlash()
	if got := lastSystemText(app); !strings.Contains(got, "permission") {
		t.Fatalf("want select error surfaced, got %q", got)
	}
}

func TestExportSlashFailureArms(t *testing.T) {
	setTestHome(t)

	t.Run("no current session", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.currentSessionEmpty = true
		app.handleExportSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "没有当前会话可导出") {
			t.Fatalf("want no-session warning, got %q", got)
		}
	})

	t.Run("json write failure", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		dir := t.TempDir() // 写入目录本身必然失败
		app.handleExportSlash([]string{"json", dir})
		if got := lastSystemText(app); !strings.Contains(got, "导出失败") {
			t.Fatalf("want write failure, got %q", got)
		}
	})

	t.Run("markdown load failure", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.loadMessagesErr = os.ErrPermission
		app.handleExportSlash([]string{"md", filepath.Join(t.TempDir(), "out.md")})
		if got := lastSystemText(app); !strings.Contains(got, "导出失败") {
			t.Fatalf("want load failure, got %q", got)
		}
	})
}

func TestRenameShareSlashArms(t *testing.T) {
	setTestHome(t)

	t.Run("rename without session", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.currentSessionEmpty = true
		app.handleRenameSlash([]string{"new-title"})
		if got := lastSystemText(app); !strings.Contains(got, "没有当前会话") {
			t.Fatalf("want no-session warning, got %q", got)
		}
	})

	t.Run("rename service error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.renameSessionErr = os.ErrPermission
		app.handleRenameSlash([]string{"new-title"})
		if got := lastSystemText(app); !strings.Contains(got, "permission") {
			t.Fatalf("want rename error, got %q", got)
		}
	})

	t.Run("share without session", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.currentSessionEmpty = true
		app.handleShareSlash()
		if got := lastSystemText(app); !strings.Contains(got, "没有当前会话可分享") {
			t.Fatalf("want no-session warning, got %q", got)
		}
	})

	t.Run("share clipboard fallback saves file", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.foregroundWS = t.TempDir()
		app.appendHistory(historyEntry{kind: "user", content: "hi"})
		// PATH 清空让 clip.exe 找不到，触发文件兜底。
		t.Setenv("PATH", t.TempDir())
		app.handleShareSlash()
		got := lastSystemText(app)
		if !strings.Contains(got, "已保存到文件") {
			t.Fatalf("want saved-to-file message, got %q", got)
		}
	})

	t.Run("share fallback write failure", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		root := t.TempDir()
		// .eos 是普通文件：MkdirAll 失败被忽略后 WriteFile 必然失败。
		if err := os.WriteFile(filepath.Join(root, ".eos"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		eng.foregroundWS = root
		app.appendHistory(historyEntry{kind: "user", content: "hi"})
		t.Setenv("PATH", t.TempDir())
		app.handleShareSlash()
		if got := lastSystemText(app); !strings.Contains(got, "分享失败") {
			t.Fatalf("want share failure, got %q", got)
		}
	})
}

func TestThemePlanStyleSettingsArms(t *testing.T) {
	setTestHome(t)

	t.Run("theme read error no args", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.getSettingsErr = os.ErrPermission
		app.handleThemeSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "读取主题失败") {
			t.Fatalf("want theme read error, got %q", got)
		}
	})

	t.Run("theme read error with arg", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.getSettingsErr = os.ErrPermission
		app.handleThemeSlash([]string{"light"})
		if got := lastSystemText(app); !strings.Contains(got, "读取主题失败") {
			t.Fatalf("want theme read error, got %q", got)
		}
	})

	t.Run("theme save error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.saveSettingsErr = os.ErrPermission
		app.handleThemeSlash([]string{"light"})
		if got := lastSystemText(app); !strings.Contains(got, "保存主题失败") {
			t.Fatalf("want theme save error, got %q", got)
		}
	})

	t.Run("theme empty shows adapter default", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		// settingsFromCoreAPI 对空 Theme 统一回退 dark，UI 层不再重复兜底。
		app.handleThemeSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "dark") {
			t.Fatalf("want adapter default theme, got %q", got)
		}
	})

	t.Run("plan-style read error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.getSettingsErr = os.ErrPermission
		app.handlePlanStyleSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "读取计划提示风格失败") {
			t.Fatalf("want plan-style read error, got %q", got)
		}
	})

	t.Run("plan-style save error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.saveSettingsErr = os.ErrPermission
		app.handlePlanStyleSlash([]string{"detailed"})
		if got := lastSystemText(app); !strings.Contains(got, "保存计划提示风格失败") {
			t.Fatalf("want plan-style save error, got %q", got)
		}
	})

	t.Run("plan-style custom text", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handlePlanStyleSlash([]string{"custom", "三段式输出"})
		if got := lastSystemText(app); !strings.Contains(got, "custom:三段式输出") {
			t.Fatalf("want custom style saved, got %q", got)
		}
	})
}

func TestGoalSlashGetDispatch(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.goalGetResp = coreapi.GoalGetResponse{Goal: &coreapi.ThreadGoal{Objective: "ship coverage"}}
	app.handleGoalSlash([]string{"get"})
	if !waitForHistory(t, app, "ship coverage") {
		t.Fatalf("goal get is async; objective never rendered, last = %q", lastSystemText(app))
	}
}

func TestOpenMemoryPanelCancelsEditing(t *testing.T) {
	setTestHome(t)
	app, _ := newTestAppModelWithEngine(t)
	app.openMemoryPanel()
	panel, ok := app.panels["memory"].(*panels.MemoryPanel)
	if !ok || panel == nil {
		t.Fatal("memory panel should be constructed")
	}
	// "a" 进入编辑态；再次 /memory 应取消编辑。
	panel.Update(tea.KeyPressMsg{Text: "a"})
	if !panel.IsEditing() {
		t.Fatal("panel should be editing after compose key")
	}
	app.openMemoryPanel()
	if panel.IsEditing() {
		t.Fatal("reopening memory panel should cancel editing")
	}
}

func TestIsSupportedApprovalModeAlias(t *testing.T) {
	if !isSupportedApprovalModeInput("no_approval") {
		t.Fatal("alias no_approval should be accepted")
	}
	if isSupportedApprovalModeInput("bogus-mode") {
		t.Fatal("unknown mode should be rejected")
	}
}

// 不可达清单补充（startup.go）：
//   - StartCoreClient 注入缝函数体（L86-88）：真启动 sidecar 子进程。
//   - StartInteractiveTUIWithOptions：进程入口 + os.Exit（同事已书面豁免）。
func TestNewSidecarStderrWriterDegradedArms(t *testing.T) {
	home := setTestHome(t)
	cfgPath := filepath.Join(home, ".eos.json")

	t.Run("log dir mkdir failure degrades", func(t *testing.T) {
		blocked := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("EOS_TEST_LOG_ROOT", blocked)
		if err := config.Save(config.Config{LogDir: "$EOS_TEST_LOG_ROOT"}, cfgPath); err != nil {
			t.Fatal(err)
		}
		w := newSidecarStderrWriter()
		if w.f != nil {
			t.Fatal("mkdir failure should degrade to nil-handle writer")
		}
		n, err := w.Write([]byte("hi"))
		if err != nil || n != 2 {
			t.Fatalf("degraded write = (%d, %v), want (2, nil)", n, err)
		}
		w.Close()
	})

	t.Run("log file open failure degrades", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("EOS_TEST_LOG_ROOT", root)
		if err := config.Save(config.Config{LogDir: "$EOS_TEST_LOG_ROOT"}, cfgPath); err != nil {
			t.Fatal(err)
		}
		// eos-core.log 位置预创建为目录，OpenFile 必然失败。
		if err := os.MkdirAll(filepath.Join(root, "core", "eos-core.log"), 0o755); err != nil {
			t.Fatal(err)
		}
		w := newSidecarStderrWriter()
		if w.f != nil {
			t.Fatal("open failure should degrade to nil-handle writer")
		}
	})
}
