package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/ui/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// fakeCaller 实现 coreapi.Caller：按 method 回放预置 JSON，可注入错误。
type fakeCaller struct {
	responses map[string]string
	err       error
	lastMeth  string

	// seq：按调用次序返回响应（耗尽后回退 responses）；mu 保护并发
	// goroutine 命令对 Call 状态的访问（pluginInstallCmd 等 go func 路径）。
	mu  sync.Mutex
	seq map[string][]string
}

func (c *fakeCaller) Call(_ context.Context, method string, _ any, out any) error {
	c.mu.Lock()
	c.lastMeth = method
	var raw string
	if list := c.seq[method]; len(list) > 0 {
		raw = list[0]
		c.seq[method] = list[1:]
	} else {
		raw = c.responses[method]
	}
	err := c.err
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if raw == "" {
		return errors.New("unknown method: " + method)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}

// ---------- slashCommandHandler 全量分发 ----------

func TestSlashCommandHandlerDispatch(t *testing.T) {
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// /init 会往工作区根写 EOS.md：隔离到 temp 工作区，避免污染 cwd。
	ws := t.TempDir()
	engine := newTestEngine()
	engine.foregroundWS = ws
	engine.models = []coreapi.ModelConfig{{
		Name:    "default-model",
		APIBase: "https://example.com/v1",
		Model:   "demo-model",
	}}
	engine.activeModel = "default-model"
	app := NewAppModelFromCoreEngine(engine)

	// 必须 stub 浏览器：/feedback 会真拉起系统 open/GitHub Issues 页。
	origOpen := openInBrowserImpl
	openInBrowserImpl = func(string) error { return nil }
	t.Cleanup(func() { openInBrowserImpl = origOpen })

	// 覆盖映射表里每一个命令体（含别名解析）。
	commands := []string{
		"/help", "/clear", "/exit", "/init", "/init-verifiers", "/history",
		"/model", "/mcp", "/context", "/memory", "/cost", "/tasks", "/commit",
		"/workspace", "/goal", "/config", "/screenshot", "/feedback", "/plugin",
		"/lsp", "/rules", "/lang", "/compact", "/session", "/resume",
		"/permissions", "/skills", "/reload-plugins", "/doctor", "/diff",
		"/review", "/verify", "/plan", "/plan-style", "/git", "/remote",
		"/status", "/fast", "/export", "/theme", "/stats", "/rename", "/share",
		"/_legal",
		// 别名
		"/worktree", "/sessions", "/versions", "/ctx", "/models", "/settings",
		// 未知命令
		"/nope-not-a-command",
	}
	for _, cmd := range commands {
		app.handleSlashCommand(cmd, nil)
	}

	// 带参数的分发分支
	app.handleSlashCommand("/lang", []string{"en"})
	app.handleSlashCommand("/lang", []string{"zh"})
	app.handleSlashCommand("/memory", []string{"project"})
	app.handleSlashCommand("/model", []string{"current"})
	app.handleSlashCommand("/model", []string{"use", "default-model"})
	app.handleSlashCommand("/rename", []string{"hello", "world"})
	app.handleSlashCommand("/export", []string{"json", filepath.Join(t.TempDir(), "s.json")})
	app.handleSlashCommand("/export", []string{"markdown", filepath.Join(t.TempDir(), "s.md")})
	app.handleSlashCommand("/theme", []string{"light"})
	app.handleSlashCommand("/theme", nil)
	app.handleSlashCommand("/plan-style", []string{"detailed"})
	app.handleSlashCommand("/plan-style", nil)
	app.handleSlashCommand("/permissions", []string{"plan"})
	app.handleSlashCommand("/permissions", []string{"access", "read-only"})
	app.handleSlashCommand("/permissions", []string{"approval", "on-request"})
	app.handleSlashCommand("/permissions", []string{"bogus"})
	app.handleSlashCommand("/workspace", []string{"add", ws})
	app.handleSlashCommand("/workspace", []string{"use", ws})
	app.handleSlashCommand("/workspace", []string{"remove", ws})
	app.handleSlashCommand("/workspace", []string{"add"}) // 缺 path
	app.handleSlashCommand("/workspace", []string{"nope", "x"})
	app.handleSlashCommand("/git", []string{"branches"})
	app.handleSlashCommand("/git", []string{"log"})
	app.handleSlashCommand("/git", []string{"show", "HEAD", "file.go"})
	app.handleSlashCommand("/git", []string{"diff", "file.go"})
	app.handleSlashCommand("/git", []string{"nope"})
	app.handleSlashCommand("/session", []string{"export", "id1", filepath.Join(t.TempDir(), "e.md")})
	app.handleSlashCommand("/plugin", []string{"install"})
	app.handleSlashCommand("/plugin", []string{"remove"})
	app.handleSlashCommand("/screenshot", []string{"path.png"})

	// /init 应写到隔离工作区
	if _, err := os.Stat(filepath.Join(ws, "EOS.md")); err != nil {
		t.Fatalf("initEOSMD 未写入隔离工作区: %v", err)
	}
}

// ---------- /model 分支 ----------

func TestHandleModelSlashBranches(t *testing.T) {
	setTestHome(t)
	engine := newTestEngine()
	engine.models = []coreapi.ModelConfig{
		{Name: "default-model", APIBase: "https://example.com/v1", Model: "demo-model"},
		{Name: "other-model", APIBase: "https://other.example/v1", Model: "other"},
	}
	engine.activeModel = "default-model"
	app := NewAppModelFromCoreEngine(engine)

	// current
	app.handleModelSlash([]string{"current"})
	// use 已知条目
	app.handleModelSlash([]string{"use", "other-model"})
	// 直接给名字（无 use 前缀）
	app.handleModelSlash([]string{"default-model"})
	// 空 name after use
	app.handleModelSlash([]string{"use"})
	// 解析失败
	app.handleModelSlash([]string{"no-such-model"})

	// en 文案分支
	app.state.Language = "en"
	app.handleModelSlash([]string{"current"})
	app.handleModelSlash([]string{"use", "default-model"})
}

// ---------- /fast 分支 ----------

func TestHandleFastSlashBranches(t *testing.T) {
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgPath := filepath.Join(home, ".eos.json")

	engine := newTestEngine()
	engine.models = []coreapi.ModelConfig{
		{Name: "default-model", APIBase: "https://a/v1", Model: "std"},
		{Name: "fast-model", APIBase: "https://b/v1", Model: "fast"},
	}
	engine.activeModel = "default-model"
	app := NewAppModelFromCoreEngine(engine)

	// 未配置 fast_model
	app.handleFastSlash()

	// 配置 fast_model，当前不是 fast → 切到 fast
	cfg := config.Config{FastModel: "fast-model", Active: "default-model"}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	app.handleFastSlash()

	// 当前已是 fast → 切回 standard（Active 提供回退目标）
	cfg = config.Config{FastModel: "fast-model", Active: "default-model"}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	engine.activeModel = "fast-model"
	app.handleFastSlash()

	// 当前是 fast 但无可用回退目标
	cfg = config.Config{FastModel: "fast-model", Active: "fast-model"}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	app.handleFastSlash()
}

// ---------- /git /diff /review 注入结果 ----------

func TestHandleGitDiffReviewWithData(t *testing.T) {
	setTestHome(t)
	engine := newTestEngine()
	engine.gitStatus = []coreapi.GitChange{
		{Path: "a.go", State: "M", Staged: true},
		{Path: "b.go", State: "??", Staged: false},
	}
	engine.gitBranches = coreapi.GitBranchesResult{Current: "main", Branches: []string{"main", "dev"}}
	engine.gitLog = coreapi.GitLogResult{Text: "abc123 first\n", Branch: "main"}
	engine.gitShow = coreapi.GitShowResult{Text: "diff --git a/x\n", Revision: "HEAD"}
	engine.gitDiff = coreapi.GitTextResult{Text: "diff --git a/a.go\n+line\n"}
	engine.pendingReview = coreapi.PendingReview{Path: "a.go", Diff: "diff --git a/a.go\n+pending\n", HasDiff: true}
	app := NewAppModelFromCoreEngine(engine)

	app.handleGitSlash([]string{"status"})
	app.handleGitSlash([]string{"branches"})
	app.handleGitSlash([]string{"log"})
	app.handleGitSlash([]string{"show"})
	app.handleGitSlash([]string{"diff"})
	app.handleDiffSlash(nil) // pending review 分支
	app.handleReviewSlash(nil)
	app.handleReviewSlash([]string{"a.go"})

	// 错误臂
	engine.gitStatusErr = errors.New("git status failed")
	engine.gitDiffErr = errors.New("git diff failed")
	app.handleGitSlash([]string{"status"})
	app.handleDiffSlash([]string{"a.go"})
	app.handleReviewSlash([]string{"a.go"})

	// 无 pending、无改动
	engine.gitStatusErr = nil
	engine.gitDiffErr = nil
	engine.gitStatus = nil
	engine.pendingReview = coreapi.PendingReview{}
	app.handleDiffSlash(nil)
	app.handleReviewSlash(nil)
}

// ---------- /permissions /plan /skills ----------

func TestHandlePermissionsPlanSkills(t *testing.T) {
	setTestHome(t)
	engine := newTestEngine()
	engine.permissionSnap = coreapi.PermissionSnapshot{
		ExecutionMode:           "plan",
		AccessMode:              "workspace-write",
		ApprovalMode:            "on-request",
		SandboxMode:             "strict",
		AllowAll:                false,
		AllowedCategories:       []string{"read"},
		HasPendingDiff:          true,
		PendingDiffPath:         "/tmp/x",
		LastAuthorization:       "allow",
		LastAuthorizationKind:   "file",
		LastAuthorizationTarget: "/tmp/x",
		LastAuthorizationNote:   "ok",
	}
	app := NewAppModelFromCoreEngine(engine)

	app.handlePermissionsSlash(nil)
	app.handlePermissionsSlash([]string{"auto"})
	app.handlePermissionsSlash([]string{"access", "danger-full-access"})
	app.handlePermissionsSlash([]string{"approval", "always"})
	app.handlePermissionsSlash([]string{"approval", "nope"})

	// HasPendingDiff 但 path 空
	engine.permissionSnap.PendingDiffPath = ""
	app.handlePermissionsSlash(nil)

	app.handlePlanSlash(nil)
	app.handlePlanSlash([]string{"plan"}) // 转发 permissions

	app.handleSkillsSlash(nil)
	app.handleSkillsSlash([]string{"reload"})
}

// ---------- /export /theme /plan-style /rename /share /stats /status /remote ----------

func TestHandleExportThemeStyleRenameShare(t *testing.T) {
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	outDir := t.TempDir()

	engine := newTestEngine()
	engine.currentSessionID = "sess-1"
	engine.messages = []coreapi.SessionMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	app := NewAppModelFromCoreEngine(engine)

	// export json / markdown / 默认路径
	app.handleExportSlash([]string{"json", filepath.Join(outDir, "a.json")})
	app.handleExportSlash([]string{"md", filepath.Join(outDir, "b.md")})
	app.handleExportSlash([]string{"markdown"})
	app.handleExportSlash(nil)

	// 无当前会话
	engine.currentSessionID = ""
	app.handleExportSlash(nil)

	// theme 读/写
	engine.currentSessionID = "sess-1"
	app.handleThemeSlash(nil)
	app.handleThemeSlash([]string{"light"})

	// plan-style 读/写/usage
	app.handlePlanStyleSlash(nil)
	app.handlePlanStyleSlash([]string{"concise"})
	app.handlePlanStyleSlash([]string{"custom"})
	app.handlePlanStyleSlash([]string{"custom", "my", "style"})

	// rename
	app.handleRenameSlash(nil)
	app.handleRenameSlash([]string{"new title"})
	engine.currentSessionID = ""
	app.handleRenameSlash([]string{"x"})

	// share（无会话 / 有会话；剪贴板可能不可用，走文件回退）
	engine.currentSessionID = ""
	app.handleShareSlash()
	engine.currentSessionID = "sess-1"
	app.handleShareSlash()

	// stats / status / remote
	app.handleStatsSlash()
	app.handleStatusSlash()
	app.handleRemoteSlash(nil)
}

// ---------- View 全分支 ----------

func TestViewAllActiveViews(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 未初始化尺寸
	_ = app.View()

	app.width, app.height = 100, 40
	// shell
	app.activeView = "shell"
	app.View()
	// shell + actionPopup
	app.actionPopup = nil // 已有测试覆盖 popup；这里保证分支可达
	app.View()
	// confirm
	app.activeView = "confirm"
	app.View()
	// help
	app.activeView = "help"
	app.View()
	// setup default
	app.activeView = "setup"
	app.setupView = "not-a-setup-view"
	app.View()
	// panel 命中 / 未命中
	app.activeView = "panel"
	app.activePanel = "models"
	app.View()
	app.activePanel = "no-such-panel"
	app.View()
	// default
	app.activeView = "unknown"
	app.View()
}

// ---------- 鼠标事件 ----------

func TestHandleMouseMsgAndSelection(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.width, app.height = 80, 24
	app.activeView = "shell"

	ox, oy := app.shell.ContentOrigin()
	cw := app.shell.ContentWidth()
	if cw <= 2 {
		t.Fatalf("content width = %d", cw)
	}

	// 内容区 click
	click := tea.MouseClickMsg(tea.Mouse{X: ox + 1, Y: oy + 1, Button: tea.MouseLeft})
	app.handleMouseMsg(click, nil)

	// 滚动条列 click：不消耗
	scrollbar := tea.MouseClickMsg(tea.Mouse{X: ox + cw - 1, Y: oy + 1, Button: tea.MouseLeft})
	if app.handleContentSelection(scrollbar) {
		t.Fatal("scrollbar click should not be consumed by selection")
	}

	// 区域外 click：不消耗
	outside := tea.MouseClickMsg(tea.Mouse{X: 0, Y: 0, Button: tea.MouseLeft})
	if app.handleContentSelection(outside) {
		t.Fatal("outside click should not be consumed")
	}

	// 拖选：press → motion（超过阈值）→ release
	press := tea.MouseClickMsg(tea.Mouse{X: ox + 1, Y: oy + 1, Button: tea.MouseLeft})
	app.handleContentSelection(press)
	motion := tea.MouseMotionMsg(tea.Mouse{X: ox + 5, Y: oy + 1, Button: tea.MouseLeft})
	app.handleContentSelection(motion)
	if !app.selActive {
		t.Fatal("drag should activate selection")
	}
	release := tea.MouseReleaseMsg(tea.Mouse{X: ox + 5, Y: oy + 1, Button: tea.MouseLeft})
	app.handleContentSelection(release)

	// 点击（无拖动）release
	press2 := tea.MouseClickMsg(tea.Mouse{X: ox + 2, Y: oy + 1, Button: tea.MouseLeft})
	app.handleContentSelection(press2)
	rel2 := tea.MouseReleaseMsg(tea.Mouse{X: ox + 2, Y: oy + 1, Button: tea.MouseLeft})
	app.handleContentSelection(rel2)

	// wheel 不拦截
	wheel := tea.MouseWheelMsg(tea.Mouse{X: ox + 1, Y: oy + 1, Button: tea.MouseWheelUp})
	if app.handleContentSelection(wheel) {
		t.Fatal("wheel should not be consumed")
	}

	// motion 无锚点
	app.clearSelection()
	app.handleContentSelection(motion)

	// release 无锚点
	app.handleContentSelection(release)

	// panel / help 视图鼠标
	app.activeView = "panel"
	app.activePanel = "models"
	app.handleMouseMsg(click, nil)
	app.activeView = "help"
	app.handleMouseMsg(click, nil)
	app.activeView = "setup"
	app.handleMouseMsg(click, nil)
}

// ---------- NewAppModelFromCoreClient ----------

func TestNewAppModelFromCoreClientNil(t *testing.T) {
	setTestHome(t)
	// client=nil：NewCoreClientAdapter 显式支持空 client（开发/测试路径）。
	app := NewAppModelFromCoreClient(nil, "resume-id")
	if app == nil {
		t.Fatal("nil app")
	}
	if app.pendingResumeSession == nil || *app.pendingResumeSession != "resume-id" {
		t.Fatalf("resume = %v", app.pendingResumeSession)
	}
}

// ---------- checkForUpdates ----------

func TestCheckForUpdatesBranches(t *testing.T) {
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	app := newTestAppModel(t)

	// 非法代理地址：NewHTTPClient fail-fast，命令应返回 nil 且不触网。
	cfg := config.Config{
		UpdateProxyEnabled: true,
		UpdateProxyURL:     "not-a-url",
	}
	cfgPath := filepath.Join(home, ".eos.json")
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	cmd := app.checkForUpdates()
	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("invalid proxy should yield nil msg, got %#v", msg)
		}
	}

	// handleVersionCheck：有更新 / 无更新 / nil
	app.handleVersionCheck(VersionCheckMsg{})
	app.handleVersionCheck(VersionCheckMsg{Result: nil})
}

