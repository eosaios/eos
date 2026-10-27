package webbridge

import (
	"testing"
)

// fullAccessSettleFixture 构造带待审 item 的最小 BridgeService（复用
// sandboxModeGatewayStub 隔离内核 RPC），并捕获 conversation-delta 推送。
func fullAccessSettleFixture() (*BridgeService, *[]ConversationDeltaPayload) {
	var deltas []ConversationDeltaPayload
	s := &BridgeService{
		runtimeGateway: &sandboxModeGatewayStub{},
		prompts:        map[string]*promptState{},
		sessions:       map[string]*sessionState{},
		emitEvent: func(name string, payload any) {
			if delta, ok := payload.(ConversationDeltaPayload); ok {
				deltas = append(deltas, delta)
			}
		},
	}
	return s, &deltas
}

// TestApplySandboxModeSemanticsFullAccessFlipsPendingItemsWithoutPrompts 复现用户
// 报告的卡死形态：prompt 已被其它路径删除（停止 fallback / 回滚 / 会话加载），
// 仅剩 item 层的 pending 审批态。进入完全访问必须直接翻转 item（内核对全部
// pending 一律 accept），否则横幅永久残留且点按钮幂等空转。
func TestApplySandboxModeSemanticsFullAccessFlipsPendingItemsWithoutPrompts(t *testing.T) {
	s, deltas := fullAccessSettleFixture()
	s.sessions["session-1"] = &sessionState{
		ID: "session-1",
		Messages: []ChatMessage{
			{
				ID: "msg-1", Role: "assistant",
				Items: []ThreadItem{
					{
						ID: "call-1", Kind: "tool_call",
						Approval: &ItemApprovalState{ApprovalID: "approval_1", Kind: "approval", State: "pending"},
					},
					{
						ID: "call-2", Kind: "tool_call",
						Approval: &ItemApprovalState{ApprovalID: "approval_2", Kind: "request_user_input", State: "pending"},
					},
				},
			},
		},
	}

	if err := s.applySandboxModeSemantics("c:/ws", "danger-full-access"); err != nil {
		t.Fatalf("applySandboxModeSemantics(danger-full-access) error = %v", err)
	}

	items := s.sessions["session-1"].Messages[0].Items
	if got := items[0].Approval.State; got != "approved" {
		t.Errorf("approval item state = %q, want approved", got)
	}
	if items[0].Approval.ResolvedAt == "" {
		t.Error("approval item ResolvedAt empty, want timestamp")
	}
	// 问询卡不是权限审批，完全访问不代替用户作答——必须保持 pending 可答。
	if got := items[1].Approval.State; got != "pending" {
		t.Errorf("request_user_input item state = %q, want pending (full access must not answer plan questions)", got)
	}
	// 翻转的审批 item 必须有 delta 推送（前端据此收起浮层卡片）。
	flipped := map[string]bool{}
	for _, delta := range *deltas {
		if delta.Item != nil && delta.Item.Approval != nil && delta.Item.Approval.State != "pending" {
			flipped[delta.ItemID] = true
		}
	}
	if !flipped["call-1"] {
		t.Errorf("conversation delta missing flipped approval item call-1")
	}
	if flipped["call-2"] {
		t.Errorf("conversation delta must not flip request_user_input item call-2")
	}
}

