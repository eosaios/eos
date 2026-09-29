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
