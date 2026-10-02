package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// BridgeService 薄委托层批测：bridge_service.go 的委托面是前端 binding
// 解析层（反射分发按 BridgeService 方法集解析，子服务缺委托 = 前端
// binding not found 静默断链）。本批对零覆盖委托逐个接线验证——策略是
// 每个委托走最快安全臂（错误臂/纯状态臂为主），并在可行处断言路由副作用
// （stub 收到对应调用），下游 service 的深链由各域 gaps 测试覆盖。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// waitForEmitsToSettle 等 emit 波次落定：emitShellUpdated 的 goroutine 会
// 异步 loadBootstrap 写 HOME（tempdir），测试结束前必须等它跑完，否则
// tempdir 清理与写目录竞态（低概率 flake）。
func waitForEmitsToSettle(t *testing.T, rec *emitRecorder) {
	t.Helper()
	last := -1
	stable := 0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		now := rec.count()
		if now == last {
			stable++
			if stable >= 15 { // ~300ms 无新事件视为落定
				return
			}
		} else {
			stable = 0
			last = now
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestBridgeServiceDelegateChatSessionDomain(t *testing.T) {
	s, _, rec := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{
		resumeErr: errors.New("resume unavailable"),
	})

	// ResumeFailedTurn：网关 resume 报错的快速返回臂。
	_, _ = s.ResumeFailedTurn("")
	// SelectSession / RenameSession / DeleteSession / ArchiveSession：
	// 未知会话 ID 走各自容错臂，不 panic 即为接线正确。
	_, _ = s.SelectSession(t.TempDir(), "missing-session")
	_, _ = s.RenameSession("missing-session", "新标题")
	_, _ = s.DeleteSession(t.TempDir(), "missing-session")
	_, _ = s.ArchiveSession("missing-session", true)
	// LoadArchivedSessions：空归档列表透传（nil/空切片均可，不 panic）。
	_ = s.LoadArchivedSessions()
	waitForEmitsToSettle(t, rec)
}

func TestBridgeServiceDelegateWorkspaceRemoteDomain(t *testing.T) {
	ws := t.TempDir()
	gateway := &chatSessionsGatewayStub{
		remoteWorkspace: adapter.RemoteWorkspace{LocalPath: ws},
	}
	s, _, rec := newChatSessionsTestBridge(t, gateway)

	if _, err := s.SelectWorkspace(ws); err != nil {
		t.Fatalf("SelectWorkspace: %v", err)
	}
	if len(gateway.useCalls) != 1 || gateway.useCalls[0] != ws {
		t.Fatalf("SelectWorkspace 未路由到 use_workspace: %v", gateway.useCalls)
	}

	// 远程工作区四式：委托路由到 gateway 记录面。
	_ = s.ListRemoteWorkspaces()
	if _, err := s.OpenRemoteWorkspace("repo-1"); err != nil {
		t.Fatalf("OpenRemoteWorkspace: %v", err)
	}
	if len(gateway.openRemoteCalls) != 1 || gateway.openRemoteCalls[0] != "repo-1" {
		t.Fatalf("OpenRemoteWorkspace 路由不符: %v", gateway.openRemoteCalls)
	}
	if _, err := s.ForgetRemoteWorkspace("repo-1"); err != nil {
		t.Fatalf("ForgetRemoteWorkspace: %v", err)
	}
	if len(gateway.forgetRemoteCalls) != 1 {
		t.Fatalf("ForgetRemoteWorkspace 路由不符: %v", gateway.forgetRemoteCalls)
	}
	if _, err := s.ClearRemoteWorkspaceCache("repo-1"); err != nil {
		t.Fatalf("ClearRemoteWorkspaceCache: %v", err)
	}
	if len(gateway.clearRemoteCalls) != 1 {
		t.Fatalf("ClearRemoteWorkspaceCache 路由不符: %v", gateway.clearRemoteCalls)
	}

	// StartRemoteRepoFlow 空 URL 校验臂：不进入 SendChat 链。
	if _, err := s.StartRemoteRepoFlow(RemoteRepoFlowRequest{}); err == nil {
		t.Fatal("StartRemoteRepoFlow 空 URL 应报错")
	}
	// emit goroutine 的 loadBootstrap 会写 HOME（tempdir），锚定事件
	// 波次落定防清理竞态。
	waitForEmitsToSettle(t, rec)
}

func TestBridgeServiceDelegatePredictRefineDomain(t *testing.T) {
	gateway := &chatSessionsGatewayStub{predictText: "下一句", refineText: "润色后"}
	s, _, _ := newChatSessionsTestBridge(t, gateway)

	got, err := s.PredictNextUserMessage("草稿")
	if err != nil || got != "下一句" {
		t.Fatalf("PredictNextUserMessage = %q, %v", got, err)
	}
	got, err = s.RefineInput("草稿")
	if err != nil || got != "润色后" {
		t.Fatalf("RefineInput = %q, %v", got, err)
	}
}

func TestBridgeServiceDelegateCommandDomain(t *testing.T) {
	gateway := &chatSessionsGatewayStub{}
	s, _, rec := newChatSessionsTestBridge(t, gateway)

	// ResolvePrompt：未知 prompt 幂等成功臂（内核已旁路 resolved）。
	if _, err := s.ResolvePrompt("missing", "allow", ""); err != nil {
		t.Fatalf("ResolvePrompt 未知 prompt 应幂等成功: %v", err)
	}
	// KillTask 路由到网关。
	if _, err := s.KillTask("task-1"); err != nil {
		t.Fatalf("KillTask: %v", err)
	}
	if len(gateway.killCalls) != 1 || gateway.killCalls[0] != "task-1" {
		t.Fatalf("KillTask 路由不符: %v", gateway.killCalls)
	}
	// DismissNotification 纯状态臂（无通知时 no-op）。
	_ = s.DismissNotification("notification-1")
	// RunCommandPalette notifications.clear：清空 + 返回 bootstrap。
	if _, err := s.RunCommandPalette("notifications.clear"); err != nil {
		t.Fatalf("RunCommandPalette: %v", err)
	}
	// RunAutomationTemplate 未知模板错误臂。
	if _, err := s.RunAutomationTemplate("missing-template"); err == nil {
		t.Fatal("RunAutomationTemplate 未知模板应报错")
	}
	waitForEmitsToSettle(t, rec)
}

func TestBridgeServiceDelegateAttachmentFilesDomain(t *testing.T) {
	s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})

	// ImportAttachment 非 base64 输入 → 解码错误臂。
	if _, err := s.ImportAttachment("a.png", "image/png", "!!!not-base64!!!"); err == nil {
		t.Fatal("ImportAttachment 非法 base64 应报错")
	}
	// PreviewAttachment 空 path 校验臂。
	if _, err := s.PreviewAttachment("  "); err == nil {
		t.Fatal("PreviewAttachment 空 path 应报错")
	}
	// PreviewWorkspaceFile / ListWorkspaceDirectory：无活动工作区时
	// resolveWorkspaceFilePath 拒绝相对路径。
	if _, err := s.PreviewWorkspaceFile("rel/path.txt", 1); err == nil {
		t.Fatal("PreviewWorkspaceFile 无工作区相对路径应报错")
	}
	if _, err := s.ListWorkspaceDirectory("rel"); err == nil {
		t.Fatal("ListWorkspaceDirectory 无工作区相对路径应报错")
	}
}

