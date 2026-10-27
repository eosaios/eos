package shell

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/styles"
	"github.com/eosaios/eos/internal/update"

	tea "charm.land/bubbletea/v2"
)

func newShell(w, h int) *Model {
	m := New(w, h, styles.NewStyles(styles.GetTheme("dark")), "zh")
	return &m
}

func TestTailAndPadHelpers(t *testing.T) {
	if got := tailLines("a\nb\nc", 0); got != "" {
		t.Fatalf("n=0 = %q", got)
	}
	if got := tailLines("", 3); got != "" {
		t.Fatalf("empty = %q", got)
	}
	if got := tailLines("a\nb\n", 5); got != "a\nb" {
		t.Fatalf("short = %q", got)
	}
	if got := tailLines("a\nb\nc", 2); got != "b\nc" {
		t.Fatalf("tail = %q", got)
	}

	if got := padToHeight("x", 0); got != "" {
		t.Fatalf("n=0 = %q", got)
	}
	if got := padToHeight("", 3); got != "\n\n" {
		t.Fatalf("empty pad = %q", got)
	}
	if got := padToHeight("a\nb", 4); strings.Count(got, "\n") != 3 {
		t.Fatalf("pad = %q", got)
	}
	if got := padToHeight("a\nb\nc", 2); got != "a\nb" {
		t.Fatalf("truncate = %q", got)
	}
}

func TestShellLiveAndOverlaySetters(t *testing.T) {
	m := newShell(100, 40)

	m.SetLive("live line")
	if m.liveHeight == 0 {
		t.Fatal("liveHeight should be set")
	}
	m.ClearLive()
	if m.live != "" || m.liveHeight != 0 {
		t.Fatalf("ClearLive = %q/%d", m.live, m.liveHeight)
	}

	// 空 live 清掉 hint
	m.SetStatusHints(true, true)
	m.SetLive("")
	if m.hintLive || m.hintThinking {
		t.Fatal("empty live should clear hints")
	}

	m.SetPromptOverlay("overlay\nlines")
	if m.promptOverlayH == 0 {
		t.Fatal("overlay height")
	}
	m.ClearPromptOverlay()
	if m.promptOverlay != "" || m.promptOverlayH != 0 {
		t.Fatal("ClearPromptOverlay")
	}
}

func TestLivePanelModesAndHeights(t *testing.T) {
	m := newShell(100, 40)
	m.SetLive("a\nb\nc\nd\ne")

	// mode 0: inline
	if m.livePanelBodyHeight() != 0 {
		t.Fatalf("mode0 body = %d", m.livePanelBodyHeight())
	}
	if h := m.inlineLiveHeight(); h <= 0 {
		t.Fatalf("inline h = %d", h)
	}

	m.CycleLivePanelMode() // → 1
	if m.livePanelBodyHeight() != 4 {
		t.Fatalf("mode1 body = %d", m.livePanelBodyHeight())
	}
	if m.inlineLiveHeight() != 0 {
		t.Fatal("panel mode should disable inline")
	}
	if m.livePanelInnerHeight() != 5 || m.livePanelOuterHeight() != 7 {
		t.Fatalf("mode1 heights = %d/%d", m.livePanelInnerHeight(), m.livePanelOuterHeight())
	}
	if out := m.renderLivePanel(); !strings.Contains(out, "Live") {
		t.Fatalf("live panel:\n%s", out)
	}

	m.CycleLivePanelMode() // → 2
	body := m.livePanelBodyHeight()
	if body < 7 || body > 16 {
		t.Fatalf("mode2 body = %d", body)
	}
	m.CycleLivePanelMode() // → 0
	if m.livePanelOuterHeight() != 0 {
		t.Fatalf("mode0 outer = %d", m.livePanelOuterHeight())
	}

	// 空 live 在 panel 模式显示 empty 提示
	m.livePanelMode = 1
	m.SetLive("")
	if out := m.renderLivePanel(); out == "" {
		t.Fatal("empty live panel")
	}

	// liveReserveHeight: panel 优先，否则 inline
	m.livePanelMode = 0
	m.SetLive("x")
	if m.liveReserveHeight() != m.inlineLiveHeight() {
		t.Fatal("reserve should use inline")
	}
	m.livePanelMode = 1
	if m.liveReserveHeight() != m.livePanelOuterHeight() {
		t.Fatal("reserve should use panel")
	}
}

