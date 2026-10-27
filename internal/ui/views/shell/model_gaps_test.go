package shell

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// model.go 余臂批测：布局重算（hints 可见）、live/overlay 高度回落、
// live 面板 detail 模式与溢出截断、路径提示限数/隐藏过滤/子目录前缀、
// Update 链式滚动键/状态帧/欢迎帧/提示可见时的鼠标与普通键分发。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRecomputeLayoutWithHintsVisible(t *testing.T) {
	m := newShell(80, 40)
	m.ShowSlashHints("")
	if !m.hints.Visible() {
		t.Fatal("hints 应可见")
	}
	before := m.contentH
	m.recomputeLayout()
	if m.contentH >= 40 || m.contentH < 10 {
		t.Fatalf("contentH = %d（应扣减 hints 高度且下限 10）", m.contentH)
	}
	_ = before // ShowSlashHints 已触发重算，这里只固化重算后的界
}

func TestLiveAndOverlayHeightFallbacks(t *testing.T) {
	m := newShell(80, 24)
	// 直接字段赋值绕过 setter：liveHeight=0 → inlineLiveHeight 用 lipgloss 回落。
	m.live = "a\nb"
	if h := m.inlineLiveHeight(); h != 2 {
		t.Fatalf("inline 回落 = %d", h)
	}
	// overlay 高度回落（promptOverlayH=0）。
	m.promptOverlay = "x\ny\nz"
	if h := m.promptOverlayReserveHeight(); h != 3 {
		t.Fatalf("overlay 回落 = %d", h)
	}
	// 面板模式下 inline 恒 0。
	m.livePanelMode = 1
	if h := m.inlineLiveHeight(); h != 0 {
		t.Fatalf("panel 模式 inline = %d", h)
	}
	m.livePanelMode = 0

	// 空 live 的 inline 回落 0。
	m.live = ""
	if h := m.inlineLiveHeight(); h != 0 {
		t.Fatalf("空 live inline = %d", h)
	}
}

func TestRenderLivePanelDetailAndOverflow(t *testing.T) {
	m := newShell(100, 24)
	m.SetLive(strings.Repeat("line\n", 30))
	m.livePanelMode = 2
	out := m.renderLivePanel()
	if !strings.Contains(out, "Live") {
		t.Fatalf("detail 面板:\n%s", out)
	}
	// 高溢出：body 超过 bodyH 时截尾。
	bodyH := m.livePanelBodyHeight()
	tall := strings.Repeat("row\n", bodyH+20)
	m.SetLive(tall)
	out2 := m.renderLivePanel()
	if lipglossHeight(out2) > bodyH+4 {
		t.Fatalf("溢出未截断: bodyH=%d out 行数=%d", bodyH, lipglossHeight(out2))
	}

	// 空 live + detail 模式：空提示占位。
	m.SetLive("")
	if out3 := m.renderLivePanel(); out3 == "" {
		t.Fatal("空 live 面板应有占位")
	}
}

