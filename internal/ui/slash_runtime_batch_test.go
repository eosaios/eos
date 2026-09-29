// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

package ui

// slash_runtime 批测：restoreSessionHistory 全形态、/session /stats /doctor
// /remote /fast /goal /skills /resume /model 的失败臂与列表分支、plugin 异步
// 命令族（install needs_confirm 两段流 / search / remove）、msg.go 31 个
// msgType marker。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

// waitForHistory 轮询等待 history 中出现包含 substr 的条目（异步 goroutine
// 命令的同步点）。
func waitForHistory(t *testing.T, app *AppModel, substr string) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, h := range app.history {
			if strings.Contains(h.content, substr) {
				return true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func lastSystemText(app *AppModel) string {
	for i := len(app.history) - 1; i >= 0; i-- {
		if app.history[i].kind == "system" {
			return app.history[i].content
		}
	}
	return ""
}

func countHistoryKind(app *AppModel, kind string) int {
	n := 0
	for _, h := range app.history {
		if h.kind == kind {
			n++
		}
	}
	return n
}

// ---------- restoreSessionHistory 全形态 ----------

func TestRestoreSessionHistoryVariants(t *testing.T) {
	setTestHome(t)

	t.Run("empty id early return", func(t *testing.T) {
		app := newTestAppModel(t)
		app.restoreSessionHistory("   ")
		if len(app.history) != 0 {
			t.Fatalf("history should stay empty, got %d", len(app.history))
		}
	})

	t.Run("load error", func(t *testing.T) {
		engine := newTestEngine()
		engine.loadMessagesErr = errors.New("boom load")
		app := NewAppModelFromCoreEngine(engine)
		app.restoreSessionHistory("s1")
		if got := lastSystemText(app); !strings.Contains(got, "boom load") {
			t.Fatalf("want load error surfaced, got %q", got)
		}
	})

	t.Run("all message shapes", func(t *testing.T) {
		engine := newTestEngine()
		engine.messages = []coreapi.SessionMessage{
			// 无 turn_id：user / system / tool / assistant + 空 content 跳过
			{Role: "user", Content: "hello"},
			{Role: "system", Content: "sys msg"},
			{Role: "tool", Content: "tool orphan"},
			{Role: "assistant", Content: "   "},
			// turn t1：agent 文本 + plan + tool_call 空 content + tool 结果
			// + 空 tool content + reasoning + 空 reasoning + status
			{Role: "assistant", Content: "part one", Metadata: map[string]any{"turn_id": "t1"}},
			{Role: "assistant", Content: "plan text", Metadata: map[string]any{"turn_id": "t1", "kind": "plan"}},
			{Role: "assistant", Content: "", Metadata: map[string]any{
				"turn_id": "t1", "tool_call": map[string]any{"name": "bash"},
			}},
			{Role: "tool", Content: "tool output", Metadata: map[string]any{
				"turn_id": "t1", "tool_call": map[string]any{"name": "bash"},
			}},
			{Role: "tool", Content: "  ", Metadata: map[string]any{"turn_id": "t1"}},
			{Role: "assistant", Content: "thinking...", Metadata: map[string]any{"turn_id": "t1", "kind": "Reasoning"}},
			{Role: "assistant", Content: "", Metadata: map[string]any{"turn_id": "t1", "kind": "reasoning"}},
			{Role: "assistant", Content: "canceled", Metadata: map[string]any{"turn_id": "t1", "kind": "status"}},
			// turn t2：触发 pending flush 与新 turn
			{Role: "assistant", Content: "second turn", Metadata: map[string]any{"turn_id": "t2"}},
			// 无 turn_id 的 assistant（独立 entry）
			{Role: "assistant", Content: "orphan answer"},
		}
		app := NewAppModelFromCoreEngine(engine)
		app.appendHistory(historyEntry{kind: "user", content: "seed"})
		app.restoreSessionHistory("s1")

		// 期望 entry：seed 之外的重建结果。
		// user: hello；system: sys msg + tool orphan(无 turn_id 的 tool 归
		// system) + canceled(status)；tool: t1 的 tool 结果；ai: t1 合并
		// (part one+plan text) + t2(second turn) + orphan answer；
		// reasoning: thinking...
		if got := countHistoryKind(app, "user"); got != 1 {
			t.Fatalf("user entries = %d, want 1", got)
		}
		if got := countHistoryKind(app, "system"); got != 3 {
			t.Fatalf("system entries = %d, want 3 (sys msg + orphan tool + status)", got)
		}
		if got := countHistoryKind(app, "tool"); got != 1 {
			t.Fatalf("tool entries = %d, want 1", got)
		}
		if got := countHistoryKind(app, "reasoning"); got != 1 {
			t.Fatalf("reasoning entries = %d, want 1", got)
		}
		aiCount := countHistoryKind(app, "ai")
		if aiCount != 3 {
			t.Fatalf("ai entries = %d, want 3 (t1 merged + t2 + orphan)", aiCount)
		}
		found := false
		for _, h := range app.history {
			if h.kind == "tool" && h.toolName == "bash" {
				found = true
			}
		}
		if !found {
			t.Fatal("tool entry should carry toolName=bash")
		}
	})
}

// ---------- /session 子命令 ----------

func TestHandleSessionSlashBatch(t *testing.T) {
	t.Run("save error", func(t *testing.T) {
		setTestHome(t)
		engine := newTestEngine()
		engine.saveSessionMsgsErr = errors.New("save fail")
		app := NewAppModelFromCoreEngine(engine)
		app.handleSessionSlash([]string{"save"})
		if got := lastSystemText(app); !strings.Contains(got, "save fail") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("save ok", func(t *testing.T) {
		setTestHome(t)
		app := newTestAppModel(t)
		app.handleSessionSlash([]string{"save"})
		if got := lastSystemText(app); !strings.Contains(got, "已保存会话") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("export no id usage", func(t *testing.T) {
		setTestHome(t)
		app := newTestAppModel(t)
		app.handleSessionSlash([]string{"export"})
		if got := lastSystemText(app); !strings.Contains(got, "用法") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("export default path then error", func(t *testing.T) {
		setTestHome(t)
		dir := t.TempDir()
		engine := newTestEngine()
		engine.foregroundWS = dir
		engine.messages = []coreapi.SessionMessage{{Role: "user", Content: "hi"}}
		app := NewAppModelFromCoreEngine(engine)
		app.handleSessionSlash([]string{"export", "sess-a"})
		want := filepath.Join(dir, ".eos", "sessions", "sess-a.md")
		data, err := os.ReadFile(want)
		if err != nil {
			t.Fatalf("exported file missing: %v", err)
		}
		if !strings.Contains(string(data), "**user**: hi") {
			t.Fatalf("unexpected markdown: %q", string(data))
		}

		engine.loadMessagesErr = errors.New("export boom")
		app.handleSessionSlash([]string{"export", "sess-b", filepath.Join(dir, "out.md")})
		if got := lastSystemText(app); !strings.Contains(got, "export boom") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("list error empty and truncation", func(t *testing.T) {
		setTestHome(t)
		engine := newTestEngine()
		engine.listSessionsErr = errors.New("list boom")
		app := NewAppModelFromCoreEngine(engine)
		app.handleSessionSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "list boom") {
			t.Fatalf("got %q", got)
		}

		engine.listSessionsErr = nil
		app.handleSessionSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "暂无已保存会话") {
			t.Fatalf("got %q", got)
		}

		engine.currentSessionID = "s01"
		list := make([]coreapi.Session, 13)
		for i := range list {
			list[i] = coreapi.Session{ID: "s13"} // 占位，下面改
		}
		for i := range list {
			id := "s" + string(rune('0'+i%10)) // s0..s9 循环，避免依赖 strconv
			if i < 10 {
				id = "s0" + string(rune('0'+i))
			} else {
				id = "s1" + string(rune('0'+i-10))
			}
			list[i] = coreapi.Session{ID: id, Metadata: map[string]any{
				"rounds": []any{i, int64(2), 1.5, "x", true, nil, i, i, i, i, i, i, i}[i],
			}}
		}
		engine.sessionList = list
		app.handleSessionSlash(nil)
		got := lastSystemText(app)
		if !strings.Contains(got, "* s01") {
			t.Fatalf("current session not marked: %q", got)
		}
		if strings.Contains(got, "s12") {
			t.Fatalf("13-item list should truncate to 12: %q", got)
		}
		if !strings.Contains(got, "rounds=2") || !strings.Contains(got, "rounds=1") {
			t.Fatalf("rounds metadata not rendered: %q", got)
		}
	})
}

// ---------- /stats ----------

func TestHandleStatsSlashBatch(t *testing.T) {
	setTestHome(t)

	t.Run("usage error", func(t *testing.T) {
		engine := newTestEngine()
		engine.usageSummaryErr = errors.New("usage boom")
		app := NewAppModelFromCoreEngine(engine)
		app.handleStatsSlash()
		if got := lastSystemText(app); !strings.Contains(got, "usage boom") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("toolstats error", func(t *testing.T) {
		engine := newTestEngine()
		engine.toolStatsErr = errors.New("stats boom")
		app := NewAppModelFromCoreEngine(engine)
		app.handleStatsSlash()
		if got := lastSystemText(app); !strings.Contains(got, "stats boom") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("ok with tool stats", func(t *testing.T) {
		in, out, cost := 100, 200, 0.5
		engine := newTestEngine()
		engine.usageSummary = coreapi.UsageSummary{
			Rounds:      3,
			InputTokens: &in,
			ReplyTokens: &out,
			TotalTokens: &out,
			CostUSD:     &cost,
		}
		engine.toolStats = []coreapi.ToolStat{
			{Tool: "bash", TotalCalls: 4, AvgDuration: 1500},
			{Tool: "read", TotalCalls: 9, AvgDuration: 500},
		}
		app := NewAppModelFromCoreEngine(engine)
		app.handleStatsSlash()
		got := lastSystemText(app)
		for _, want := range []string{"对话轮数: 3", "read: 9 calls", "bash: 4 calls"} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q in %q", want, got)
			}
		}
	})
}

// ---------- /doctor ----------

func TestHandleDoctorSlashBatch(t *testing.T) {
	setTestHome(t)
	engine := newTestEngine()
	engine.taskList = []coreapi.TaskSnapshot{{}}
	engine.agentList = []coreapi.Agent{{ID: "a1"}}
	engine.todoList = []coreapi.TodoItem{{}, {}}
	engine.skills = []coreapi.SkillInfo{{Name: "sk"}}
	engine.plugins = []coreapi.PluginInfo{{Name: "pl"}}
	engine.browserStatus = coreapi.BrowserRuntimeStatus{
		Running:     true,
		BrowserKind: "chromium",
		LastError:   "",
	}
	engine.toolStats = []coreapi.ToolStat{
		{Tool: "zeta", TotalCalls: 7},
		{Tool: "alpha", TotalCalls: 7},
		{Tool: "mid", TotalCalls: 1},
		{Tool: "cut1", TotalCalls: 2},
		{Tool: "cut2", TotalCalls: 3},
		{Tool: "cut3", TotalCalls: 4},
		{Tool: "cut4", TotalCalls: 5},
	}
	// 时间线只渲染最近 5 条：cached/error 形态都放在窗口内。
	engine.toolTraces = []coreapi.ToolTrace{
		{Tool: "t0"}, {Tool: "t1"},
		{Tool: "t2", Success: false},
		{Tool: "t3"}, {Tool: "t4"},
		{Tool: "t5", Success: true, Cached: true},
		{Tool: "t6"},
	}
	engine.lspDiagnostics = []string{"diag line"}
	app := NewAppModelFromCoreEngine(engine)
	app.handleDoctorSlash()
	got := lastSystemText(app)
	for _, want := range []string{
		"Doctor 摘要", "后台任务: 1", "代理任务: 1", "待办项: 2",
		"可用 skills: 1", "已注册插件: 1", "运行中", "工具统计:",
		"zeta: calls=7", "alpha: calls=7", "cut4", "最近工具时间线:",
		"t5 [ok,cached]", "t2 [error]", "t6", "LSP 诊断摘要:", "diag line",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in doctor output:\n%s", want, got)
		}
	}
	if strings.Contains(got, "t1 [ok]") && !strings.Contains(got, "ok,cached") {
		t.Fatal("cached flag not rendered")
	}

	// 异常浏览器行 + LastError。
	engine.browserStatus = coreapi.BrowserRuntimeStatus{LastError: "conn refused"}
	app.handleDoctorSlash()
	got = lastSystemText(app)
	if !strings.Contains(got, "异常") || !strings.Contains(got, "conn refused") {
		t.Fatalf("browser error row missing: %q", got)
	}

	// 空诊断（diag 为空不追加 LSP 段）。
	engine.lspDiagnostics = nil
	engine.toolStats = nil
	engine.toolTraces = nil
	app.handleDoctorSlash()
	got = lastSystemText(app)
	if strings.Contains(got, "LSP") || strings.Contains(got, "工具统计") {
		t.Fatalf("empty sections should be omitted: %q", got)
	}
}

// ---------- /remote ----------

func TestHandleRemoteSlashBatch(t *testing.T) {
	setTestHome(t)

	t.Run("error", func(t *testing.T) {
		engine := newTestEngine()
		engine.remoteErr = errors.New("remote boom")
		app := NewAppModelFromCoreEngine(engine)
		app.handleRemoteSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "remote boom") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("no active remote", func(t *testing.T) {
		engine := newTestEngine()
		app := NewAppModelFromCoreEngine(engine)
		app.handleRemoteSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "没有活跃的远程仓库") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("with and without account", func(t *testing.T) {
		engine := newTestEngine()
		engine.remoteOK = true
		engine.remoteRepo = coreapi.RemoteRepoState{
			Platform:      "github",
			Owner:         "eosaios",
			Repo:          "eos",
			RepoURL:       "https://github.com/eosaios/eos",
			WorkingBranch: "dev",
			LocalPath:     "/x/y",
			AccountLogin:  "leo",
		}
		app := NewAppModelFromCoreEngine(engine)
		app.handleRemoteSlash(nil)
		got := lastSystemText(app)
		for _, want := range []string{"github", "eosaios/eos", "dev", "leo"} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q in %q", want, got)
			}
		}

		engine.remoteRepo.AccountLogin = ""
		engine.remoteRepo.AccountName = ""
		app.handleRemoteSlash(nil)
		if got := lastSystemText(app); strings.Contains(got, "账号") {
			t.Fatalf("account row should be omitted: %q", got)
		}
	})
}

// ---------- /fast ----------

func writeFastConfig(t *testing.T, doc string) {
	t.Helper()
	home := setTestHome(t)
	if err := os.WriteFile(filepath.Join(home, ".eos.json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHandleFastSlashBatch(t *testing.T) {
	t.Run("fast model unconfigured", func(t *testing.T) {
		setTestHome(t)
		app := newTestAppModel(t)
		app.handleFastSlash()
		if got := lastSystemText(app); !strings.Contains(got, "fast_model") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("model context error", func(t *testing.T) {
		writeFastConfig(t, `{"fast_model":"flash"}`)
		engine := newTestEngine()
		engine.modelCtxErr = errors.New("ctx boom")
		app := NewAppModelFromCoreEngine(engine)
		app.handleFastSlash()
		if got := lastSystemText(app); !strings.Contains(got, "ctx boom") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("switch back via active model", func(t *testing.T) {
		writeFastConfig(t, `{"fast_model":"flash","active_model":"main"}`)
		engine := newTestEngine()
		engine.modelCtxSnapshot = coreapi.ModelContextSnapshot{ResolvedModelName: "flash"}
		app := NewAppModelFromCoreEngine(engine)
		app.handleFastSlash()
		if got := lastSystemText(app); !strings.Contains(got, "已切换回标准模型: main") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("switch back falls to workspace default", func(t *testing.T) {
		writeFastConfig(t, `{"fast_model":"flash"}`)
		engine := newTestEngine()
		engine.modelCtxSnapshot = coreapi.ModelContextSnapshot{
			ResolvedModelName:  "flash",
			WorkspaceModelName: "ws-default",
			GlobalDefaultName:  "g-default",
		}
		app := NewAppModelFromCoreEngine(engine)
		app.handleFastSlash()
		if got := lastSystemText(app); !strings.Contains(got, "已切换回标准模型: ws-default") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("switch back impossible", func(t *testing.T) {
		writeFastConfig(t, `{"fast_model":"flash","active_model":"flash"}`)
		engine := newTestEngine()
		engine.modelCtxSnapshot = coreapi.ModelContextSnapshot{
			ResolvedModelName: "flash",
			GlobalDefaultName: "flash",
		}
		app := NewAppModelFromCoreEngine(engine)
		app.handleFastSlash()
		if got := lastSystemText(app); !strings.Contains(got, "无法切换回标准模型") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("switch to fast ok and error", func(t *testing.T) {
		writeFastConfig(t, `{"fast_model":"flash","active_model":"main"}`)
		engine := newTestEngine()
		engine.modelCtxSnapshot = coreapi.ModelContextSnapshot{ResolvedModelName: "main"}
		app := NewAppModelFromCoreEngine(engine)
		app.handleFastSlash()
		if got := lastSystemText(app); !strings.Contains(got, "已切换到快速模型: flash") {
			t.Fatalf("got %q", got)
		}

		engine2 := newTestEngine()
		engine2.modelCtxSnapshot = coreapi.ModelContextSnapshot{ResolvedModelName: "main"}
		engine2.selectModelErr = errors.New("select boom")
		app2 := NewAppModelFromCoreEngine(engine2)
		app2.handleFastSlash()
		if got := lastSystemText(app2); !strings.Contains(got, "select boom") {
			t.Fatalf("got %q", got)
		}
	})
}

// ---------- /goal ----------

func TestHandleGoalSlashBatch(t *testing.T) {
	setTestHome(t)

	t.Run("no args runs get", func(t *testing.T) {
		engine := newTestEngine()
		engine.goalGetResp = coreapi.GoalGetResponse{}
		app := NewAppModelFromCoreEngine(engine)
		app.handleGoalSlash(nil)
		if !waitForHistory(t, app, "没有目标") {
			t.Fatalf("goal get empty not surfaced: %q", lastSystemText(app))
		}
	})

	t.Run("set usage and empty objective", func(t *testing.T) {
		app := newTestAppModel(t)
		app.handleGoalSlash([]string{"set"})
		if got := lastSystemText(app); !strings.Contains(got, "用法") {
			t.Fatalf("got %q", got)
		}
		// "budget=200" 整段被预算语法吃掉 → 目标文本为空。
		app.handleGoalSlash([]string{"set", "budget=200"})
		if got := lastSystemText(app); !strings.Contains(got, "目标文本不能为空") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("set ok with budget", func(t *testing.T) {
		engine := newTestEngine()
		engine.goalSetResp = coreapi.ThreadGoal{Objective: "ship it", Status: "active"}
		app := NewAppModelFromCoreEngine(engine)
		app.handleGoalSlash([]string{"set", "ship", "it", "budget=300"})
		got := lastSystemText(app)
		if !strings.Contains(got, "ship it") || !strings.Contains(got, "目标已设定") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("set error", func(t *testing.T) {
		engine := newTestEngine()
		engine.goalSetErr = errors.New("set boom")
		app := NewAppModelFromCoreEngine(engine)
		app.handleGoalSlash([]string{"set", "obj"})
		if got := lastSystemText(app); !strings.Contains(got, "set boom") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("get error and with goal", func(t *testing.T) {
		engine := newTestEngine()
		engine.goalGetErr = errors.New("get boom")
		app := NewAppModelFromCoreEngine(engine)
		app.runGoalGet()
		if !waitForHistory(t, app, "get boom") {
			t.Fatalf("get error not surfaced: %q", lastSystemText(app))
		}

		engine2 := newTestEngine()
		engine2.goalGetResp = coreapi.GoalGetResponse{
			Goal: &coreapi.ThreadGoal{Objective: "cover all", Status: "active"},
		}
		app2 := NewAppModelFromCoreEngine(engine2)
		app2.runGoalGet()
		if !waitForHistory(t, app2, "当前提问目标") {
			t.Fatalf("goal display not surfaced: %q", lastSystemText(app2))
		}
	})

	t.Run("pause resume clear ok and error", func(t *testing.T) {
		engine := newTestEngine()
		engine.goalPauseResumeResp = coreapi.ThreadGoal{Status: "paused"}
		app := NewAppModelFromCoreEngine(engine)
		app.handleGoalSlash([]string{"pause"})
		if got := lastSystemText(app); !strings.Contains(got, "目标已暂停") {
			t.Fatalf("got %q", got)
		}
		engine.goalPauseResumeResp = coreapi.ThreadGoal{Status: "active"}
		app.handleGoalSlash([]string{"resume"})
		if got := lastSystemText(app); !strings.Contains(got, "目标已恢复") {
			t.Fatalf("got %q", got)
		}
		app.handleGoalSlash([]string{"clear"})
		if got := lastSystemText(app); !strings.Contains(got, "目标已清除") {
			t.Fatalf("got %q", got)
		}

		engine.goalPauseErr = errors.New("pause boom")
		app.handleGoalSlash([]string{"pause"})
		if got := lastSystemText(app); !strings.Contains(got, "pause boom") {
			t.Fatalf("got %q", got)
		}
		engine.goalPauseErr = nil
		engine.goalResumeErr = errors.New("resume boom")
		app.handleGoalSlash([]string{"resume"})
		if got := lastSystemText(app); !strings.Contains(got, "resume boom") {
			t.Fatalf("got %q", got)
		}
		engine.goalResumeErr = nil
		engine.goalClearErr = errors.New("clear boom")
		app.handleGoalSlash([]string{"clear"})
		if got := lastSystemText(app); !strings.Contains(got, "clear boom") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("unknown subcommand", func(t *testing.T) {
		app := newTestAppModel(t)
		app.handleGoalSlash([]string{"warp"})
		if got := lastSystemText(app); !strings.Contains(got, "用法") {
			t.Fatalf("got %q", got)
		}
	})
}

// ---------- /skills 与 /resume ----------

func TestHandleSkillsSlashBatch(t *testing.T) {
	setTestHome(t)

	t.Run("reload ok and error", func(t *testing.T) {
		app := newTestAppModel(t)
		// reload 成功后 handler 继续渲染 skills 列表，重载提示在前一条。
		app.handleSkillsSlash([]string{"reload"})
		if !waitForHistory(t, app, "已重载 skills") {
			t.Fatalf("reload toast not found: %q", lastSystemText(app))
		}

		engine := newTestEngine()
		engine.reloadSkillsErr = errors.New("reload boom")
		app2 := NewAppModelFromCoreEngine(engine)
		app2.handleSkillsSlash([]string{"reload"})
		if got := lastSystemText(app2); !strings.Contains(got, "reload boom") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("empty and listed skills", func(t *testing.T) {
		engine := newTestEngine()
		app := NewAppModelFromCoreEngine(engine)
		app.handleSkillsSlash(nil)
		if got := lastSystemText(app); !strings.Contains(got, "暂无可用 skills") {
			t.Fatalf("got %q", got)
		}

		engine.skills = []coreapi.SkillInfo{
			{Name: "zeta", Active: true, Description: "d1"},
			{Name: "alpha", Location: "loc", BaseDir: "base"},
		}
		app.handleSkillsSlash(nil)
		got := lastSystemText(app)
		if !strings.Contains(got, "* zeta") || !strings.Contains(got, "alpha [loc]") ||
			!strings.Contains(got, "(无描述)") {
			t.Fatalf("skills rows wrong: %q", got)
		}
	})
}

func TestHandleResumeSlashError(t *testing.T) {
	setTestHome(t)
	engine := newTestEngine()
	engine.resumeSessionErr = errors.New("resume boom")
	app := NewAppModelFromCoreEngine(engine)
	app.handleResumeSlash([]string{"s1"})
	if got := lastSystemText(app); !strings.Contains(got, "resume boom") {
		t.Fatalf("got %q", got)
	}
}

// ---------- plugin 异步命令族 ----------

func TestPluginAsyncCmdsBatch(t *testing.T) {
	setTestHome(t)

	t.Run("install needs confirm then confirmed", func(t *testing.T) {
		engine := newTestEngine()
		engine.caller = &fakeCaller{seq: map[string][]string{
			"plugin/install": {
				`{"name":"p","version":"1.0.0","needs_confirm":true,"permissions":["net","fs"]}`,
				`{"name":"p","version":"1.1.0","mcp_registered":true,"skills_installed":["s1","s2"]}`,
			},
		}}
		app := NewAppModelFromCoreEngine(engine)
		app.pluginInstallCmd("src-1")
		if !waitForHistory(t, app, "需要以下权限") {
			t.Fatalf("permission prompt not surfaced: %q", lastSystemText(app))
		}
		if app.pendingPluginConfirm != "src-1" {
			t.Fatalf("pendingPluginConfirm = %q", app.pendingPluginConfirm)
		}
		if got := lastSystemText(app); !strings.Contains(got, "net") || !strings.Contains(got, "fs") {
			t.Fatalf("permissions not listed: %q", got)
		}

		// y 确认 → clearPendingPluginConfirm + 二次调用。
		if src := app.clearPendingPluginConfirm(); src != "src-1" {
			t.Fatalf("clearPendingPluginConfirm = %q", src)
		}
		app.pluginInstallConfirm("src-1")
		// confirm 成功消息只带 MCP 徽标（skills 徽标仅在 install 直装分支）。
		if got := lastSystemText(app); !strings.Contains(got, "权限已确认") ||
			!strings.Contains(got, "MCP 已注册") {
			t.Fatalf("confirm result wrong: %q", got)
		}
	})

	t.Run("install needs confirm without permissions", func(t *testing.T) {
		engine := newTestEngine()
		engine.caller = &fakeCaller{seq: map[string][]string{
			"plugin/install": {
				`{"name":"q","version":"2.0.0","needs_confirm":true}`,
				`{"name":"q","version":"2.0.0","mcp_registered":true}`,
			},
		}}
		app := NewAppModelFromCoreEngine(engine)
		app.pluginInstallCmd("src-2")
		if !waitForHistory(t, app, "已安装") {
			t.Fatalf("direct install not surfaced: %q", lastSystemText(app))
		}
		if got := lastSystemText(app); !strings.Contains(got, "q v2.0.0 已安装") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("install plain and error", func(t *testing.T) {
		engine := newTestEngine()
		engine.caller = &fakeCaller{responses: map[string]string{
			"plugin/install": `{"name":"r","version":"3.0.0"}`,
		}}
		app := NewAppModelFromCoreEngine(engine)
		app.pluginInstallCmd("src-3")
		if !waitForHistory(t, app, "r v3.0.0 已安装") {
			t.Fatalf("plain install not surfaced: %q", lastSystemText(app))
		}

		engine.caller = &fakeCaller{err: errors.New("install rpc down")}
		app.pluginInstallCmd("src-4")
		if !waitForHistory(t, app, "安装失败") {
			t.Fatalf("install error not surfaced: %q", lastSystemText(app))
		}
	})

	t.Run("search empty results error and hit", func(t *testing.T) {
		engine := newTestEngine()
		engine.caller = &fakeCaller{responses: map[string]string{
			"plugin/search": `{"total":0}`,
		}}
		app := NewAppModelFromCoreEngine(engine)
		app.pluginSearchCmd("nothing")
		if !waitForHistory(t, app, "未找到匹配") {
			t.Fatalf("empty search not surfaced: %q", lastSystemText(app))
		}

		engine.caller = &fakeCaller{responses: map[string]string{
			"plugin/search": `{"results":[{"name":"pp","description":"dd","version":"9","author":"au","permissions":["net"]},
				{"name":"pn","description":"dn","version":"8","author":"an"}],"total":2}`,
		}}
		app.pluginSearchCmd("p")
		if !waitForHistory(t, app, "找到 2 个插件") {
			t.Fatalf("search hit not surfaced: %q", lastSystemText(app))
		}
		if got := lastSystemText(app); !strings.Contains(got, "[权限: net]") || !strings.Contains(got, "pn v8") {
			t.Fatalf("search rows wrong: %q", got)
		}

		engine.caller = &fakeCaller{err: errors.New("search rpc down")}
		app.pluginSearchCmd("x")
		if !waitForHistory(t, app, "搜索失败") {
			t.Fatalf("search error not surfaced: %q", lastSystemText(app))
		}
	})

	t.Run("remove async ok and error", func(t *testing.T) {
		engine := newTestEngine()
		engine.caller = &fakeCaller{responses: map[string]string{
			"plugin/remove": `{}`,
		}}
		app := NewAppModelFromCoreEngine(engine)
		app.pluginRemoveCmd("pp")
		if !waitForHistory(t, app, "已卸载") {
			t.Fatalf("remove ok not surfaced: %q", lastSystemText(app))
		}

		engine.caller = &fakeCaller{err: errors.New("remove rpc down")}
		app.pluginRemoveCmd("pp")
		if !waitForHistory(t, app, "卸载失败") {
			t.Fatalf("remove error not surfaced: %q", lastSystemText(app))
		}
	})
}

// ---------- msg.go 31 个 msgType marker ----------

func TestMsgTypeMarkersBatch(t *testing.T) {
	BrowserTakeoverConfirmMsg{}.msgType()
	BrowserTakeoverStartedMsg{}.msgType()
	BrowserTakeoverEndedMsg{}.msgType()
	BrowserActionMsg{}.msgType()
	BrowserDownloadDoneMsg{}.msgType()
	BrowserPickSelectedMsg{}.msgType()
	WindowSizeMsg{}.msgType()
	TickMsg{}.msgType()
	KeyMsg{}.msgType()
	MouseMsg{}.msgType()
	AIRequestMsg{}.msgType()
	AIResponseMsg{}.msgType()
	ItemStartedMsg{}.msgType()
	ItemDeltaMsg{}.msgType()
	ItemCompletedMsg{}.msgType()
	InvokeDoneMsg{}.msgType()
	PredictionUpdateMsg{}.msgType()
	ThinkingMsg{}.msgType()
	ToolCallMsg{}.msgType()
	ToolResultMsg{}.msgType()
	AgentTaskMsg{}.msgType()
	AgentFinalMsg{}.msgType()
	ModeChangedMsg{}.msgType()
	PromptRequestMsg{}.msgType()
	PromptResultMsg{}.msgType()
	PanelOpenMsg{}.msgType()
	PanelCloseMsg{}.msgType()
	PanelActionMsg{}.msgType()
	SettingsUpdateMsg{}.msgType()
	VersionCheckMsg{}.msgType()
	ErrorMsg{}.msgType()
}

// ---------- /model use 成功链 ----------

func TestHandleModelSlashUse(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.handleModelSlash([]string{"use", "default-model"})
	got := lastSystemText(app)
	if !strings.Contains(got, "已切换模型") || !strings.Contains(got, "default-model") {
		t.Fatalf("got %q", got)
	}

	// current 子命令（engine 默认 snapshot 走 activeModel）。
	app.handleModelSlash([]string{"current"})
	if got := lastSystemText(app); !strings.Contains(got, "当前模型") {
		t.Fatalf("got %q", got)
	}
}
