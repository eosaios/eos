package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"log/slog"
	"strings"
)

// UnmarshalWithErrorHandling 统一的 JSON 反序列化函数，包含错误处理和日志记录
// data: JSON 数据
// v: 目标对象（必须是指针）
// context: 上下文描述，用于日志（如 "config.load", "tool.parse"）
// 返回错误，如果解析失败则记录日志
func UnmarshalWithErrorHandling(data []byte, v any, context string) error {
	if err := json.Unmarshal(data, v); err != nil {
		slog.Error(context+".unmarshal.error",
			"component", ComponentSystem,
			"data_size", len(data),
			"error", err,
		)
		return err
	}
	return nil
}

// UnmarshalSilently 静默的 JSON 反序列化，不记录日志
// 用于不需要日志记录的场景，如测试可选字段解析
func UnmarshalSilently(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// MarshalWithErrorHandling 统一的 JSON 序列化函数，包含错误处理和日志记录
// v: 源对象
// context: 上下文描述，用于日志
// 返回 JSON 数据和错误
func MarshalWithErrorHandling(v any, context string) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error(context+".marshal.error",
			"component", ComponentSystem,
			"error", err,
		)
		return nil, err
	}
	return data, nil
}

// MarshalIndent 格式化 JSON 序列化，用于需要可读输出的场景
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// FixJSONEscapeSequences 修复 JSON 中的无效转义序列
// 主要用于修复 AI 生成的 Windows 路径，如 "C:\home\demo" 应为 "C:\\home\\demo"
// 有效转义序列：\", \\, \/, \b, \f, \n, \r, \t, \uXXXX
func FixJSONEscapeSequences(data string) string {
	validEscapes := map[string]bool{
		`\"`: true, `\\`: true, `\/`: true,
		`\b`: true, `\f`: true, `\n`: true, `\r`: true, `\t`: true,
	}

	var result strings.Builder
	result.Grow(len(data) + len(data)/10)

	i := 0
	for i < len(data) {
		if data[i] == '\\' && i+1 < len(data) {
			next := data[i+1]
			if validEscapes[string([]byte{'\\', next})] {
				result.WriteByte(data[i])
				result.WriteByte(next)
				i += 2
				continue
			}

			if next == 'u' && i+5 < len(data) {
				result.WriteString(data[i : i+6])
				i += 6
				continue
			}

			result.WriteString(`\\`)
			result.WriteByte(next)
			i += 2
			continue
		}

		result.WriteByte(data[i])
		i++
	}

	return result.String()
}

// UnmarshalWithEscapeFix 先修复转义序列再解析 JSON
// 用于处理 AI 生成的可能包含无效转义序列的 JSON
func UnmarshalWithEscapeFix(data string, v any) error {
	fixed := FixJSONEscapeSequences(data)
	if err := json.Unmarshal([]byte(fixed), v); err != nil {
		fixed = FixCommonJSONErrors(fixed)
		return json.Unmarshal([]byte(fixed), v)
	}
	return nil
}

func FixCommonJSONErrors(data string) string {
	fixed := fixCommaAfterKey(data)
	fixed = fixTrailingCommas(fixed)
	return fixed
}

func fixCommaAfterKey(data string) string {
	inString := false
	escape := false
	stringStartChar := byte(0)
	braceDepth := 0
	lastStringEnd := -1
	expectKey := false

	for i := 0; i < len(data); i++ {
		ch := data[i]

		if inString {
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == stringStartChar {
				inString = false
				if expectKey && braceDepth > 0 {
					lastStringEnd = i
				}
			}
			continue
		}

		switch ch {
		case '"', '\'':
			inString = true
			stringStartChar = ch
		case '{':
			braceDepth++
			expectKey = true
		case '}':
			braceDepth--
			expectKey = false
		case '[':
			expectKey = false
		case ']':
			expectKey = false
		case ',':
			if braceDepth > 0 && lastStringEnd >= 0 && i == lastStringEnd+1 {
				j := i + 1
				for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
					j++
				}
				if j < len(data) {
					next := data[j]
					if next == '"' || next == '\'' || next == '{' || next == '[' ||
						next == 't' || next == 'f' || next == 'n' ||
						next == '-' || (next >= '0' && next <= '9') {
						fixed := data[:i] + ":" + data[i+1:]
						return fixCommaAfterKey(fixed)
					}
				}
			}
			if braceDepth > 0 {
				expectKey = true
			}
			lastStringEnd = -1
		case ':':
			expectKey = false
			lastStringEnd = -1
		case ' ', '\t', '\n', '\r':
		default:
			expectKey = false
			lastStringEnd = -1
		}
	}

	return data
}

func fixTrailingCommas(data string) string {
	var result strings.Builder
	result.Grow(len(data))

	inString := false
	escape := false
	stringStartChar := byte(0)

	for i := 0; i < len(data); i++ {
		ch := data[i]

		if inString {
			result.WriteByte(ch)
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == stringStartChar {
				inString = false
			}
			continue
		}

		if ch == '"' || ch == '\'' {
			inString = true
			stringStartChar = ch
			result.WriteByte(ch)
			continue
		}

		if ch == ',' {
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue
			}
		}

		result.WriteByte(ch)
	}

	return result.String()
}