func lipglossHeight(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func TestPathHintsLimitHiddenAndSubdirPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// 25 个可见文件（超 20 上限）+ 隐藏文件。
	for i := 0; i < 25; i++ {
		if err := os.WriteFile(filepath.Join(dir, "f"+string(rune('a'+i%26))+strings.Repeat("x", i)+".txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(".hidden", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("pkg", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newShell(80, 24)
	m.ShowPathHints("")
	// 上限 20 + 隐藏文件过滤：经 View 文本断言（组件无列表 getter）。
	view := m.hints.View()
	if !strings.Contains(view, ".txt") {
		t.Fatalf("应列出文件: %s", view)
	}
	if strings.Contains(view, ".hidden") {
		t.Fatalf("隐藏文件不应入选: %s", view)
	}
	if got := strings.Count(view, "\n"); got > 22 {
		t.Fatalf("提示行数超限: %d", got)
	}
	m.HideHints()

	// 子目录前缀：pkg/ 查询返回带 pkg/ 前缀的条目。
	m.ShowPathHints("pkg/main")
	view = m.hints.View()
	if !strings.Contains(view, "pkg/") {
		t.Fatalf("子目录条目缺前缀: %s", view)
	}
	m.HideHints()

	// 无匹配查询：空列表占位仍可见。
	m.ShowPathHints("zzzz-nope")
	if !m.hints.Visible() {
		t.Fatal("空匹配应仍显示占位")
	}
}

func TestUpdateChainedScrollAndTicks(t *testing.T) {
	m := newShell(80, 20)
	for i := 0; i < 80; i++ {
		m.AppendContentLine("line-" + string(rune('a'+i%26)))
	}

	// 链式滚动：PgUp 离底 → PgDown 回底 → Home/End 变换（Update 返回新模型）。
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m2.content.AtBottom() {
		t.Fatal("PgUp 后应离底")
	}
	m3, _ := m2.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m4, _ := m3.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if m4.content.AtBottom() {
		t.Fatal("Home 后应在顶")
	}
	m5, _ := m4.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if !m5.content.AtBottom() {
		t.Fatal("End 后应在底")
	}

	// statusTickMsg：processing 时推进动画并续排。
	m5.SetProcessing(true)
	m6, cmd := m5.Update(statusTickMsg{})
	if cmd == nil {
		t.Fatal("processing 状态帧应续排")
	}
	_ = m6

	// WelcomeTickMsg：welcome 开时续排、关时停。
	m5.welcome = NewWelcomeCard(m5.styles, m5.language)
	m5.showWelcome = true
	_, wcmd := m5.Update(WelcomeTickMsg{})
	if wcmd == nil {
		t.Fatal("welcome 帧应续排")
	}
	m5.showWelcome = false
	_, wcmd2 := m5.Update(WelcomeTickMsg{})
	if wcmd2 != nil {
		t.Fatal("welcome 关闭后帧应停")
	}
}

func TestUpdateHintsVisibleMouseAndPlainKeys(t *testing.T) {
	m := newShell(80, 24)
	m.ShowSlashHints("")

	// hints 可见 + 鼠标滚轮：内容区仍滚动。
	m2, _ := m.Update(tea.MouseMsg(tea.MouseClickMsg{}))
	_ = m2

	// hints 可见 + 非导航键：分发到 hints 与 input。
	m3, _ := m.Update(tea.KeyPressMsg{Code: 'x'})
	_ = m3

	// 正常模式（hints 隐藏）普通键：全组件更新 + 输入高度变化重算。
	m4 := newShell(80, 24)
	m4.HideHints()
	m5, _ := m4.Update(tea.KeyPressMsg{Code: 'a'})
	_ = m5
}

func TestViewCompositionAndStatusBarArms(t *testing.T) {
	m := newShell(100, 30)

	// bash 模式标签臂 + bg 任务计数 + git 三段 + 上下文比例三档。
	m.SetMode(ModeBash)
	if bar := m.renderStatusBar(); !strings.Contains(bar, "命令") {
		t.Fatalf("bash 标签: %q", bar)
	}
	m.SetMode(ModeAI)
	m.bgTaskCount = 3
	m.gitBranch = "main"
	m.gitDirty = 2
	m.gitAhead = 1
	m.ctxVisible = true
	m.ctxRatio = 0.9
	bar := m.renderStatusBar()
	if !strings.Contains(bar, "⎇ main") || !strings.Contains(bar, "●2") || !strings.Contains(bar, "↑1") {
		t.Fatalf("git 三段: %q", bar)
	}
	m.ctxRatio = 0.7
	bar = m.renderStatusBar()
	m.ctxRatio = 0.3
	bar = m.renderStatusBar()
	m.ctxVisible = false
	m.ctxRatio = -0.5 // 负值夹逼臂（ctxVisible 关闭时不渲染）
	m.ctxVisible = true
	bar = m.renderStatusBar()
	if bar == "" {
		t.Fatal("状态栏不应为空")
	}
	m.ctxRatio = 2.0 // 超界夹逼臂
	bar = m.renderStatusBar()

	// 选区高亮：selFrom 置位后 View 走 applySelectionHighlight。
	m2 := newShell(80, 24)
	m2.AppendContentLine("alpha line")
	m2.AppendContentLine("beta line")
	m2.selFrom = 0
	m2.selTo = 1
	_ = m2.View()

	// inline live 溢出截断 + View 组装（welcome 关闭 + live 内联 + overlay + hints）。
	m3 := newShell(80, 12)
	m3.showWelcome = false
	m3.AppendContentLine("content line")
	m3.SetLive(strings.Repeat("tall\n", 30)) // 溢出走 tailLines
	m3.SetPromptOverlay("overlay")
	m3.ShowSlashHints("")
	out := m3.View()
	if !strings.Contains(out, "content line") || !strings.Contains(out, "overlay") {
		t.Fatalf("View 组装缺段:\n%s", out)
	}
}