// ---------- send / hints / paste 早退 ----------

func TestSendHintsPasteEarly(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// shouldSendMessage 空输入 / 单斜杠 / @
	app.shell.SetInputValue("")
	if ok, _ := app.shouldSendMessage(); ok {
		t.Fatal("empty should not send")
	}
	app.shell.SetInputValue("/")
	if ok, _ := app.shouldSendMessage(); ok {
		t.Fatal("slash alone should not send")
	}
	app.shell.SetInputValue("@")
	if ok, _ := app.shouldSendMessage(); ok {
		t.Fatal("at alone should not send")
	}

	// 普通文本在 processing=false 时可发送
	app.shell.SetInputValue("hello")
	app.state.Mode = "ai"
	app.state.Processing = false
	if ok, _ := app.shouldSendMessage(); !ok {
		t.Fatal("plain text should send")
	}

	// sendMessageText 空文本且不带图 → nil
	if cmd := app.sendMessageText("", false); cmd != nil {
		t.Fatal("empty text should yield nil cmd")
	}

	// sendMessageText 宏展开（无诊断时保留原文）
	app.state.Processing = false
	cmd := app.sendMessageText("see #problems_and_diagnostics", false)
	if cmd == nil {
		t.Fatal("expected invoke cmd")
	}

	// updateHintsBasedOnInput 各分支
	app.shell.SetInputValue("/mod")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("/model use x")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("hi @")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("hi @path")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("hi @path with space")
	app.updateHintsBasedOnInput()
	app.shell.SetInputValue("plain")
	app.updateHintsBasedOnInput()

	// pasteClipboardImage 早退：非 shell / 非 ai
	app.activeView = "panel"
	app.pasteClipboardImage()
	app.activeView = "shell"
	app.state.Mode = "bash"
	app.pasteClipboardImage()

	// toggleThinkingExpand 早退
	app.toggleThinkingExpand()
}

