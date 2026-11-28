package webbridge

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/eosaios/eos/pkg/chatqueue"
)

type QueuedChatMessage = chatqueue.Message[AttachmentRef]
type ChatQueue = chatqueue.State[AttachmentRef]
type RunningChatInput = chatqueue.Input

func (s *BridgeService) SubmitRunningMessage(sessionID string, input RunningChatInput) (BootstrapState, error) {
	input.Content = strings.TrimSpace(input.Content)
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" || len(input.ID) > 128 || input.Content == "" && len(compactPaths(input.Attachments)) == 0 {
		return BootstrapState{}, errors.New(s.t("conversation.input_required"))
	}
	if input.Mode == "steer" {
		return s.steerRunningMessage(sessionID, input)
	}
	if input.Mode != "queue" {
		return BootstrapState{}, errors.New(s.t("conversation.queue_unavailable"))
	}
	s.stateMu.Lock()
	session := s.sessions[sessionID]
	if session == nil {
		s.stateMu.Unlock()
		return BootstrapState{}, errors.New(s.t("conversation.queue_unavailable"))
	}
	for _, entry := range session.Queue.Messages {
		if entry.ID == input.ID {
			s.stateMu.Unlock()
			return s.bootstrapForSession(sessionID), nil
		}
	}
	if findSessionMessageByID(session, input.ID) != nil {
		s.stateMu.Unlock()
		return s.bootstrapForSession(sessionID), nil
	}
	refs, _, err := s.attachmentService().PrepareForSession(input.Attachments, session.WorkspacePath)
	if err != nil {
		s.stateMu.Unlock()
		return BootstrapState{}, err
	}
	previous := session.Queue
	session.Queue.Messages = append(session.Queue.Messages, QueuedChatMessage{
		ID: input.ID, Content: input.Content, Reasoning: input.Reasoning, Attachments: refs,
	})
	if session.NeedsAttention || (s.runningConversations[sessionID] != nil && s.runningConversations[sessionID].Cancelled) {
		session.Queue.Paused = true
	}
	if err := s.saveChatQueueLocked(session); err != nil {
		session.Queue = previous
		s.stateMu.Unlock()
		return BootstrapState{}, err
	}
	s.startNextQueuedMessageLocked(session)
	s.stateMu.Unlock()
	return s.publishQueuedChat(sessionID), nil
}

// Edit and removal share the same lock as reservation. A stale editor can
// never mutate a message after it has been handed to the turn-start path.
func (s *BridgeService) EditQueuedMessage(sessionID, messageID, content string, attachments []string) (BootstrapState, error) {
	content = strings.TrimSpace(content)
	s.stateMu.Lock()
	session, index, err := s.editableQueueEntryLocked(sessionID, messageID)
	if err == nil {
		var refs []AttachmentRef
		refs, _, err = s.attachmentService().PrepareForSession(attachments, session.WorkspacePath)
		if err == nil && content == "" && len(refs) == 0 {
			err = errors.New(s.t("conversation.input_required"))
		}
		if err == nil {
			previous := session.Queue.Messages[index]
			session.Queue.Messages[index].Content = content
			session.Queue.Messages[index].Attachments = refs
			if err = s.saveChatQueueLocked(session); err != nil {
				session.Queue.Messages[index] = previous
			}
		}
	}
	s.stateMu.Unlock()
	if err != nil {
		return BootstrapState{}, err
	}
	return s.publishQueuedChat(sessionID), nil
}

func (s *BridgeService) RemoveQueuedMessage(sessionID, messageID string) (BootstrapState, error) {
	s.stateMu.Lock()
	session, index, err := s.editableQueueEntryLocked(sessionID, messageID)
	if err == nil {
		previous := session.Queue
		session.Queue.Messages = slices.Delete(slices.Clone(session.Queue.Messages), index, index+1)
		if len(session.Queue.Messages) == 0 {
			session.Queue.Paused = false
		}
		if err = s.saveChatQueueLocked(session); err != nil {
			session.Queue = previous
		}
	}
	s.stateMu.Unlock()
	if err != nil {
		return BootstrapState{}, err
	}
	return s.publishQueuedChat(sessionID), nil
}

func (s *BridgeService) ResumeQueuedMessages(sessionID string) (BootstrapState, error) {
	s.stateMu.Lock()
	session := s.sessions[sessionID]
	if session == nil || len(session.Queue.Messages) == 0 || !session.Queue.Paused {
		s.stateMu.Unlock()
		return BootstrapState{}, errors.New(s.t("conversation.queue_unavailable"))
	}
	if session.Queue.Starting != "" && s.runningConversations[sessionID] == nil {
		if err := s.reconcileQueuedStartLocked(session, nil); err != nil {
			s.stateMu.Unlock()
			return BootstrapState{}, err
		}
	}
	session.Queue.Paused = false
	if err := s.saveChatQueueLocked(session); err != nil {
		session.Queue.Paused = true
		s.stateMu.Unlock()
		return BootstrapState{}, err
	}
	s.startNextQueuedMessageLocked(session)
	s.stateMu.Unlock()
	return s.publishQueuedChat(sessionID), nil
}

