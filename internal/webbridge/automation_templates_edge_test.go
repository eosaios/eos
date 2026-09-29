package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"strings"
	"testing"
)

func TestSaveAutomationTemplateValidation(t *testing.T) {
	withTempHome(t)
	s := &BridgeService{}
	svc := NewAutomationService(s)

	// nil bridge
	_, err := NewAutomationService(nil).SaveAutomationTemplate(AutomationSaveRequest{Title: "t", Prompt: "p"})
	if err == nil {
		t.Fatal("nil bridge")
	}

	// 缺标题
	if _, err := svc.SaveAutomationTemplate(AutomationSaveRequest{Prompt: "p"}); err == nil {
		t.Fatal("title")
	}
	// 缺 prompt
	if _, err := svc.SaveAutomationTemplate(AutomationSaveRequest{Title: "t"}); err == nil {
		t.Fatal("prompt")
	}
	// 坏 cron
	if _, err := svc.SaveAutomationTemplate(AutomationSaveRequest{Title: "t", Prompt: "p", Schedule: "bad"}); err == nil {
		t.Fatal("cron")
	}
}

func TestNewID(t *testing.T) {
	a := newID("prefix")
	if !strings.HasPrefix(a, "prefix") {
		t.Fatalf("id = %q", a)
	}
	if newID("p") == a {
		t.Fatal("unique")
	}
}

func TestVerifyCronExpressionBinding(t *testing.T) {
	s := &BridgeService{}
	svc := NewAutomationService(s)
	// VerifyCronExpression 绑定方法
	_ = svc
}

func TestGetBuildInfoAndVerifyCron(t *testing.T) {
	s := &BridgeService{}
	ok := s.VerifyCronExpression("*/5 * * * *")
	if !ok.Valid {
		t.Fatalf("valid cron = %+v", ok)
	}
	bad := s.VerifyCronExpression("bad")
	if bad.Valid {
		t.Fatalf("bad cron = %+v", bad)
	}
	if s.GetBuildInfo().AppName == "" {
		t.Fatal("build info")
	}
	if CurrentBuildMetadata().Version == "" {
		t.Fatal("metadata")
	}
}

func TestAutomationTemplateCRUD(t *testing.T) {
	withTempHome(t)
	s := &BridgeService{}
	svc := NewAutomationService(s)

	// 保存新模板
	_, err := svc.SaveAutomationTemplate(AutomationSaveRequest{
		Title: "T", Prompt: "P", Schedule: "0 0 * * *", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	all := s.allAutomationTemplatesReadOnly()
	found := false
	var id string
	for _, item := range all {
		if item.Title == "T" {
			found = true
			id = item.ID
		}
	}
	if !found {
		t.Fatalf("saved template missing: %+v", all)
	}

	// 编辑（保留 cron）
	_, err = svc.SaveAutomationTemplate(AutomationSaveRequest{
		OriginalID: id, Title: "T2", Prompt: "P2", Schedule: "0 0 * * *",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Toggle 启用（有 cron）
	if _, err := svc.ToggleAutomationTemplate(id, true); err != nil {
		t.Fatal(err)
	}
	// Toggle 空 id
	if _, err := svc.ToggleAutomationTemplate("", true); err == nil {
		t.Fatal("empty id")
	}
	// Toggle 不存在
	if _, err := svc.ToggleAutomationTemplate("nope", true); err == nil {
		t.Fatal("missing")
	}

	// 无 cron 的模板不允许启用
	_, err = svc.SaveAutomationTemplate(AutomationSaveRequest{Title: "NoCron", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range s.allAutomationTemplatesReadOnly() {
		if item.Title == "NoCron" {
			if _, err := svc.ToggleAutomationTemplate(item.ID, true); err == nil {
				t.Fatal("no cron enable")
			}
		}
	}

	// Delete
	if _, err := svc.DeleteAutomationTemplate(id); err != nil {
		t.Fatal(err)
	}
	// Delete 空
	if _, err := svc.DeleteAutomationTemplate(""); err == nil {
		t.Fatal("empty delete")
	}
	// Delete 不存在
	if _, err := svc.DeleteAutomationTemplate("nope"); err == nil {
		t.Fatal("missing delete")
	}
}
