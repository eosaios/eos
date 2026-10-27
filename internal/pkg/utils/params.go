package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"log/slog"
)

// GetParamString 从参数 map 中获取字符串值
// 如果参数不存在或类型不匹配，返回默认值
func GetParamString(params map[string]any, key string, defaultValue string) string {
	if params == nil {
		return defaultValue
	}

	val, ok := params[key]
	if !ok {
		return defaultValue
	}

	// 类型断言检查
	str, ok := val.(string)
	if !ok {
		slog.Warn("params.get_string.type_mismatch",
			"component", ComponentTool,
			"key", key,
			"expected_type", "string",
			"actual_type", fmt.Sprintf("%T", val),
		)
		return defaultValue
	}

	return str
}

// GetParamInt 从参数 map 中获取整数值
// 如果参数不存在或类型不匹配，返回默认值
func GetParamInt(params map[string]any, key string, defaultValue int) int {
	if params == nil {
		return defaultValue
	}

	val, ok := params[key]
	if !ok {
		return defaultValue
	}

	// 支持多种数字类型
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		slog.Warn("params.get_int.type_mismatch",
			"component", ComponentTool,
			"key", key,
			"expected_type", "int",
			"actual_type", fmt.Sprintf("%T", val),
		)
		return defaultValue
	}
}

// GetParamBool 从参数 map 中获取布尔值
// 如果参数不存在或类型不匹配，返回默认值
func GetParamBool(params map[string]any, key string, defaultValue bool) bool {
	if params == nil {
		return defaultValue
	}

	val, ok := params[key]
	if !ok {
		return defaultValue
	}

	boolVal, ok := val.(bool)
	if !ok {
		slog.Warn("params.get_bool.type_mismatch",
			"component", ComponentTool,
			"key", key,
			"expected_type", "bool",
			"actual_type", fmt.Sprintf("%T", val),
		)
		return defaultValue
	}

	return boolVal
}

// GetParamStringSlice 从参数 map 中获取字符串切片
// 如果参数不存在或类型不匹配，返回默认值
func GetParamStringSlice(params map[string]any, key string) []string {
	if params == nil {
		return nil
	}

	val, ok := params[key]
	if !ok {
		return nil
	}

	// 处理 []string 类型
	if slice, ok := val.([]string); ok {
		return slice
	}

	// 处理 []any 类型，尝试转换为 []string
	if slice, ok := val.([]any); ok {
		result := make([]string, 0, len(slice))
		for i, item := range slice {
			if str, ok := item.(string); ok {
				result = append(result, str)
			} else {
				slog.Warn("params.get_string_slice.element_type_mismatch",
					"component", ComponentTool,
					"key", key,
					"index", i,
					"expected_type", "string",
					"actual_type", fmt.Sprintf("%T", item),
				)
			}
		}
		return result
	}

	slog.Warn("params.get_string_slice.type_mismatch",
		"component", ComponentTool,
		"key", key,
		"expected_type", "[]string or []any",
		"actual_type", fmt.Sprintf("%T", val),
	)
	return nil
}

// ValidateParams 验证参数是否包含必需的字段
// 返回缺失的字段列表和验证是否通过
func ValidateParams(params map[string]any, requiredKeys []string) ([]string, bool) {
	var missing []string

	for _, key := range requiredKeys {
		if params == nil {
			missing = append(missing, key)
			continue
		}

		val, ok := params[key]
		if !ok {
			missing = append(missing, key)
			continue
		}

		// 检查空值
		if isEmptyValue(val) {
			missing = append(missing, key)
		}
	}

	return missing, len(missing) == 0
}

// isEmptyValue 检查值是否为空
func isEmptyValue(val any) bool {
	if val == nil {
		return true
	}

	switch v := val.(type) {
	case string:
		return v == ""
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		return false
	}
}

// CheckPositiveInt 检查整数值是否为正数
// 返回验证结果和错误消息
func CheckPositiveInt(value int, fieldName string) (bool, string) {
	if value <= 0 {
		return false, fmt.Sprintf("%s must be positive, got %d", fieldName, value)
	}
	return true, ""
}

// CheckNonNegativeInt 检查整数值是否非负
// 返回验证结果和错误消息
func CheckNonNegativeInt(value int, fieldName string) (bool, string) {
	if value < 0 {
		return false, fmt.Sprintf("%s must be non-negative, got %d", fieldName, value)
	}
	return true, ""
}

// CheckRangeInt 检查整数值是否在指定范围内
// 返回验证结果和错误消息
func CheckRangeInt(value, min, max int, fieldName string) (bool, string) {
	if value < min || value > max {
		return false, fmt.Sprintf("%s must be between %d and %d, got %d", fieldName, min, max, value)
	}
	return true, ""
}

// CheckStringLength 检查字符串长度是否在指定范围内
// 返回验证结果和错误消息
func CheckStringLength(value string, minLen, maxLen int, fieldName string) (bool, string) {
	length := len(value)
	if length < minLen {
		return false, fmt.Sprintf("%s must be at least %d characters, got %d", fieldName, minLen, length)
	}
	if maxLen > 0 && length > maxLen {
		return false, fmt.Sprintf("%s must be at most %d characters, got %d", fieldName, maxLen, length)
	}
	return true, ""
}
