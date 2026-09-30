// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

package ui

// app_panels_mcp 第二轮批测：toggle 四臂（列表失败/未知名/setEnabled 失败/
// 成功含 reload cmd 执行）、edit 三臂、delete 两臂、save 的 reload cmd、
// refresh 的列表失败与浏览器状态回退、configSubmit 全家（空文本/坏 JSON/
// 编辑多条/空名/重命名新增失败/重命名删旧失败/直接更新失败/新增失败/成功）。

import (
	"errors"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/internal/ui/views/setup"
	"github.com/eosaios/eos/pkg/coreapi"
	tea "charm.land/bubbletea/v2"
)

func mcpServerFixture() []coreapi.MCPServer {
	return []coreapi.MCPServer{{Name: "demo", Enabled: false}}
}

func execCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestMCPToggleAllArms(t *testing.T) {
	setTestHome(t)

	t.Run("list error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpListErr = errors.New("boom list")
		app.handleMCPToggle(panels.MCPToggleMsg{Name: "demo"})
		if got := lastSystemText(app); !strings.Contains(got, "Failed to toggle") {
			t.Fatalf("want toggle error, got %q", got)
		}
	})

	t.Run("unknown name", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpList = mcpServerFixture()
		app.handleMCPToggle(panels.MCPToggleMsg{Name: "ghost"})
		if got := lastSystemText(app); !strings.Contains(got, "Failed to toggle") {
			t.Fatalf("want unknown-name error, got %q", got)
		}
	})

	t.Run("set enabled error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpList = mcpServerFixture()
		eng.mcpSetEnabledErr = errors.New("boom set")
		app.handleMCPToggle(panels.MCPToggleMsg{Name: "demo"})
		if got := lastSystemText(app); !strings.Contains(got, "Failed to toggle") {
			t.Fatalf("want set error, got %q", got)
		}
	})

	t.Run("success enables and reload cmd errors gracefully", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpList = mcpServerFixture()
		cmd := app.handleMCPToggle(panels.MCPToggleMsg{Name: "demo"})
		found := false
		for _, h := range app.history {
			if strings.Contains(h.content, "demo") && h.level == "success" {
				found = true
			}
		}
		if !found {
			t.Fatalf("want toggle success toast, last = %q", lastSystemText(app))
		}
		// reload cmd：engine-only 环境 Reload 走 sidecar 报错，消息带回 Err
		msg := execCmd(cmd)
		done, ok := msg.(MCPReloadDoneMsg)
		if !ok {
			t.Fatalf("cmd msg = %T, want MCPReloadDoneMsg", msg)
		}
		if done.Err == nil {
			t.Fatal("engine-only Reload should error (sidecar nil)")
		}
	})
}

func TestMCPEditAndDeleteArms(t *testing.T) {
	setTestHome(t)

	t.Run("edit load error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpListErr = errors.New("boom list")
		app.handleMCPEdit(panels.MCPEditMsg{Name: "demo"})
		if got := lastSystemText(app); !strings.Contains(got, "加载 MCP 配置失败") {
			t.Fatalf("want load error surfaced, got %q", got)
		}
	})

	t.Run("edit not found", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpList = mcpServerFixture()
		app.handleMCPEdit(panels.MCPEditMsg{Name: "ghost"})
		if got := lastSystemText(app); !strings.Contains(got, "ghost") {
			t.Fatalf("want not-found warning, got %q", got)
		}
	})

	t.Run("edit opens editor view", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpList = mcpServerFixture()
		app.handleMCPEdit(panels.MCPEditMsg{Name: "demo"})
		if app.activeView != "setup" || app.setupView == nil {
			t.Fatalf("view = %q, setupView = %v", app.activeView, app.setupView)
		}
	})

	t.Run("delete error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpDeleteErr = errors.New("boom delete")
		app.handleMCPDelete(panels.MCPDeleteMsg{Name: "demo"})
		if got := lastSystemText(app); !strings.Contains(got, "Failed to delete") {
			t.Fatalf("want delete error, got %q", got)
		}
	})

	t.Run("delete success and reload cmd", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		cmd := app.handleMCPDelete(panels.MCPDeleteMsg{Name: "demo"})
		if _, ok := execCmd(cmd).(MCPReloadDoneMsg); !ok {
			t.Fatal("delete should return reload cmd")
		}
	})
}

