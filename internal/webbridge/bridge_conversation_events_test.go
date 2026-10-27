package webbridge

import (
	"context"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// 403 等失败路径没有任何 turn.item_* 事件，IsPlaceholder 只在 item 事件翻转——
// 终态（failed/completed）必须主动清除占位标志，否则前端一直渲染「思考中」。
func TestSetMessageStatusClearsPlaceholderOnTerminalState(t *testing.T) {
	cases := []struct {
		name            string
		state           string
		wantPlaceholder bool
	}{
		{name: "failed 终态清除占位", state: "failed", wantPlaceholder: false},
		{name: "completed 终态清除占位", state: "completed", wantPlaceholder: false},
		{name: "waiting 等待态保留占位（审批/问询仍在等待）", state: "waiting", wantPlaceholder: true},
		{name: "streaming 进行态保留占位", state: "streaming", wantPlaceholder: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &BridgeService{}
			session := &sessionState{
				ID: "sess-1",
				Messages: []ChatMessage{
					{ID: "msg-1", Role: "assistant", Content: "思考中", State: "streaming", IsPlaceholder: true},
				},
			}
			s.setMessageStatus(session, "msg-1", "HTTP 403: invalid api key", "error", tc.state)
			msg := findSessionMessageByID(session, "msg-1")
			if msg == nil {
				t.Fatal("message not found after setMessageStatus")
			}
			if msg.IsPlaceholder != tc.wantPlaceholder {
				t.Fatalf("IsPlaceholder=%v, want %v", msg.IsPlaceholder, tc.wantPlaceholder)
			}
			if msg.State != tc.state {
				t.Fatalf("State=%q, want %q", msg.State, tc.state)
			}
		})
	}
}

// 失败终态的错误文本必须写入 status item：前端 items 时间线内联渲染错误，
// 随消息持久保留（发送新消息后仍可见）。
func TestSetMessageStatusAppendsErrorStatusItem(t *testing.T) {
	s := &BridgeService{}
	session := &sessionState{
		ID: "sess-1",
		Messages: []ChatMessage{
			{ID: "msg-1", Role: "assistant", Content: "思考中", State: "streaming", IsPlaceholder: true},
		},
	}
	s.setMessageStatus(session, "msg-1", "HTTP 403: invalid api key", "error", "failed")
	msg := findSessionMessageByID(session, "msg-1")
	if msg == nil {
		t.Fatal("message not found after setMessageStatus")
	}
	if len(msg.Items) != 1 {
		t.Fatalf("len(Items)=%d, want 1", len(msg.Items))
	}
	item := msg.Items[0]
	if item.Kind != "status" || item.Level != "error" || item.Text != "HTTP 403: invalid api key" {
		t.Fatalf("unexpected status item: kind=%q level=%q text=%q", item.Kind, item.Level, item.Text)
	}
}

// 「无 item 事件直接完成」的边界：turn.completed 到达但占位标志仍是 true 时，
// completeAssistantMessage 必须清除占位。
func TestCompleteAssistantMessageClearsPlaceholder(t *testing.T) {
	s := &BridgeService{}
	session := &sessionState{
		ID: "sess-1",
		Messages: []ChatMessage{
			{ID: "msg-1", Role: "assistant", Content: "思考中", State: "streaming", IsPlaceholder: true},
		},
	}
	s.completeAssistantMessage(session, "msg-1")
	msg := findSessionMessageByID(session, "msg-1")
	if msg == nil {
		t.Fatal("message not found after completeAssistantMessage")
	}
	if msg.IsPlaceholder {
		t.Fatal("IsPlaceholder still true after completeAssistantMessage")
	}
	if msg.State != "completed" {
		t.Fatalf("State=%q, want completed", msg.State)
	}
}

