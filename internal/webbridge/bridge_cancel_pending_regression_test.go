package webbridge

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Stop must withdraw pending item projections before signalling the stream.
// An interrupt failure remains visible; it cannot leave actionable shell cards.
func TestCancelSessionWithdrawsPendingItems(t *testing.T) {
	for _, mode := range []string{"active_stream", "no_stream", "interrupt_failure"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := newRustStdioBridgeWithMockForTest(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			session := &sessionState{ID: "cancel-fixture", WorkspacePath: t.TempDir(), Running: true, NeedsAttention: true,
				Messages: []ChatMessage{{ID: "assistant-fixture", Role: "assistant", State: "waiting", Items: []ThreadItem{
					{ID: "pending-call", Kind: "tool_call", Status: "waiting", Approval: &ItemApprovalState{ApprovalID: "pending-approval", Kind: "approval", State: "pending"}},
					{ID: "pending-question", Kind: "tool_call", Status: "waiting", Approval: &ItemApprovalState{ApprovalID: "pending-inquiry", Kind: "request_user_input", State: "pending"}},
					{ID: "resolved-call", Kind: "tool_call", Status: "completed", Approval: &ItemApprovalState{ApprovalID: "resolved-approval", Kind: "approval", State: "approved", ResolvedAt: "original-time"}},
				}}}}
			var deltaMu sync.Mutex
			deltas := map[string]string{}
			s.emitEvent = func(_ string, payload any) {
				d, ok := payload.(ConversationDeltaPayload)
				if ok && d.Item != nil && d.Item.Approval != nil {
					deltaMu.Lock()
					deltas[d.ItemID] = d.Item.Approval.State
					deltaMu.Unlock()
				}
			}
			interrupted := 0
			s.stateMu.Lock()
			s.sessions[session.ID] = session
			s.currentSessionID = session.ID
			for id, call := range map[string]string{"pending-approval": "pending-call", "pending-inquiry": "pending-question"} {
				kind := "approval"
				if id == "pending-inquiry" {
					kind = "request_user_input"
				}
				s.prompts[id] = &promptState{PromptCard: PromptCard{ID: id, Kind: kind, SessionID: session.ID, CallID: call}, AssistantMessageID: "assistant-fixture"}
			}
			unrelated := &promptState{PromptCard: PromptCard{ID: "unrelated-approval", Kind: "approval", SessionID: "unrelated-session"}}
			s.prompts[unrelated.ID] = unrelated
			if mode != "no_stream" {
				s.runningConversations[session.ID] = &runningConversationState{AssistantMessageID: "assistant-fixture", Cancel: cancel, Interrupt: func(context.Context) error {
					interrupted++
					s.stateMu.RLock()
					for _, id := range []string{"pending-approval", "pending-inquiry"} {
						if s.prompts[id] != nil {
							t.Errorf("pending %s survived until interrupt", id)
						}
					}
					s.stateMu.RUnlock()
					if ctx.Err() != context.Canceled {
						t.Errorf("interrupt preceded context cancellation: %v", ctx.Err())
					}
					if mode == "interrupt_failure" {
						return errors.New("synthetic interrupt failure")
					}
					return nil
				}}
			}
			s.stateMu.Unlock()
			state, err := s.CancelSession(session.ID)
			if mode == "interrupt_failure" {
				if err == nil || !strings.Contains(err.Error(), "synthetic interrupt failure") {
					t.Fatalf("interrupt error lost: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			s.stateMu.RLock()
			pending := []bool{s.prompts["pending-approval"] != nil, s.prompts["pending-inquiry"] != nil}
			itemStates := []string{session.Messages[0].Items[0].Approval.State, session.Messages[0].Items[1].Approval.State}
			resolved := *session.Messages[0].Items[2].Approval
			timestamps := session.Messages[0].Items[0].Approval.ResolvedAt != "" && session.Messages[0].Items[1].Approval.ResolvedAt != ""
			running, attention := session.Running, session.NeedsAttention
			untouched := s.prompts[unrelated.ID] == unrelated
			s.stateMu.RUnlock()
			got := struct {
				Running, Attention, Timestamps, Unrelated bool
				Pending                                   []bool
				States                                    []string
			}{running, attention, timestamps, untouched, pending, itemStates}
			want := struct {
				Running, Attention, Timestamps, Unrelated bool
				Pending                                   []bool
				States                                    []string
			}{false, false, true, true, []bool{false, false}, []string{"cancelled", "cancelled"}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("cancel projection = %#v; want %#v", got, want)
			}
			if !reflect.DeepEqual(resolved, ItemApprovalState{ApprovalID: "resolved-approval", Kind: "approval", State: "approved", ResolvedAt: "original-time"}) {
				t.Errorf("resolved history changed: %#v", resolved)
			}
			deltaMu.Lock()
			emitted := map[string]string{}
			for k, v := range deltas {
				emitted[k] = v
			}
			deltaMu.Unlock()
			if !reflect.DeepEqual(emitted, map[string]string{"pending-call": "cancelled", "pending-question": "cancelled"}) {
				t.Errorf("projected item deltas = %#v", emitted)
			}
			returnedAssistant := false
			for _, message := range state.Messages {
				if message.ID == "assistant-fixture" {
					returnedAssistant = true
				}
				for _, item := range message.Items {
					if item.Approval != nil && item.Approval.State == "pending" {
						t.Errorf("returned bootstrap retained pending item: %#v", item)
					}
				}
			}
			if mode != "interrupt_failure" && !returnedAssistant {
				t.Error("successful cancel returned no assistant projection")
			}
			if mode != "no_stream" && (ctx.Err() != context.Canceled || interrupted != 1) {
				t.Errorf("cancel/interrupt propagation: ctx=%v interrupts=%d", ctx.Err(), interrupted)
			}
			if mode == "no_stream" && (ctx.Err() != nil || interrupted != 0) {
				t.Errorf("inactive stream received signals: ctx=%v interrupts=%d", ctx.Err(), interrupted)
			}
		})
	}
}
