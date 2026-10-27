package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// webbridge 批十批测：capability 模型域（upsert/save/activate/select/delete
// 与三级模型上下文写）/ 规则落盘（global/workspace/空内容模板）/ 版本操作。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// ---- stub 扩展：模型与版本写 ----

func (g *chatSessionsGatewayStub) modelGate() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.modelErr
}

func (g *chatSessionsGatewayStub) CoreUpsertModelRPC(_ context.Context, name, base, key, model string) error {
	if err := g.modelGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.modelUpserts = append(g.modelUpserts, [4]string{name, base, key, model})
	return nil
}

func (g *chatSessionsGatewayStub) CoreSaveModelRPC(_ context.Context, req adapter.ModelSaveRequest) error {
	if err := g.modelGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.modelSaves = append(g.modelSaves, req)
	return nil
}

func (g *chatSessionsGatewayStub) CoreActivateModelRPC(_ context.Context, name string) error {
	if err := g.modelGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.modelActivations = append(g.modelActivations, name)
	return nil
}

func (g *chatSessionsGatewayStub) CoreDeleteModelRPC(_ context.Context, name string) error {
	if err := g.modelGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.modelDeletes = append(g.modelDeletes, name)
	return nil
}

func (g *chatSessionsGatewayStub) CoreSetSessionModelRPC(_ context.Context, sessionID, name string) error {
	if err := g.modelGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sessionModelSets = append(g.sessionModelSets, [2]string{sessionID, name})
	return nil
}

func (g *chatSessionsGatewayStub) CoreSetWorkspaceModelRPC(_ context.Context, workspace, name string) error {
	if err := g.modelGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.workspaceModelSets = append(g.workspaceModelSets, [2]string{workspace, name})
	return nil
}

func (g *chatSessionsGatewayStub) CoreRollbackVersionRPC(_ context.Context, id string) error {
	if err := g.modelGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.versionRollbacks = append(g.versionRollbacks, id)
	return nil
}

func (g *chatSessionsGatewayStub) CoreDeleteVersionRPC(_ context.Context, id string) error {
	if err := g.modelGate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.versionDeletes = append(g.versionDeletes, id)
	return nil
}