// 应用退出时进行中的 turn 被杀死，streaming/waiting 的 assistant 消息以死状态
// 持久化；重启加载时必须归一为中断终态——否则消息流永远残留"思考中"占位。
func TestNormalizeInterruptedAssistantMessages(t *testing.T) {
	t.Run("streaming 占位消息归一为 failed + 清占位 + 中断提示", func(t *testing.T) {
		messages := []ChatMessage{
			{ID: "user-1", Role: "user", Content: "你好", State: "completed"},
			{
				ID:            "msg-1",
				Role:          "assistant",
				Content:       "思考中",
				State:         "streaming",
				IsPlaceholder: true,
				Items: []ThreadItem{
					{ID: "tool-1", Kind: "tool_call", ToolName: "shell_exec", Status: "streaming"},
				},
			},
		}
		if !normalizeInterruptedAssistantMessages(messages, "已中断（应用已重启）") {
			t.Fatal("changed = false, want true")
		}
		msg := messages[1]
		if msg.State != "failed" {
			t.Fatalf("State=%q, want failed", msg.State)
		}
		if msg.IsPlaceholder {
			t.Fatal("IsPlaceholder still true after normalize")
		}
		if msg.Items[0].Status != "failed" {
			t.Fatalf("tool item Status=%q, want failed", msg.Items[0].Status)
		}
		last := msg.Items[len(msg.Items)-1]
		if last.Kind != "status" || last.Text != "已中断（应用已重启）" || last.Level != "warning" {
			t.Fatalf("interrupt notice missing: kind=%q text=%q level=%q", last.Kind, last.Text, last.Level)
		}
		// user 消息不受影响
		if messages[0].State != "completed" {
			t.Fatalf("user message State=%q, want completed (untouched)", messages[0].State)
		}
	})

	t.Run("waiting 消息同样归一", func(t *testing.T) {
		messages := []ChatMessage{
			{ID: "msg-1", Role: "assistant", State: "waiting", IsPlaceholder: true},
		}
		if !normalizeInterruptedAssistantMessages(messages, "已中断（应用已重启）") {
			t.Fatal("changed = false, want true")
		}
		if messages[0].State != "failed" || messages[0].IsPlaceholder {
			t.Fatalf("State=%q IsPlaceholder=%v, want failed/false", messages[0].State, messages[0].IsPlaceholder)
		}
	})

	t.Run("终态消息不动，返回 false", func(t *testing.T) {
		messages := []ChatMessage{
			{ID: "msg-1", Role: "assistant", State: "completed"},
			{ID: "msg-2", Role: "assistant", State: "failed"},
		}
		if normalizeInterruptedAssistantMessages(messages, "已中断（应用已重启）") {
			t.Fatal("changed = true, want false (terminal messages untouched)")
		}
		if messages[0].State != "completed" || messages[1].State != "failed" {
			t.Fatalf("terminal states mutated: %q / %q", messages[0].State, messages[1].State)
		}
	})

	// 程序退出 = 用户中止：弹出的待确认审批/问询卡片不应在重启后复活。
	// 前端浮层只按 approval.state=="pending" 投影卡片，归一必须把它翻成终态。
	t.Run("pending 审批随中断消息翻为 cancelled，不再被浮层投影", func(t *testing.T) {
		messages := []ChatMessage{
			{
				ID:    "msg-1",
				Role:  "assistant",
				State: "waiting",
				Items: []ThreadItem{
					{
						ID:       "tool-1",
						Kind:     "tool_call",
						ToolName: "shell_exec",
						Status:   "waiting",
						Approval: &ItemApprovalState{
							ApprovalID: "approval_1",
							Kind:       "approval",
							State:      "pending",
						},
					},
				},
			},
		}
		if !normalizeInterruptedAssistantMessages(messages, "已中断（应用已重启）") {
			t.Fatal("changed = false, want true")
		}
		ap := messages[0].Items[0].Approval
		if ap == nil {
			t.Fatal("Approval nil after normalize")
		}
		if ap.State != "cancelled" {
			t.Fatalf("Approval.State=%q, want cancelled", ap.State)
		}
		if ap.ResolvedAt == "" {
			t.Fatal("Approval.ResolvedAt empty, want timestamp set on cancel")
		}
		// 前端浮层只按 approval.state=="pending" 投影卡片（见 workbench-approvals-logic.ts）；
		// 翻成 cancelled 后不再等于 pending，卡片不会复活。
		if ap.State == "pending" {
			t.Fatal("Approval.State still pending, would re-project as card after restart")
		}
	})
}