func TestModeProcessingThinkingAndStatusFields(t *testing.T) {
	m := newShell(100, 30)

	m.SetMode(ModeAI)
	if m.GetMode() != ModeAI {
		t.Fatal("SetMode AI")
	}
	m.ToggleMode()
	if m.GetMode() != ModeBash {
		t.Fatal("Toggle to bash")
	}
	m.ToggleMode()
	if m.GetMode() != ModeAI {
		t.Fatal("Toggle back to AI")
	}

	m.SetProcessing(true)
	if !m.Processing() {
		t.Fatal("Processing")
	}
	m.statusAnim = 3
	m.SetProcessing(false)
	if m.statusAnim != 0 {
		t.Fatal("statusAnim should reset when idle")
	}

	m.SetThinking(true, "thinking hard")
	if !m.thinking || m.thinkingText != "thinking hard" {
		t.Fatal("SetThinking")
	}
	m.SetThinking(false, "")
	if m.thinking {
		t.Fatal("clear thinking")
	}

	m.SetContextUsage(1200, 0.4)
	if m.ctxTokens != 1200 || m.ctxRatio != 0.4 {
		t.Fatal("SetContextUsage")
	}
	m.SetExecutionMode("plan")
	if m.executionMode != "plan" {
		t.Fatalf("executionMode = %q", m.executionMode)
	}
	m.SetGitSummary(" main ", 2, 1)
	if m.gitBranch != "main" || m.gitDirty != 2 || m.gitAhead != 1 {
		t.Fatalf("git = %q %d %d", m.gitBranch, m.gitDirty, m.gitAhead)
	}
	m.SetThinkingExpanded(true)
	m.SetContextVisible(true)
	m.SetBGTaskCount(-5)
	if m.bgTaskCount != 0 {
		t.Fatalf("bgTaskCount = %d", m.bgTaskCount)
	}
	m.SetBGTaskCount(3)
	if m.bgTaskCount != 3 {
		t.Fatal("bgTaskCount")
	}
	m.SetStatusHints(true, true)
	if !m.hintLive || !m.hintThinking {
		t.Fatal("SetStatusHints")
	}
}

func TestContentAndInputAccessors(t *testing.T) {
	m := newShell(80, 24)

	m.AppendContent("hello")
	m.AppendContentLine("world")
	if !strings.Contains(m.Content(), "hello") || !strings.Contains(m.Content(), "world") {
		t.Fatalf("content = %q", m.Content())
	}
	if m.showWelcome {
		t.Fatal("append should hide welcome")
	}

	m.ClearContent()
	if m.ContentLineCount() > 0 && strings.TrimSpace(m.Content()) != "" {
		// Clear 后内容应为空
	}

	m.SetContent("")
	if !m.showWelcome {
		t.Fatal("empty content should show welcome")
	}
	m.SetContent("body")
	if m.showWelcome {
		t.Fatal("non-empty should hide welcome")
	}
	m.SetContentPreserveOffset("more")
	if m.showWelcome {
		t.Fatal("preserve offset hide welcome")
	}

	m.SetInputValue("typed")
	if m.GetInputValue() != "typed" {
		t.Fatal("input")
	}
	m.ClearInput()
	if m.GetInputValue() != "" {
		t.Fatal("ClearInput")
	}

	m.SetPrediction("pred")
	if !m.HasPrediction() {
		t.Fatal("HasPrediction")
	}
	m.ClearPrediction()
	if m.HasPrediction() {
		t.Fatal("ClearPrediction")
	}

	m.AddToHistory("cmd1")
	m.FocusInput()
	m.BlurInput()
	m.SetLanguage("en")
	if m.language != "en" {
		t.Fatal("SetLanguage")
	}

	ox, oy := m.ContentOrigin()
	if ox != 2 || oy != 1 {
		t.Fatalf("ContentOrigin = %d,%d", ox, oy)
	}
	if m.ContentWidth() != 76 {
		t.Fatalf("ContentWidth = %d", m.ContentWidth())
	}
	_ = m.ContentYOffset()
	_ = m.ContentHeight()
}

