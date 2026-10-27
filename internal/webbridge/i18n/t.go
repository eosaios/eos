package i18n

import "fmt"

var textByLang = map[string]map[string]string{
	"zh": zhText,
	"en": enText,
}

func T(key string, lang string, args ...any) string {
	messages := textByLang[lang]
	if messages == nil {
		messages = textByLang["zh"]
	}
	text := messages[key]
	if text == "" && lang != "en" {
		if fallback := textByLang["en"]; fallback != nil {
			text = fallback[key]
		}
	}
	if text == "" {
		text = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}
