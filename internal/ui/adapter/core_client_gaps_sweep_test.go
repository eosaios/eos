package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// core_client.go 零覆盖扫尾批：nil receiver / nil engine 两轮反射扫荡
// 点亮「core client is not available」守卫臂，加上纯转换函数与导出链的
// 直调（ExportSessionMarkdown / UpsertMCPEntry / RespondPrompt 等）。

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
)

var sweepCtxType = reflect.TypeOf((*context.Context)(nil)).Elem()
var sweepErrorType = reflect.TypeOf((*error)(nil)).Elem()

// sweepCoreClientMethods 反射调用 *CoreClientAdapter 的全部导出方法。
// skip 列表放会改变生命周期或产生副作用的入口；recover 只记账不失败
// （nil receiver 未守卫属已知行为面，逐个补守卫不在本批范围）。
func sweepCoreClientMethods(t *testing.T, a *CoreClientAdapter, label string, skip map[string]bool) {
	t.Helper()
	gw := reflect.ValueOf(a)
	typ := gw.Type()
	called, panics := 0, 0
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		if m.Type.IsVariadic() || skip[m.Name] {
			continue
		}
		args := make([]reflect.Value, 0, m.Type.NumIn()-1)
		for j := 1; j < m.Type.NumIn(); j++ {
			pt := m.Type.In(j)
			if pt == sweepCtxType {
				args = append(args, reflect.ValueOf(context.Background()))
				continue
			}
			args = append(args, reflect.Zero(pt))
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					panics++
				}
			}()
			gw.Method(i).Call(args)
			called++
		}()
	}
	t.Logf("%s sweep: called=%d panics=%d", label, called, panics)
	if called < 60 {
		t.Fatalf("%s sweep called=%d 异常偏少", label, called)
	}
}

func TestCoreClientNilGuardSweeps(t *testing.T) {
	// nil engine 构造：所有守卫臂走「core client is not available」。
	a := NewCoreClientAdapterFromEngine(nil)
	defer a.Close()
	sweepCoreClientMethods(t, a, "nil-engine", map[string]bool{"Close": true})

	// nil receiver：a == nil 守卫臂。
	var nilAdapter *CoreClientAdapter
	sweepCoreClientMethods(t, nilAdapter, "nil-receiver", map[string]bool{
		"Close": true, "Events": true, // Events nil 通道无意义
	})
}

// TestCoreClientFailingEngineSweep 引擎全失败：adapter 包装层的引擎调用
// 错误臂整体点亮（failingEngine 由 coreapi 接口定义机械生成）。
func TestCoreClientFailingEngineSweep(t *testing.T) {
	a := NewCoreClientAdapterFromEngine(&failingEngine{&fakeEngine{}})
	defer a.Close()
	sweepCoreClientMethods(t, a, "failing-engine", map[string]bool{
		"Close":  true,
		"Events": true, // 订阅失败路径由事件测试单独覆盖
	})

	// 抽样断言错误确实透传。
	if _, err := a.Workspaces(context.Background()); err == nil {
		t.Fatal("Workspaces 应透传引擎错误")
	}
	if _, err := a.LoadSessionMessages(context.Background(), "s1"); err == nil {
		t.Fatal("LoadSessionMessages 应透传引擎错误")
	}
	if err := a.SetExecutionMode(context.Background(), "auto"); err == nil {
		t.Fatal("SetExecutionMode 应透传引擎错误")
	}
	if _, err := a.Invoke(context.Background(), "hi", "auto", nil, false); err == nil {
		t.Fatal("Invoke 前置链应透传引擎错误")
	}
}

