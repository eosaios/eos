package webbridge

import "testing"

// TestResolveSendSessionLockedHonorsWorkspaceConsistency 验证发消息时 session 解析的
// workspace 一致性校验——这是「切换工作区后发消息仍跑到旧工作区 session」bug 的核心修复。
//
// 根因：前端 sendChat 曾不传 workspace，Go 的 resolveSendSessionLocked 非空 sessionID
// 分支盲信 sessionID、无 workspace 校验，于是陈旧 sessionID（属旧工作区）会被原样返回，
// turn 跑在旧工作区 session 上，<cwd> 注入旧路径。修复后非空分支要求 session.workspace
// 与传入 workspace 一致，不一致则忽略该 sessionID、按 workspace 重新解析。
func TestResolveSendSessionLockedHonorsWorkspaceConsistency(t *testing.T) {
	const w1 = `C:\home\eos\eos-app`
	const w2 = `C:\Users\10144\eos\workspace`

	cases := []struct {
		name        string
		sessionID   string
		workspace   string
		current     string // s.currentSessionID（命中 ensureActiveSessionLockedWithError 的复用分支，避免 RPC）
		wantSession string
	}{
		{
			name:        "sessionID 与 workspace 一致 → 返回该 session",
			sessionID:   "sess-w1",
			workspace:   w1,
			current:     "sess-w1",
			wantSession: "sess-w1",
		},
		{
			// 核心回归用例：前端传了属旧工作区的陈旧 sessionID，但 workspace 指向新工作区。
			// 修复后必须忽略该 sessionID、按新工作区解析，绝不能复用旧工作区的 session。
			name:        "sessionID 属旧工作区 + workspace 指向新工作区 → 不复用旧 session，按新工作区解析",
			sessionID:   "sess-w1",
			workspace:   w2,
			current:     "sess-w2",
			wantSession: "sess-w2",
		},
		{
			name:        "空 sessionID + workspace → 按 workspace 解析",
			sessionID:   "",
			workspace:   w2,
			current:     "sess-w2",
			wantSession: "sess-w2",
		},
		{
			// 向后兼容：自动化等无显式工作区的入口传空 workspace，此时盲信 sessionID（保持原行为）。
			name:        "sessionID 属旧工作区 + 空 workspace → 返回该 session（兼容自动化入口）",
			sessionID:   "sess-w1",
			workspace:   "",
			current:     "sess-w1",
			wantSession: "sess-w1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &BridgeService{
				sessions: map[string]*sessionState{
					"sess-w1": {ID: "sess-w1", WorkspacePath: w1},
					"sess-w2": {ID: "sess-w2", WorkspacePath: w2},
				},
				currentSessionID: tc.current,
				activeWorkspace:  w2,
			}
			got, err := s.resolveSendSessionLocked(tc.sessionID, tc.workspace)
			if err != nil {
				t.Fatalf("resolveSendSessionLocked err = %v", err)
			}
			if got == nil {
				t.Fatalf("resolveSendSessionLocked returned nil session")
			}
			if got.ID != tc.wantSession {
				t.Fatalf("resolved session = %q, want %q (must not reuse stale session from another workspace)", got.ID, tc.wantSession)
			}
		})
	}
}

// TestResolveSendSessionLockedWorkspaceCaseInsensitive 验证 workspace 比较大小写/分隔符
// 归一化（sameWorkspacePath 走 ToLower + filepath.Clean），Windows 下不同写法应判等。
func TestResolveSendSessionLockedWorkspaceCaseInsensitive(t *testing.T) {
	w1Session := &sessionState{ID: "sess-w1", WorkspacePath: `C:\home\eos\eos-app`}
	s := &BridgeService{
		sessions: map[string]*sessionState{
			"sess-w1": w1Session,
		},
		currentSessionID: "sess-w1",
		activeWorkspace:  `C:\home\eos\eos-app`,
	}
	// 正斜杠 + 大小写不同，应仍判定为同一 workspace，返回该 session。
	got, err := s.resolveSendSessionLocked("sess-w1", `c:/home/eos/eos-app`)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.ID != "sess-w1" {
		t.Fatalf("resolved session = %+v, want sess-w1 (workspace 比较应归一化)", got)
	}
}
