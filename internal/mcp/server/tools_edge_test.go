package server

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestToolDescription(t *testing.T) {
	d := coreapi.ToolDefinition{Name: "n", Description: "d", ReadOnly: true, RiskLevel: "low", RequiresFullAccess: true}
	got := toolDescription(d)
	if got == "" || !containsStr(got, "read-only") || !containsStr(got, "risk:low") {
		t.Fatalf("desc = %q", got)
	}
	d2 := coreapi.ToolDefinition{Name: "only-name"}
	if toolDescription(d2) != "only-name" {
		t.Fatalf("name fallback = %q", toolDescription(d2))
	}
}

func TestSessionIDFromMetaAndMarshalArgs(t *testing.T) {
	req := mcp.CallToolRequest{}
	if sessionIDFromMeta(req) != "" {
		t.Fatal("empty")
	}
	req.Params.Meta = &mcp.Meta{AdditionalFields: map[string]any{metaSessionIDKey: " sid-1 "}}
	if sessionIDFromMeta(req) != " sid-1 " {
		t.Fatalf("sid = %q", sessionIDFromMeta(req))
	}

	// RawArguments 优先
	raw := json.RawMessage(`{"a":1}`)
	req2 := mcp.CallToolRequest{}
	req2.Params.RawArguments = raw
	if string(marshalArguments(req2)) != string(raw) {
		t.Fatal("raw args")
	}
	// 空
	req3 := mcp.CallToolRequest{}
	if marshalArguments(req3) != nil {
		t.Fatal("empty args")
	}
}

func TestExtractOutputTextAndToolResultText(t *testing.T) {
	if extractOutputText(nil) != "" {
		t.Fatal("nil")
	}
	if extractOutputText(json.RawMessage(`{"text":"hi"}`)) != "hi" {
		t.Fatal("text field")
	}
	if extractOutputText(json.RawMessage(`{"output":"out"}`)) != "out" {
		t.Fatal("output field")
	}

	r := coreapi.ToolResult{Status: "success", Display: "disp", Error: "err"}
	if toolResultText(r) != "disp" {
		t.Fatalf("text = %q", toolResultText(r))
	}
	r2 := coreapi.ToolResult{Status: "success", Error: "only-err"}
	if toolResultText(r2) != "only-err" {
		t.Fatalf("err text = %q", toolResultText(r2))
	}

	// toolResultToMCP
	m := toolResultToMCP(coreapi.ToolResult{Status: "error", Display: "x"})
	if !m.IsError {
		t.Fatal("error status")
	}
	m2 := toolResultToMCP(coreapi.ToolResult{Status: "success", Display: "ok"})
	if m2.IsError {
		t.Fatal("success")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOfStr(s, sub) >= 0)
}

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