func TestMCPSaveAndRefreshArms(t *testing.T) {
	setTestHome(t)

	t.Run("save executes reload cmd", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		if _, ok := execCmd(app.handleMCPSave()).(MCPReloadDoneMsg); !ok {
			t.Fatal("save should return reload cmd")
		}
	})

	t.Run("refresh list error", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpListErr = errors.New("boom list")
		app.refreshMCPPanel()
		if got := lastSystemText(app); !strings.Contains(got, "Failed to load MCP") {
			t.Fatalf("want refresh error, got %q", got)
		}
	})

	t.Run("refresh browser status fallback", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpList = mcpServerFixture()
		eng.browserStatusErr = errors.New("browser down")
		app.refreshMCPPanel()
		// 浏览器状态失败回退零值：面板不炸、无 MCP 错误
		if got := lastSystemText(app); strings.Contains(got, "Failed to load MCP") {
			t.Fatalf("browser fallback should not error: %q", got)
		}
	})
}

func TestMCPConfigSubmitAllArms(t *testing.T) {
	setTestHome(t)
	entryJSON := `[{"name":"demo","type":"stdio","command":"demo-cmd"}]`

	t.Run("empty text", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handleMCPConfigSubmit(setup.MCPConfigSubmitMsg{Text: "  "})
		if got := lastSystemText(app); !strings.Contains(got, "请输入") {
			t.Fatalf("want empty warning, got %q", got)
		}
	})

	t.Run("bad json", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handleMCPConfigSubmit(setup.MCPConfigSubmitMsg{Text: "{not-json"})
		if got := lastSystemText(app); !strings.Contains(got, "解析") && !strings.Contains(got, "parse") {
			t.Fatalf("want parse error, got %q", got)
		}
	})

	t.Run("edit multiple entries", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handleMCPConfigSubmit(setup.MCPConfigSubmitMsg{
			Text: `[{"name":"a"},{"name":"b"}]`, Edit: true,
		})
		if got := lastSystemText(app); !strings.Contains(got, "只支持一个") {
			t.Fatalf("want single-entry warning, got %q", got)
		}
	})

	t.Run("edit empty name", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		app.handleMCPConfigSubmit(setup.MCPConfigSubmitMsg{
			Text: `[{"name":"  ","type":"stdio"}]`, Edit: true,
		})
		if got := lastSystemText(app); !strings.Contains(got, "名称") && !strings.Contains(got, "name") {
			t.Fatalf("want name-required warning, got %q", got)
		}
	})

	t.Run("rename add failure", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpImportErr = errors.New("boom import")
		app.handleMCPConfigSubmit(setup.MCPConfigSubmitMsg{
			Text: entryJSON, Edit: true, OriginalName: "old-name",
		})
		if got := lastSystemText(app); !strings.Contains(got, "失败") {
			t.Fatalf("want rename add failure, got %q", got)
		}
	})

	t.Run("rename delete-old failure rolls back", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpDeleteErr = errors.New("boom delete")
		app.handleMCPConfigSubmit(setup.MCPConfigSubmitMsg{
			Text: entryJSON, Edit: true, OriginalName: "old-name",
		})
		if got := lastSystemText(app); !strings.Contains(got, "old-name") {
			t.Fatalf("want delete-old failure naming old name, got %q", got)
		}
	})

	t.Run("direct update failure", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpUpsertErr = errors.New("boom upsert")
		app.handleMCPConfigSubmit(setup.MCPConfigSubmitMsg{
			Text: entryJSON, Edit: true, OriginalName: "demo",
		})
		if got := lastSystemText(app); !strings.Contains(got, "更新失败") && !strings.Contains(got, "update") {
			t.Fatalf("want update failure, got %q", got)
		}
	})

	t.Run("add mode failure", func(t *testing.T) {
		app, eng := newTestAppModelWithEngine(t)
		eng.mcpImportErr = errors.New("boom import")
		app.handleMCPConfigSubmit(setup.MCPConfigSubmitMsg{Text: entryJSON})
		if got := lastSystemText(app); !strings.Contains(got, "失败") {
			t.Fatalf("want add failure, got %q", got)
		}
	})

	t.Run("add mode success returns to panel", func(t *testing.T) {
		app, _ := newTestAppModelWithEngine(t)
		cmd := app.handleMCPConfigSubmit(setup.MCPConfigSubmitMsg{Text: entryJSON})
		if app.activeView != "panel" || app.activePanel != "mcp" {
			t.Fatalf("view = %q/%q, want panel/mcp", app.activeView, app.activePanel)
		}
		if _, ok := execCmd(cmd).(MCPReloadDoneMsg); !ok {
			t.Fatal("submit success should return reload cmd")
		}
	})
}