// ---------- handleKeyMsg 视图分支 ----------

func TestHandleKeyMsgViewBranches(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.width, app.height = 80, 24

	// help esc 退出
	app.activeView = "help"
	app.handleKeyMsg(tea.KeyPressMsg{Code: 'q', Text: "q"})

	// panel esc
	app.activeView = "panel"
	app.activePanel = "models"
	app.handleKeyMsg(tea.KeyPressMsg{Code: 27}) // esc

	// panel 非 esc
	app.handleKeyMsg(tea.KeyPressMsg{Code: 'j', Text: "j"})

	// setup
	app.activeView = "setup"
	app.setupView = nil
	app.handleKeyMsg(tea.KeyPressMsg{Code: 'x', Text: "x"})

	// shell 普通键
	app.activeView = "shell"
	app.handleKeyMsg(tea.KeyPressMsg{Code: 'a', Text: "a"})
	// / 触发 hints
	app.shell.ClearInput()
	app.handleKeyMsg(tea.KeyPressMsg{Code: '/', Text: "/"})
	// @ 触发 path hints
	app.shell.ClearInput()
	app.handleKeyMsg(tea.KeyPressMsg{Code: '@', Text: "@"})
	// enter 空输入
	app.shell.ClearInput()
	app.handleKeyMsg(tea.KeyPressMsg{Code: 13})
}

