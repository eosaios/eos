package webbridge

import (
	"os"
	"strings"
)

// withSessionLocked looks up a session while stateMu is already held.
func (s *BridgeService) withSessionLocked(sessionID string, fn func(*sessionState) error) error {
	if fn == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return os.ErrNotExist
	}
	session := s.ensureSessionByIDLocked(sessionID)
	if session == nil {
		return os.ErrNotExist
	}
	return fn(session)
}

func (s *BridgeService) persistSessionOrNotifyLocked(session *sessionState, title string) error {
	if err := s.persistSessionLocked(session); err != nil {
		s.pushNotificationLocked(title, err.Error(), "danger")
		return err
	}
	return nil
}
