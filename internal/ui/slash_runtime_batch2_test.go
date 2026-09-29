// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

package ui

// slash_runtime 批测第二批：/plugin 列表与子命令 usage、/plugins reload
// 失败臂、plugin 直装徽标（MCP/skills）、/workspace add|remove、/model
// current 各回退、/permissions 失败臂与授权回显。

import (
	"errors"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestHandlePluginSlashBatch(t *testing.T) {
	setTestHome(t)

	t.Run("subcommand usage", func(t *testing.T) {
		app := newTestAppModel(t)
		app.handlePluginSlash("install")
		if got := lastSystemText(app); !strings.Contains(got, "用法") {
			t.Fatalf("got %q", got)
		}
		app.handlePluginSlash("remove")
		if got := lastSystemText(app); !strings.Contains(got, "用法") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("list rows", func(t *testing.T) {
		engine := newTestEngine()
		engine.plugins = []coreapi.PluginInfo{
			{Name: "zeta", Description: "d1", Source: "src1", Enabled: true},
			{Name: "alpha", Command: "cmd1"},
		}
		app := NewAppModelFromCoreEngine(engine)
		app.handlePluginSlash()
		got := lastSystemText(app)
		for _, want := range []string{
			"插件: 2", "- zeta [src1, enabled]: d1",
			"- alpha [cmd1, disabled]: (无描述)",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q in %q", want, got)
			}
		}
	})
}

func TestHandleReloadPluginsSlashError(t *testing.T) {
	setTestHome(t)
	// testEngine 模式无 sidecar 进程，Reload 必失败（core client is not
	// available）→ 覆盖失败臂。
	app := newTestAppModel(t)
	app.handleReloadPluginsSlash()
	if got := lastSystemText(app); !strings.Contains(got, "插件重载失败") {
		t.Fatalf("got %q", got)
	}
}

func TestPluginInstallPlainBadges(t *testing.T) {
	setTestHome(t)
	engine := newTestEngine()
	engine.caller = &fakeCaller{responses: map[string]string{
		"plugin/install": `{"name":"badge","version":"1.2.3","mcp_registered":true,"skills_installed":["sa","sb"]}`,
	}}
	app := NewAppModelFromCoreEngine(engine)
	app.pluginInstallCmd("src-badge")
	if !waitForHistory(t, app, "badge v1.2.3 已安装") {
		t.Fatalf("install not surfaced: %q", lastSystemText(app))
	}
	if got := lastSystemText(app); !strings.Contains(got, "（MCP 已注册）") ||
		!strings.Contains(got, "sa, sb") {
		t.Fatalf("badges missing: %q", got)
	}
}

func TestHandleWorkspaceSlashBatch(t *testing.T) {
	setTestHome(t)

	t.Run("add not a directory", func(t *testing.T) {
		app := newTestAppModel(t)
		app.handleWorkspaceSlash([]string{"add", "no-such-dir-xyz"})
		if got := lastSystemText(app); !strings.Contains(got, "路径不是目录") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("add ok and remove ok", func(t *testing.T) {
		dir := t.TempDir()
		app := newTestAppModel(t)
		app.handleWorkspaceSlash([]string{"add", dir})
		if got := lastSystemText(app); !strings.Contains(got, "已添加工作区") {
			t.Fatalf("got %q", got)
		}
		app.handleWorkspaceSlash([]string{"remove", dir})
		if got := lastSystemText(app); !strings.Contains(got, "已移除工作区") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("use untrusted opens confirm", func(t *testing.T) {
		dir := t.TempDir()
		app := newTestAppModel(t)
		cmd := app.handleWorkspaceSlash([]string{"use", dir})
		// 未信任路径 → 挂起确认，不直接切换（返回 nil cmd，等待用户）。
		if cmd != nil {
			t.Fatalf("untrusted use should park, got cmd %v", cmd)
		}
		if app.trustPendingPath != dir {
			t.Fatalf("trustPendingPath = %q", app.trustPendingPath)
		}
	})

	t.Run("subcommand without path", func(t *testing.T) {
		app := newTestAppModel(t)
		app.handleWorkspaceSlash([]string{"add"})
		if got := lastSystemText(app); !strings.Contains(got, "用法") {
			t.Fatalf("got %q", got)
		}
	})
}

func TestHandleModelSlashCurrentFallbacks(t *testing.T) {
	setTestHome(t)

	t.Run("context error", func(t *testing.T) {
		engine := newTestEngine()
		engine.modelCtxErr = errors.New("mc boom")
		app := NewAppModelFromCoreEngine(engine)
		app.handleModelSlash([]string{"current"})
		if got := lastSystemText(app); !strings.Contains(got, "mc boom") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("unconfigured fallback", func(t *testing.T) {
		app := newEmptyTestAppModel(t)
		app.handleModelSlash([]string{"current"})
		got := lastSystemText(app)
		if !strings.Contains(got, "未配置") {
			t.Fatalf("fallback missing: %q", got)
		}
	})

	t.Run("scope labels", func(t *testing.T) {
		for _, scope := range []string{"session", "workspace", "global", ""} {
			engine := newTestEngine()
			engine.modelCtxSnapshot = coreapi.ModelContextSnapshot{
				ResolvedModelName: "default-model",
				ResolvedScope:     scope,
			}
			app := NewAppModelFromCoreEngine(engine)
			app.handleModelSlash([]string{"current"})
			if got := lastSystemText(app); !strings.Contains(got, "当前模型") {
				t.Fatalf("scope %q: got %q", scope, got)
			}
		}
	})

	t.Run("use select error", func(t *testing.T) {
		engine := newTestEngine()
		engine.models = []coreapi.ModelConfig{{Name: "default-model"}}
		engine.activeModel = "default-model"
		engine.selectModelErr = errors.New("sel boom")
		app := NewAppModelFromCoreEngine(engine)
		app.handleModelSlash([]string{"use", "default-model"})
		if got := lastSystemText(app); !strings.Contains(got, "sel boom") {
			t.Fatalf("got %q", got)
		}
	})
}

func TestHandlePermissionsSlashErrors(t *testing.T) {
	setTestHome(t)

	t.Run("access mode switches and renders snapshot", func(t *testing.T) {
		engine := newTestEngine()
		engine.permissionSnap.AccessMode = "plan"
		app := NewAppModelFromCoreEngine(engine)
		// 成功 toast 之后 fall-through 渲染最新快照，两条都在 history。
		app.handlePermissionsSlash([]string{"access", "danger-full-access"})
		if !waitForHistory(t, app, "访问模式已切换为 danger-full-access") {
			t.Fatalf("switch toast missing: %q", lastSystemText(app))
		}
		if got := lastSystemText(app); !strings.Contains(got, "审批模式: never") {
			// EnterFullAccess 应同步审批模式（快照 fall-through 校验）。
			t.Fatalf("snapshot not refreshed: %q", got)
		}
	})

	t.Run("snapshot authorization rows", func(t *testing.T) {
		engine := newTestEngine()
		engine.permissionSnap = coreapi.PermissionSnapshot{
			AccessMode:              "plan",
			ApprovalMode:            "on-request",
			SandboxMode:             "workspace-write",
			AllowedCategories:       []string{"net"},
			HasPendingDiff:          true,
			PendingDiffPath:         "",
			LastAuthorization:       "approved",
			LastAuthorizationKind:   "fs",
			LastAuthorizationTarget: "/x/y.txt",
			LastAuthorizationNote:   "user",
		}
		app := NewAppModelFromCoreEngine(engine)
		app.handlePermissionsSlash(nil)
		got := lastSystemText(app)
		for _, want := range []string{
			"net", "待审批 diff", "(未标记路径)", "approved",
			"fs", "/x/y.txt", "user",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q in %q", want, got)
			}
		}
	})

	t.Run("unknown input falls back to usage", func(t *testing.T) {
		app := newTestAppModel(t)
		app.handlePermissionsSlash([]string{"warp"})
		if got := lastSystemText(app); !strings.Contains(got, "用法") {
			t.Fatalf("got %q", got)
		}
	})
}