func (g *chatSessionsGatewayStub) CoreClearVersionsRPC(context.Context) (int, error) {
	if err := g.modelGate(); err != nil {
		return 0, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.versionClears++
	return 3, nil
}

// ---- 模型域 ----

func TestCapabilityModelArms(t *testing.T) {
	newModelBridge := func(t *testing.T) (*BridgeService, *chatSessionsGatewayStub, *CapabilityService) {
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
		return s, gateway, NewCapabilityService(s)
	}

	t.Run("nil bridge 全方法", func(t *testing.T) {
		svc := NewCapabilityService(nil)
		if _, err := svc.UpsertModel("n", "b", "k", "m"); err == nil {
			t.Fatal("UpsertModel(nil) error = nil")
		}
		if _, err := svc.SaveModel(ModelSaveRequest{}); err == nil {
			t.Fatal("SaveModel(nil) error = nil")
		}
		if _, err := svc.ActivateModel("n"); err == nil {
			t.Fatal("ActivateModel(nil) error = nil")
		}
		if _, err := svc.SelectCurrentModel("n"); err == nil {
			t.Fatal("SelectCurrentModel(nil) error = nil")
		}
		if _, err := svc.DeleteModel("n"); err == nil {
			t.Fatal("DeleteModel(nil) error = nil")
		}
	})

	t.Run("UpsertModel trim 转发", func(t *testing.T) {
		_, gateway, svc := newModelBridge(t)
		if _, err := svc.UpsertModel(" n ", " base ", " key ", " m "); err != nil {
			t.Fatalf("UpsertModel error = %v", err)
		}
		gateway.mu.Lock()
		upserts := append([][4]string(nil), gateway.modelUpserts...)
		gateway.mu.Unlock()
		if len(upserts) != 1 || upserts[0] != [4]string{"n", "base", "key", "m"} {
			t.Fatalf("upserts = %v", upserts)
		}
	})

	t.Run("SaveModel 新增切当前（session+workspace 双写）", func(t *testing.T) {
		s, gateway, svc := newModelBridge(t)
		s.stateMu.Lock()
		s.activeWorkspace = "/ws"
		s.currentSessionID = "sess-m"
		s.stateMu.Unlock()
		_, err := svc.SaveModel(ModelSaveRequest{Name: " 新模型 ", Mode: "custom_provider", APIKey: " sk-1 "})
		if err != nil {
			t.Fatalf("SaveModel error = %v", err)
		}
		gateway.mu.Lock()
		saves := len(gateway.modelSaves)
		sessionSets := append([][2]string(nil), gateway.sessionModelSets...)
		workspaceSets := append([][2]string(nil), gateway.workspaceModelSets...)
		gateway.mu.Unlock()
		if saves != 1 {
			t.Fatalf("saves = %d", saves)
		}
		if len(sessionSets) != 1 || sessionSets[0] != [2]string{"sess-m", "新模型"} {
			t.Fatalf("sessionModelSets = %v", sessionSets)
		}
		if len(workspaceSets) != 1 || workspaceSets[0] != [2]string{"/ws", "新模型"} {
			t.Fatalf("workspaceModelSets = %v", workspaceSets)
		}

		// 更新（OriginalName 非空）不触发切换。
		gateway.mu.Lock()
		before := len(gateway.sessionModelSets)
		gateway.mu.Unlock()
		if _, err := svc.SaveModel(ModelSaveRequest{OriginalName: "旧名", Name: "新模型"}); err != nil {
			t.Fatalf("SaveModel(update) error = %v", err)
		}
		gateway.mu.Lock()
		after := len(gateway.sessionModelSets)
		gateway.mu.Unlock()
		if after != before {
			t.Fatal("model update should not re-select current")
		}
	})

	t.Run("SaveModel workspace 从会话兜底", func(t *testing.T) {
		s, gateway, svc := newModelBridge(t)
		s.stateMu.Lock()
		s.activeWorkspace = ""
		s.currentSessionID = "sess-m"
		s.sessions["sess-m"] = &sessionState{ID: "sess-m", WorkspacePath: "/from-session"}
		s.stateMu.Unlock()
		if _, err := svc.SaveModel(ModelSaveRequest{Name: "m1"}); err != nil {
			t.Fatalf("SaveModel error = %v", err)
		}
		gateway.mu.Lock()
		workspaceSets := append([][2]string(nil), gateway.workspaceModelSets...)
		gateway.mu.Unlock()
		if len(workspaceSets) != 1 || workspaceSets[0][0] != "/from-session" {
			t.Fatalf("workspace fallback = %v", workspaceSets)
		}
	})

	t.Run("SelectCurrentModel 三级回落", func(t *testing.T) {
		s, gateway, svc := newModelBridge(t)
		// session 级。
		s.stateMu.Lock()
		s.activeWorkspace = "/ws"
		s.currentSessionID = "s1"
		s.stateMu.Unlock()
		if _, err := svc.SelectCurrentModel("m"); err != nil {
			t.Fatalf("SelectCurrentModel(session) error = %v", err)
		}
		// workspace 级（无 session）。
		s.stateMu.Lock()
		s.currentSessionID = ""
		s.stateMu.Unlock()
		if _, err := svc.SelectCurrentModel("m"); err != nil {
			t.Fatalf("SelectCurrentModel(workspace) error = %v", err)
		}
		// global 级（全空）。
		s.stateMu.Lock()
		s.activeWorkspace = ""
		s.stateMu.Unlock()
		if _, err := svc.SelectCurrentModel("m"); err != nil {
			t.Fatalf("SelectCurrentModel(global) error = %v", err)
		}
		gateway.mu.Lock()
		sessions := len(gateway.sessionModelSets)
		workspaces := len(gateway.workspaceModelSets)
		activations := len(gateway.modelActivations)
		gateway.mu.Unlock()
		if sessions != 1 || workspaces != 2 || activations != 1 {
			t.Fatalf("model select calls = %d/%d/%d, want 1/2/1", sessions, workspaces, activations)
		}
	})

	t.Run("Activate/Delete 与失败透传", func(t *testing.T) {
		_, gateway, svc := newModelBridge(t)
		if _, err := svc.ActivateModel(" m "); err != nil {
			t.Fatalf("ActivateModel error = %v", err)
		}
		if _, err := svc.DeleteModel(" m "); err != nil {
			t.Fatalf("DeleteModel error = %v", err)
		}
		gateway.modelErr = errors.New("catalog locked")
		if _, err := svc.UpsertModel("n", "b", "k", "m"); err == nil {
			t.Fatal("UpsertModel error = nil on failure")
		}
		if _, err := svc.SaveModel(ModelSaveRequest{Name: "x"}); err == nil {
			t.Fatal("SaveModel error = nil on failure")
		}
		if _, err := svc.ActivateModel("x"); err == nil {
			t.Fatal("ActivateModel error = nil on failure")
		}
		if _, err := svc.SelectCurrentModel("x"); err == nil {
			t.Fatal("SelectCurrentModel error = nil on failure")
		}
		if _, err := svc.DeleteModel("x"); err == nil {
			t.Fatal("DeleteModel error = nil on failure")
		}
	})

	t.Run("空模型名拒绝", func(t *testing.T) {
		s, _, _ := newModelBridge(t)
		if _, err := s.selectCurrentModelRPC("/ws", "s1", "  "); err == nil || !strings.Contains(err.Error(), "name is required") {
			t.Fatalf("empty name error = %v", err)
		}
	})
}

// ---- 规则落盘与版本 ----

// rulesFileIn 返回 workspace 根下实际落点的规则文件（EOS.md/AGENTS.md，
// 由 resolveInstructionsTarget 决定，内核只发现这两个名字）。
func rulesFileIn(t *testing.T, workspace string) string {
	t.Helper()
	for _, name := range []string{"EOS.md", "AGENTS.md"} {
		path := filepath.Join(workspace, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Fatalf("rules file (EOS.md/AGENTS.md) not found in %s", workspace)
	return ""
}

func TestRulesAndVersionsArms(t *testing.T) {
	newRulesBridge := func(t *testing.T) (*BridgeService, *chatSessionsGatewayStub, *CapabilityService) {
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
		return s, gateway, NewCapabilityService(s)
	}

	t.Run("SaveRules workspace 落盘", func(t *testing.T) {
		_, _, svc := newRulesBridge(t)
		workspace := t.TempDir()
		if _, err := svc.SaveRules(RulesSaveRequest{Scope: " workspace ", WorkspacePath: workspace, Value: "  规则内容  "}); err != nil {
			t.Fatalf("SaveRules error = %v", err)
		}
		content, err := os.ReadFile(rulesFileIn(t, workspace))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "规则内容") {
			t.Fatalf("content = %q", content)
		}
	})

	t.Run("SaveRules 空内容用默认模板", func(t *testing.T) {
		_, _, svc := newRulesBridge(t)
		workspace := t.TempDir()
		if _, err := svc.SaveRules(RulesSaveRequest{Scope: "workspace", WorkspacePath: workspace, Value: "   "}); err != nil {
			t.Fatalf("SaveRules(empty) error = %v", err)
		}
		content, err := os.ReadFile(rulesFileIn(t, workspace))
		if err != nil {
			t.Fatal(err)
		}
		if len(strings.TrimSpace(string(content))) == 0 {
			t.Fatal("default rules template is empty")
		}
	})

	t.Run("SaveRules 校验拒绝", func(t *testing.T) {
		_, _, svc := newRulesBridge(t)
		if _, err := svc.SaveRules(RulesSaveRequest{Scope: "unknown"}); err == nil {
			t.Fatal("unknown scope error = nil")
		}
		if _, err := svc.SaveRules(RulesSaveRequest{Scope: "workspace", WorkspacePath: "  "}); err == nil {
			t.Fatal("missing workspace error = nil")
		}
		if _, err := (NewCapabilityService(nil)).SaveRules(RulesSaveRequest{Scope: "global"}); err == nil {
			t.Fatal("SaveRules(nil) error = nil")
		}
	})

	t.Run("ResetRules 走模板", func(t *testing.T) {
		_, _, svc := newRulesBridge(t)
		workspace := t.TempDir()
		if _, err := svc.ResetRules(RulesResetRequest{Scope: "workspace", WorkspacePath: workspace}); err != nil {
			t.Fatalf("ResetRules error = %v", err)
		}
		if content, err := os.ReadFile(rulesFileIn(t, workspace)); err != nil || len(strings.TrimSpace(string(content))) == 0 {
			t.Fatalf("reset template not written: %v", err)
		}
	})

	t.Run("版本操作转发", func(t *testing.T) {
		_, gateway, svc := newRulesBridge(t)
		if _, err := svc.RollbackVersion(" v1 "); err != nil {
			t.Fatalf("RollbackVersion error = %v", err)
		}
		if _, err := svc.DeleteVersion(" v1 "); err != nil {
			t.Fatalf("DeleteVersion error = %v", err)
		}
		gateway.mu.Lock()
		rollbacks := append([]string(nil), gateway.versionRollbacks...)
		deletes := append([]string(nil), gateway.versionDeletes...)
		gateway.mu.Unlock()
		if len(rollbacks) != 1 || rollbacks[0] != "v1" || len(deletes) != 1 || deletes[0] != "v1" {
			t.Fatalf("version calls = %v %v", rollbacks, deletes)
		}

		gateway.modelErr = errors.New("store down")
		if _, err := svc.RollbackVersion("v"); err == nil {
			t.Fatal("RollbackVersion error = nil on failure")
		}
		if _, err := svc.DeleteVersion("v"); err == nil {
			t.Fatal("DeleteVersion error = nil on failure")
		}
	})
}
