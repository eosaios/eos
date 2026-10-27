package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"log/slog"
	"strings"
	"unicode/utf8"
)

const (
	// MaxUserInputLength 用户输入最大长度 (100KB)
	MaxUserInputLength = 100 * 1024

	// MaxPathLength 路径最大长度
	MaxPathLength = 4096

	// MaxCommandLength 命令最大长度
	MaxCommandLength = 10000
)

// InputValidationResult 输入验证结果
type InputValidationResult struct {
	IsValid   bool   // 是否有效
	ErrMsg    string // 错误消息
	Processed string // 处理后的输入
}

// ValidateUserInput 验证用户输入
// 检查：长度限制、空输入、有效 UTF-8
func ValidateUserInput(input string) InputValidationResult {
	// 检查空输入
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return InputValidationResult{
			IsValid: false,
			ErrMsg:  "input is empty",
		}
	}

	// 检查 UTF-8 有效性
	if !utf8.ValidString(input) {
		slog.Warn("validation.input.invalid_utf8",
			"component", ComponentUser,
			"length", len(input),
		)
		return InputValidationResult{
			IsValid: false,
			ErrMsg:  "input contains invalid UTF-8 characters",
		}
	}

	// 检查长度限制
	// 使用 utf8.RuneCountInString 获取字符数（而非字节数）
	runeCount := utf8.RuneCountInString(input)
	if runeCount > MaxUserInputLength {
		slog.Warn("validation.input.too_long",
			"component", ComponentUser,
			"length", runeCount,
			"max_length", MaxUserInputLength,
		)
		return InputValidationResult{
			IsValid: false,
			ErrMsg:  "input exceeds maximum length",
		}
	}

	return InputValidationResult{
		IsValid:   true,
		Processed: trimmed,
	}
}

// ValidatePath 验证路径输入
// 检查：长度限制、空输入、特殊字符
func ValidatePath(path string) InputValidationResult {
	if strings.TrimSpace(path) == "" {
		return InputValidationResult{
			IsValid: false,
			ErrMsg:  "path is empty",
		}
	}

	// 检查长度限制
	if len(path) > MaxPathLength {
		slog.Warn("validation.path.too_long",
			"component", ComponentUser,
			"length", len(path),
			"max_length", MaxPathLength,
		)
		return InputValidationResult{
			IsValid: false,
			ErrMsg:  "path exceeds maximum length",
		}
	}

	// 检查可疑的路径模式
	if containsSuspiciousPathPatterns(path) {
		slog.Warn("validation.path.suspicious",
			"component", ComponentUser,
			"path", path,
		)
		return InputValidationResult{
			IsValid: false,
			ErrMsg:  "path contains suspicious patterns",
		}
	}

	return InputValidationResult{
		IsValid:   true,
		Processed: strings.TrimSpace(path),
	}
}

// ValidateCommand 验证命令输入
// 检查：长度限制、空输入、危险命令
func ValidateCommand(cmd string) InputValidationResult {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return InputValidationResult{
			IsValid: false,
			ErrMsg:  "command is empty",
		}
	}

	// 检查长度限制
	if len(cmd) > MaxCommandLength {
		slog.Warn("validation.command.too_long",
			"component", ComponentUser,
			"length", len(cmd),
			"max_length", MaxCommandLength,
		)
		return InputValidationResult{
			IsValid: false,
			ErrMsg:  "command exceeds maximum length",
		}
	}

	return InputValidationResult{
		IsValid:   true,
		Processed: trimmed,
	}
}

// containsSuspiciousPathPatterns 检查路径中是否包含可疑模式。
// Windows 保留设备名按路径段/基名精确匹配——旧实现用子串匹配 "CON"，
// 会把 config/console/concepts 等常规路径误杀。
func containsSuspiciousPathPatterns(path string) bool {
	substringSuspicious := []string{
		"../",
		"..\\",
		"~/.config", // 尝试访问系统配置
		"/etc/passwd",
		"/etc/shadow",
		"\\\\", // Windows UNC path potential
	}
	for _, pattern := range substringSuspicious {
		if strings.Contains(path, pattern) {
			return true
		}
	}

	// Windows 保留设备名（含带扩展名形式：COM1.txt 保留，fileCOM1.txt 不保留）
	reserved := map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM0": true, "COM1": true, "COM2": true, "COM3": true, "COM4": true,
		"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
		"LPT0": true, "LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
		"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	}
	upper := strings.ToUpper(path)
	for _, seg := range strings.FieldsFunc(upper, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if reserved[seg] {
			return true
		}
		base := seg
		if i := strings.IndexByte(base, '.'); i >= 0 {
			base = base[:i]
		}
		if reserved[base] {
			return true
		}
	}
	return false
}

// SanitizeString 清理字符串输入
// 移除控制字符，保留可打印字符
func SanitizeString(s string) string {
	var result strings.Builder
	for _, r := range s {
		// 保留可打印字符和常见空白字符
		if r == '\n' || r == '\r' || r == '\t' || (r >= 32 && r <= 126) || r >= 128 {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// TruncateString 截断字符串到指定长度（按字符数）
func TruncateString(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}

	runes := []rune(s)
	if len(runes) > maxLen {
		runes = runes[:maxLen]
		return string(runes) + "..."
	}
	return string(runes)
}
