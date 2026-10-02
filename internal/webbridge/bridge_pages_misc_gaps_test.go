package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// webbridge 零覆盖扫尾批：bridge_pages 委托面（模型/MCP/LSP/技能/规则/
// 设置/沙箱入口）、network/memory/screenshot/git-summary 等域方法、
// 生命周期（Start/Close/心跳/启动事件）、server 壳小件与纯 helper。

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// eventNames 快照已发事件名（心跳/启动波次断言用）。
func (r *emitRecorder) eventNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// delegateExtrasGateway 在 chatSessionsGatewayStub 上补齐本批需要的
// gateway 方法（stub 主体在各批 gaps 测试文件中演进，新域方法就近扩展）。
type delegateExtrasGateway struct {
	*chatSessionsGatewayStub

	networkList     coreapi.NetworkListResult
	networkListErr  error
	networkListArg  int
	networkClear    int
	networkClearErr error

	memorySaveErr error
	memorySaves   []string

	toolExecErr  error
	toolExecJSON []byte

	worktree          adapter.Worktree
	worktreeErr       error
	removeWorktreeErr error
	worktreeNames     []string

	gitSummary     coreapi.GitSummaryResult
	gitSummaryErr  error
	gitSummaryArgs []string

	verifyResp coreapi.ModelVerifyResponse
	verifyErr  error

	bashEmptyInputErr error
}

func (g *delegateExtrasGateway) CoreNetworkListRPC(_ context.Context, limit int) (coreapi.NetworkListResult, error) {
	g.networkListArg = limit
	return g.networkList, g.networkListErr
}

func (g *delegateExtrasGateway) CoreNetworkClearRPC(context.Context) (int, error) {
	return g.networkClear, g.networkClearErr
}

func (g *delegateExtrasGateway) CoreMemorySaveRPC(_ context.Context, content string) error {
	g.memorySaves = append(g.memorySaves, content)
	return g.memorySaveErr
}

func (g *delegateExtrasGateway) CoreToolExecuteRPC(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	if g.toolExecErr != nil {
		return nil, g.toolExecErr
	}
	return g.toolExecJSON, nil
}

func (g *delegateExtrasGateway) CoreCreateWorktreeRPC(_ context.Context, name string) (adapter.Worktree, error) {
	g.worktreeNames = append(g.worktreeNames, name)
	return g.worktree, g.worktreeErr
}

func (g *delegateExtrasGateway) CoreRemoveWorktreeRPC(_ context.Context, _ string, _ bool) error {
	return g.removeWorktreeErr
}

func (g *delegateExtrasGateway) CoreGitSummaryRPC(_ context.Context, workspaceRoot string) (coreapi.GitSummaryResult, error) {
	g.gitSummaryArgs = append(g.gitSummaryArgs, workspaceRoot)
	return g.gitSummary, g.gitSummaryErr
}

func (g *delegateExtrasGateway) CoreVerifyModelRPC(context.Context, adapter.ModelSaveRequest) (coreapi.ModelVerifyResponse, error) {
	return g.verifyResp, g.verifyErr
}