// TestApplySandboxModeSemanticsFullAccessKeepsRequestUserInputPending 问询卡不是
// 权限审批：进入完全访问后必须保持 pending、可继续作答（内核 enter_full_access
// 同样跳过 request_user_input 条目）。
func TestApplySandboxModeSemanticsFullAccessKeepsRequestUserInputPending(t *testing.T) {
	s, _ := fullAccessSettleFixture()
	s.sessions["session-1"] = &sessionState{
		ID: "session-1",
		Messages: []ChatMessage{
			{
				ID: "msg-1", Role: "assistant",
				Items: []ThreadItem{
					{
						ID: "call-1", Kind: "tool_call",
						Approval: &ItemApprovalState{ApprovalID: "approval_9", Kind: "request_user_input", State: "pending"},
					},
				},
			},
		},
	}
	s.prompts["approval_9"] = &promptState{
		PromptCard: PromptCard{
			ID: "approval_9", Kind: "request_user_input", SessionID: "session-1", CallID: "call-1",
		},
		AssistantMessageID: "msg-1",
		Source:             "request-user-input",
	}

	if err := s.applySandboxModeSemantics("c:/ws", "danger-full-access"); err != nil {
		t.Fatalf("applySandboxModeSemantics(danger-full-access) error = %v", err)
	}

	if _, ok := s.prompts["approval_9"]; !ok {
		t.Fatal("request_user_input prompt must survive full access (still answerable)")
	}
	item := s.sessions["session-1"].Messages[0].Items[0]
	if got := item.Approval.State; got != "pending" {
		t.Fatalf("request_user_input item state = %q, want pending", got)
	}
}

// TestApplySandboxModeSemanticsFullAccessFlipsItemWhenCallIDLinkBroken prompt 存在
// 但 CallID 链接失配（找不到对应 item）时，prompt 收口翻不了 item——item 扫表兜底
// 必须仍然把横幅收起。
func TestApplySandboxModeSemanticsFullAccessFlipsItemWhenCallIDLinkBroken(t *testing.T) {
	s, _ := fullAccessSettleFixture()
	s.sessions["session-1"] = &sessionState{
		ID: "session-1",
		Messages: []ChatMessage{
			{
				ID: "msg-1", Role: "assistant",
				Items: []ThreadItem{
					{
						ID: "call-1", Kind: "tool_call",
						Approval: &ItemApprovalState{ApprovalID: "approval_1", Kind: "approval", State: "pending"},
					},
				},
			},
		},
	}
	// CallID 指向不存在的 item：settleItemApprovalLocked 按设计静默跳过。
	s.prompts["approval_1"] = &promptState{
		PromptCard: PromptCard{
			ID: "approval_1", Kind: "approval", SessionID: "session-1", CallID: "call-missing",
		},
		AssistantMessageID: "msg-1",
	}

	if err := s.applySandboxModeSemantics("c:/ws", "danger-full-access"); err != nil {
		t.Fatalf("applySandboxModeSemantics(danger-full-access) error = %v", err)
	}

	item := s.sessions["session-1"].Messages[0].Items[0]
	if got := item.Approval.State; got != "approved" {
		t.Fatalf("item state = %q, want approved (sweep rescues broken CallID link)", got)
	}
}

// TestSettleSessionPromptsCancelledLockedFlipsItems 停止撤回必须把 item 翻成
// cancelled 并推送 delta——此前停止 fallback 分支裸删 s.prompts，item 永远 pending。
func TestSettleSessionPromptsCancelledLockedFlipsItems(t *testing.T) {
	s, deltas := fullAccessSettleFixture()
	session := &sessionState{
		ID: "session-1",
		Messages: []ChatMessage{
			{
				ID: "msg-1", Role: "assistant",
				Items: []ThreadItem{
					{
						ID: "call-1", Kind: "tool_call",
						Approval: &ItemApprovalState{ApprovalID: "approval_1", Kind: "approval", State: "pending"},
					},
				},
			},
		},
	}
	s.sessions["session-1"] = session
	s.prompts["approval_1"] = &promptState{
		PromptCard: PromptCard{
			ID: "approval_1", Kind: "approval", SessionID: "session-1", CallID: "call-1",
		},
		AssistantMessageID: "msg-1",
	}

	s.settleSessionPromptsCancelledLocked(session)

	if len(s.prompts) != 0 {
		t.Fatalf("prompts = %d items, want 0 (cancelled on stop)", len(s.prompts))
	}
	item := session.Messages[0].Items[0]
	if got := item.Approval.State; got != "cancelled" {
		t.Fatalf("item state = %q, want cancelled", got)
	}
	found := false
	for _, delta := range *deltas {
		if delta.ItemID == "call-1" && delta.Item != nil {
			found = true
		}
	}
	if !found {
		t.Error("conversation delta for call-1 not emitted")
	}
}
