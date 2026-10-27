package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// session_store 余臂批测：findOrRestore 各分支、restoreSessions 批量恢复、
// restoreRuntimeSession 的一致性校验/空 workspace 回落/meta 命中。

import (
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

func TestFindOrRestoreSessionArms(t *testing.T) {
	workspace := t.TempDir()

	t.Run("空 sessionID 直 nil", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		if got := s.findOrRestoreSession(workspace, "  "); got != nil {
			t.Fatalf("空 id = %+v", got)
		}
	})

	t.Run("内存命中+workspace 校验", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		known := &sessionState{ID: "s-in", WorkspacePath: workspace}
		s.sessions["s-in"] = known
		if got := s.findOrRestoreSession(workspace, "s-in"); got != known {
			t.Fatal("内存命中应复用")
		}
		// workspace 不一致 → 走恢复链（无 meta → nil）。
		if got := s.findOrRestoreSession("/other/ws", "s-in"); got != nil {
			t.Fatalf("跨 workspace 不应复用: %+v", got)
		}
		// 空 workspace 参数：命中即返回。
		if got := s.findOrRestoreSession("", "s-in"); got != known {
			t.Fatal("空 workspace 命中应返回")
		}
	})

	t.Run("经 meta 恢复", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{
			sessions: []coreapi.Session{{ID: "s-meta", WorkspaceRoot: workspace}},
		}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		got := s.findOrRestoreSession(workspace, "s-meta")
		if got == nil || got.ID != "s-meta" {
			t.Fatalf("meta 恢复 = %+v", got)
		}
		// 未知 id：恢复链空 → nil。
		if got := s.findOrRestoreSession(workspace, "s-missing"); got != nil {
			t.Fatalf("未知 id = %+v", got)
		}
	})
}

func TestRestoreSessionsFromRuntimeLocked(t *testing.T) {
	workspace := t.TempDir()

	t.Run("无历史与列表失败", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		if id, ok := s.restoreSessionsFromRuntimeLocked(workspace); ok || id != "" {
			t.Fatalf("无历史 = %q,%v", id, ok)
		}
	})

	t.Run("批量恢复+current 回落首条", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{
			sessions: []coreapi.Session{
				{ID: "s-a", WorkspaceRoot: workspace},
				{ID: "s-b", WorkspaceRoot: workspace},
			},
		}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		id, ok := s.restoreSessionsFromRuntimeLocked(workspace)
		if !ok || id != "s-a" {
			t.Fatalf("批量恢复 = %q,%v", id, ok)
		}
		s.stateMu.RLock()
		count := len(s.sessions)
		s.stateMu.RUnlock()
		if count != 2 {
			t.Fatalf("恢复数 = %d", count)
		}
	})

	t.Run("restoreCurrent 空 current 回 nil", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		if got := s.restoreCurrentRuntimeSessionLocked(workspace); got != nil {
			t.Fatalf("空 current = %+v", got)
		}
	})
}

func TestRestoreRuntimeSessionLockedArms(t *testing.T) {
	workspace := t.TempDir()

	t.Run("空 id 与既有复用", func(t *testing.T) {
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		if got := s.restoreRuntimeSessionLocked("  ", workspace); got != nil {
			t.Fatalf("空 id = %+v", got)
		}
		known := &sessionState{ID: "s-x", WorkspacePath: workspace}
		s.sessions["s-x"] = known
		if got := s.restoreRuntimeSessionLocked("s-x", workspace); got != known {
			t.Fatal("既有复用")
		}
		// 跨 workspace 一致性拒绝。
		if got := s.restoreRuntimeSessionLocked("s-x", "/other"); got != nil {
			t.Fatalf("不一致 = %+v", got)
		}
		// 空白工作区会话 + 显式 workspace：一致性校验拒绝（返回 nil，
		// 让调用方按 workspace 新建——不把无主会话注入指定工作区）。
		blank := &sessionState{ID: "s-blank"}
		s.sessions["s-blank"] = blank
		if got := s.restoreRuntimeSessionLocked("s-blank", workspace); got != nil {
			t.Fatalf("空白会话应拒 = %+v", got)
		}
	})

	t.Run("空 workspace 三级回落", func(t *testing.T) {
		// 1) snapshot 兜底；2) activeWorkspace 兜底；3) 都空 → nil。
		gateway := &chatSessionsGatewayStub{
			snapshot: adapter.RuntimeSnapshot{Sessions: []adapter.SessionSnapshot{}},
		}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		if got := s.restoreRuntimeSessionLocked("s-none", ""); got != nil {
			t.Fatalf("全空回落 = %+v", got)
		}
	})

	t.Run("列表错误与 meta 未命中", func(t *testing.T) {
		// 空会话列表 → meta nil → nil。
		s, _, _ := newChatSessionsTestBridge(t, &chatSessionsGatewayStub{})
		if got := s.restoreRuntimeSessionLocked("s-any", t.TempDir()); got != nil {
			t.Fatalf("meta 未命中 = %+v", got)
		}
	})
}