type restoreRuntimeGatewayStub struct {
	bridgeRuntimeGateway
	messages           []adapter.SessionMessage
	pendingApprovals   coreapi.PendingApprovalList
	respondedApprovals []string
}

func (g *restoreRuntimeGatewayStub) CoreLoadSessionMessagesRPC(context.Context, string, string) ([]adapter.SessionMessage, error) {
	return append([]adapter.SessionMessage(nil), g.messages...), nil
}

func (g *restoreRuntimeGatewayStub) CoreApprovalListRPC(context.Context, coreapi.PendingApprovalListRequest) (coreapi.PendingApprovalList, error) {
	return g.pendingApprovals, nil
}

func (g *restoreRuntimeGatewayStub) CoreGetSettingsRPC(context.Context) (adapter.GUISettings, error) {
	return adapter.GUISettings{Language: "zh-CN"}, nil
}

func (g *restoreRuntimeGatewayStub) CoreRespondApprovalRPC(_ context.Context, approvalID string, decision coreapi.ApprovalDecision) error {
	if decision != coreapi.ApprovalCancel {
		return nil
	}
	g.respondedApprovals = append(g.respondedApprovals, approvalID)
	return nil
}

func (g *restoreRuntimeGatewayStub) CoreSetCurrentSessionRPC(context.Context, string, string) error {
	return nil
}

func (g *restoreRuntimeGatewayStub) CoreRenameSessionRPC(context.Context, string, string, string) (adapter.SessionMeta, error) {
	return adapter.SessionMeta{}, nil
}

func (g *restoreRuntimeGatewayStub) ThreadCoreIfExists(string) adapter.Core {
	return nil
}

func TestRestoreRuntimeSessionCancelsPendingApprovalsAfterRestart(t *testing.T) {
	gateway := &restoreRuntimeGatewayStub{
		messages: []adapter.SessionMessage{
			{
				Role:    "assistant",
				Content: "思考中",
				Time:    time.Unix(1700000000, 0).UTC(),
				Metadata: map[string]any{
					guiRuntimeMetadataKey: map[string]any{
						"id":    "msg-1",
						"state": "waiting",
					},
				},
			},
		},
		pendingApprovals: coreapi.PendingApprovalList{
			Approvals: []coreapi.PendingApprovalItem{
				{ApprovalID: "approval-1", SessionID: "sess-1"},
				{ApprovalID: "approval-2", SessionID: "sess-1"},
			},
		},
	}
	s := &BridgeService{
		runtimeGateway: gateway,
		sessions:       map[string]*sessionState{},
		prompts: map[string]*promptState{
			"approval-1": {PromptCard: PromptCard{ID: "approval-1", SessionID: "sess-1"}},
			"approval-x": {PromptCard: PromptCard{ID: "approval-x", SessionID: "sess-2"}},
		},
		saveSessionMessages: func(adapter.Core, string, string, []adapter.SessionMessage) (string, error) {
			return "sess-1", nil
		},
	}

	session := s.restoreRuntimeSessionFromMetaLocked(adapter.SessionMeta{ID: "sess-1", Title: "历史会话"}, `C:\workspace`)
	if session == nil {
		t.Fatal("session = nil, want restored session")
	}
	if len(session.Messages) != 1 || session.Messages[0].State != "failed" {
		t.Fatalf("restored messages = %+v, want normalized failed assistant message", session.Messages)
	}
	if len(gateway.respondedApprovals) != 2 {
		t.Fatalf("responded approvals = %v, want 2 cancelled approvals", gateway.respondedApprovals)
	}
	if _, ok := s.prompts["approval-1"]; ok {
		t.Fatal("approval-1 prompt still present after restart cancel")
	}
	if _, ok := s.prompts["approval-x"]; !ok {
		t.Fatal("other-session prompt was removed unexpectedly")
	}
}