func (s *BridgeService) editableQueueEntryLocked(sessionID, messageID string) (*sessionState, int, error) {
	session := s.sessions[sessionID]
	if session != nil && session.Queue.Starting != messageID {
		index := slices.IndexFunc(session.Queue.Messages, func(item QueuedChatMessage) bool { return item.ID == messageID })
		if index >= 0 {
			return session, index, nil
		}
	}
	return nil, -1, errors.New(s.t("conversation.queue_entry_unavailable"))
}

func (s *BridgeService) publishQueuedChat(sessionID string) BootstrapState {
	s.emitShellUpdatedForSession(sessionID)
	return s.bootstrapForSession(sessionID)
}

func (s *BridgeService) startNextQueuedMessageLocked(session *sessionState) {
	if session.Running || session.Queue.Paused || len(session.Queue.Messages) == 0 ||
		s.runningConversations[session.ID] != nil {
		return
	}
	entry := session.Queue.Messages[0]
	session.Queue.Starting = entry.ID
	if !s.recordChatQueueLocked(session) {
		session.Queue.Starting = ""
		session.Queue.Paused = true
		return
	}
	session.Running = true
	ctx, cancel := context.WithCancel(context.Background())
	assistantID := newID("assistant")
	s.runningConversations[session.ID] = &runningConversationState{
		AssistantMessageID: assistantID, WorkspacePath: session.WorkspacePath, Cancel: cancel,
	}
	s.conversationWG.Add(1)
	go s.runQueuedMessage(ctx, session.ID, assistantID, entry)
}

func (s *BridgeService) runQueuedMessage(ctx context.Context, sessionID, assistantID string, entry QueuedChatMessage) {
	paths := make([]string, 0, len(entry.Attachments))
	for _, attachment := range entry.Attachments {
		paths = append(paths, attachment.Path)
	}
	_, attachments, err := s.attachmentService().PrepareForSession(paths, "")
	if err == nil && entry.Reasoning != "" && ctx.Err() == nil {
		err = s.setReasoningLevelRPC(entry.Reasoning)
	}
	if err != nil {
		defer s.conversationWG.Done()
		s.stateMu.Lock()
		if session := s.sessions[sessionID]; session != nil {
			s.pushNotificationLocked(s.t("notification.request_failed.title"), err.Error(), "danger")
		}
		s.stateMu.Unlock()
		s.finishConversation(sessionID, assistantID, ctx)
		return
	}
	input := entry.Content
	if input == "" {
		input = "Read and process the attachments."
	}
	s.runConversation(ctx, sessionID, assistantID, input, attachments, false)
}

func (s *BridgeService) queuedTurnStartedLocked(session *sessionState, assistantID, turnID string) {
	if session.Queue.Starting == "" || len(session.Queue.Messages) == 0 {
		return
	}
	entry := session.Queue.Messages[0]
	now := time.Now().Format(time.RFC3339Nano)
	session.Messages = append(session.Messages,
		ChatMessage{ID: entry.ID, Role: "user", State: "completed", Content: entry.Content, Attachments: entry.Attachments, CreatedAt: now, UpdatedAt: now},
		ChatMessage{ID: assistantID, Role: "assistant", State: "streaming", IsPlaceholder: true, CreatedAt: now, UpdatedAt: now, turnID: turnID, TurnID: turnID},
	)
	session.Queue.Starting = ""
	session.Queue.Messages = slices.Delete(session.Queue.Messages, 0, 1)
	s.recordChatQueueLocked(session)
	session.NeedsAttention = false
}

func (s *BridgeService) finishChatQueueLocked(session *sessionState, running *runningConversationState) {
	if session == nil || running == nil {
		return
	}
	if running.Cancelled {
		session.Running = false
	}
	if session.Queue.Starting != "" {
		session.Queue.Paused = true
		session.Running = false
		if err := s.reconcileQueuedStartLocked(session, running); err != nil {
			s.pushNotificationLocked(s.t("notification.request_failed.title"), err.Error(), "danger")
		}
		s.recordChatQueueLocked(session)
		return
	}
	if len(session.Queue.Messages) == 0 {
		session.Queue.Paused = false
		return
	}
	if running.Cancelled || !running.completedSuccessfully {
		session.Queue.Paused = true
		s.recordChatQueueLocked(session)
		return
	}
	s.startNextQueuedMessageLocked(session)
}

// Read Core's durable receipt before releasing a reservation whose start
// notification was lost. If Core is unreachable, the queue stays paused and
// the reservation remains in the journal for reconciliation.
func (s *BridgeService) reconcileQueuedStartLocked(session *sessionState, running *runningConversationState) error {
	messages, err := s.loadWorkspaceSessionMessagesReadOnly(session.WorkspacePath, session.ID)
	if err != nil {
		return err
	}
	for _, item := range messages {
		if item.Role != "user" || metadataString(item.Metadata["id"]) != session.Queue.Starting {
			continue
		}
		entry := chatMessageFromRuntime(item)
		if findSessionMessageByID(session, entry.ID) == nil {
			session.Messages = append(session.Messages, entry)
		}
		session.Queue.Messages = slices.DeleteFunc(session.Queue.Messages, func(message QueuedChatMessage) bool {
			return message.ID == entry.ID
		})
		if running != nil {
			s.pushNotificationLocked(s.t("notification.request_failed.title"), s.t("conversation.queue_started_stream_lost"), "warning")
		}
		break
	}
	session.Queue.Starting = ""
	return s.saveChatQueueLocked(session)
}
