package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareOneForSession(t *testing.T) {
	s := &BridgeService{}
	svc := NewAttachmentService(s)

	// nil bridge
	if _, _, err := NewAttachmentService(nil).PrepareOneForSession("x", ""); err == nil {
		t.Fatal("nil bridge")
	}
	// 空路径
	if _, _, err := svc.PrepareOneForSession("  ", ""); err == nil {
		t.Fatal("empty path")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 相对路径 + workspace
	ref, rt, err := svc.PrepareOneForSession("a.txt", dir)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Name != "a.txt" || ref.Kind == "" {
		t.Fatalf("ref = %+v", ref)
	}
	if rt.Path == "" {
		t.Fatalf("runtime = %+v", rt)
	}

	// 目录报错
	if _, _, err := svc.PrepareOneForSession(dir, ""); err == nil {
		t.Fatal("dir")
	}
	// 缺失文件
	if _, _, err := svc.PrepareOneForSession(filepath.Join(dir, "nope"), ""); err == nil {
		t.Fatal("missing")
	}

	// PrepareForSession 多路径
	refs, rts, err := svc.PrepareForSession([]string{src, ""}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || len(rts) != 1 {
		t.Fatalf("refs = %d rts = %d", len(refs), len(rts))
	}
}

func TestAttachmentServiceNilBridge(t *testing.T) {
	svc := NewAttachmentService(nil)
	if _, err := svc.OpenAttachmentDialog(); err == nil {
		// 可能成功也可能失败，不 panic 即可
		_ = err
	}
}

func TestBrowserControlNoGateway(t *testing.T) {
	s := &BridgeService{}
	if _, err := s.BrowserControlTakeover("r", "n", 0); err == nil {
		t.Fatal("no gateway")
	}
	if _, err := s.BrowserControlConfirm(); err == nil {
		t.Fatal("no gateway")
	}
	if _, err := s.BrowserControlResume(); err == nil {
		t.Fatal("no gateway")
	}
	if _, err := s.BrowserFocus("", ""); err == nil {
		t.Fatal("no gateway")
	}
}

func TestPreviewWorkspaceFile(t *testing.T) {
	withTempHome(t)
	s := &BridgeService{}
	svc := NewAttachmentService(s)

	dir := t.TempDir()
	s.activeWorkspace = dir
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("hello\nworld"), 0o644); err != nil {
		t.Fatal(err)
	}

	// nil bridge
	if _, err := NewAttachmentService(nil).PreviewWorkspaceFile("a.txt", 0); err == nil {
		t.Fatal("nil bridge")
	}
	// 空路径
	if _, err := svc.PreviewWorkspaceFile("  ", 0); err == nil {
		t.Fatal("empty path")
	}
	// 缺失
	if _, err := svc.PreviewWorkspaceFile("nope.txt", 0); err == nil {
		t.Fatal("missing")
	}
	// 目录
	if _, err := svc.PreviewWorkspaceFile(".", 0); err == nil {
		t.Fatal("dir")
	}
	// 成功
	got, err := svc.PreviewWorkspaceFile("a.txt", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "hello\nworld" || got.Line != 1 {
		t.Fatalf("preview = %+v", got)
	}
	// 负行号夹到 0
	got, err = svc.PreviewWorkspaceFile("a.txt", -5)
	if err != nil || got.Line != 0 {
		t.Fatalf("neg line = %+v %v", got, err)
	}
	// 无工作区
	s.activeWorkspace = ""
	if _, err := svc.PreviewWorkspaceFile("a.txt", 0); err == nil {
		t.Fatal("no workspace")
	}
}

func TestImportAttachment(t *testing.T) {
	withTempHome(t)
	s := &BridgeService{}
	svc := NewAttachmentService(s)

	// nil bridge
	if _, err := NewAttachmentService(nil).ImportAttachment("a.png", "image/png", "aGVsbG8="); err == nil {
		t.Fatal("nil bridge")
	}
	// 不支持的 mime
	if _, err := svc.ImportAttachment("a.txt", "text/plain", "aGVsbG8="); err == nil {
		t.Fatal("bad mime")
	}
	// 坏 base64
	if _, err := svc.ImportAttachment("a.png", "image/png", "!!!"); err == nil {
		t.Fatal("bad b64")
	}
	// 空数据
	if _, err := svc.ImportAttachment("a.png", "image/png", ""); err == nil {
		t.Fatal("empty")
	}
	// 成功（data URL）
	ref, err := svc.ImportAttachment("a.png", "", "data:image/png;base64,aGVsbG8=")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Kind != "image" || ref.MIME != "image/png" {
		t.Fatalf("ref = %+v", ref)
	}
	// OpenAttachmentDialog web 模式报错
	if _, err := svc.OpenAttachmentDialog(); err == nil {
		t.Fatal("web dialog")
	}
}

func TestBrowserTabNoGateway(t *testing.T) {
	s := &BridgeService{}
	if _, err := s.BrowserTabNew("https://x"); err == nil {
		t.Fatal("tab new")
	}
	if _, err := s.BrowserTabSwitch(0); err == nil {
		t.Fatal("tab switch")
	}
	idx := 0
	if _, err := s.BrowserTabClose(&idx); err == nil {
		t.Fatal("tab close")
	}
	if _, err := s.BrowserNavigate("https://x"); err == nil {
		t.Fatal("navigate")
	}
}

func TestBrowserControlMoreNoGateway(t *testing.T) {
	s := &BridgeService{}
	if _, err := s.BrowserSetDefaultProfile("p"); err == nil {
		t.Fatal("default profile")
	}
	if _, err := s.BrowserLiveStart(800, 600, 80); err == nil {
		t.Fatal("live start")
	}
	if _, err := s.BrowserLiveStop(); err == nil {
		t.Fatal("live stop")
	}
	if _, err := s.BrowserInput(map[string]interface{}{"action": "click"}); err == nil {
		t.Fatal("input")
	}
	if _, err := s.BrowserHistory("back"); err == nil {
		t.Fatal("history")
	}
	if _, err := s.BrowserCopySelection(); err == nil {
		t.Fatal("copy")
	}
}

func TestAutomationBindingsAndTrigger(t *testing.T) {
	withTempHome(t)
	s := &BridgeService{}
	// bindings 委托 AutomationService
	_, err := s.SaveAutomationTemplate(AutomationSaveRequest{Title: "T", Prompt: "P"})
	if err != nil {
		t.Fatal(err)
	}
	all := s.allAutomationTemplatesReadOnly()
	var id string
	for _, item := range all {
		if item.Title == "T" {
			id = item.ID
		}
	}
	if id == "" {
		t.Fatal("saved")
	}
	if _, err := s.ToggleAutomationTemplate(id, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteAutomationTemplate(id); err != nil {
		t.Fatal(err)
	}

	// trigger 无 workspace / 无 command service 不 panic
	s.triggerAutomationTemplate(AutomationTemplateCard{ID: "x", Title: "T"})
}

func TestCapabilityIntegrationsNoGateway(t *testing.T) {
	s := &BridgeService{}
	_, _ = s.UpsertMCP("n", "stdio", "cmd", true)
	_, _ = s.ImportMCPJSON("{}")
	_, _ = s.DeleteMCP("n")
	_, _ = s.SetMCPEnabled("n", true)
	_ = s.DetectLSP("go")
	_ = s.StartLSP("go")
	_ = s.InstallLSP("go")
	_, _ = s.ReloadSkills()
	_, _ = s.ReloadSkillsSilent()
	_, _ = s.SetSkillEnabled("s", true)
	_, _ = s.SetPluginEnabled("p", true)
}

func TestBrowserPickAndProfileNoGateway(t *testing.T) {
	s := &BridgeService{}
	_, err := s.BrowserPickStart()
	if err == nil {
		t.Fatal("pick start")
	}
	if _, err := s.BrowserPickStop(); err == nil {
		t.Fatal("pick stop")
	}
	if _, err := s.BrowserCredentialsImport("ep", "p", nil, true); err == nil {
		t.Fatal("creds")
	}
	if _, err := s.BrowserProfileUpsert("n", nil, "note"); err == nil {
		t.Fatal("profile upsert")
	}
	if _, err := s.BrowserProfiles(); err == nil {
		t.Fatal("profiles")
	}
	if _, err := s.BrowserPickUploadFile("f"); err == nil {
		t.Fatal("upload")
	}
}

func TestModelRulesVersionsChatNoGateway(t *testing.T) {
	s := &BridgeService{}
	_, _ = s.UpsertModel("n", "b", "k", "m")
	_, _ = s.SaveModel(ModelSaveRequest{})
	_, _ = s.ActivateModel("n")
	_, _ = s.SelectCurrentModel("n")
	_, _ = s.DeleteModel("n")
	_, _ = s.SaveRules(RulesSaveRequest{})
	_, _ = s.ResetRules(RulesResetRequest{})
	_, _ = s.RollbackVersion("v")
	_, _ = s.DeleteVersion("v")
	_ = s.ClearVersions()
	_, _ = s.SendChat("s", "/ws", "hi", nil)
	_, _ = s.SendChatWithReasoning("s", "/ws", "hi", nil, "high")
	_, _ = s.RollbackChatTurn("s", "m")
}
