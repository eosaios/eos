package headless

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/protocol"
)

func TestNewTurnID(t *testing.T) {
	id := NewTurnID("turn")
	if !strings.HasPrefix(id, "turn_") {
		t.Fatalf("id = %q", id)
	}
	id2 := NewTurnID("x")
	if id2 == id {
		t.Fatal("ids should differ")
	}
}

func TestFailureMessage(t *testing.T) {
	if got := FailureMessage(protocol.EventType("x"), map[string]any{"error": "boom"}); got != "boom" {
		t.Fatalf("msg = %q", got)
	}
	if got := FailureMessage(protocol.EventTypeTurnCancelled, map[string]any{}); got != "request cancelled" {
		t.Fatalf("cancel = %q", got)
	}
	if got := FailureMessage(protocol.EventTypeTurnInterrupted, map[string]any{}); got != "request interrupted" {
		t.Fatalf("interrupt = %q", got)
	}
	if got := FailureMessage(protocol.EventType("other"), map[string]any{}); got != "request failed" {
		t.Fatalf("failed = %q", got)
	}
}

func TestPayloadText(t *testing.T) {
	if got := PayloadText("fb", map[string]any{"a": " x "}, "a"); got != "x" {
		t.Fatalf("trim = %q", got)
	}
	if got := PayloadText("fb", map[string]any{}, "a"); got != "fb" {
		t.Fatalf("fallback = %q", got)
	}
	if got := PayloadText("  fb  ", map[string]any{"a": 1, "b": "hit"}, "a", "b"); got != "hit" {
		t.Fatalf("multi = %q", got)
	}
}
