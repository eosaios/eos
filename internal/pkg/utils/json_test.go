package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestFixCommonJSONErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "键后面跟逗号而不是冒号",
			input:    `{"key", "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "多个键值对使用逗号分隔",
			input:    `{"name", "test", "age", 25}`,
			expected: `{"name": "test", "age": 25}`,
		},
		{
			name:     "嵌套对象使用逗号",
			input:    `{"user", {"name", "test"}}`,
			expected: `{"user": {"name": "test"}}`,
		},
		{
			name:     "对象末尾的多余逗号",
			input:    `{"key": "value",}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "数组末尾的多余逗号",
			input:    `[1, 2, 3,]`,
			expected: `[1, 2, 3]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FixCommonJSONErrors(tt.input)
			if result != tt.expected {
				t.Fatalf("FixCommonJSONErrors() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnmarshalWithEscapeFix_FixesCommaAfterKey(t *testing.T) {
	var v map[string]any
	if err := UnmarshalWithEscapeFix(`{"items", [{"id": "1", "content": "任务1"}]}`, &v); err != nil {
		t.Fatalf("UnmarshalWithEscapeFix() error = %v", err)
	}
	items, ok := v["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items parsed unexpected: %#v", v["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item parsed unexpected: %#v", items[0])
	}
	if item["id"] != "1" || item["content"] != "任务1" {
		t.Fatalf("item fields unexpected: %#v", item)
	}
}
