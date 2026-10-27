package webbridge

import (
	"strings"

	"github.com/eosaios/eos/internal/webbridge/i18n"
)

func (s *BridgeService) guiLanguage() string {
	if s == nil {
		return "zh"
	}
	lang := strings.ToLower(strings.TrimSpace(s.settingsReadOnly().Language))
	switch lang {
	case "en":
		return "en"
	case "zh":
		return "zh"
	default:
		return "zh"
	}
}

func (s *BridgeService) t(key string, args ...any) string {
	return i18n.T(key, s.guiLanguage(), args...)
}