// TestCoreClientStagedFailureArms 有实参 + 按服务名单选择性失败：
// 走进被零值参数输入校验拦截的引擎链错误臂（顺序链逐段点亮）。
func TestCoreClientStagedFailureArms(t *testing.T) {
	ctx := context.Background()
	newMixed := func(fail ...string) *CoreClientAdapter {
		set := map[string]bool{}
		for _, f := range fail {
			set[f] = true
		}
		a := NewCoreClientAdapterFromEngine(&mixedEngine{fakeEngine: &fakeEngine{}, fail: set})
		t.Cleanup(func() { _ = a.Close() })
		return a
	}

	// ExecuteBash：Sessions 失败（ensureSessionID 臂）与 Tools 失败（Execute 臂）。
	if _, err := newMixed("Sessions").ExecuteBash(ctx, "ls"); err == nil {
		t.Fatal("ExecuteBash 会话链失败应报错")
	}
	if _, err := newMixed("Tools").ExecuteBash(ctx, "ls"); err == nil {
		t.Fatal("ExecuteBash 工具链失败应报错")
	}
	// StartContextContext 三态：空 path 校验 / Workspaces 失败 / 成功。
	if err := newMixed().StartContextEngine(ctx, "  "); err == nil {
		t.Fatal("StartContextEngine 空 path 应报错")
	}
	if err := newMixed("Workspaces").StartContextEngine(ctx, "/ws"); err == nil {
		t.Fatal("StartContextEngine 引擎失败应报错")
	}
	if err := newMixed().StartContextEngine(ctx, "/ws"); err != nil {
		t.Fatalf("StartContextEngine 成功链: %v", err)
	}
	// ApplyAccessMode：danger-full-access 的 EnterFullAccess 臂 + 常规链
	// 逐段（Modes → Permissions.SetAccessMode → Sandbox.DerivePolicy →
	// SetPolicy → SetApprovalMode 收尾）。
	if err := newMixed("Permissions").ApplyAccessMode(ctx, "danger-full-access"); err == nil {
		t.Fatal("danger-full-access EnterFullAccess 失败应报错")
	}
	if err := newMixed().ApplyAccessMode(ctx, "danger-full-access"); err != nil {
		t.Fatalf("danger-full-access 成功链: %v", err)
	}
	if err := newMixed("Modes").ApplyAccessMode(ctx, "workspace-write"); err == nil {
		t.Fatal("Modes.SetSandboxMode 失败应报错")
	}
	if err := newMixed("Permissions").ApplyAccessMode(ctx, "workspace-write"); err == nil {
		t.Fatal("Permissions.SetAccessMode 失败应报错")
	}
	if err := newMixed("Sandbox").ApplyAccessMode(ctx, "workspace-write"); err == nil {
		t.Fatal("Sandbox.DerivePolicy 失败应报错")
	}
	if err := newMixed().ApplyAccessMode(ctx, "workspace-write"); err != nil {
		t.Fatalf("workspace-write 成功链: %v", err)
	}
	if err := newMixed().ApplyAccessMode(ctx, "  "); err != nil {
		t.Fatal("空 mode 应幂等成功")
	}
	// SyncModeSnapshots：三段非空逐段失败 + 全成功。
	if err := newMixed("Modes").SyncModeSnapshots(ctx, "s", "a", "p"); err == nil {
		t.Fatal("SyncModeSnapshots sandbox 段失败应报错")
	}
	if err := newMixed("Permissions").SyncModeSnapshots(ctx, "s", "a", "p"); err == nil {
		t.Fatal("SyncModeSnapshots access 段失败应报错")
	}
	if err := newMixed().SyncModeSnapshots(ctx, "s", "a", "p"); err != nil {
		t.Fatalf("SyncModeSnapshots 成功链: %v", err)
	}
	// Events() nil receiver 守卫臂。
	var nilAdapter *CoreClientAdapter
	if nilAdapter.Events() != nil {
		t.Fatal("nil receiver Events() 应返回 nil")
	}
}

