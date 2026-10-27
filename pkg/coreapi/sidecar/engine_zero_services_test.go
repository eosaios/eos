package sidecar

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// 零覆盖 service 方法批测：fakeCaller 按 method 返回预设 JSON 或错误，
// 表驱动覆盖 extensions/browser/goal/git/permission/insight/model 族与
// Diagnostics.Startup / Caller 的成功与失败臂。

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// replyCaller 按 method 回放预设结果。
type replyCaller struct {
	replies map[string]string
	errs    map[string]error
	calls   []string
}

func (c *replyCaller) Call(_ context.Context, method string, _ any, out any) error {
	c.calls = append(c.calls, method)
	if err, ok := c.errs[method]; ok {
		return err
	}
	reply, ok := c.replies[method]
	if !ok {
		reply = `{}`
	}
	return json.Unmarshal([]byte(reply), out)
}

func newEngineWithReplies(replies map[string]string, errs map[string]error) *RemoteEngine {
	return &RemoteEngine{caller: &replyCaller{replies: replies, errs: errs}}
}

func TestRemoteEngineCallerAccessor(t *testing.T) {
	e := &RemoteEngine{}
	if e.Caller() != nil {
		t.Fatal("empty engine Caller should be nil")
	}
	caller := &replyCaller{}
	e2 := &RemoteEngine{caller: caller}
	if e2.Caller() != caller {
		t.Fatal("Caller accessor mismatch")
	}
}

