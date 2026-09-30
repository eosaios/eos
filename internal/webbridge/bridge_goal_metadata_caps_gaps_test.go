package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// webbridge 批六批测：conversation metadata 纯转换全家 / goal 四操作全臂 /
// capability MCP·Skills·Plugins 写操作转发 / external_apps 目录与校验。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

// ---- stub 扩展：goal / capability 写 ----

func (g *chatSessionsGatewayStub) goalGate() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.capErr
}

func (g *chatSessionsGatewayStub) CoreGoalSetRPC(_ context.Context, req coreapi.GoalSetRequest) (coreapi.ThreadGoal, error) {
	if err := g.goalGate(); err != nil {
		return coreapi.ThreadGoal{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.goalSets = append(g.goalSets, req)
	return coreapi.ThreadGoal{GoalID: "goal-1", Status: "active", Objective: req.Objective}, nil
}

func (g *chatSessionsGatewayStub) CoreGoalPauseRPC(_ context.Context, sessionID string) (coreapi.ThreadGoal, error) {
	if err := g.goalGate(); err != nil {
		return coreapi.ThreadGoal{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.goalPaused = append(g.goalPaused, sessionID)
	return coreapi.ThreadGoal{Status: "paused"}, nil
}

func (g *chatSessionsGatewayStub) CoreGoalResumeRPC(_ context.Context, sessionID string) (coreapi.ThreadGoal, error) {
	if err := g.goalGate(); err != nil {
		return coreapi.ThreadGoal{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.goalResumed = append(g.goalResumed, sessionID)
	return coreapi.ThreadGoal{Status: "active"}, nil
}

func (g *chatSessionsGatewayStub) CoreGoalClearRPC(_ context.Context, sessionID string) error {
	if err := g.goalGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.goalCleared = append(g.goalCleared, sessionID)
	return nil
}

func (g *chatSessionsGatewayStub) CoreUpsertMCPRPC(_ context.Context, name, kind, target string, enabled bool) error {
	if err := g.goalGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mcpUpserts = append(g.mcpUpserts, [4]interface{}{name, kind, target, enabled})
	return nil
}

func (g *chatSessionsGatewayStub) CoreImportMCPJSONRPC(_ context.Context, raw string) error {
	if err := g.goalGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mcpImports = append(g.mcpImports, raw)
	return nil
}

func (g *chatSessionsGatewayStub) CoreDeleteMCPRPC(_ context.Context, name string) error {
	if err := g.goalGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mcpDeletes = append(g.mcpDeletes, name)
	return nil
}

func (g *chatSessionsGatewayStub) CoreSetMCPEnabledRPC(_ context.Context, name string, enabled bool) error {
	if err := g.goalGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mcpEnabled = append(g.mcpEnabled, [2]interface{}{name, enabled})
	return nil
}

func (g *chatSessionsGatewayStub) CoreDetectLSPRPC(_ context.Context, language string) (string, error) {
	if err := g.goalGate(); err != nil {
		return "", err
	}
	return "detected:" + language, nil
}

func (g *chatSessionsGatewayStub) CoreReloadSkillsRPC(context.Context) error { return g.goalGate() }

func (g *chatSessionsGatewayStub) CoreSetSkillEnabledRPC(_ context.Context, id string, enabled bool) error {
	if err := g.goalGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.skillEnabled = append(g.skillEnabled, [2]interface{}{id, enabled})
	return nil
}

func (g *chatSessionsGatewayStub) CoreSetPluginEnabledRPC(_ context.Context, id string, enabled bool) error {
	if err := g.goalGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pluginEnabled = append(g.pluginEnabled, [2]interface{}{id, enabled})
	return nil
}

// ---- goal 四操作 ----

func TestGoalOperationsArms(t *testing.T) {
	newGoalBridge := func(t *testing.T) (*BridgeService, *chatSessionsGatewayStub) {
		gateway := &chatSessionsGatewayStub{}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		s := &BridgeService{
			runtimeGateway: gateway,
			sessions:       map[string]*sessionState{},
			prompts:        map[string]*promptState{},
			emitEvent:      func(string, any) {},
		}
		// 不变量：currentSessionID 必须指向 sessions 里真实存在的会话，
		// 否则 LoadBootstrap（SetGoal/PauseGoal 结尾会调用）会判定当前会话
		// 不存在并把 currentSessionID 清空。workspace 也必须与会话一致。
		s.stateMu.Lock()
		s.activeWorkspace = home
		s.sessions["sess-goal"] = &sessionState{ID: "sess-goal", WorkspacePath: home}
		s.currentSessionID = "sess-goal"
		s.stateMu.Unlock()
		return s, gateway
	}

	t.Run("无活动会话拒绝", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s := &BridgeService{runtimeGateway: gateway}
		for _, check := range []struct {
			name string
			call func() (BootstrapState, error)
		}{
			{"SetGoal", func() (BootstrapState, error) { return s.SetGoal("x", nil) }},
			{"PauseGoal", s.PauseGoal},
			{"ResumeGoal", s.ResumeGoal},
			{"ClearGoal", s.ClearGoal},
		} {
			if _, err := check.call(); err == nil || !strings.Contains(err.Error(), "no active session") {
				t.Fatalf("%s error = %v", check.name, err)
			}
		}
	})

	t.Run("SetGoal 校验与转发", func(t *testing.T) {
		s, gateway := newGoalBridge(t)
		if _, err := s.SetGoal("   ", nil); err == nil || !strings.Contains(err.Error(), "must not be empty") {
			t.Fatalf("SetGoal('') error = %v", err)
		}
		budget := int64(5000)
		if _, err := s.SetGoal(" 完成重构 ", &budget); err != nil {
			t.Fatalf("SetGoal error = %v", err)
		}
		gateway.mu.Lock()
		sets := append([]coreapi.GoalSetRequest(nil), gateway.goalSets...)
		gateway.mu.Unlock()
		if len(sets) != 1 || sets[0].SessionID != "sess-goal" || sets[0].Objective != "完成重构" || sets[0].TokenBudget == nil || *sets[0].TokenBudget != 5000 {
			t.Fatalf("goalSets = %+v", sets)
		}
		gateway.capErr = errors.New("goal busy")
		if _, err := s.SetGoal("x", nil); err == nil || !strings.Contains(err.Error(), "set goal") {
			t.Fatalf("SetGoal failure error = %v", err)
		}
	})

	t.Run("Pause/Resume/Clear 转发与失败", func(t *testing.T) {
		s, gateway := newGoalBridge(t)
		if _, err := s.PauseGoal(); err != nil {
			t.Fatalf("PauseGoal error = %v", err)
		}
		if _, err := s.ResumeGoal(); err != nil {
			t.Fatalf("ResumeGoal error = %v", err)
		}
		if _, err := s.ClearGoal(); err != nil {
			t.Fatalf("ClearGoal error = %v", err)
		}
		gateway.mu.Lock()
		paused, resumed, cleared := len(gateway.goalPaused), len(gateway.goalResumed), len(gateway.goalCleared)
		gateway.mu.Unlock()
		if paused != 1 || resumed != 1 || cleared != 1 {
			t.Fatalf("goal calls = %d/%d/%d", paused, resumed, cleared)
		}
		gateway.capErr = errors.New("kernel down")
		if _, err := s.PauseGoal(); err == nil {
			t.Fatal("PauseGoal error = nil on failure")
		}
		if _, err := s.ResumeGoal(); err == nil {
			t.Fatal("ResumeGoal error = nil on failure")
		}
		if _, err := s.ClearGoal(); err == nil {
			t.Fatal("ClearGoal error = nil on failure")
		}
	})
}

// ---- capability 写操作 ----

func TestCapabilityWriteArms(t *testing.T) {
	t.Run("nil bridge 全方法", func(t *testing.T) {
		svc := NewCapabilityService(nil)
		if _, err := svc.UpsertMCP("n", "k", "t", true); err == nil {
			t.Fatal("UpsertMCP(nil) error = nil")
		}
		if _, err := svc.ImportMCPJSON("{}"); err == nil {
			t.Fatal("ImportMCPJSON(nil) error = nil")
		}
		if _, err := svc.DeleteMCP("n"); err == nil {
			t.Fatal("DeleteMCP(nil) error = nil")
		}
		if _, err := svc.SetMCPEnabled("n", true); err == nil {
			t.Fatal("SetMCPEnabled(nil) error = nil")
		}
	})

	t.Run("MCP 四操作成功与失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		s := &BridgeService{runtimeGateway: gateway, sessions: map[string]*sessionState{}, prompts: map[string]*promptState{}, emitEvent: func(string, any) {}}
		svc := NewCapabilityService(s)

		if _, err := svc.UpsertMCP("  ctx  ", "stdio", "/bin/ctx", true); err != nil {
			t.Fatalf("UpsertMCP error = %v", err)
		}
		if _, err := svc.ImportMCPJSON(`{"mcpServers":{}}`); err != nil {
			t.Fatalf("ImportMCPJSON error = %v", err)
		}
		if _, err := svc.DeleteMCP("ctx"); err != nil {
			t.Fatalf("DeleteMCP error = %v", err)
		}
		if _, err := svc.SetMCPEnabled("ctx", false); err != nil {
			t.Fatalf("SetMCPEnabled error = %v", err)
		}
		gateway.mu.Lock()
		upserts, imports, deletes, enabled := len(gateway.mcpUpserts), len(gateway.mcpImports), len(gateway.mcpDeletes), len(gateway.mcpEnabled)
		gateway.mu.Unlock()
		if upserts != 1 || imports != 1 || deletes != 1 || enabled != 1 {
			t.Fatalf("mcp calls = %d/%d/%d/%d", upserts, imports, deletes, enabled)
		}

		gateway.capErr = errors.New("store down")
		if _, err := svc.UpsertMCP("n", "k", "t", true); err == nil {
			t.Fatal("UpsertMCP error = nil on failure")
		}
		if _, err := svc.ImportMCPJSON("{}"); err == nil {
			t.Fatal("ImportMCPJSON error = nil on failure")
		}
		if _, err := svc.DeleteMCP("n"); err == nil {
			t.Fatal("DeleteMCP error = nil on failure")
		}
		if _, err := svc.SetMCPEnabled("n", true); err == nil {
			t.Fatal("SetMCPEnabled error = nil on failure")
		}
	})

	t.Run("DetectLSP 容错", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		s := &BridgeService{runtimeGateway: gateway, sessions: map[string]*sessionState{}, prompts: map[string]*promptState{}, emitEvent: func(string, any) {}}
		svc := NewCapabilityService(s)
		if got := svc.DetectLSP("  go  "); got.AppVersion == "" {
			t.Fatal("DetectLSP should return bootstrap state")
		}
		gateway.capErr = errors.New("detect failed")
		if got := svc.DetectLSP("go"); got.AppVersion == "" {
			t.Fatal("DetectLSP failure should still return bootstrap")
		}
		if got := (NewCapabilityService(nil)).DetectLSP("go"); got.CurrentSessionID != "" || got.ActiveWorkspace != "" {
			t.Fatalf("DetectLSP(nil) = %+v", got)
		}
	})

	t.Run("Skills/Plugins 开关", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		s := &BridgeService{runtimeGateway: gateway, sessions: map[string]*sessionState{}, prompts: map[string]*promptState{}, emitEvent: func(string, any) {}}
		svc := NewCapabilityService(s)

		if _, err := svc.SetSkillEnabled("skill-1", false); err != nil {
			t.Fatalf("SetSkillEnabled error = %v", err)
		}
		if _, err := svc.SetPluginEnabled("plugin-1", true); err != nil {
			t.Fatalf("SetPluginEnabled error = %v", err)
		}
		gateway.mu.Lock()
		skills, plugins := len(gateway.skillEnabled), len(gateway.pluginEnabled)
		gateway.mu.Unlock()
		if skills != 1 || plugins != 1 {
			t.Fatalf("skill/plugin calls = %d/%d", skills, plugins)
		}

		gateway.capErr = errors.New("busy")
		if _, err := svc.SetSkillEnabled("s", true); err == nil {
			t.Fatal("SetSkillEnabled error = nil on failure")
		}
		if _, err := svc.SetPluginEnabled("p", true); err == nil {
			t.Fatal("SetPluginEnabled error = nil on failure")
		}
	})
}

// ---- metadata 纯转换 ----

func TestMetadataConverters(t *testing.T) {
	t.Run("metadataChangeSet/TurnRollback 边界", func(t *testing.T) {
		if metadataChangeSet(nil) != nil {
			t.Fatal("metadataChangeSet(nil) != nil")
		}
		if metadataChangeSet(map[string]any{"id": "cs-1"}) != nil {
			t.Fatal("changeset without files should be nil")
		}
		cs := metadataChangeSet(map[string]any{"id": "cs-1", "files": []any{map[string]any{"path": "a.go"}}})
		if cs == nil || len(cs.Files) != 1 || cs.Files[0].Path != "a.go" {
			t.Fatalf("changeset = %+v", cs)
		}
		if metadataTurnRollback(nil) != nil {
			t.Fatal("metadataTurnRollback(nil) != nil")
		}
		rb := metadataTurnRollback(map[string]any{"userMessageId": "u1"})
		if rb == nil || rb.UserMessageID != "u1" {
			t.Fatalf("rollback = %+v", rb)
		}
		if metadataTurnRollback(map[string]any{}) != nil {
			t.Fatal("empty rollback should be nil")
		}
		// 不可序列化值 → nil（Marshal 失败臂）。
		if metadataChangeSet(make(chan int)) != nil {
			t.Fatal("unmarshalable changeset should be nil")
		}
		if metadataTurnRollback(make(chan int)) != nil {
			t.Fatal("unmarshalable rollback should be nil")
		}
	})

	t.Run("metadataMap/String/StringSlice", func(t *testing.T) {
		if m, ok := metadataMap(map[string]any{"a": 1}); !ok || m["a"] != 1 {
			t.Fatalf("metadataMap(map[string]any) = %v %v", m, ok)
		}
		if m, ok := metadataMap(map[string]string{"a": "x"}); !ok || m["a"] != "x" {
			t.Fatalf("metadataMap(map[string]string) = %v %v", m, ok)
		}
		if _, ok := metadataMap(42); ok {
			t.Fatal("metadataMap(int) ok = true")
		}

		if got := metadataString("  hi  "); got != "hi" {
			t.Fatalf("metadataString = %q", got)
		}
		if got := metadataString(123); got != "" {
			t.Fatalf("metadataString(int) = %q", got)
		}
		if got := metadataString(stringerValue{}); got != "stringer" {
			t.Fatalf("metadataString(Stringer) = %q", got)
		}

		if got := metadataStringSlice([]string{"a", "", " b "}); len(got) != 2 || got[1] != "b" {
			t.Fatalf("metadataStringSlice([]string) = %v", got)
		}
		if got := metadataStringSlice([]any{"a", 42, "  "}); len(got) != 1 || got[0] != "a" {
			t.Fatalf("metadataStringSlice([]any) = %v", got)
		}
		if got := metadataStringSlice("nope"); got != nil {
			t.Fatalf("metadataStringSlice(string) = %v", got)
		}
	})

	t.Run("metadataRuntimeEvents 三形态", func(t *testing.T) {
		typed := metadataRuntimeEvents([]RuntimeEvent{{ID: "e1"}})
		if len(typed) != 1 || typed[0].ID != "e1" {
			t.Fatalf("typed = %+v", typed)
		}
		fromMaps := metadataRuntimeEvents([]map[string]any{{
			"id": "e2", "type": "tool", "title": "调用工具", "status": "running",
			"durationMs": 42,
		}})
		if len(fromMaps) != 1 || fromMaps[0].ID != "e2" || fromMaps[0].Title != "调用工具" || fromMaps[0].DurationMS != 42 {
			t.Fatalf("fromMaps = %+v", fromMaps)
		}
		fromAny := metadataRuntimeEvents([]any{map[string]any{"id": "e3"}, 42})
		if len(fromAny) != 1 || fromAny[0].ID != "e3" {
			t.Fatalf("fromAny = %+v", fromAny)
		}
		if metadataRuntimeEvents(nil) != nil {
			t.Fatal("metadataRuntimeEvents(nil) != nil")
		}
		ev := runtimeEventFromMetadata(map[string]any{
			"id": "x", "type": "lifecycle", "title": "t", "detail": "d",
			"status": "completed", "timestamp": "2026", "durationMs": 7,
		})
		if ev.Type != "lifecycle" || ev.Detail != "d" || ev.Timestamp != "2026" || ev.DurationMS != 7 {
			t.Fatalf("runtimeEventFromMetadata = %+v", ev)
		}
	})

	t.Run("metadataInt64 与 Breakdown", func(t *testing.T) {
		if metadataInt64(42) != 42 || metadataInt64(int64(99)) != 99 {
			t.Fatal("int64 conversions wrong")
		}
		if metadataInt64(float64(3.5)) != 3 {
			t.Fatalf("metadataInt64(float) = %d", metadataInt64(float64(3.5)))
		}
		// metadataInt64 只认数值类型（内核这些字段固定发 JSON number）；
		// 字符串等非数值类型一律按 0 计，不做字符串数字强转。
		if metadataInt64(json.Number("12")) != 12 {
			t.Fatal("json.Number conversion wrong")
		}
		if metadataInt64("12") != 0 {
			t.Fatal("string must not be coerced to number")
		}
		if metadataInt64("bad") != 0 || metadataInt64(nil) != 0 {
			t.Fatal("invalid conversions should be 0")
		}
		bd := metadataBreakdown(map[string]any{
			"messages":      10,
			"system_tools":  20,
			"mcp_tools":     30,
			"skills":        40,
			"system_prompt": 50,
			"other":         60,
		})
		if bd == nil ||
			bd.Messages != 10 || bd.SystemTools != 20 || bd.McpTools != 30 ||
			bd.Skills != 40 || bd.SystemPrompt != 50 || bd.Other != 60 {
			t.Fatalf("breakdown = %+v", bd)
		}
		// 缺字段/非数值字段一律 0，不做字符串数字强转。
		if got := metadataBreakdown(map[string]any{"messages": "7"}); got == nil || got.Messages != 0 {
			t.Fatalf("breakdown string must not be coerced = %+v", got)
		}
		if metadataBreakdown("nope") != nil {
			t.Fatal("metadataBreakdown(string) != nil")
		}
	})

	t.Run("runtimeStringValue/Nested", func(t *testing.T) {
		payload := map[string]any{"message": " m "}
		if got := runtimeStringValue(payload, "missing", "message"); got != "m" {
			t.Fatalf("runtimeStringValue = %q", got)
		}
		if got := runtimeStringValue(payload, "missing"); got != "" {
			t.Fatalf("runtimeStringValue(missing) = %q", got)
		}
		// nested 语义 = 顶层找不到时回落到 result 子对象（不是点号路径）。
		wrapped := map[string]any{"result": map[string]any{"summary": " s "}}
		if got := runtimeNestedStringValue(wrapped, "summary"); got != "s" {
			t.Fatalf("runtimeNestedStringValue(result) = %q", got)
		}
		if got := runtimeNestedStringValue(map[string]any{"summary": " top "}, "summary"); got != "top" {
			t.Fatalf("runtimeNestedStringValue(top-level) = %q", got)
		}
		if got := runtimeNestedStringValue(map[string]any{"other": "x"}, "summary"); got != "" {
			t.Fatalf("runtimeNestedStringValue(absent) = %q", got)
		}
		// keys 是候选键名（不是点号路径），且只收集字符串元素；
		// 顶层取不到时回落到 result 子对象。
		if got := runtimeNestedStringSliceValue(map[string]any{"steps": []any{"a", 42, " b "}}, "title", "steps"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("runtimeNestedStringSliceValue(top-level) = %v", got)
		}
		wrappedSlice := map[string]any{"result": map[string]any{"steps": []string{"a", "b"}}}
		if got := runtimeNestedStringSliceValue(wrappedSlice, "title", "steps"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("runtimeNestedStringSliceValue(result) = %v", got)
		}
		if got := runtimeNestedStringSliceValue(map[string]any{"other": "x"}, "steps"); got != nil {
			t.Fatalf("runtimeNestedStringSliceValue(absent) = %v", got)
		}
	})
}

type stringerValue struct{}

func (stringerValue) String() string { return " stringer " }

// ---- external_apps 目录 ----

func TestExternalAppsCatalog(t *testing.T) {
	apps := externalAppCatalog()
	if len(apps) == 0 {
		t.Fatal("externalAppCatalog empty")
	}
	ids := map[string]bool{}
	for _, app := range apps {
		ids[app.ID] = true
		if app.Name == "" {
			t.Fatalf("app %q missing name", app.ID)
		}
	}
	if !ids["terminal"] {
		t.Fatalf("terminal app missing from catalog: %+v", apps)
	}
	// 未知 appID 底层启动失败（找不到应用）。
	if err := OpenInExternalApp("no-such-app", "."); err == nil {
		t.Fatal("OpenInExternalApp(unknown) error = nil")
	}
}