// TestCoreClientInvokeEventLoopArms Invoke 事件循环余臂：item 段落终稿、
// rid 错配跳过、非文本 delta 跳过、订阅通道关闭收口、Turns 失败 result.err、
// nil ctx 兜底。
func TestCoreClientInvokeEventLoopArms(t *testing.T) {
	ctx := context.Background()

	pump := func(events chan protocol.Envelope, ets ...protocol.Envelope) (stop chan struct{}) {
		stop = make(chan struct{})
		go func() {
			ticker := time.NewTicker(15 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					for _, ev := range ets {
						select {
						case events <- ev:
						case <-stop:
							return
						}
					}
				}
			}
		}()
		return stop
	}

	// item 段落终稿 + 非文本 delta 跳过 + rid 错配跳过 + done 兜底 content。
	engine := &fakeEngine{}
	events := make(chan protocol.Envelope, 64)
	engine.eventsSub = &fakeEvents{ch: events}
	a := NewCoreClientAdapterFromEngine(engine)
	defer a.Close()
	stop := pump(events,
		protocol.Envelope{EventType: protocol.EventTypeItemDelta, Payload: map[string]any{"delta": "正文", "delta_type": "text"}},
		protocol.Envelope{EventType: protocol.EventTypeItemDelta, Payload: map[string]any{"delta": "思考", "delta_type": "reasoning"}},
		protocol.Envelope{RequestID: "other-turn", EventType: protocol.EventTypeItemCompleted, Payload: map[string]any{
			"item": map[string]any{"kind": "agent_message", "text": " 错配轮次的段落 "},
		}},
		protocol.Envelope{EventType: protocol.EventTypeItemCompleted, Payload: map[string]any{
			"item": map[string]any{"kind": "agent_message", "text": "  段落终稿  "},
		}},
		protocol.Envelope{EventType: protocol.EventTypeRequestDone},
	)
	out, err := a.Invoke(ctx, "q", "auto", nil, false)
	close(stop)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out != "段落终稿" {
		t.Fatalf("Invoke output = %q, want 段落终稿", out)
	}

	// 订阅者通道关闭 → 循环 !ok 收口返回已积累内容。触发路径是
	// adapter.Close()（泵退出 → notificationCh 关闭 → closeSubscribers）；
	// 引擎通道自行关闭时泵不透传关闭（生产由 15min 安全网兜底），勿用。
	engine2 := &fakeEngine{}
	events2 := make(chan protocol.Envelope, 8)
	engine2.eventsSub = &fakeEvents{ch: events2, ctxAware: true}
	a2 := NewCoreClientAdapterFromEngine(engine2)
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = a2.Close()
	}()
	if _, err := a2.Invoke(ctx, "q", "auto", nil, false); err != nil {
		t.Fatalf("订阅通道关闭 Invoke 应收口: %v", err)
	}

	// Turns 失败 → start result.err 臂；nil ctx 走 Background 兜底。
	engine3 := &fakeEngine{}
	events3 := make(chan protocol.Envelope, 8)
	engine3.eventsSub = &fakeEvents{ch: events3}
	a3 := NewCoreClientAdapterFromEngine(&mixedEngine{fakeEngine: engine3, fail: map[string]bool{"Turns": true}})
	defer a3.Close()
	if _, err := a3.Invoke(nil, "q", "auto", nil, false); err == nil {
		t.Fatal("Turns 失败 Invoke 应报错")
	}

	// done 兜底 content（无终稿只有 delta）+ failed 无载荷回落默认文案。
	engine4 := &fakeEngine{}
	events4 := make(chan protocol.Envelope, 64)
	engine4.eventsSub = &fakeEvents{ch: events4}
	a4 := NewCoreClientAdapterFromEngine(engine4)
	defer a4.Close()
	stop4 := pump(events4,
		protocol.Envelope{EventType: protocol.EventTypeItemDelta, Payload: map[string]any{"delta": "增量正文"}},
		protocol.Envelope{EventType: protocol.EventTypeRequestDone},
	)
	out4, err := a4.Invoke(ctx, "q", "auto", nil, false)
	close(stop4)
	if err != nil || out4 != "增量正文" {
		t.Fatalf("done 兜底 = %q, %v", out4, err)
	}

	engine5 := &fakeEngine{}
	events5 := make(chan protocol.Envelope, 64)
	engine5.eventsSub = &fakeEvents{ch: events5}
	a5 := NewCoreClientAdapterFromEngine(engine5)
	defer a5.Close()
	stop5 := pump(events5, protocol.Envelope{EventType: protocol.EventTypeRequestFailed})
	_, err = a5.Invoke(ctx, "q", "auto", nil, false)
	close(stop5)
	if err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("无载荷 failed 回落 = %v", err)
	}
}

