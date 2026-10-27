package webbridge

import (
	"strings"
)

func assistantMessageCompleted(session *sessionState, messageID string) bool {
	item := findSessionMessageByID(session, messageID)
	return item != nil && strings.EqualFold(strings.TrimSpace(item.State), "completed")
}

func findSessionMessageByID(session *sessionState, messageID string) *ChatMessage {
	if session == nil {
		return nil
	}
	for index := range session.Messages {
		if session.Messages[index].ID == messageID {
			return &session.Messages[index]
		}
	}
	return nil
}

func normalizeConversationEventMessage(kind, message string) string {
	switch kind {
	case "text.delta", "text.final", "turn.item_delta", "turn.item_started", "turn.item_completed":
		return message
	default:
		return strings.TrimSpace(message)
	}
}
