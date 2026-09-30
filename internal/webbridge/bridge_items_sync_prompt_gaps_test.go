package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NRL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// webbridge 批七批测：conversation items 纯函数（tool result 归一/输出格式化/
// 完成 folding）/ state sync 启动链（fsnotify 真文件监听）/ respondPromptRPC
// 全臂 / macOS 更新安装 fallback。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// ---- conversation items 纯函数 ----

func TestConversationItemsPureArms(t *testing.T) {
	t.Run("applyItemCompleted 命中与未命中", func(t *testing.T) {
		msg := &ChatMessage{Items: []ThreadItem{{ID: "i1", Status: "streaming"}}}
		applyItemCompleted(msg, nil) // payload nil 早退
		applyItemCompleted(nil, map[string]any{"item": map[string]any{"id": "x"}})
		applyItemCompleted(msg, map[string]any{}) // 缺 item 早退

		payload := map[string]any{"item": map[string]any{"id": "i1", "kind": "tool_call"}}
		applyItemCompleted(msg, payload)
		if msg.Items[0].Status != "completed" {
			t.Fatalf("existing item not completed: %+v", msg.Items[0])
		}
		appendPayload := map[string]any{"item": map[string]any{"id": "i2", "kind": "status"}}
		applyItemCompleted(msg, appendPayload)
		if len(msg.Items) != 2 || msg.Items[1].ID != "i2" || msg.Items[1].Status != "completed" {
			t.Fatalf("append = %+v", msg.Items)
		}
	})

	t.Run("finalizeMessageItems 收尾", func(t *testing.T) {
		finalizeMessageItems(nil)
		msg := &ChatMessage{Items: []ThreadItem{
			{ID: "a", Status: "streaming"},
			{ID: "b", Status: ""},
			{ID: "c", Status: "failed"},
		}}
		finalizeMessageItems(msg)
		if msg.Items[0].Status != "completed" || msg.Items[1].Status != "completed" {
			t.Fatalf("finalize = %+v", msg.Items)
		}
		if msg.Items[2].Status != "failed" {
			t.Fatalf("terminal status must not flip: %+v", msg.Items[2])
		}
	})

	t.Run("extractItemToolResult 状态归一", func(t *testing.T) {
		cases := map[string]string{
			"ok":     "completed",
			"":       "completed",
			"error":  "failed",
			"denied": "failed",
			"failed": "failed",
			// 未知值原样透传（switch 只归一已知词表；running 属进行态透传）。
			"weird":   "weird",
			"running": "running",
		}
		for input, want := range cases {
			result := extractItemToolResult(map[string]any{"status": input})
			if result.Status != want {
				t.Fatalf("status %q → %q, want %q", input, result.Status, want)
			}
		}
		result := extractItemToolResult(map[string]any{
			"error":       "boom",
			"duration_ms": float64(1200),
		})
		if result.Error != "boom" || result.DurationMS != 1200 {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("formatToolResultOutput 与 joinStringSlice", func(t *testing.T) {
		if got := formatToolResultOutput(map[string]any{"display": " 展示 ", "output": "raw"}); got != " 展示 " {
			t.Fatalf("display priority = %q", got)
		}
		if got := formatToolResultOutput(map[string]any{"display": "   ", "output": "raw"}); got != "raw" {
			t.Fatalf("blank display fallback = %q", got)
		}
		if got := formatToolResultOutput(map[string]any{"output": map[string]any{"k": 1}}); got != "" {
			t.Fatalf("non-string output = %q", got)
		}
		if got := formatToolResultOutput(nil); got != "" {
			t.Fatalf("nil output = %q", got)
		}

		if got := joinStringSlice("直接"); got != "直接" {
			t.Fatalf("joinStringSlice(string) = %q", got)
		}
		if got := joinStringSlice([]any{"a", 42, "b"}); got != "ab" {
			t.Fatalf("joinStringSlice([]any) = %q", got)
		}
		if got := joinStringSlice(42); got != "" {
			t.Fatalf("joinStringSlice(int) = %q", got)
		}
		if got := joinStringSlice(nil); got != "" {
			t.Fatalf("joinStringSlice(nil) = %q", got)
		}
	})
}

// ---- state sync 启动链 ----

func TestStartStateSynchronizersGuards(t *testing.T) {
	// nil receiver / 无 stopCh 早退。
	var nilBridge *BridgeService
	nilBridge.startStateSynchronizers()
	(&BridgeService{}).startStateSynchronizers()

	// 无 gateway：runtime synchronizer 早退，file synchronizer 正常起（有 stopCh）。
	gateway := &chatSessionsGatewayStub{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	s := &BridgeService{
		runtimeGateway: gateway,
		sessions:       map[string]*sessionState{},
		prompts:        map[string]*promptState{},
		emitEvent:      func(string, any) {},
	}
	stop := make(chan struct{})
	s.stopCh = stop
	s.startStateSynchronizers()
	close(stop)
}

func TestStartRuntimeStateSynchronizerStreamsEvents(t *testing.T) {
	gateway := &chatSessionsGatewayStub{}
	gateway.eventCh = make(chan adapter.Event, 8)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	rec := &emitRecorder{}
	s := &BridgeService{
		runtimeGateway: gateway,
		sessions:       map[string]*sessionState{},
		prompts:        map[string]*promptState{},
		emitEvent:      rec.record,
	}
	stop := make(chan struct{})
	s.stopCh = stop
	s.startRuntimeStateSynchronizer()

	// 事件进入订阅通道 → debounce 后 shellUpdated。
	gateway.eventCh <- adapter.Event{Type: "turn.completed"}
	eventually(t, "shellUpdated from runtime events", func() bool { return rec.has(shellUpdatedEventName) })
	close(stop)
}

func TestStartFileStateSynchronizerWatchesWorkspace(t *testing.T) {
	gateway := &chatSessionsGatewayStub{}
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(workspace, ".eos"), 0o755); err != nil {
		t.Fatal(err)
	}
	gateway.defaultWorkspace = workspace
	rec := &emitRecorder{}
	s := &BridgeService{
		runtimeGateway: gateway,
		sessions:       map[string]*sessionState{},
		prompts:        map[string]*promptState{},
		emitEvent:      rec.record,
	}
	stop := make(chan struct{})
	s.stopCh = stop
	s.stateMu.Lock()
	s.activeWorkspace = workspace
	s.stateMu.Unlock()
	s.startFileStateSynchronizer()

	// 等 watcher 装配（reconcile 是异步 goroutine 内），再写状态文件。
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(workspace, ".eos.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	eventually(t, "shellUpdated on state file write", func() bool { return rec.has(shellUpdatedEventName) })
	close(stop)
}

func TestEnqueueShellSyncDropOnFull(t *testing.T) {
	ch := make(chan string, 1)
	enqueueShellSync(ch, "first")
	enqueueShellSync(ch, "second") // 桶满静默丢弃
	if len(ch) != 1 {
		t.Fatalf("channel len = %d, want 1", len(ch))
	}
	if got := <-ch; got != "first" {
		t.Fatalf("got = %q", got)
	}
}

// ---- respondPromptRPC 全臂 ----

func TestRespondPromptRPCArms(t *testing.T) {
	newPromptBridge := func(t *testing.T) (*BridgeService, *chatSessionsGatewayStub) {
		gateway := &chatSessionsGatewayStub{}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		s := &BridgeService{
			runtimeGateway: gateway,
			sessions:       map[string]*sessionState{},
			prompts:        map[string]*promptState{},
			emitEvent:      func(string, any) {},
		}
		return s, gateway
	}

	t.Run("无 gateway / nil prompt 拒绝", func(t *testing.T) {
		s := &BridgeService{}
		if err := s.respondPromptRPC(nil, "accept", ""); err == nil {
			t.Fatal("respondPromptRPC(nil gateway) error = nil")
		}
		s2, _ := newPromptBridge(t)
		if err := s2.respondPromptRPC(nil, "accept", ""); err == nil {
			t.Fatal("respondPromptRPC(nil prompt) error = nil")
		}
	})

	t.Run("空 id / 空 session 拒绝", func(t *testing.T) {
		s, _ := newPromptBridge(t)
		if err := s.respondPromptRPC(&promptState{}, "accept", ""); err == nil || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("empty ids error = %v", err)
		}
	})

	t.Run("普通审批决策映射与非法 token", func(t *testing.T) {
		s, gateway := newPromptBridge(t)
		prompt := &promptState{PromptCard: PromptCard{ID: "apr-1", SessionID: "sess-1"}}
		if err := s.respondPromptRPC(prompt, " accept ", ""); err != nil {
			t.Fatalf("respondPromptRPC error = %v", err)
		}
		gateway.mu.Lock()
		err := gateway.respondErr
		gateway.mu.Unlock()
		if err != nil {
			t.Fatalf("unexpected injected error: %v", err)
		}
		// 注意：CoreRespondApprovalRPC 由既有 stub 处理；非法 token 在
		// approvalDecisionFromToken 处拒绝。
		if err := s.respondPromptRPC(prompt, "maybe", ""); err == nil || !strings.Contains(err.Error(), "unknown approval decision") {
			t.Fatalf("invalid token error = %v", err)
		}
	})

	t.Run("request-user-input note 路径与坏 JSON", func(t *testing.T) {
		s, _ := newPromptBridge(t)
		prompt := &promptState{PromptCard: PromptCard{ID: "apr-2", SessionID: "sess-1"}, Source: "request-user-input"}
		good := `{"answers":{"q1":{"answers":["opt-1"]}}}`
		if err := s.respondPromptRPC(prompt, "accept", good); err != nil {
			t.Fatalf("respondPromptRPC(note) error = %v", err)
		}
		if err := s.respondPromptRPC(prompt, "accept", "not-json"); err == nil || !strings.Contains(err.Error(), "invalid request_user_input") {
			t.Fatalf("bad note error = %v", err)
		}
		if err := s.respondPromptRPC(prompt, "accept", `{"answers":{}}`); err == nil || !strings.Contains(err.Error(), "answers are empty") {
			t.Fatalf("empty answers error = %v", err)
		}
	})

	t.Run("request-user-input 旧单题路径", func(t *testing.T) {
		s, _ := newPromptBridge(t)
		prompt := &promptState{PromptCard: PromptCard{
			ID: "apr-3", SessionID: "sess-1",
			Questions: []bridgeRequestUserInputQuestion{{ID: "q-first"}},
		}, Source: "request-user-input"}
		if err := s.respondPromptRPC(prompt, " opt-9 ", ""); err != nil {
			t.Fatalf("respondPromptRPC(single answer) error = %v", err)
		}
	})

	t.Run("listTasksRPC 转发", func(t *testing.T) {
		s, gateway := newPromptBridge(t)
		gateway.mu.Lock()
		gateway.tasksList = []coreapi.TaskSnapshot{{ID: "task-1"}}
		gateway.mu.Unlock()
		tasks, err := s.listTasksRPC()
		if err != nil || len(tasks) != 1 {
			t.Fatalf("listTasksRPC = %v %v", tasks, err)
		}
	})
}

