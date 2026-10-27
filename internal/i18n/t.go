package i18n

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "fmt"

var textByLang = map[string]map[string]string{
	"zh": zhText,
	"en": enText,
}

func T(key string, lang string, args ...any) string {
	m := textByLang[lang]
	if m == nil {
		m = textByLang["zh"]
	}
	s := m[key]
	if s == "" {
		// Fallback to English if not found in current language
		if lang != "en" {
			if enM := textByLang["en"]; enM != nil {
				s = enM[key]
			}
		}
		if s == "" {
			s = key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
