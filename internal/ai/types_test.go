package ai

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"testing"
)

func TestMessageJson(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "hello",
	}

	bytes, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	expected := `{"role":"user","content":"hello"}`
	if string(bytes) != expected {
		t.Errorf("Expected JSON %s, got %s", expected, string(bytes))
	}

	var decoded Message
	err = json.Unmarshal(bytes, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if decoded.Role != msg.Role {
		t.Errorf("Expected role %s, got %s", msg.Role, decoded.Role)
	}
	if decoded.Content != msg.Content {
		t.Errorf("Expected content %s, got %s", msg.Content, decoded.Content)
	}
}
