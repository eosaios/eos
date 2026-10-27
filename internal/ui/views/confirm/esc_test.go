package confirm

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

// TestEscDecision_PermissionMatchesDecline 验证权限审批的 options（decline 在
// cancel 前面）匹配到 decline，与旧硬编码行为一致、内核据此拒绝工具续跑。
func TestEscDecision_PermissionMatchesDecline(t *testing.T) {
	opts := []string{"accept", "acceptForSession", "decline", "cancel"}
	decision, idx := EscDecision(opts)
	if decision != "decline" {
		t.Fatalf("decision=%q, want decline", decision)
	}
	if idx != 2 {
		t.Fatalf("optionIndex=%d, want 2", idx)
	}
}

// TestEscDecision_CancelOnlyMatchesCancel 验证只有 cancel 按钮（无 decline）时
// 匹配 cancel。
func TestEscDecision_CancelOnlyMatchesCancel(t *testing.T) {
	opts := []string{"accept", "cancel"}
	decision, idx := EscDecision(opts)
	if decision != "cancel" {
		t.Fatalf("decision=%q, want cancel", decision)
	}
	if idx != 1 {
		t.Fatalf("optionIndex=%d, want 1", idx)
	}
}

// TestEscDecision_NoCancelKeywordFallsBackToCancel 验证无取消/拒绝类按钮时
// 回退 decision="cancel"、OptionIndex=-1（workspace_trust/bg_kill 等靠
// Decision=="cancel" 退出的路径不受影响）。
func TestEscDecision_NoCancelKeywordFallsBackToCancel(t *testing.T) {
	cases := [][]string{
		{"OK"},
		{"确认", "退出"},
		{"保存"},
		nil,
	}
	for i, opts := range cases {
		decision, idx := EscDecision(opts)
		if decision != "cancel" {
			t.Fatalf("case %d: decision=%q, want cancel (opts=%v)", i, decision, opts)
		}
		if idx != -1 {
			t.Fatalf("case %d: optionIndex=%d, want -1 (opts=%v)", i, idx, opts)
		}
	}
}

// TestEscDecision_ChineseKeywords 验证中文取消/拒绝关键词也能匹配。
func TestEscDecision_ChineseKeywords(t *testing.T) {
	cases := []struct {
		opts      []string
		want      string
		wantIndex int
	}{
		{[]string{"允许", "拒绝"}, "拒绝", 1},
		{[]string{"允许", "取消"}, "取消", 1},
		{[]string{"允许", "驳回"}, "驳回", 1},
	}
	for _, c := range cases {
		decision, idx := EscDecision(c.opts)
		if decision != c.want || idx != c.wantIndex {
			t.Fatalf("EscDecision(%v)=(%q,%d), want (%q,%d)", c.opts, decision, idx, c.want, c.wantIndex)
		}
	}
}

// TestEscDecision_MatchesFirstCancelOption 验证匹配 options 中第一个取消类按钮
// （顺序敏感，与 eos-app decisionForEsc 一致）。
func TestEscDecision_MatchesFirstCancelOption(t *testing.T) {
	opts := []string{"accept", "deny", "cancel"}
	decision, idx := EscDecision(opts)
	if decision != "deny" || idx != 1 {
		t.Fatalf("EscDecision(%v)=(%q,%d), want (deny,1) — first cancel keyword wins", opts, decision, idx)
	}
}