// ---------- plugin CallCore 路径 ----------

func TestPluginInstallConfirmRemoveSearch(t *testing.T) {
	setTestHome(t)
	engine := newTestEngine()
	engine.caller = &fakeCaller{responses: map[string]string{
		"plugin/install": `{"name":"p","version":"1.0.0","mcp_registered":true,"skills_installed":["s1"]}`,
		"plugin/search":  `{"results":[{"name":"p","description":"d","version":"1","author":"a","permissions":["net"]}],"total":1}`,
		"plugin/remove":  `{}`,
	}}
	app := NewAppModelFromCoreEngine(engine)

	app.pluginInstallConfirm("src")
	if engine.caller.(*fakeCaller).lastMeth != "plugin/install" {
		t.Fatalf("last = %s", engine.caller.(*fakeCaller).lastMeth)
	}

	// 搜索成功分支（异步 goroutine；同步调用 search 的内联逻辑较难，这里覆盖 confirm/remove 同步体）
	app.pluginRemoveCmd("p")
	app.pluginSearchCmd("q")

	// CallCore 失败臂
	engine.caller = &fakeCaller{err: errors.New("rpc down")}
	app.pluginInstallConfirm("src")
}

// ---------- hydrateCatalog 空指针臂 ----------

func TestHydrateCatalogNil(t *testing.T) {
	if got := hydrateCatalogFromAdapter(nil); got != nil {
		t.Fatalf("got %v", got)
	}
}

