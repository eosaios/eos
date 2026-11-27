package cli

import (
	"errors"
	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/i18n"
	"github.com/eosaios/eos/pkg/protocol"
)

// A one-shot command cannot collect interactive decisions. Surface the kernel's
// waiting state immediately; permission decisions remain in the kernel.
func headlessInteractionError(raw protocol.EventType, sessionID string) error {
	key := ""
	switch {
	case protocol.NormalizeEventType(raw) == protocol.EventTypeApprovalReq:
		key = "cli.headless.approval_required"
	case string(raw) == "turn.request_user_input":
		key = "cli.headless.input_required"
	default:
		return nil
	}
	cfg, _ := config.Load()
	return errors.New(i18n.T(key, cfg.Language, sessionID))
}