func newExtrasBridge(t *testing.T, extras *delegateExtrasGateway) (*BridgeService, *emitRecorder) {
	t.Helper()
	if extras.chatSessionsGatewayStub == nil {
		extras.chatSessionsGatewayStub = &chatSessionsGatewayStub{}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	rec := &emitRecorder{}
	s := &BridgeService{
		runtimeGateway:       extras,
		sessions:             map[string]*sessionState{},
		runningConversations: map[string]*runningConversationState{},
		prompts:              map[string]*promptState{},
		emitEvent:            rec.record,
	}
	return s, rec
}

func TestBridgePagesCapabilityDelegates(t *testing.T) {
	extras := &delegateExtrasGateway{
		verifyResp: coreapi.ModelVerifyResponse{Ok: true},
		worktree:   adapter.Worktree{Name: "wt-1", Path: "/tmp/wt-1"},
	}
	s, rec := newExtrasBridge(t, extras)

	// 模型域委托：验证走 gateway 往返。
	if _, err := s.VerifyModel(ModelSaveRequest{Name: "gpt"}); err != nil {
		t.Fatalf("VerifyModel: %v", err)
	}
	// 工作树两式：创建走成功链（emit），删除空路径走校验臂。
	if _, err := s.CreateWorktree("feature-x"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if len(extras.worktreeNames) != 1 || extras.worktreeNames[0] != "feature-x" {
		t.Fatalf("CreateWorktree 路由不符: %v", extras.worktreeNames)
	}
	if _, err := s.RemoveWorktree("  ", false); err == nil {
		t.Fatal("RemoveWorktree 空路径应报错")
	}
	// 信任/移除工作区委托。
	if _, err := s.TrustWorkspace(t.TempDir()); err != nil {
		t.Fatalf("TrustWorkspace: %v", err)
	}
	if _, err := s.RemoveWorkspace("/not/registered"); err != nil {
		t.Fatalf("RemoveWorkspace: %v", err)
	}
	// bash 委托：空命令校验臂（流式执行路径由 bash 域测试覆盖）。
	if _, err := s.RunBashCommand("   "); err == nil {
		t.Fatal("RunBashCommand 空命令应报错")
	}
	waitForEmitsToSettle(t, rec)
}

func TestBridgePagesSettingsSandboxDelegates(t *testing.T) {
	// SaveSettings 会按内核 config path 建目录落盘——stub 默认 /config
	// 不可写，必须指到 tempdir。
	extras := &delegateExtrasGateway{}
	extras.chatSessionsGatewayStub = &chatSessionsGatewayStub{
		configPathOverride: filepath.Join(t.TempDir(), "config", "eos.toml"),
	}
	s, rec := newExtrasBridge(t, extras)

	if _, err := s.SaveSettings(SettingsSaveRequest{Language: "zh"}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	// 执行模式委托：返回 bootstrap，不 panic 即接线正确（字段形状由设置域测试固化）。
	_ = s.SetExecutionMode("auto")
	// 审批模式 UI 入口：透传内核 + 回 bootstrap（失败也返回 bootstrap）。
	_ = s.SetApprovalModeForUI("on-request")
	// 沙箱模式 UI 入口：read-only 档 + 会话空时跳过持久化。
	if _, err := s.SetSandboxModeForUI("read-only"); err != nil {
		t.Fatalf("SetSandboxModeForUI: %v", err)
	}
	// 完全访问入口：danger-full-access 复合态。
	if _, err := s.EnterFullAccessForUI(); err != nil {
		t.Fatalf("EnterFullAccessForUI: %v", err)
	}
	// 推理档位委托。
	if _, err := s.SetReasoningLevel("high"); err != nil {
		t.Fatalf("SetReasoningLevel: %v", err)
	}
	waitForEmitsToSettle(t, rec)
}

func TestNetworkMemoryScreenshotGitSummaryDelegates(t *testing.T) {
	extras := &delegateExtrasGateway{
		networkList: coreapi.NetworkListResult{Enabled: true},
		networkClear: 3,
		gitSummary: coreapi.GitSummaryResult{
			Branch: "main",
			Changes: []coreapi.GitChange{
				{Path: "a.go", State: "modified", Staged: true},
				{Path: " b.go ", State: " added "},
			},
		},
	}
	s, rec := newExtrasBridge(t, extras)

	list, err := s.NetworkInspect(5)
	if err != nil || !list.Enabled {
		t.Fatalf("NetworkInspect = %+v, %v", list, err)
	}
	if extras.networkListArg != 5 {
		t.Fatalf("NetworkInspect limit 透传不符: %d", extras.networkListArg)
	}
	cleared, err := s.NetworkClear()
	if err != nil || cleared != 3 {
		t.Fatalf("NetworkClear = %d, %v", cleared, err)
	}

	if _, err := s.SaveMemoryNote("  记忆笔记  "); err != nil {
		t.Fatalf("SaveMemoryNote: %v", err)
	}
	if len(extras.memorySaves) != 1 || extras.memorySaves[0] != "记忆笔记" {
		t.Fatalf("SaveMemoryNote 内容未 trim 透传: %q", extras.memorySaves)
	}
	if _, err := s.SaveMemoryNote("   "); err == nil {
		t.Fatal("SaveMemoryNote 空内容应报错")
	}

	// 截图委托：工具执行失败快速返回。
	extras.toolExecErr = errors.New("screencap denied")
	if _, err := s.CaptureScreenshot(""); err == nil {
		t.Fatal("CaptureScreenshot 失败臂应报错")
	}

	// git 概览：成功映射（trim 透传）+ 空网关双臂。
	summary := s.GetWorkspaceGitSummary(" /ws ")
	if !summary.Ok || len(summary.Changes) != 2 {
		t.Fatalf("GetWorkspaceGitSummary = %+v", summary)
	}
	if summary.Changes[1].Path != "b.go" || summary.Changes[1].State != "added" {
		t.Fatalf("changes 未 trim: %+v", summary.Changes[1])
	}
	extras.gitSummaryErr = errors.New("not a repo")
	if got := s.GetWorkspaceGitSummary(""); got.Ok {
		t.Fatalf("GetWorkspaceGitSummary 失败臂应 Ok=false: %+v", got)
	}
	bare := &BridgeService{}
	if got := bare.GetWorkspaceGitSummary("x"); got.Ok {
		t.Fatal("无网关时 git 概览应为空")
	}
	waitForEmitsToSettle(t, rec)
}

func TestLifecycleStartCloseAndEmitters(t *testing.T) {
	gateway := &delegateExtrasGateway{}
	s, rec := newExtrasBridge(t, gateway)
	s.stopCh = make(chan struct{})
	s.terminalSessions = map[string]*terminalSessionHandle{}

	// 启动事件：注入 emitter 后首发 shell-updated，1s/3s 重试被 stopCh 掐掉。
	s.emitStartupBootstrap()
	deadline := time.Now().Add(3 * time.Second)
	for rec.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if rec.count() == 0 {
		t.Fatal("emitStartupBootstrap 未发出首波事件")
	}

	// 心跳：1s 周期，emitter 记录 heartbeat 事件后关闭。
	heartbeatSeen := func() bool {
		for _, name := range rec.eventNames() {
			if name == heartbeatEventName {
				return true
			}
		}
		return false
	}
	go s.emitHeartbeat()
	deadline = time.Now().Add(3 * time.Second)
	for !heartbeatSeen() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !heartbeatSeen() {
		t.Fatal("emitHeartbeat 未发出心跳事件")
	}

	// 任务监看：nil stopCh 臂 + 正常入口（goroutine 由 stopCh 收口）。
	(&BridgeService{}).startTaskWatcher()
	s.startTaskWatcher()

	// Start 全链 + Close 收口：不 panic 即接线正确（订阅走 stub eventCh）。
	s.Start()
	s.Close()
	// Close 幂等性由 stopOnce 保证，二次调用不应 panic。
	s.Close()
}

func TestStayInTrayAndPromptTimeoutSync(t *testing.T) {
	s, _ := newExtrasBridge(t, &delegateExtrasGateway{})
	if !s.StayInTrayEnabled() {
		t.Fatal("默认驻留托盘应为开")
	}
	// 读取失败臂：configPath 指向不可读位置仍应回落默认开。
	s.syncPromptTimeoutAtStartup() // 不 panic 即可
}

func TestServerShellHelpersAndPureFunctions(t *testing.T) {
	// server 壳小件。
	if webLang() != "zh" {
		t.Fatalf("默认语言应为 zh: %q", webLang())
	}
	t.Setenv("EOS_LANG", "EN")
	if webLang() != "en" {
		t.Fatalf("EOS_LANG=EN 应归一 en: %q", webLang())
	}
	if msg := webServerReadyMessage("http://127.0.0.1:0", "/ui"); msg == "" {
		t.Fatal("webServerReadyMessage 应有文案")
	}
	if msg := webServerShutdownMessage(); msg == "" {
		t.Fatal("webServerShutdownMessage 应有文案")
	}
	if runtimeGOOS() == "" {
		t.Fatal("runtimeGOOS 不应为空")
	}

	// 纯 helper。
	if isDialogCancelledError(nil) || isDialogCancelledError(errors.New("boom")) {
		t.Fatal("isDialogCancelledError 只认 cancelled by user")
	}
	if !isDialogCancelledError(errors.New(" Cancelled By User ")) {
		t.Fatal("isDialogCancelledError 应大小写不敏感")
	}
	if got := runtimeToolTitle(map[string]any{"tool_name": "shell"}, "工具"); got != "工具：shell" {
		t.Fatalf("runtimeToolTitle = %q", got)
	}
	if got := runtimeToolTitle(map[string]any{}, "fallback"); got != "fallback" {
		t.Fatalf("runtimeToolTitle 兜底不符: %q", got)
	}
	if got := splitBridgeRuntimeGatewayArgs("  a\tb  "); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitBridgeRuntimeGatewayArgs = %v", got)
	}
	if splitBridgeRuntimeGatewayArgs("   ") != nil {
		t.Fatal("空参数应为 nil")
	}
	if got := toCountLabel(-1); got != "0" {
		t.Fatalf("toCountLabel(-1) = %q", got)
	}
	if got := toCountLabel(7); got != "7" {
		t.Fatalf("toCountLabel(7) = %q", got)
	}
	if got := injectWebRuntimeMarker("<html><head></head></html>"); !strings.Contains(got, webRuntimeMarker) {
		t.Fatalf("injectWebRuntimeMarker 未注入标记: %q", got)
	}
	if got := injectWebRuntimeMarker("<html>no head</html>"); strings.Contains(got, webRuntimeMarker) {
		t.Fatalf("无 head 时应原样返回: %q", got)
	}

	// env 组装与超时解析。
	t.Setenv("EOS_GUI_RUNTIME_GATEWAY", "rust")
	t.Setenv("EOS_GUI_CORE_PATH", "/custom/core")
	t.Setenv("EOS_GUI_APP_SERVER_START_TIMEOUT_MS", "2500")
	opts := defaultBridgeServiceOptions(" log ", " /ws ")
	if opts.RuntimeGateway != "rust" || opts.CorePath != "/custom/core" {
		t.Fatalf("defaultBridgeServiceOptions = %+v", opts)
	}
	if got := bridgeRuntimeGatewayStartTimeoutFromEnv(); got != 2500*time.Millisecond {
		t.Fatalf("start timeout = %v", got)
	}
	t.Setenv("EOS_GUI_APP_SERVER_START_TIMEOUT_MS", "bogus")
	if got := bridgeRuntimeGatewayStartTimeoutFromEnv(); got != defaultBridgeRuntimeGatewayStartTimeout {
		t.Fatalf("非法超时应回落默认: %v", got)
	}
	t.Setenv("EOS_GUI_APP_SERVER_START_TIMEOUT_MS", "")
	if got := bridgeRuntimeGatewayStartTimeoutFromEnv(); got != defaultBridgeRuntimeGatewayStartTimeout {
		t.Fatalf("空超时应回落默认: %v", got)
	}

	// 任务卡指纹：字段拼接，状态/可杀性/文案变化都应改指纹。
	base := taskCardsFingerprint([]TaskCard{{ID: "t1", Status: "running", Detail: "d", UpdatedAt: "now"}})
	if base == "" {
		t.Fatal("taskCardsFingerprint 不应为空")
	}
	if taskCardsFingerprint([]TaskCard{{ID: "t1", Status: "done", Detail: "d", UpdatedAt: "now"}}) == base {
		t.Fatal("状态变化应改变指纹")
	}
	if taskCardsFingerprint([]TaskCard{{ID: "t1", Status: "running", CanKill: true, Detail: "d", UpdatedAt: "now"}}) == base {
		t.Fatal("可杀性变化应改变指纹")
	}
	if taskCardsFingerprint(nil) == base {
		t.Fatal("空卡列表指纹应不同")
	}

	// 退出标志（包级状态，置位后由后续读取消费，无需复位——语义是单向退出）。
	if IsShutdownRequested() {
		t.Skip("包级退出标志已被环境置位，跳过断言")
	}
	RequestShutdown()
	if !IsShutdownRequested() {
		t.Fatal("RequestShutdown 后标志应为真")
	}
}

func TestFakeOpenInterceptedCommands(t *testing.T) {
	fakeBin := t.TempDir()
	callsFile := filepath.Join(fakeBin, "calls")
	script := "#!/bin/sh\nprintf '%s %s\\n' \"$0\" \"$*\" >> " + callsFile + "\nexit 0\n"
	for _, name := range []string{"open", "xdg-open"} {
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0o755); err != nil {
			t.Fatalf("写假 %s: %v", name, err)
		}
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	waitCalls := func(t *testing.T, substr string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if data, err := os.ReadFile(callsFile); err == nil && strings.Contains(string(data), substr) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("假命令未拦截到 %q", substr)
	}

	// 反馈入口：https 成功臂 + 非 https/空校验臂。
	s, _ := newExtrasBridge(t, &delegateExtrasGateway{})
	if err := s.OpenExternalURL("https://example.com/feedback"); err != nil {
		t.Fatalf("OpenExternalURL https 臂: %v", err)
	}
	waitCalls(t, "https://example.com/feedback")
	if err := s.OpenExternalURL(""); err == nil {
		t.Fatal("OpenExternalURL 空 URL 应报错")
	}
	if err := s.OpenExternalURL("file:///etc/passwd"); err == nil {
		t.Fatal("OpenExternalURL 非 https 应拒绝")
	}

	// server 壳：后台命令 + openBrowser 均走 PATH 解析。
	if err := runBackgroundCommand("open", "https://example.com"); err != nil {
		t.Fatalf("runBackgroundCommand: %v", err)
	}
	waitCalls(t, "https://example.com")
	openBrowser("http://127.0.0.1:1")
	waitCalls(t, "http://127.0.0.1:1")

	// 定位核心：darwin 走 open <dir>（无 MkdirAll）。
	dir := t.TempDir()
	if err := openDirectoryNoMkdir(dir); err != nil {
		t.Fatalf("openDirectoryNoMkdir: %v", err)
	}
	waitCalls(t, dir)

	// 系统终端打开：darwin 走 open -a Terminal。
	if err := openTerminalApp(dir); err != nil {
		t.Fatalf("openTerminalApp: %v", err)
	}
	waitCalls(t, dir)
}

func TestExternalAppCatalogAndProbes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	apps := externalAppCatalog()
	if len(apps) == 0 {
		t.Fatal("外部应用目录不应为空")
	}
	if !lookPathExists("sh") {
		t.Fatal("PATH 里应能找到 sh")
	}
	if lookPathExists("definitely-not-a-command-xyz") {
		t.Fatal("不存在的命令不应命中")
	}

	// darwin 安装探测：HOME 级 Applications 下放置 bundle 即命中。
	userApps := filepath.Join(home, "Applications")
	if err := os.MkdirAll(filepath.Join(userApps, "Cursor.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, app := range externalAppCatalog() {
		if app.ID == "cursor" && app.Installed {
			found = true
		}
	}
	if !found {
		t.Fatal("Cursor 放入 ~/Applications 后应探测为已安装")
	}

	// windows 探测函数在 darwin 上的行为固化：cache 目录下命中。
	cache := filepath.Join(home, "Library", "Caches", "Programs", "Trae", "Trae.exe")
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte("exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := windowsInstalledExe(externalAppSpecs[1]); got != cache {
		t.Fatalf("windowsInstalledExe = %q, want %q", got, cache)
	}
	if got := windowsInstalledExe(externalAppSpecs[4]); got != "" {
		t.Fatalf("无 windows 版本的应用应恒空: %q", got)
	}
	// darwin 上无 x-terminal-emulator 系命令 → nil。
	if linuxTerminalCommand("") != nil {
		t.Fatal("darwin 上 linuxTerminalCommand 应为 nil")
	}
}

func TestTerminalShellAndControlDelegates(t *testing.T) {
	s, rec := newExtrasBridge(t, &delegateExtrasGateway{})
	s.terminalSessions = map[string]*terminalSessionHandle{}

	// shell 探测委托：darwin 恒 /bin/bash 可用，缓存后失效重探。
	status := s.GetTerminalShellStatus()
	if !status.Available || status.Path != "/bin/bash" {
		t.Fatalf("GetTerminalShellStatus = %+v", status)
	}
	s.terminalShellInvalidate()

	// 终端控制四式的校验/不存在臂。
	if _, err := s.CreateTerminalSession("  "); err == nil {
		t.Fatal("CreateTerminalSession 空工作区应报错")
	}
	if err := s.WriteTerminalInput("", "data"); err == nil {
		t.Fatal("WriteTerminalInput 空会话 ID 应报错")
	}
	if err := s.ResizeTerminalSession("nope", 80, 24); err == nil {
		t.Fatal("ResizeTerminalSession 未知会话应报错")
	}
	if _, err := s.CloseTerminalSession("nope"); err == nil {
		t.Fatal("CloseTerminalSession 未知会话应报错")
	}
	waitForEmitsToSettle(t, rec)
}

func TestRunRejectsMissingUIDirEarly(t *testing.T) {
	// Run 的 UIDir 校验臂：在任何内核装配之前快速失败。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := Run(ctx, ServerOptions{UIDir: "/definitely/not/a/ui/dir", NoOpenBrowser: true})
	if err == nil || !strings.Contains(err.Error(), "web ui dir") {
		t.Fatalf("Run 缺失 UI 目录应快速报错: %v", err)
	}
}

func TestStdioGatewayStartErrorArm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// 显式 CorePath 不存在 + 空 CoreBinDir：解析在给定根内失败，
	// 不会触碰 ~/.eos/core 真内核。
	emptyRoot := t.TempDir()
	_, closeFn, _, err := startBridgeStdioGatewayProcess(context.Background(), adapter.StdioClientOptions{
		CorePath:     filepath.Join(emptyRoot, "missing-eos-core"),
		CoreBinDir:   emptyRoot,
		Workspace:    home,
		StoreDir:     filepath.Join(home, ".eos", "core"),
		SandboxMode:  "workspace-write",
	})
	if err == nil {
		if closeFn != nil {
			_ = closeFn()
		}
		t.Fatal("不存在的 CorePath 应报错")
	}
}

func TestUpdateDownloadDirAndDevStateDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := updateDownloadDir()
	if !strings.Contains(dir, "eos") {
		t.Fatalf("updateDownloadDir = %q", dir)
	}
	if dev := DevStateDir(); dev == "" || !strings.Contains(dev, ".tmp") {
		t.Fatalf("DevStateDir = %q", dev)
	}
}

