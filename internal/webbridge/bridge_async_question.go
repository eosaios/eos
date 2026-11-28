package webbridge

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

// An async question uses the approval transport but neither consumes an
// approval grant nor enters the ordinary message queue.
type asyncQuestionReceipt struct {
	State      string                                    `json:"state"`
	Answers    map[string]coreapi.RequestUserInputAnswer `json:"answers"`
	FollowupID string                                    `json:"followup_id"`
	CreatedAt  string                                    `json:"created_at"`
}

func (s *BridgeService) resolveAsyncQuestionLocked(prompt *promptState, note string) error {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return err
	}
	session := s.sessions[prompt.SessionID]
	if session == nil {
		return os.ErrNotExist
	}
	if running := s.runningConversations[session.ID]; running != nil && running.Cancelled {
		return errors.New(s.t("request_user_input.closed"))
	}
	params, err := json.Marshal(map[string]string{"approval_id": prompt.ID, "decision": "accept", "reason": note})
	if err != nil {
		return err
	}
	raw, err := gateway.CoreCallRPC(coreCtx(), "approval/respond", params)
	if err != nil {
		return err
	}
	var response struct {
		Accepted bool                  `json:"accepted"`
		Question *asyncQuestionReceipt `json:"question"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return err
	}
	if !response.Accepted || response.Question == nil {
		return errors.New(s.t("request_user_input.receipt_missing"))
	}
	receipt := *response.Question
	s.projectAsyncQuestionLocked(session, prompt, receipt)
	if err := s.persistSessionLocked(session); err != nil {
		s.pushNotificationLocked(s.t("notification.request_failed.title"), err.Error(), "danger")
	}
	if receipt.FollowupID != "" && !slices.Contains(session.QuestionFollowups, receipt.FollowupID) {
		session.QuestionFollowups = append(session.QuestionFollowups, receipt.FollowupID)
		s.startQuestionFollowupLocked(session)
	}
	return nil
}

func (s *BridgeService) startQuestionFollowupLocked(session *sessionState) {
	if session == nil || session.Running || s.runningConversations[session.ID] != nil || len(session.QuestionFollowups) == 0 {
		return
	}
	id := session.QuestionFollowups[0]
	session.QuestionFollowups = session.QuestionFollowups[1:]
	now := time.Now().Format(time.RFC3339Nano)
	assistant := ChatMessage{ID: newID("assistant"), Role: "assistant", State: "streaming",
		CreatedAt: now, UpdatedAt: now, IsPlaceholder: true}
	session.Messages = append(session.Messages, assistant)
	session.Running = true
	session.NeedsAttention = false
	ctx, cancel := context.WithCancel(context.Background())
	s.runningConversations[session.ID] = &runningConversationState{
		AssistantMessageID: assistant.ID, WorkspacePath: session.WorkspacePath, Cancel: cancel, QuestionResponseID: id,
	}
	s.conversationWG.Add(1)
	go s.runConversation(ctx, session.ID, assistant.ID, "", nil, false)
}

func (s *BridgeService) projectAsyncQuestionLocked(session *sessionState, prompt *promptState, receipt asyncQuestionReceipt) {
	original := findSessionMessageByID(session, prompt.AssistantMessageID)
	if original == nil {
		return
	}
	if index := findItemIndex(original.Items, prompt.CallID); index >= 0 {
		item := &original.Items[index]
		if item.Approval != nil {
			item.Approval.State = receipt.State
			item.Approval.Answers = receipt.Answers
			s.emitQuestionItemLocked(session.ID, original.ID, *item)
		}
	}
	if receipt.State == "pending" {
		return
	}
	delete(s.prompts, prompt.ID)
	receiptID := "question-answer:" + prompt.ID
	for i := range session.Messages {
		if findItemIndex(session.Messages[i].Items, receiptID) >= 0 {
			return
		}
	}
	// Append at receipt time. Keep the tool call in its original place.
	var owner *ChatMessage
	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].Role == "assistant" {
			owner = &session.Messages[i]
			break
		}
	}
	if owner == nil {
		return
	}
	title := prompt.Title
	if len(prompt.Questions) > 0 && prompt.Questions[0].Header != "" {
		title = prompt.Questions[0].Header
	}
	pairs := make([]QuestionAnswer, 0, len(prompt.Questions))
	for _, question := range prompt.Questions {
		pairs = append(pairs, QuestionAnswer{
			Question: question.Question, Answer: strings.Join(receipt.Answers[question.ID].Answers, "\n"),
		})
	}
	item := ThreadItem{ID: receiptID, Kind: "status", Level: "async_question_receipt",
		Text: title, Status: "completed", QuestionStatus: receipt.State, QuestionAnswers: pairs}
	owner.Items = append(owner.Items, item)
	owner.IsPlaceholder = false
	s.emitQuestionItemLocked(session.ID, owner.ID, item)
}

func (s *BridgeService) handleAsyncQuestionExpiredLocked(frame conversationEventFrame) bool {
	payload := payloadForItem(frame.event)
	prompt := s.prompts[metadataString(payload["approval_id"])]
	if prompt == nil || !prompt.Background || prompt.SessionID != frame.sessionID {
		return false
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	var receipt asyncQuestionReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return false
	}
	s.projectAsyncQuestionLocked(frame.session, prompt, receipt)
	return true
}

func (s *BridgeService) emitQuestionItemLocked(sessionID, messageID string, item ThreadItem) {
	s.emitConversationDelta(ConversationDeltaPayload{
		SessionID: sessionID, MessageID: messageID, ItemID: item.ID, Kind: item.Kind, Status: item.Status, Item: &item,
	})
}

func (s *BridgeService) restoreAsyncQuestionsLocked(session *sessionState) {
	if !slices.ContainsFunc(session.Messages, func(message ChatMessage) bool {
		return slices.ContainsFunc(message.Items, func(item ThreadItem) bool {
			return item.Approval != nil && item.Approval.Background && item.Approval.State == "pending"
		})
	}) {
		return
	}
	records, err := s.listPendingApprovalsRPC(session.ID)
	if err != nil {
		slog.Warn("restore async questions", "error", err)
		return
	}
	known := make(map[string]bool, len(records))
	for _, record := range records {
		if metadataString(record.Metadata["kind"]) != "async_question" {
			continue
		}
		known[record.ApprovalID] = true
		for i := range session.Messages {
			message := &session.Messages[i]
			if message.turnID != record.TurnID {
				continue
			}
			index := findItemIndex(message.Items, record.CallID)
			if index < 0 || message.Items[index].Approval == nil {
				continue
			}
			approval := message.Items[index].Approval
			if approval.ApprovalID != record.ApprovalID {
				continue
			}
			var answers map[string]coreapi.RequestUserInputAnswer
			raw, err := json.Marshal(record.Metadata["question_answers"])
			if err == nil {
				err = json.Unmarshal(raw, &answers)
			}
			if err != nil {
				slog.Warn("restore question answers", "error", err)
				continue
			}
			prompt := &promptState{PromptCard: PromptCard{
				ID: record.ApprovalID, Kind: "request_user_input", Title: approval.Title, Message: approval.Message,
				SessionID: session.ID, WorkspacePath: session.WorkspacePath, AssistantMessageID: message.ID,
				CallID: record.CallID, Questions: approval.Questions, Background: true,
				Status: "pending", CreatedAt: metadataString(record.Metadata["question_created_at"]),
			}, Source: "request-user-input", AssistantMessageID: message.ID}
			state := metadataString(record.Metadata["question_state"])
			if state == "" || state == "pending" {
				state = "pending"
				s.prompts[prompt.ID] = prompt
			}
			s.projectAsyncQuestionLocked(session, prompt, asyncQuestionReceipt{State: state, Answers: answers})
			break
		}
	}
	// Upgraded clients can have a view of a question the previous runtime
	// discarded. A successful ledger read distinguishes it from a lost RPC.
	for i := range session.Messages {
		message := &session.Messages[i]
		for _, item := range message.Items {
			approval := item.Approval
			if approval == nil || !approval.Background || approval.State != "pending" || known[approval.ApprovalID] {
				continue
			}
			prompt := &promptState{PromptCard: PromptCard{
				ID: approval.ApprovalID, Title: approval.Title, SessionID: session.ID,
				AssistantMessageID: message.ID, CallID: item.ID, Questions: approval.Questions,
			}, AssistantMessageID: message.ID}
			s.projectAsyncQuestionLocked(session, prompt, asyncQuestionReceipt{State: "expired", Answers: approval.Answers})
		}
	}
}

func (s *BridgeService) closeAsyncQuestionsLocked(session *sessionState, assistantID string, running *runningConversationState) bool {
	status := "expired"
	if running.Cancelled {
		status = "cancelled"
	}
	changed := false
	for _, prompt := range s.prompts {
		if !prompt.Background || prompt.SessionID != session.ID || prompt.AssistantMessageID != assistantID {
			continue
		}
		var answers map[string]coreapi.RequestUserInputAnswer
		if message := findSessionMessageByID(session, assistantID); message != nil {
			if i := findItemIndex(message.Items, prompt.CallID); i >= 0 && message.Items[i].Approval != nil {
				answers = message.Items[i].Approval.Answers
			}
		}
		s.projectAsyncQuestionLocked(session, prompt, asyncQuestionReceipt{State: status, Answers: answers})
		changed = true
	}
	return changed
}
