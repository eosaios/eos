package webbridge

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

type SteeringMessageState struct {
	TurnID      string `json:"turnId"`
	AfterItemID string `json:"afterItemId,omitempty"`
	Status      string `json:"status"`
}

func (s *BridgeService) SteerChat(sessionID, messageID, content string) (BootstrapState, error) {
	return s.steerRunningMessage(sessionID, RunningChatInput{ID: messageID, Content: strings.TrimSpace(content)})
}

func (s *BridgeService) steerRunningMessage(sessionID string, input RunningChatInput) (BootstrapState, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return BootstrapState{}, err
	}
	s.stateMu.Lock()
	err = s.acceptSteeringLocked(gateway, sessionID, input)
	s.stateMu.Unlock()
	if err != nil {
		return BootstrapState{}, err
	}
	return s.publishQueuedChat(sessionID), nil
}

func (s *BridgeService) acceptSteeringLocked(gateway bridgeRuntimeGateway, sessionID string, input RunningChatInput) error {
	session := s.sessions[sessionID]
	running := s.runningConversations[sessionID]
	if session == nil || !session.Running || running == nil || running.Cancelled || running.TurnID == "" {
		return errors.New(s.t("conversation.steer_unavailable"))
	}
	if previous := findSessionMessageByID(session, input.ID); previous != nil && previous.Steering != nil {
		return nil
	}
	refs, files, err := s.attachmentService().PrepareForSession(input.Attachments, session.WorkspacePath)
	if err != nil {
		return err
	}
	content, attachments, err := adapter.CoreAPITurnInput(input.Content, files)
	if err != nil {
		return err
	}
	state := SteeringMessageState{TurnID: running.TurnID, Status: "received"}
	if assistant := findSessionMessageByID(session, running.AssistantMessageID); assistant != nil && len(assistant.Items) != 0 {
		state.AfterItemID = assistant.Items[len(assistant.Items)-1].ID
	}
	params, err := json.Marshal(map[string]any{
		"session_id": sessionID, "turn_id": running.TurnID, "message_id": input.ID, "input": content,
		"after_item_id": state.AfterItemID, "attachments": attachments, "display_input": input.Content,
		"display_attachments": refs,
	})
	if err != nil {
		return err
	}
	if _, err := gateway.CoreCallRPC(coreCtx(), "turn/steer", params); err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339Nano)
	session.Messages = append(session.Messages, ChatMessage{
		ID: input.ID, Role: "user", Content: input.Content, State: "completed", CreatedAt: now, UpdatedAt: now,
		Attachments: refs, Steering: &state,
	})
	return nil
}

func (s *BridgeService) settleSteeringLocked(session *sessionState, running *runningConversationState) {
	if session == nil || running == nil {
		return
	}
	status := "failed"
	if running.Cancelled {
		status = "cancelled"
	}
	changed := false
	for i := range session.Messages {
		message := &session.Messages[i]
		if message.Steering != nil && message.Steering.TurnID == running.TurnID &&
			(message.Steering.Status == "received" || message.Steering.Status == "delivering") {
			state := *message.Steering
			state.Status = status
			message.Steering = &state
			changed = true
		}
	}
	if changed {
		if err := s.persistSessionForTurnLocked(session, running.TurnID); err != nil {
			s.pushNotificationLocked(s.t("notification.request_failed.title"), err.Error(), "danger")
		}
	}
}

func applySteeringEvent(session *sessionState, event adapter.Event) {
	payload := payloadForItem(event)
	id, status := metadataString(payload["message_id"]), metadataString(payload["status"])
	if status != "delivering" && status != "processed" && status != "failed" && status != "cancelled" {
		return
	}
	message := findSessionMessageByID(session, id)
	if message == nil || message.Steering == nil || message.Steering.TurnID != event.TurnID {
		return
	}
	state := *message.Steering
	state.Status = status
	message.Steering = &state
}

func markRestoredSteeringInterrupted(messages []ChatMessage) bool {
	changed := false
	for index := range messages {
		state := messages[index].Steering
		if state != nil && (state.Status == "received" || state.Status == "delivering") {
			state.Status = "failed"
			changed = true
		}
	}
	return changed
}