func TestRemoteServicesZeroCoverageArms(t *testing.T) {
	ctx := context.Background()

	t.Run("Diagnostics.Startup", func(t *testing.T) {
		e := newEngineWithReplies(map[string]string{
			jsonrpc.MethodDiagnosticsStartup: `{"ok":true}`,
		}, nil)
		if _, err := e.Diagnostics().Startup(ctx); err != nil {
			t.Fatalf("Startup error = %v", err)
		}
		e2 := newEngineWithReplies(nil, map[string]error{
			jsonrpc.MethodDiagnosticsStartup: errors.New("boom"),
		})
		if _, err := e2.Diagnostics().Startup(ctx); err == nil {
			t.Fatal("Startup failure not surfaced")
		}
	})

	t.Run("Extensions skills/plugins/browser", func(t *testing.T) {
		e := newEngineWithReplies(map[string]string{
			jsonrpc.MethodExtensionsSkillsList:  `[{"id":"s1"}]`,
			jsonrpc.MethodExtensionsSkillInvoke: `{"name":"s1","invoked":true}`,
			jsonrpc.MethodExtensionsPluginsList: `[{"name":"p1"}]`,
			jsonrpc.MethodBrowserStatus:         `{"running":true}`,
			jsonrpc.MethodBrowserTabs:           `[{"id":"t1"}]`,
			jsonrpc.MethodBrowserProfiles:       `[{"name":"external"}]`,
		}, nil)
		ext := e.Extensions()

		skills, err := ext.ListSkills(ctx)
		if err != nil || len(skills) != 1 {
			t.Fatalf("ListSkills = %v %v", skills, err)
		}
		if err := ext.ReloadSkills(ctx); err != nil {
			t.Fatalf("ReloadSkills error = %v", err)
		}
		if err := ext.SetSkillEnabled(ctx, coreapi.SetExtensionEnabledRequest{}); err != nil {
			t.Fatalf("SetSkillEnabled error = %v", err)
		}
		result, err := ext.InvokeSkill(ctx, coreapi.InvokeSkillRequest{})
		if err != nil || !result.Invoked {
			t.Fatalf("InvokeSkill = %+v %v", result, err)
		}
		plugins, err := ext.ListPlugins(ctx)
		if err != nil || len(plugins) != 1 {
			t.Fatalf("ListPlugins = %v %v", plugins, err)
		}
		if err := ext.SetPluginEnabled(ctx, coreapi.SetExtensionEnabledRequest{}); err != nil {
			t.Fatalf("SetPluginEnabled error = %v", err)
		}

		status, err := ext.BrowserStatus(ctx)
		if err != nil || !status.Running {
			t.Fatalf("BrowserStatus = %+v %v", status, err)
		}
		if err := ext.BrowserLaunch(ctx, coreapi.BrowserLaunchRequest{}); err != nil {
			t.Fatalf("BrowserLaunch error = %v", err)
		}
		if err := ext.BrowserClose(ctx, coreapi.BrowserCloseRequest{}); err != nil {
			t.Fatalf("BrowserClose error = %v", err)
		}
		if err := ext.BrowserControlTakeover(ctx, coreapi.BrowserControlTakeoverRequest{}); err != nil {
			t.Fatalf("BrowserControlTakeover error = %v", err)
		}
		if err := ext.BrowserControlConfirm(ctx); err != nil {
			t.Fatalf("BrowserControlConfirm error = %v", err)
		}
		if err := ext.BrowserControlResume(ctx); err != nil {
			t.Fatalf("BrowserControlResume error = %v", err)
		}
		tabs, err := ext.BrowserTabs(ctx)
		if err != nil || len(tabs) != 1 {
			t.Fatalf("BrowserTabs = %v %v", tabs, err)
		}
		profiles, err := ext.BrowserProfiles(ctx)
		if err != nil || len(profiles) != 1 {
			t.Fatalf("BrowserProfiles = %v %v", profiles, err)
		}
	})

	t.Run("Extensions 失败臂", func(t *testing.T) {
		e := newEngineWithReplies(nil, map[string]error{jsonrpc.MethodExtensionsSkillsList: errors.New("down")})
		if _, err := e.Extensions().ListSkills(ctx); err == nil {
			t.Fatal("ListSkills failure not surfaced")
		}
	})

	t.Run("Git.Summary", func(t *testing.T) {
		e := newEngineWithReplies(map[string]string{
			jsonrpc.MethodGitSummary: `{"branch":"main","ahead":1}`,
		}, nil)
		summary, err := e.Git().Summary(ctx, coreapi.GitSummaryRequest{})
		if err != nil || summary.Branch != "main" {
			t.Fatalf("Summary = %+v %v", summary, err)
		}
	})

	t.Run("Goals 全族", func(t *testing.T) {
		e := newEngineWithReplies(map[string]string{
			jsonrpc.MethodGoalSet:    `{"goalId":"g1","status":"active"}`,
			jsonrpc.MethodGoalGet:    `{"goal":{"goalId":"g1"}}`,
			jsonrpc.MethodGoalPause:  `{"status":"paused"}`,
			jsonrpc.MethodGoalResume: `{"status":"active"}`,
		}, nil)
		goals := e.Goals()
		set, err := goals.Set(ctx, coreapi.GoalSetRequest{})
		if err != nil || set.GoalID != "g1" {
			t.Fatalf("Goal.Set = %+v %v", set, err)
		}
		got, err := goals.Get(ctx, coreapi.GoalRefRequest{})
		if err != nil || got.Goal == nil {
			t.Fatalf("Goal.Get = %+v %v", got, err)
		}
		paused, err := goals.Pause(ctx, coreapi.GoalRefRequest{})
		if err != nil || paused.Status != "paused" {
			t.Fatalf("Goal.Pause = %+v %v", paused, err)
		}
		resumed, err := goals.Resume(ctx, coreapi.GoalRefRequest{})
		if err != nil || resumed.Status != "active" {
			t.Fatalf("Goal.Resume = %+v %v", resumed, err)
		}
		if err := goals.Clear(ctx, coreapi.GoalRefRequest{}); err != nil {
			t.Fatalf("Goal.Clear error = %v", err)
		}
	})

	t.Run("Permissions.EnterFullAccess 与 Insight.RefineInput", func(t *testing.T) {
		e := newEngineWithReplies(map[string]string{
			jsonrpc.MethodInsightRefineInput: `{"text":"润色后"}`,
		}, nil)
		if err := e.Permissions().EnterFullAccess(ctx, coreapi.EnterFullAccessRequest{}); err != nil {
			t.Fatalf("EnterFullAccess error = %v", err)
		}
		refined, err := e.Insights().RefineInput(ctx, coreapi.RefineInputRequest{})
		if err != nil || refined != "润色后" {
			t.Fatalf("RefineInput = %q %v", refined, err)
		}
		e2 := newEngineWithReplies(nil, map[string]error{
			jsonrpc.MethodInsightRefineInput: errors.New("no model"),
		})
		if _, err := e2.Insights().RefineInput(ctx, coreapi.RefineInputRequest{}); err == nil {
			t.Fatal("RefineInput failure not surfaced")
		}
	})

	t.Run("Models 上下文族", func(t *testing.T) {
		e := newEngineWithReplies(map[string]string{
			jsonrpc.MethodModelContext: `{"global_default_name":"m1"}`,
		}, nil)
		models := e.Models()
		snapshot, err := models.Context(ctx, coreapi.ModelContextRequest{})
		if err != nil || snapshot.GlobalDefaultName != "m1" {
			t.Fatalf("Model.Context = %+v %v", snapshot, err)
		}
		if err := models.SetWorkspace(ctx, coreapi.SetWorkspaceModelRequest{}); err != nil {
			t.Fatalf("SetWorkspace error = %v", err)
		}
		if err := models.ClearWorkspace(ctx, coreapi.ClearWorkspaceModelRequest{}); err != nil {
			t.Fatalf("ClearWorkspace error = %v", err)
		}
		if err := models.SetSession(ctx, coreapi.SetSessionModelRequest{}); err != nil {
			t.Fatalf("SetSession error = %v", err)
		}
		if err := models.ClearSession(ctx, coreapi.ClearSessionModelRequest{}); err != nil {
			t.Fatalf("ClearSession error = %v", err)
		}
	})
}