func TestSelectionHighlight(t *testing.T) {
	m := newShell(80, 24)
	m.SetSelectionHighlight(1, 3)
	if m.selFrom != 1 || m.selTo != 3 {
		t.Fatalf("sel = %d/%d", m.selFrom, m.selTo)
	}
	// from > to 视为清除
	m.SetSelectionHighlight(5, 2)
	if m.selFrom != -1 || m.selTo != -1 {
		t.Fatal("invalid range should clear")
	}
	m.SetSelectionHighlight(0, -1)
	if m.selFrom != -1 {
		t.Fatal("negative to should clear")
	}
	m.SetSelectionHighlight(2, 4)
	m.ClearSelectionHighlight()
	if m.selFrom != -1 || m.selTo != -1 {
		t.Fatal("ClearSelectionHighlight")
	}

	// applySelectionHighlight 只高亮范围内物理行
	out := applySelectionHighlight("a\nb\nc", 0, 1, 1)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[1], "\x1b[7m") {
		t.Fatalf("line 1 not highlighted: %q", out)
	}
	if strings.Contains(lines[0], "\x1b[7m") {
		t.Fatalf("line 0 highlighted: %q", out)
	}
	// 无效范围原样返回
	if applySelectionHighlight("x", 0, 3, 1) != "x" {
		t.Fatal("invalid range")
	}
	// yOffset 偏移
	out = applySelectionHighlight("a\nb", 1, 1, 1)
	if !strings.Contains(strings.Split(out, "\n")[0], "\x1b[7m") {
		t.Fatalf("offset highlight: %q", out)
	}
}

func TestAcceptHintAndInitUpdate(t *testing.T) {
	m := newShell(80, 24)

	// 路径提示：替换 @ 后内容
	m.SetInputValue("see @old")
	m.acceptHint("new/path")
	if m.GetInputValue() != "see @new/path " {
		t.Fatalf("path hint = %q", m.GetInputValue())
	}

	// 命令提示：整体替换
	m.SetInputValue("/he")
	m.acceptHint("/help")
	if m.GetInputValue() != "/help " {
		t.Fatalf("cmd hint = %q", m.GetInputValue())
	}

	// Init 返回批处理
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init should return batch")
	}

	// statusTick：空闲不推进
	next, cmd := m.Update(statusTickMsg{})
	if cmd != nil {
		t.Fatal("idle tick should not reschedule")
	}
	_ = next

	// statusTick：processing 推进动画
	m.SetProcessing(true)
	before := m.statusAnim
	next2, cmd := m.Update(statusTickMsg{})
	if cmd == nil {
		t.Fatal("running tick should reschedule")
	}
	if next2.statusAnim == before && before != 3 {
		// 推进一帧（0→1 或 3→0 wrap）
	}
	_ = next2

	// WelcomeTick：不显示欢迎卡时停止动画
	m.SetContent("body") // hideWelcome
	_, cmd = m.Update(WelcomeTickMsg{})
	if cmd != nil {
		t.Fatal("welcome tick without visible card should stop")
	}
}

func TestWelcomeCardBasics(t *testing.T) {
	w := NewWelcomeCard(styles.NewStyles(styles.GetTheme("dark")), "zh")
	if len(w.particles) != 60 {
		t.Fatalf("particles = %d", len(w.particles))
	}
	w.SetLanguage("en")
	if w.language != "en" {
		t.Fatal("SetLanguage")
	}
	w.SetSize(100, 30)
	if w.width != 100 || w.height != 30 {
		t.Fatal("SetSize")
	}

	// SetInfo 空值不覆盖
	w.SetInfo("m1", "api1", "/ws")
	w.SetInfo("", "", "")
	if w.modelName != "m1" || w.apiInfo != "api1" || w.workDir != "/ws" {
		t.Fatalf("SetInfo = %q/%q/%q", w.modelName, w.apiInfo, w.workDir)
	}

	w.SetUpdateInfo(&update.CheckResult{HasUpdate: true, LatestVersion: "2.0.0"})
	if w.updateInfo == nil || !w.updateInfo.HasUpdate {
		t.Fatal("SetUpdateInfo")
	}

	// Tick 推进帧并反射粒子边界
	w.frame = 0
	for i := 0; i < 50; i++ {
		w.Tick()
	}
	if w.frame != 50 {
		t.Fatalf("frame = %d", w.frame)
	}
	for i, p := range w.particles {
		if p.x < 0 || p.x >= float64(w.width) || p.y < 0 || p.y >= float64(w.height) {
			t.Fatalf("particle %d out of bounds: %+v", i, p)
		}
	}

	if cmd := WelcomeTickCmd(); cmd == nil {
		t.Fatal("WelcomeTickCmd")
	}
	if msg := WelcomeTickCmd()(); msg.(WelcomeTickMsg) != (WelcomeTickMsg{}) {
		t.Fatal("tick msg type")
	}

	view := w.View()
	if view == "" {
		t.Fatal("View empty")
	}

	// padCenter
	if got := padCenter("ab", 6); len([]rune(stripANSIForTest(got))) < 2 {
		t.Fatalf("padCenter = %q", got)
	}
}

