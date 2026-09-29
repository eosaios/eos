package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestPrintHelpers(t *testing.T) {
	// applyPrintModeEnv nil engine
	if err := applyPrintModeEnv(context.Background(), nil, PrintOptions{}); err == nil {
		t.Fatal("nil engine")
	}
	eng := &fakeExecSessionsEngine{}
	if err := applyPrintModeEnv(context.Background(), eng, PrintOptions{Workspace: "/ws"}); err != nil {
		t.Fatal(err)
	}
	if err := applyPrintModeEnv(context.Background(), eng, PrintOptions{}); err != nil {
		t.Fatal(err)
	}

	writePrintError("json", errors.New("e"))
	writePrintError("text", errors.New("e"))

	if err := emitPrintResult("text", PrintResult{Content: "c"}, time.Now(), coreapi.UsageSummary{}, "m"); err != nil {
		t.Fatal(err)
	}
	if err := emitPrintResult("json", PrintResult{Content: "c"}, time.Now(), coreapi.UsageSummary{}, "m"); err != nil {
		t.Fatal(err)
	}

	if headlessRustCoreStoreDir() == "" {
		// 可能无 HOME
	}
}

func TestRunSingleTurnValidation(t *testing.T) {
	if _, err := runSingleTurn(context.Background(), nil, "q", "text", ""); err == nil {
		t.Fatal("nil engine")
	}
	eng := &fakeExecSessionsEngine{}
	if _, err := runSingleTurn(context.Background(), eng, "  ", "text", ""); err == nil {
		t.Fatal("empty query")
	}
}

func TestDoctorConfigBranches(t *testing.T) {
	setTestHome(t)
	var buf bytesBuffer
	r := &doctorReporter{w: &buf}
	// 缺失配置
	doctorConfig(r)

	// 合法 JSON 空模型
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgPath := filepath.Join(home, ".eos.json")
	if err := os.WriteFile(cfgPath, []byte(`{"models":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	doctorConfig(r)

	// 坏 JSON
	if err := os.WriteFile(cfgPath, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	doctorConfig(r)

	// 有模型 + 掩码 key
	if err := os.WriteFile(cfgPath, []byte(`{"models":[{"name":"m","api_key":"abcd...wxyz","model":"x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	doctorConfig(r)
}

type bytesBuffer struct{ s string }

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.s += string(p)
	return len(p), nil
}
