package webbridge

import (
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func TestIsPromptResolvedEvent(t *testing.T) {
	cases := map[string]bool{
		"tool.approval_approved": true,
		"tool.approval_rejected": true,
		"approval.responded":     true,
		"inquiry.responded":      true,
		"tool.approval_required": false,
		"approval.required":      false,
		"turn.completed":         false,
		"":                       false,
	}
	for kind, want := range cases {
		if got := isPromptResolvedEvent(kind); got != want {
			t.Fatalf("isPromptResolvedEvent(%q) = %v, want %v", kind, got, want)
		}
	}
}

func TestResolutionApprovalIDPrefersPayload(t *testing.T) {
	tests := []struct {
		name  string
		event adapter.Event
		want  string
	}{
		{
			name:  "approval_id from payload",
			event: adapter.Event{Payload: map[string]any{"approval_id": "approval-123"}},
			want:  "approval-123",
		},
		{
			name:  "inquiry_id from payload",
			event: adapter.Event{Payload: map[string]any{"inquiry_id": "inquiry-456"}},
			want:  "inquiry-456",
		},
		{
			name:  "payload empty falls back to RequestID",
			event: adapter.Event{RequestID: "req-789"},
			want:  "req-789",
		},
		{
			name:  "approval_id takes precedence over RequestID",
			event: adapter.Event{RequestID: "req-789", Payload: map[string]any{"approval_id": "approval-1"}},
			want:  "approval-1",
		},
		{
			name:  "Data fallback when Payload empty",
			event: adapter.Event{Data: map[string]any{"approval_id": "approval-from-data"}},
			want:  "approval-from-data",
		},
		{
			name:  "nothing available returns empty",
			event: adapter.Event{},
			want:  "",
		},
		{
			name:  "whitespace-only treated as empty",
			event: adapter.Event{Payload: map[string]any{"approval_id": "   "}},
			want:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolutionApprovalID(tc.event); got != tc.want {
				t.Fatalf("resolutionApprovalID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHandleConversationResolutionLockedDeletesMatchingPrompt(t *testing.T) {
	s := &BridgeService{prompts: map[string]*promptState{}}
	s.prompts["approval-1"] = &promptState{
		PromptCard: PromptCard{ID: "approval-1", Kind: "approval"},
	}

	frame := conversationEventFrame{
		sessionID: "session-1",
		kind:      "tool.approval_approved",
		event:     adapter.Event{Payload: map[string]any{"approval_id": "approval-1"}},
	}
	result := s.handleConversationResolutionLocked(frame)

	if !result.emitBootstrap {
		t.Fatalf("emitBootstrap = false, want true (resolved must trigger immediate sync)")
	}
	if _, stillThere := s.prompts["approval-1"]; stillThere {
		t.Fatalf("prompt not deleted from s.prompts after resolved event")
	}
}

// 内核侧 resolved（切 never 模式 auto-approve / 无人值守 auto-deny / 旁路决策）时，
// 消息流的"等待确认…"状态行必须替换为决策结果——否则随后到达的 ResolvePrompt
// 扑空（ErrNotExist），状态行永远残留，用户分不清"运行中"与"卡死"。
func TestHandleConversationResolutionLockedUpdatesMessageStatus(t *testing.T) {
	cases := []struct {
		name      string
		kind      string
		wantLevel string
	}{
		{name: "approved 映射为已允许(success)", kind: "tool.approval_approved", wantLevel: "success"},
		{name: "rejected 映射为已拒绝(warning)", kind: "tool.approval_rejected", wantLevel: "warning"},
		{name: "responded 映射为已处理(info)", kind: "approval.responded", wantLevel: "info"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &BridgeService{prompts: map[string]*promptState{}}
			s.prompts["approval-1"] = &promptState{
				PromptCard:         PromptCard{ID: "approval-1", Kind: "approval"},
				AssistantMessageID: "msg-1",
			}
			session := &sessionState{
				ID: "session-1",
				Messages: []ChatMessage{
					{
						ID:    "msg-1",
						Role:  "assistant",
						State: "waiting",
						Items: []ThreadItem{
							{ID: "st-1", Kind: "status", Text: "等待确认…", Level: "warning", Status: "waiting"},
						},
					},
				},
			}

			frame := conversationEventFrame{
				session:   session,
				sessionID: "session-1",
				kind:      tc.kind,
				event:     adapter.Event{Payload: map[string]any{"approval_id": "approval-1"}},
			}
			result := s.handleConversationResolutionLocked(frame)

			if !result.emitBootstrap {
				t.Fatalf("emitBootstrap = false, want true")
			}
			msg := findSessionMessageByID(session, "msg-1")
			if msg == nil {
				t.Fatal("message not found")
			}
			if len(msg.Items) != 1 {
				t.Fatalf("len(Items) = %d, want 1 (status item should be replaced in place)", len(msg.Items))
			}
			item := msg.Items[0]
			if item.Text == "等待确认…" {
				t.Fatalf("status text not updated: still %q", item.Text)
			}
			if item.Level != tc.wantLevel {
				t.Fatalf("status level = %q, want %q", item.Level, tc.wantLevel)
			}
			if item.Status != "completed" {
				t.Fatalf("status = %q, want completed (approval row should be settled after resolution)", item.Status)
			}
		})
	}
}

func TestHandleConversationResolutionLockedIdempotentWhenMissing(t *testing.T) {
	s := &BridgeService{prompts: map[string]*promptState{}}
	// 不预置 approval-1，模拟"已被 ResolvePrompt RPC 删除"或"不在本壳层管辖"。

	frame := conversationEventFrame{
		sessionID: "session-1",
		kind:      "tool.approval_rejected",
		event:     adapter.Event{Payload: map[string]any{"approval_id": "approval-missing"}},
	}
	result := s.handleConversationResolutionLocked(frame)

	if result.emitBootstrap {
		t.Fatalf("emitBootstrap = true, want false (no change → no emit)")
	}
	if len(s.prompts) != 0 {
		t.Fatalf("s.prompts mutated unexpectedly: %v", s.prompts)
	}
}

// 回归：审批悬起期间内核又推了 tool_call，把"等待确认…"压在下面（不再是末尾）。
// resolved 到达时必须回扫原位替换该 waiting 行，而不是在末尾追加新行——否则
// 用户确认后旧的"等待确认…"残留，状态看似没变。
func TestResolutionReplacesBuriedWaitingStatusItem(t *testing.T) {
	s := &BridgeService{prompts: map[string]*promptState{}}
	s.prompts["approval-1"] = &promptState{
		PromptCard:         PromptCard{ID: "approval-1", Kind: "approval"},
		AssistantMessageID: "msg-1",
	}
	session := &sessionState{
		ID: "session-1",
		Messages: []ChatMessage{
			{
				ID:    "msg-1",
				Role:  "assistant",
				State: "waiting",
				Items: []ThreadItem{
					{ID: "st-1", Kind: "status", Text: "等待确认…", Level: "warning", Status: "waiting"},
					{ID: "tool-1", Kind: "tool_call", ToolName: "shell_exec", Status: "completed"},
					{ID: "tool-2", Kind: "tool_call", ToolName: "read_file", Status: "completed"},
				},
			},
		},
	}

	frame := conversationEventFrame{
		session:   session,
		sessionID: "session-1",
		kind:      "tool.approval_approved",
		event:     adapter.Event{Payload: map[string]any{"approval_id": "approval-1"}},
	}
	s.handleConversationResolutionLocked(frame)

	msg := findSessionMessageByID(session, "msg-1")
	if msg == nil {
		t.Fatal("message not found")
	}
	if len(msg.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3 (waiting row replaced in place, no append)", len(msg.Items))
	}
	first := msg.Items[0]
	if first.Text == "等待确认…" {
		t.Fatalf("buried waiting status not replaced: still %q", first.Text)
	}
	if first.Status != "completed" {
		t.Fatalf("status = %q, want completed (approval row should be settled)", first.Status)
	}
	// 确认没有新增一条 status：status 行应仍只有一条。
	statusCount := 0
	for _, it := range msg.Items {
		if it.Kind == "status" {
			statusCount++
		}
	}
	if statusCount != 1 {
		t.Fatalf("status item count = %d, want 1 (no duplicate appended)", statusCount)
	}
}

// 串行语义：一次只应有一个活跃待确认。新审批触发（beginMessageStatus）会先把
// 上一轮残留的旧 waiting 行收尾（completed），再追加本次的新 waiting 行；resolved
// 只更新当前审批那条。模拟"审批1等待→resolved→审批2触发"的完整序列。
func TestSerialApprovalLifecycleKeepsSingleWaitingRow(t *testing.T) {
	s := &BridgeService{prompts: map[string]*promptState{}}
	session := &sessionState{
		ID: "session-1",
		Messages: []ChatMessage{
			{ID: "msg-1", Role: "assistant", State: "streaming"},
		},
	}

	// 审批1触发 → 一条 waiting 行。
	s.beginMessageStatus(session, "msg-1", "等待确认…", "warning")
	msg := findSessionMessageByID(session, "msg-1")
	waiting := countWaitingStatus(msg)
	if waiting != 1 {
		t.Fatalf("after approval1 begin, waiting rows = %d, want 1", waiting)
	}

	// 审批1 resolved（走 ResolvePrompt 同款的 setMessageStatusWithItemState）→ 该行收尾为 completed。
	s.prompts["a1"] = &promptState{PromptCard: PromptCard{ID: "a1", Kind: "approval"}, AssistantMessageID: "msg-1"}
	s.setMessageStatusWithItemState(session, "msg-1", "已允许", "info", "streaming", "completed")
	waiting = countWaitingStatus(msg)
	if waiting != 0 {
		t.Fatalf("after approval1 resolved, waiting rows = %d, want 0", waiting)
	}

	// turn 恢复跑工具，审批2触发 → 旧行保持 completed/streaming，新增一条 waiting。
	msg.Items = append(msg.Items, ThreadItem{ID: "tool-1", Kind: "tool_call", ToolName: "shell_exec", Status: "completed"})
	s.beginMessageStatus(session, "msg-1", "等待确认…", "warning")
	waiting = countWaitingStatus(msg)
	if waiting != 1 {
		t.Fatalf("after approval2 begin, waiting rows = %d, want 1 (only current)", waiting)
	}
	// 旧的审批1行不应被改回 waiting：除最后一条（当前审批）外无 waiting。
	statusRows := 0
	for idx, it := range msg.Items {
		if it.Kind != "status" {
			continue
		}
		statusRows++
		isLast := idx == len(msg.Items)-1
		if it.Status == "waiting" && !isLast {
			t.Fatalf("stale status row idx=%d text=%q unexpectedly waiting", idx, it.Text)
		}
	}
	_ = statusRows
}

func countWaitingStatus(msg *ChatMessage) int {
	n := 0
	for _, it := range msg.Items {
		if it.Kind == "status" && it.Status == "waiting" {
			n++
		}
	}
	return n
}

func TestHandleConversationResolutionLockedNoApprovalIDNoOp(t *testing.T) {
	s := &BridgeService{prompts: map[string]*promptState{}}
	s.prompts["approval-1"] = &promptState{
		PromptCard: PromptCard{ID: "approval-1"},
	}

	frame := conversationEventFrame{
		sessionID: "session-1",
		kind:      "approval.responded",
		event:     adapter.Event{}, // 既无 payload approval_id，也无 RequestID
	}
	result := s.handleConversationResolutionLocked(frame)

	if result.emitBootstrap {
		t.Fatalf("emitBootstrap = true, want false (no approval id → no-op)")
	}
	if _, missing := s.prompts["approval-1"]; !missing {
		t.Fatalf("prompt should remain when event carries no approval id")
	}
}

// ResolvePrompt 对"已被内核/旁路 resolved"的审批必须幂等成功（nil error），
// 不能返回 ErrNotExist 让前端误报"处理审批失败"——用户放行意图已达成，
// 状态行由 resolved 事件路径负责更新。
func TestResolvePromptIdempotentWhenAlreadyResolved(t *testing.T) {
	svc := &CommandService{bridge: &BridgeService{prompts: map[string]*promptState{}}}
	if _, err := svc.ResolvePrompt("approval-missing", "允许", ""); err != nil {
		t.Fatalf("ResolvePrompt on already-resolved prompt returned error: %v (want nil, idempotent success)", err)
	}
}

func TestSettlePromptLockedDeletesPromptAndSettlesWaitingStatus(t *testing.T) {
	s := &BridgeService{
		prompts: map[string]*promptState{
			"approval-1": {
				PromptCard: PromptCard{
					ID:                 "approval-1",
					Kind:               "approval",
					SessionID:          "session-1",
					AssistantMessageID: "msg-1",
				},
				AssistantMessageID: "msg-1",
			},
		},
		sessions: map[string]*sessionState{
			"session-1": {
				ID: "session-1",
				Messages: []ChatMessage{
					{
						ID:    "msg-1",
						Role:  "assistant",
						State: "waiting",
						Items: []ThreadItem{
							{ID: "st-1", Kind: "status", Text: "等待确认…", Level: "warning", Status: "waiting"},
						},
					},
				},
			},
		},
	}

	session := s.settlePromptLocked(nil, s.prompts["approval-1"], "已允许", "success", "streaming")
	if session == nil {
		t.Fatal("settlePromptLocked returned nil session")
	}
	if _, exists := s.prompts["approval-1"]; exists {
		t.Fatal("prompt still exists after settlePromptLocked")
	}
	msg := findSessionMessageByID(session, "msg-1")
	if msg == nil {
		t.Fatal("message not found")
	}
	if msg.State != "streaming" {
		t.Fatalf("message state = %q, want streaming", msg.State)
	}
	if len(msg.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(msg.Items))
	}
	if msg.Items[0].Text != "已允许" || msg.Items[0].Status != "completed" {
		t.Fatalf("status item = %+v, want 已允许/completed", msg.Items[0])
	}
}

// TestUnsettlePromptLockedRestoresPendingState 验证 respond RPC 失败后的回滚
// （settle 的逆操作）：
//  1. prompt 重新挂回 s.prompts（ResolvePrompt 逆索引恢复，用户可重试）；
//  2. ToolCall item 的 Approval.State 翻回 pending、ResolvedAt 清空，并 delta 推送；
//  3. 状态行从「已允许」翻回挂起文案（waiting），消息整体保持 waiting。
func TestUnsettlePromptLockedRestoresPendingState(t *testing.T) {
	var emitted []ConversationDeltaPayload
	s := &BridgeService{
		prompts: map[string]*promptState{},
		emitEvent: func(name string, data any) {
			if name == conversationDeltaEventName {
				if payload, ok := data.(ConversationDeltaPayload); ok {
					emitted = append(emitted, payload)
				}
			}
		},
	}
	statusKey := promptStatusKey("approval_1")
	session := &sessionState{
		ID: "session-1",
		Messages: []ChatMessage{
			{ID: "msg-1", Role: "assistant", State: "streaming"},
		},
	}
	// settle 后的现场：status 行已收尾为「已允许」，item approval 已翻 approved。
	session.Messages[0].Items = []ThreadItem{
		{ID: statusKey, Kind: "status", Text: "已允许", Level: "success", Status: "completed"},
		{ID: "call_abc", Kind: "tool_call", ToolName: "shell", Status: "streaming",
			Approval: &ItemApprovalState{ApprovalID: "approval_1", Kind: "approval", State: "approved", ResolvedAt: "2026-08-13T02:34:45Z"}},
	}
	prompt := &promptState{
		PromptCard:         PromptCard{ID: "approval_1", Kind: "approval", SessionID: "session-1", CallID: "call_abc"},
		AssistantMessageID: "msg-1",
	}

	s.unsettlePromptLocked(session, prompt)

	if _, exists := s.prompts["approval_1"]; !exists {
		t.Fatal("prompt should be re-attached to s.prompts after unsettle")
	}
	msg := findSessionMessageByID(session, "msg-1")
	if msg == nil {
		t.Fatal("message not found")
	}
	if msg.State != "waiting" {
		t.Fatalf("message state = %q, want waiting", msg.State)
	}
	statusIdx := findStatusItemIndex(msg.Items, statusKey)
	if statusIdx < 0 {
		t.Fatal("status item not found")
	}
	if got := msg.Items[statusIdx]; got.Text != "等待确认…" || got.Status != "waiting" {
		t.Fatalf("status item = %+v, want 等待确认…/waiting", got)
	}
	idx := findItemIndex(msg.Items, "call_abc")
	if idx < 0 {
		t.Fatal("tool_call item not found")
	}
	if approval := msg.Items[idx].Approval; approval == nil || approval.State != "pending" || approval.ResolvedAt != "" {
		t.Fatalf("item approval = %+v, want pending with empty ResolvedAt", approval)
	}
	var itemDelta *ConversationDeltaPayload
	for i, d := range emitted {
		if d.ItemID == "call_abc" {
			itemDelta = &emitted[i]
		}
	}
	if itemDelta == nil {
		t.Fatalf("no delta emitted for tool_call item; got %d deltas", len(emitted))
	}
	if itemDelta.Item == nil || itemDelta.Item.Approval == nil || itemDelta.Item.Approval.State != "pending" {
		t.Fatalf("delta item approval = %+v, want pending", itemDelta.Item)
	}
}
