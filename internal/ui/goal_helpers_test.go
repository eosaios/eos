package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestGoalStatusLabel(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	cases := map[string]string{
		"active":        "进行中",
		"paused":        "已暂停",
		"blocked":       "已阻塞",
		"usageLimited":  "用量受限",
		"budgetLimited": "预算耗尽",
		"complete":      "已完成",
		"unknown-x":     "unknown-x",
	}
	for in, want := range cases {
		got := app.goalStatusLabel(in)
		if !strings.Contains(got, want) {
			t.Fatalf("label(%q) = %q, want contains %q", in, got, want)
		}
	}

	app.state.Language = "en"
	if !strings.Contains(app.goalStatusLabel("active"), "active") {
		t.Fatal("en label")
	}
}

func TestGoalUsageText(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 无预算
	got := app.goalUsageText(coreapi.ThreadGoal{TokensUsed: 10, TimeUsedSeconds: 5})
	if !strings.Contains(got, "10 tokens") || !strings.Contains(got, "5s") {
		t.Fatalf("usage = %q", got)
	}

	// 有预算剩余
	budget := int64(100)
	got = app.goalUsageText(coreapi.ThreadGoal{TokensUsed: 30, TokenBudget: &budget})
	if !strings.Contains(got, "70") {
		t.Fatalf("remaining = %q", got)
	}

	// 超预算 → 0
	budget2 := int64(10)
	got = app.goalUsageText(coreapi.ThreadGoal{TokensUsed: 30, TokenBudget: &budget2})
	if !strings.Contains(got, "0") {
		t.Fatalf("over budget = %q", got)
	}
}

func TestLimitSignals(t *testing.T) {
	items := []string{"a", "b", "c"}
	if got := limitSignals(items, 0); len(got) != 3 {
		t.Fatalf("max 0 = %v", got)
	}
	if got := limitSignals(items, 5); len(got) != 3 {
		t.Fatalf("max > len = %v", got)
	}
	if got := limitSignals(items, 2); len(got) != 2 {
		t.Fatalf("trunc = %v", got)
	}
}

func TestStrconvParseInt64(t *testing.T) {
	n, err := strconvParseInt64("42")
	if err != nil || n != 42 {
		t.Fatalf("parse = %d %v", n, err)
	}
	if _, err := strconvParseInt64("x"); err == nil {
		t.Fatal("bad")
	}
}

func TestHandleGoalSlash(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)

	// 无参 → get
	app.handleGoalSlash(nil)
	// set 缺目标
	app.handleGoalSlash([]string{"set"})
	// set 带目标
	app.handleGoalSlash([]string{"set", "do", "the", "thing", "budget=100"})
	// pause/resume/clear
	app.handleGoalSlash([]string{"pause"})
	app.handleGoalSlash([]string{"resume"})
	app.handleGoalSlash([]string{"clear"})
	// 未知子命令
	app.handleGoalSlash([]string{"nope"})
}
