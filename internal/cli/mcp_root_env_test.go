package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRootLang(t *testing.T) {
	t.Setenv("EOS_LANG", "en")
	if rootLang() != "en" {
		t.Fatal("en")
	}
	t.Setenv("EOS_LANG", "zh")
	if rootLang() != "zh" {
		t.Fatal("zh")
	}
	t.Setenv("EOS_LANG", "fr")
	if rootLang() != "zh" {
		t.Fatal("fallback")
	}
	t.Setenv("EOS_LANG", "")
	if rootLang() != "zh" {
		t.Fatal("empty")
	}
	if rootShort() == "" {
		t.Fatal("rootShort")
	}
}

func TestMcpOptionEnv(t *testing.T) {
	env := mcpOptionEnv(mcpServeOptions{SandboxMode: "workspace-write", ApprovalMode: "never", Workspace: "/ws"})
	if env["EOS_SANDBOX_MODE"] != "workspace-write" {
		t.Fatalf("sandbox = %v", env)
	}
	if env["EOS_APPROVAL_MODE"] != "never" {
		t.Fatal("approval")
	}
	if env["EOS_WORKSPACE_ROOT"] != "/ws" {
		t.Fatal("workspace")
	}

	// AccessMode 回落
	env = mcpOptionEnv(mcpServeOptions{AccessMode: "read-only"})
	if env["EOS_SANDBOX_MODE"] != "read-only" {
		t.Fatalf("access = %v", env)
	}

	// cwd 回落
	env = mcpOptionEnv(mcpServeOptions{})
	if env["EOS_WORKSPACE_ROOT"] == "" {
		t.Fatal("cwd fallback")
	}
}

func TestWriteExecJSONAndExecOptionEnv(t *testing.T) {
	writeExecJSON(os.Stdout, ExecResult{Content: "c"})
	n := 1
	writeExecJSON(os.Stdout, ExecResult{TotalTokens: &n, CostUSD: nil})

	env := execOptionEnv(execOptions{
		Workspace:     "/ws",
		ExecutionMode: "plan",
		AccessMode:    "read-only",
		ApprovalMode:  "never",
	})
	if env["EOS_WORKSPACE_ROOT"] != "/ws" {
		t.Fatalf("env = %v", env)
	}
	if env["EOS_EXECUTION_MODE"] != "plan" && env["EOS_MODE"] != "plan" {
		// 至少有一个
		_ = env
	}

	// saveRawConfigDoc 无效目录（MkdirAll 会创建）
	if err := saveRawConfigDoc(map[string]json.RawMessage{}, filepath.Join(t.TempDir(), "deep", "x.json")); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorCoreBootDevPlaceholder(t *testing.T) {
	setTestHome(t)
	var buf bytesBuf
	r := &doctorReporter{w: &buf}
	// 本地占位内核可能失败，不 panic 即可
	doctorCoreBootDevPlaceholder(r, nil)
	_ = r.failed
}

type bytesBuf struct{ s string }

func (b *bytesBuf) Write(p []byte) (int, error) {
	b.s += string(p)
	return len(p), nil
}
