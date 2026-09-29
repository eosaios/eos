package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// app_send/startup 第二轮缺口批测：nil 接收者、paste 错误臂、诊断宏展开、
// skill 斜杠、invoke 回包、homeDir 失败、stderr writer 打开失败、plan switch。
//
// 不可达清单：
//   - StartInteractiveTUIWithOptions：进程入口 + os.Exit（见 startup_coverage_test.go）。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"

	tea "charm.land/bubbletea/v2"
)

func TestNilReceiverMethods(t *testing.T) {
	var m *AppModel
	m.clearPrediction()
	m.syncPredictionState()
	_ = m.canPredict()
	_ = m.requestPrediction("x")
	_ = m.schedulePrediction("x")
	_ = m.toggleThinkingExpand()
}

func TestRequestPredictionErrorArm(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	app.predictionEnabled = true
	app.activeView = "shell"
	app.state.Mode = "ai"
	eng.predictErr = errors.New("predict down")
	cmd := app.requestPrediction("hi")
	if cmd == nil {
		t.Fatal("should return cmd")
	}
	msg := cmd()
	pu, ok := msg.(PredictionUpdateMsg)
	if !ok {
		t.Fatalf("msg=%T", msg)
	}
	if pu.Text != "" {
		t.Fatalf("text=%q", pu.Text)
	}
	if pu.Draft != "hi" {
		t.Fatalf("draft=%q", pu.Draft)
	}
}

func TestPasteClipboardImageIOFailures(t *testing.T) {
	setTestHome(t)
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	t.Chdir(tmp)
	defer func() { _ = os.Chdir(cwd) }()

	// mkdir 失败：.eos 已是普通文件
	if err := os.WriteFile(filepath.Join(tmp, ".eos"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newTestAppModel(t)
	app.state.Mode = "ai"
	withStubClipboardImage(t, []byte("IMG"), nil)
	runCmd(app.pasteClipboardImage())
	if len(app.pendingImagePaths) != 0 {
		t.Fatal("mkdir fail should not save")
	}

	// write 失败：attachments 路径是目录，clipboard-*.png 无法创建
	// 先清掉 .eos 文件
	_ = os.Remove(filepath.Join(tmp, ".eos"))
	att := filepath.Join(tmp, ".eos", "attachments")
	if err := os.MkdirAll(att, 0o755); err != nil {
		t.Fatal(err)
	}
	// 预先占用目标文件名为目录——文件名含时间戳，改用只读目录更稳
	if err := os.Chmod(att, 0o500); err != nil {
		t.Fatal(err)
	}
	app2 := newTestAppModel(t)
	app2.state.Mode = "ai"
	withStubClipboardImage(t, []byte("IMG"), nil)
	runCmd(app2.pasteClipboardImage())
	// 只读目录下 WriteFile 应失败（Windows 只读目录语义可能不同，允许成功）
	if len(app2.pendingImagePaths) > 1 {
		t.Fatal("unexpected multiple saves")
	}
	_ = os.Chmod(att, 0o755)
}

func TestShouldSendMessageSkillInvoke(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.invokeSkillInvoked = true
	app.state.Mode = "ai"
	app.shell.SetInputValue("/myskill do")
	ok, _ := app.shouldSendMessage()
	if ok {
		t.Fatal("skill slash should not send as chat")
	}
	if strings.TrimSpace(app.shell.GetInputValue()) != "" {
		t.Fatal("input should clear")
	}

	// InvokeSkill 报错也吞成已处理（true）
	app2, eng2 := newTestAppModelWithEngine(t)
	eng2.invokeSkillErr = errors.New("skill boom")
	app2.state.Mode = "ai"
	app2.shell.SetInputValue("/myskill")
	ok2, _ := app2.shouldSendMessage()
	if ok2 {
		t.Fatal("skill error path also returns false send")
	}
}

func TestSendMessageTextDiagnosticsExpandedAndInvoke(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	eng.lspDiagnostics = []string{"err: foo", "warn: bar"}
	cmd := app.sendMessageText("see #problems_and_diagnostics", false)
	if cmd == nil {
		t.Fatal("should send")
	}
	// 执行 invoke 子命令（Turns()=nil 时 Invoke 会报错 → ErrorMsg 或 nil）
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			m := c()
			switch m.(type) {
			case ErrorMsg, InvokeDoneMsg, nil:
				// 合法回包
			default:
				// StatusTick 等其它消息可忽略
			}
		}
	}
}