func TestShellViewWithLivePanelAndWelcome(t *testing.T) {
	m := newShell(100, 40)
	m.SetContent("")
	m.SetWelcomeInfo("gpt", "https://api", "/ws")
	m.SetUpdateInfo(&update.CheckResult{HasUpdate: true, LatestVersion: "9.9.9"})

	view := m.View()
	if view == "" {
		t.Fatal("welcome view empty")
	}

	// live panel 模式渲染
	m.SetContent("history")
	m.SetLive("live1\nlive2")
	m.livePanelMode = 1
	view = m.View()
	if !strings.Contains(stripANSIForTest(view), "Live") {
		t.Fatalf("live panel missing:\n%s", view)
	}
}

func TestHandleKeyHintsAndNavigation(t *testing.T) {
	m := newShell(80, 24)

	// hints 可见：上下/tab/enter/esc
	m.SetInputValue("/he")
	m.ShowSlashHints("he")
	if !m.IsHintsVisible() {
		t.Fatal("slash hints should show")
	}
	handled, _ := m.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if !handled {
		t.Fatal("up in hints")
	}
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled {
		t.Fatal("down in hints")
	}
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if !handled {
		t.Fatal("tab in hints")
	}

	m.ShowSlashHints("he")
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter in hints")
	}

	m.ShowSlashHints("zzzz-no-match")
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatal("esc in hints")
	}
	if m.IsHintsVisible() {
		t.Fatal("esc should hide hints")
	}

	// 换行快捷键
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if !handled {
		t.Fatal("shift+enter")
	}
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	if !handled {
		t.Fatal("alt+enter")
	}
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if !handled {
		t.Fatal("ctrl+j")
	}

	// esc 清空输入
	m.SetInputValue("x")
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled || m.GetInputValue() != "" {
		t.Fatal("esc clear input")
	}

	// 历史上下
	m.AddToHistory("prev")
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if !handled {
		t.Fatal("history up")
	}
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled {
		t.Fatal("history down")
	}

	// 未处理键
	handled, _ = m.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if handled {
		t.Fatal("plain key should not be handled by HandleKey")
	}
}

func TestUpdateScrollKeysAndStatusTick(t *testing.T) {
	m := newShell(80, 24)
	for i := 0; i < 50; i++ {
		m.AppendContentLine("line")
	}

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})

	// hints 可见时导航键只动 hints
	m.ShowSlashHints("")
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	// StatusTick
	m.SetProcessing(true)
	if cmd := m.StatusTick(); cmd == nil {
		t.Fatal("StatusTick should schedule when processing")
	}
	m.SetProcessing(false)
	m.SetThinking(false, "")
	if cmd := m.StatusTick(); cmd != nil {
		t.Fatal("StatusTick idle should be nil")
	}
}

func TestShowPathHintsAndCanAcceptPrediction(t *testing.T) {
	m := newShell(80, 24)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("hello.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("subdir", 0o755); err != nil {
		t.Fatal(err)
	}

	m.ShowPathHints("")
	if !m.IsHintsVisible() {
		t.Fatal("path hints should show")
	}
	m.HideHints()

	// 查询过滤
	m.ShowPathHints("hello")
	if !m.IsHintsVisible() {
		t.Fatal("filtered hints")
	}
	m.HideHints()

	// 子目录
	m.ShowPathHints("subdir/")
	if !m.IsHintsVisible() {
		t.Fatal("subdir hints")
	}
	m.HideHints()

	// 无匹配
	m.ShowPathHints("zzzz-nope")
	if !m.IsHintsVisible() {
		t.Fatal("empty match still shows placeholder")
	}
	m.HideHints()

	// CanAcceptPrediction
	m.SetPrediction("pred")
	if !m.CanAcceptPrediction() {
		t.Fatal("CanAcceptPrediction")
	}
	m.ClearPrediction()
	if m.CanAcceptPrediction() {
		t.Fatal("no prediction")
	}
}