func TestReadOnlyProjectionsAndWorktreeRemove(t *testing.T) {
	extras := &delegateExtrasGateway{}
	s, rec := newExtrasBridge(t, extras)

	// 诊断报告：多域只读快照拼接（stub 空值回落占位文案）。
	report := s.buildDiagnosticsReport()
	if !strings.Contains(report, "运行时诊断") || !strings.Contains(report, "暂无日志内容") {
		t.Fatalf("buildDiagnosticsReport 形状不符:\n%s", report)
	}
	// 启动诊断只读投影。
	_ = s.coreStartupDiagnosticsReadOnly()
	// 会话工作区解析：空结果透传 + 无网关臂。
	if got := s.resolveSessionWorkspaceReadOnly("s1"); got != "" {
		t.Fatalf("resolveSessionWorkspaceReadOnly = %q", got)
	}
	if got := (&BridgeService{}).resolveSessionWorkspaceReadOnly("x"); got != "" {
		t.Fatal("无网关时会话工作区解析应为空")
	}
	// 工作树删除成功链（removeWorktreeRPC 往返）。
	if _, err := s.RemoveWorktree("/tmp/wt-1", true); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	waitForEmitsToSettle(t, rec)
}

func TestDarwinInstallChainWithFakeBins(t *testing.T) {
	fakeBin := t.TempDir()
	mountPoint := t.TempDir()
	appBundle := filepath.Join(mountPoint, "EOS.app")
	if err := os.MkdirAll(appBundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appBundle, "new-marker"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 假 hdiutil：attach 弹 mount-point plist，detach 记账。
	hdiutilCalls := filepath.Join(fakeBin, "hdiutil-calls")
	hdiutil := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + hdiutilCalls + "\n" +
		"if [ \"$1\" = attach ]; then\n" +
		"  printf '<?xml><dict><key>mount-point</key><string>" + mountPoint + "</string></dict>'\n" +
		"fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "hdiutil"), []byte(hdiutil), 0o755); err != nil {
		t.Fatal(err)
	}
	// 假 ditto：目录级复制（真实 ditto 语义的足够子集）。
	ditto := "#!/bin/sh\nmkdir -p \"$2\" && cp -R \"$1\"/. \"$2\"/\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "ditto"), []byte(ditto), 0o755); err != nil {
		t.Fatal(err)
	}
	// 假 open：relaunch 调度断言。
	openCalls := filepath.Join(fakeBin, "open-calls")
	openScript := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + openCalls + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "open"), []byte(openScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	// 挂载：plist 解析回假挂载点。
	got, err := mountMacOSDmg("/fake/eos.dmg")
	if err != nil || got != mountPoint {
		t.Fatalf("mountMacOSDmg = %q, %v", got, err)
	}
	detachMacOSDmg(mountPoint)

	// swap 全链：旧 bundle 原地替换为 dmg 内新 bundle。
	target := filepath.Join(t.TempDir(), "EOS.app")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old-marker"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := swapMacOSBundle("/fake/eos.dmg", target); err != nil {
		t.Fatalf("swapMacOSBundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "new-marker")); err != nil {
		t.Fatalf("替换后应存在新标记: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "old-marker")); !os.IsNotExist(err) {
		t.Fatal("旧内容不应残留")
	}

	// relaunch 调度：1s 后经 PATH 命中假 open。
	bundle := filepath.Join(t.TempDir(), "EOS.app")
	scheduleMacOSRelaunch(bundle)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(openCalls); err == nil && strings.Contains(string(data), bundle) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("scheduleMacOSRelaunch 未触发假 open")
}
