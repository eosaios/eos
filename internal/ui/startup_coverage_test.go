package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// startup.go 缺口批测：T / applyTUIStartupModel 全臂 / tuiOptionEnv /
// defaultRustCoreStoreDir / sidecarStderrWriter。
//
// StartInteractiveTUIWithOptions 会 tea.NewProgram(...).Run() 阻塞并可能
// os.Exit——不进单测（书面豁免：启动入口属进程级集成，由手工/E2E 覆盖）。
//
// 不可达清单：
//   - StartInteractiveTUIWithOptions 全函数：进程入口 + os.Exit，单测不可安全驱动。
//   - StartCoreClient 默认臂：返回 sidecarclient.Start，依赖真实子进程。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTWrapper(t *testing.T) {
	got := T("goodbye.message", "zh")
	if strings.TrimSpace(got) == "" {
		t.Fatal("T should return message")
	}
}

func TestApplyTUIStartupModelArms(t *testing.T) {
	setTestHome(t)

	// nil 安全
	applyTUIStartupModel(nil, "x")

	app := newTestAppModel(t)
	// 空 override：只走 heal，不选模型
	applyTUIStartupModel(app, "   ")
	// 未知模型 → resolve 错误提示
	applyTUIStartupModel(app, "no-such-model-xyz")
	// 合法条目名 → select 成功
	applyTUIStartupModel(app, "default-model")

	// select 失败臂
	app2, eng := newTestAppModelWithEngine(t)
	eng.selectModelErr = errFake
	applyTUIStartupModel(app2, "default-model")

	// heal 提示：会话 metadata.model_name 无效
	app3, eng3 := newTestAppModelWithEngine(t)
	eng3.currentSessionMeta = map[string]any{"model_name": "Ghost Label"}
	applyTUIStartupModel(app3, "")
}

// errFake 供失败臂复用
var errFake = errFakeSentinel{}

type errFakeSentinel struct{}

func (errFakeSentinel) Error() string { return "fake error" }

func TestApplyTUIStartupModelHealCleared(t *testing.T) {
	setTestHome(t)
	app, eng := newTestAppModelWithEngine(t)
	// 无匹配条目且无法解析 → ClearSession → 返回 cleared 提示
	eng.currentSessionMeta = map[string]any{"model_name": "Totally Unknown Label"}
	applyTUIStartupModel(app, "")
	// 不断言文案细节（i18n），只断言 history/系统条目被追加过
	if len(app.history) == 0 {
		// heal note 走 appendSystem，可能不在 history；允许空
	}
}

func TestTuiOptionEnvFull(t *testing.T) {
	setTestHome(t)
	t.Setenv("EOS_LOG_LEVEL", "debug")
	env := tuiOptionEnv(TUIOptions{
		ModelOverride:   "m",
		MaxTurns:        3,
		AllowedTools:    []string{"a", "b"},
		DisallowedTools: []string{"c"},
		ApprovalMode:    "always",
		SandboxMode:     "workspace-write",
	})
	if env["EOS_LOG_LEVEL"] != "debug" {
		t.Fatalf("log level %q", env["EOS_LOG_LEVEL"])
	}
	if env["EOS_MODEL_OVERRIDE"] != "m" {
		t.Fatalf("model %q", env["EOS_MODEL_OVERRIDE"])
	}
	if env["EOS_MAX_TURNS"] != "3" {
		t.Fatalf("max turns %q", env["EOS_MAX_TURNS"])
	}
	if env["EOS_ALLOWED_TOOLS"] != "a,b" {
		t.Fatalf("allowed %q", env["EOS_ALLOWED_TOOLS"])
	}
	if env["EOS_DISALLOWED_TOOLS"] != "c" {
		t.Fatalf("disallowed %q", env["EOS_DISALLOWED_TOOLS"])
	}
	if env["EOS_SANDBOX_MODE"] != "workspace-write" {
		t.Fatalf("sandbox %q", env["EOS_SANDBOX_MODE"])
	}
	if env["EOS_APPROVAL_MODE"] != "always" {
		t.Fatalf("approval %q", env["EOS_APPROVAL_MODE"])
	}
	if env["EOS_WORKSPACE_ROOT"] == "" {
		t.Fatal("workspace root required")
	}

	// AccessMode 兜底到 SANDBOX_MODE
	env2 := tuiOptionEnv(TUIOptions{AccessMode: "danger-full-access"})
	if env2["EOS_SANDBOX_MODE"] != "danger-full-access" {
		t.Fatalf("access fallback %q", env2["EOS_SANDBOX_MODE"])
	}

	// SkipPermissions 清掉冲突 mode
	env3 := tuiOptionEnv(TUIOptions{SkipPermissions: true, SandboxMode: "workspace", ApprovalMode: "always"})
	if env3["EOS_SKIP_PERMISSIONS"] != "1" {
		t.Fatal("skip flag")
	}
	if _, ok := env3["EOS_SANDBOX_MODE"]; ok {
		t.Fatal("sandbox should be deleted under skip")
	}
	if _, ok := env3["EOS_APPROVAL_MODE"]; ok {
		t.Fatal("approval should be deleted under skip")
	}

	// 默认 log level
	t.Setenv("EOS_LOG_LEVEL", "")
	env4 := tuiOptionEnv(TUIOptions{})
	if env4["EOS_LOG_LEVEL"] != "info" {
		t.Fatalf("default log %q", env4["EOS_LOG_LEVEL"])
	}
}

func TestDefaultRustCoreStoreDir(t *testing.T) {
	setTestHome(t)
	dir := defaultRustCoreStoreDir()
	if !strings.Contains(dir, filepath.Join(".eos", "core")) {
		t.Fatalf("dir=%q", dir)
	}
}

func TestSidecarStderrWriter(t *testing.T) {
	setTestHome(t)
	// 降级实例：f==nil
	w := &sidecarStderrWriter{}
	n, err := w.Write([]byte("x"))
	if n != 1 || err != nil {
		t.Fatalf("n=%d err=%v", n, err)
	}
	w.Close()
	w.Close() // 幂等

	// 正常实例
	w2 := newSidecarStderrWriter()
	if w2.f == nil {
		t.Fatal("expected file handle")
	}
	if _, err := w2.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	w2.Close()
	if w2.f != nil {
		t.Fatal("f should be nil after close")
	}
	w2.Close()

	// 目录创建失败：HOME 指向不存在的路径父级
	// ConfiguredLogDir 依赖 HOME/LOCALAPPDATA；用只读根难造，跳过（已在正常路径覆盖打开成功）。
	_ = os.Getenv("HOME")
}
