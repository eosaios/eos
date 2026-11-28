package webbridge

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/eosaios/eos/pkg/chatqueue"
)

func chatQueueDir() string {
	return filepath.Join(bridgeCoreStoreDir(), "web-chat-queue")
}

func (s *BridgeService) saveChatQueueLocked(session *sessionState) error {
	return chatqueue.Save(chatQueueDir(), session.ID, session.Queue)
}

func (s *BridgeService) recordChatQueueLocked(session *sessionState) bool {
	if err := s.saveChatQueueLocked(session); err != nil {
		s.pushNotificationLocked(s.t("notification.request_failed.title"), err.Error(), "danger")
		return false
	}
	return true
}

func (s *BridgeService) restoreChatQueueLocked(session *sessionState) {
	queue, err := chatqueue.Load[AttachmentRef](chatQueueDir(), session.ID)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		s.pushNotificationLocked(s.t("notification.request_failed.title"), err.Error(), "danger")
		return
	}
	queue.Messages = slices.DeleteFunc(queue.Messages, func(entry QueuedChatMessage) bool {
		message := findSessionMessageByID(session, entry.ID)
		if message == nil {
			return false
		}
		message.Content = entry.Content
		message.Attachments = entry.Attachments
		return true
	})
	queue.Starting = ""
	queue.Paused = len(queue.Messages) != 0
	session.Queue = queue
	s.recordChatQueueLocked(session)
}