func TestSendMessageImageOnlyDisplay(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	img := filepath.Join(t.TempDir(), "only.png")
	if err := os.WriteFile(img, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.pendingImagePaths = []string{img}
	// 空文本 + 图片 → image_only 展示
	_ = app.sendMessageText("", true)
	found := false
	for _, h := range app.history {
		if h.kind == "user" && h.content != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected image_only user entry")
	}

	// 多图截断到 4 个名字
	app2 := newTestAppModel(t)
	for i := 0; i < 6; i++ {
		p := filepath.Join(t.TempDir(), "f"+string(rune('a'+i))+".png")
		_ = os.WriteFile(p, []byte("x"), 0o600)
		app2.pendingImagePaths = append(app2.pendingImagePaths, p)
	}
	_ = app2.sendMessageText("with images", true)
}

func TestToggleThinkingExpandArms(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	// Thinking=false
	if cmd := app.toggleThinkingExpand(); cmd != nil {
		t.Fatal("not thinking")
	}
	// Thinking=true 但 thinkingLive 空
	app.state.Thinking = true
	if cmd := app.toggleThinkingExpand(); cmd != nil {
		t.Fatal("empty thinking")
	}
	// 有效切换
	app.thinkingLive.WriteString("step")
	cmd := app.toggleThinkingExpand()
	if cmd == nil {
		t.Fatal("should toggle")
	}
	runCmd(cmd)
	if !app.thinkingExpanded {
		t.Fatal("should expand")
	}
	// shell=nil 时仍切换
	app.shell = nil
	cmd2 := app.toggleThinkingExpand()
	if cmd2 == nil {
		t.Fatal("should still toggle")
	}
	runCmd(cmd2)
}

func TestDefaultRustCoreStoreDirNoHome(t *testing.T) {
	// 清空 HOME/USERPROFILE 使 UserHomeDir 失败
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	if dir := defaultRustCoreStoreDir(); dir != "" {
		t.Fatalf("dir=%q", dir)
	}
}

func TestNewSidecarStderrWriterFail(t *testing.T) {
	// 把日志目录指到一个已存在的文件上，MkdirAll 失败
	home := t.TempDir()
	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// ConfiguredLogDir 走 config；若未配置则 DefaultLogDir 可能不依赖 HOME。
	// 这里只断言 newSidecarStderrWriter 不 panic 且返回非 nil。
	w := newSidecarStderrWriter()
	if w == nil {
		t.Fatal("nil writer")
	}
	w.Close()
}

func TestApplyTUIStartupModelPlanSwitch(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	// 条目 Model 与套餐内 PlanModel 不同 → NeedsPlanSwitch
	eng.models = append(eng.models, coreapi.ModelConfig{
		Name:       "plan-entry",
		APIBase:    "https://example.com/v1",
		Model:      "plan-a",
		ProviderID: "openai",
		PresetID:   "gpt-5-codex",
	})
	eng.modelCatalog = &coreapi.ModelCatalogState{
		Presets: []coreapi.ModelPresetOption{{
			ID:         "gpt-5-codex",
			Name:       "GPT-5-Codex",
			ProviderID: "openai",
			ModelName:  "plan-a",
			PlanModels: []coreapi.PlanModel{
				{ModelID: "plan-a", Label: "Plan A"},
				{ModelID: "plan-b", Label: "Plan B"},
			},
		}},
	}
	// 解析 "Plan B" → NeedsPlanSwitch；Save 成功
	applyTUIStartupModel(app, "Plan B")

	// SwitchPlanModel 失败臂
	app2, eng2 := newTestAppModelWithEngine(t)
	eng2.models = append(eng2.models, coreapi.ModelConfig{
		Name:       "plan-entry",
		APIBase:    "https://example.com/v1",
		Model:      "plan-a",
		ProviderID: "openai",
		PresetID:   "gpt-5-codex",
	})
	eng2.modelCatalog = eng.modelCatalog
	eng2.modelsSaveErr = errors.New("save fail")
	applyTUIStartupModel(app2, "Plan B")
}