func TestBridgeServiceDelegateSystemDialogDomain(t *testing.T) {
	s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})

	// web 模式五项不支持臂：必须返回明确错误而非静默成功。
	if _, err := s.OpenWorkspaceDialog(); err == nil {
		t.Fatal("OpenWorkspaceDialog web 模式应报不支持")
	}
	if _, err := s.ChooseLogDirectory(); err == nil {
		t.Fatal("ChooseLogDirectory web 模式应报不支持")
	}
	if _, err := s.ExportDiagnosticsBundle(); err == nil {
		t.Fatal("ExportDiagnosticsBundle web 模式应报不支持")
	}
	if _, err := s.SaveTextFileDialog("a.txt", "content"); err == nil {
		t.Fatal("SaveTextFileDialog web 模式应报不支持")
	}
	if _, err := s.SaveZipFileDialog("a.zip", []ZipEntry{{Name: "a.txt"}}); err == nil {
		t.Fatal("SaveZipFileDialog web 模式应报不支持")
	}
}

func TestBridgeServiceDelegateSystemFilesDomain(t *testing.T) {
	s, _, rec := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})

	// OpenLogDirectory：PATH 前置假 open 拦截真实 Finder 唤起，
	// 脚本记录参数即可断言委托确实解析了日志目录。
	fakeBin := t.TempDir()
	openCalls := filepath.Join(fakeBin, "open-calls")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> " + openCalls + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "open"), []byte(script), 0o755); err != nil {
		t.Fatalf("写假 open: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := s.OpenLogDirectory(); err != nil {
		t.Fatalf("OpenLogDirectory: %v", err)
	}
	// exec.Start 异步起进程，轮询等假脚本落记录（Start 不等退出）。
	var recorded string
	for i := 0; i < 100; i++ {
		if data, err := os.ReadFile(openCalls); err == nil && strings.TrimSpace(string(data)) != "" {
			recorded = string(data)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if recorded == "" {
		t.Fatal("假 open 未记录目录调用")
	}

	// RevealInFileManager 空 path / 相对路径无工作区两个校验臂。
	if err := s.RevealInFileManager("  "); err == nil {
		t.Fatal("RevealInFileManager 空 path 应报错")
	}
	if err := s.RevealInFileManager("rel/file.txt"); err == nil {
		t.Fatal("RevealInFileManager 无工作区相对路径应报错")
	}

	// ListExternalApps：静态目录，返回不报错且非空。
	apps, err := s.ListExternalApps()
	if err != nil || len(apps) == 0 {
		t.Fatalf("ListExternalApps = %v, %v", apps, err)
	}
	// OpenInExternalApp 未知应用 → 外部应用不可用错误臂（不执行任何真实命令）。
	if err := s.OpenInExternalApp("definitely-not-an-app", t.TempDir()); err == nil {
		t.Fatal("OpenInExternalApp 未知应用应报错")
	}

	// ReadClipboardText：web 模式服务端剪贴板恒空。
	if got := s.ReadClipboardText(); got != "" {
		t.Fatalf("ReadClipboardText web 模式应恒空, got %q", got)
	}

	// AcknowledgeCrashReport：HOME 已锚定 tempdir，无论落盘成败均应
	// 返回 bootstrap 而非 panic。
	_, _ = s.AcknowledgeCrashReport()

	// ProbeInvoke：注入 Invoke 错误走快速失败臂。
	s2, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{
		invokeErr: errors.New("invoke down"),
	})
	probe := s2.ProbeInvoke("")
	if probe.Error != "invoke down" {
		t.Fatalf("ProbeInvoke 错误臂不符: %+v", probe)
	}

	// ToggleFastMode：委托路由到设置域，返回 bootstrap 不报错。
	if _, err := s.ToggleFastMode(); err != nil {
		t.Fatalf("ToggleFastMode: %v", err)
	}
	// SetTheme / WriteClipboardText：web 模式写剪贴板恒 false，两者
	// 均退化为纯 bootstrap 返回。
	_ = s.SetTheme("dark")
	_ = s.WriteClipboardText("text")
	// GetStatus：运行时状态快照委托。
	_ = s.GetStatus()
	waitForEmitsToSettle(t, rec)
}