func TestCoreClientConstructorsAndClientGetter(t *testing.T) {
	// NewCoreClientAdapter(nil)：无 sidecar 的空壳形态（dispatcher 直启）。
	a := NewCoreClientAdapter(nil)
	if a.Client() != nil {
		t.Fatal("nil client 构造后 Client() 应为 nil")
	}
	if a.Engine() != nil {
		t.Fatal("nil client 构造后 Engine() 应为 nil")
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// FromEngine 构造 + Client() 恒 nil（无 sidecar）。
	b := NewCoreClientAdapterFromEngine(&fakeEngine{})
	defer b.Close()
	if b.Client() != nil {
		t.Fatal("FromEngine 构造 Client() 应为 nil")
	}
}

func TestApprovalDecisionFromPromptMapping(t *testing.T) {
	cases := []struct {
		in   string
		want coreapi.ApprovalDecision
	}{
		{"accept", coreapi.ApprovalAccept},
		{" Allow_Once ", coreapi.ApprovalAccept},
		{"approve", coreapi.ApprovalAccept},
		{"AcceptForSession", coreapi.ApprovalAcceptForSession},
		{"allow_session", coreapi.ApprovalAcceptForSession},
		{"session", coreapi.ApprovalAcceptForSession},
		{"decline", coreapi.ApprovalDecline},
		{"DENY", coreapi.ApprovalDecline},
		{"cancel", coreapi.ApprovalCancel},
		{"", ""},
		{"bogus", ""},
	}
	for _, tc := range cases {
		if got := approvalDecisionFromPrompt(tc.in); got != tc.want {
			t.Fatalf("approvalDecisionFromPrompt(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestItemCompletedTextArms(t *testing.T) {
	if got := itemCompletedText(nil); got != "" {
		t.Fatalf("nil data: %q", got)
	}
	if got := itemCompletedText(map[string]any{"item": "not-a-map"}); got != "" {
		t.Fatalf("非 map item: %q", got)
	}
	if got := itemCompletedText(map[string]any{"item": map[string]any{"kind": "tool_call", "text": "x"}}); got != "" {
		t.Fatalf("非 agent_message/plan kind: %q", got)
	}
	if got := itemCompletedText(map[string]any{"item": map[string]any{"kind": "agent_message", "text": "  hello  "}}); got != "hello" {
		t.Fatalf("agent_message 文本: %q", got)
	}
	if got := itemCompletedText(map[string]any{"item": map[string]any{"kind": "plan", "text": "p"}}); got != "p" {
		t.Fatalf("plan 文本: %q", got)
	}
}

func TestRenderSessionMarkdownArms(t *testing.T) {
	out := renderSessionMarkdown(" s1 ", []coreapi.SessionMessage{
		{Role: "user", Content: " hi "},
		{Role: "", Content: "no role"},
		{Role: "assistant", Content: "   "},
	})
	if !strings.Contains(out, "# Session: s1") {
		t.Fatalf("标题不符: %q", out)
	}
	if !strings.Contains(out, "**user**: hi") {
		t.Fatalf("user 行不符: %q", out)
	}
	if !strings.Contains(out, "**unknown**: no role") {
		t.Fatalf("空 role 回落 unknown: %q", out)
	}
	if strings.Contains(out, "assistant") {
		t.Fatalf("空内容应跳过: %q", out)
	}
}

func TestExportSessionMarkdownAndMCPUpsert(t *testing.T) {
	a := NewCoreClientAdapterFromEngine(&fakeEngine{})
	defer a.Close()
	ctx := context.Background()

	// 导出校验臂。
	if err := a.ExportSessionMarkdown(ctx, "  ", "/tmp/x.md"); err == nil {
		t.Fatal("空 session id 应报错")
	}
	if err := a.ExportSessionMarkdown(ctx, "s1", "  "); err == nil {
		t.Fatal("空输出路径应报错")
	}
	// 成功链：落 tempdir 后校验渲染产物。
	dir := t.TempDir()
	outPath := dir + "/export/s1.md"
	if err := a.ExportSessionMarkdown(ctx, "s1", outPath); err != nil {
		t.Fatalf("ExportSessionMarkdown: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil || !strings.Contains(string(raw), "# Session: s1") {
		t.Fatalf("导出产物不符: %q, %v", raw, err)
	}

	// MCP upsert 成功链（转换 + 网关往返）。
	entry := config.MCPEntry{
		Name:    "fetch",
		Type:    config.MCPTypeStdio,
		Command: " mcp-fetch ",
		Args:    []string{"--x"},
		Enabled: true,
	}
	if err := a.UpsertMCPEntry(ctx, entry); err != nil {
		t.Fatalf("UpsertMCPEntry: %v", err)
	}
}

func TestRespondPromptDecisionPaths(t *testing.T) {
	a := NewCoreClientAdapterFromEngine(&fakeEngine{})
	defer a.Close()
	ctx := context.Background()

	// 空 id 幂等成功。
	if err := a.RespondPrompt(ctx, "  ", "approval", PromptResponse{}); err != nil {
		t.Fatalf("空 id: %v", err)
	}
	// inquiry 分支。
	if err := a.RespondPrompt(ctx, "q1", "inquiry", PromptResponse{Option: "opt", Text: " t "}); err != nil {
		t.Fatalf("inquiry: %v", err)
	}
	// approval 分支：decision 映射 + 空回落 decline。
	if err := a.RespondPrompt(ctx, "ap1", "approval", PromptResponse{Decision: "allow_session"}); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if err := a.RespondPrompt(ctx, "ap2", "approval", PromptResponse{Option: "approve"}); err != nil {
		t.Fatalf("approval via option: %v", err)
	}
	if err := a.RespondPrompt(ctx, "ap3", "approval", PromptResponse{}); err != nil {
		t.Fatalf("approval 默认 decline: %v", err)
	}
}