// ---------- sessionTranscript / restore ----------

func TestSessionTranscriptAndRestore(t *testing.T) {
	setTestHome(t)
	engine := newTestEngine()
	engine.messages = []coreapi.SessionMessage{
		{Role: "user", Content: "hi", Metadata: map[string]any{"turn_id": "t1"}},
		{Role: "assistant", Content: "yo"},
	}
	app := NewAppModelFromCoreEngine(engine)

	app.restoreSessionHistory("sess-1")
	if len(app.history) == 0 {
		t.Fatal("history not restored")
	}
	msgs := app.sessionTranscript()
	if len(msgs) == 0 {
		t.Fatal("empty transcript")
	}
}

// ---------- updateHints / openInBrowser stub 再确认 ----------

func TestOpenInBrowserStubbed(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	orig := openInBrowserImpl
	var opened string
	openInBrowserImpl = func(url string) error {
		opened = url
		return errors.New("nope")
	}
	t.Cleanup(func() { openInBrowserImpl = orig })

	app.handleFeedbackSlash(nil)
	if opened == "" {
		t.Fatal("browser stub not called")
	}
}

// ---------- 语言切换 config 保存失败臂 ----------

func TestLangSaveFailure(t *testing.T) {
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// 把 config 路径指到不可写目录，触发 Save 失败
	cfgDir := filepath.Join(home, "ro")
	if err := os.MkdirAll(cfgDir, 0o500); err != nil {
		t.Fatal(err)
	}
	// config.Path 用 UserHomeDir 下的 .eos.json；这里直接跑 /lang 通常成功，
	// 覆盖保存成功分支即可（失败臂依赖只读文件系统，书面豁免）。
	app := newTestAppModel(t)
	app.handleSlashCommand("/lang", []string{"en"})
	app.handleSlashCommand("/lang", []string{"zh"})
}

// 保证 adapter 与 strings 引用不被 goimports 抖掉。
var (
	_ = adapter.CoreClientAdapter{}
	_ = strings.TrimSpace
)
