package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// message_codec 余臂批测：turn 组合并的空组/ID 回落/ChangeSet·Rollback
// 扫描、gui metadata 富字段往返、item 转换的状态/时间回落。

import (
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

func TestChatMessageFromTurnGroupEdges(t *testing.T) {
	// 空组：占位消息。
	msg := chatMessageFromTurnGroup(nil)
	if msg.ID == "" || msg.Role != "assistant" || msg.State != "completed" {
		t.Fatalf("空组 = %+v", msg)
	}

	// ChangeSet / Rollback 扫描命中 + 中段消息恢复富 ID。
	created := time.Now().UTC().Truncate(time.Second)
	msgs := []adapter.SessionMessage{
		{Role: "assistant", Content: "a", Time: created},
		{Role: "assistant", Content: "b", Time: created, ChangeSet: &coreapi.MessageChangeSet{ID: "cs-1", Additions: 1, Files: []coreapi.ChangedFile{{Path: "a.go", Status: "modified"}}}},
		{Role: "assistant", Content: "c", Time: created, Rollback: &coreapi.TurnRollback{AssistantMessageID: "m1"}},
	}
	out := chatMessageFromTurnGroup(msgs)
	if out.ChangeSet == nil || out.ChangeSet.Additions != 1 {
		t.Fatalf("ChangeSet 扫描 = %+v", out.ChangeSet)
	}
	if out.Rollback == nil || out.Rollback.AssistantMessageID != "m1" {
		t.Fatalf("Rollback 扫描 = %+v", out.Rollback)
	}
}

func TestRuntimeMetadataRoundTripRichFields(t *testing.T) {
	src := ChatMessage{
		ID:                   "msg-1",
		State:                "streaming",
		UpdatedAt:            "2026-10-03T00:00:00Z",
		RuntimeSummary:       "摘要",
		ImplementationResult: "实现结果",
		VerificationResult:   "验证结果",
		VerificationVerdict:  "通过",
		VerificationSummary:  "验证摘要",
		VerificationCovered:  []string{"用例A", " 用例B "},
	}
	guiRaw := runtimeMetadataFromChatMessage(src)
	gui, ok := guiRaw[guiRuntimeMetadataKey].(map[string]any)
	if !ok {
		t.Fatalf("gui 嵌套结构不符: %#v", guiRaw)
	}
	for _, key := range []string{"id", "state", "updatedAt", "runtimeSummary",
		"implementationResult", "verificationResult", "verificationVerdict",
		"verificationSummary"} {
		if _, ok := gui[key]; !ok {
			t.Fatalf("gui 缺 %q: %v", key, gui)
		}
	}
	covered, _ := gui["verificationCoveredChecks"].([]string)
	if len(covered) != 2 || covered[1] != "用例B" {
		t.Fatalf("covered = %#v", gui["verificationCoveredChecks"])
	}

	// 反向恢复：富字段回到消息。
	dst := ChatMessage{}
	applyRuntimeMetadataToChatMessage(&dst, guiRaw)
	if dst.ID != "msg-1" || dst.State != "streaming" || dst.RuntimeSummary != "摘要" ||
		dst.ImplementationResult != "实现结果" || dst.VerificationResult != "验证结果" ||
		dst.VerificationVerdict != "通过" || dst.VerificationSummary != "验证摘要" {
		t.Fatalf("恢复 = %+v", dst)
	}
	if len(dst.VerificationCovered) != 2 {
		t.Fatalf("covered 恢复 = %#v", dst.VerificationCovered)
	}

	// 空消息：gui 为空 map（无空串污染）。
	empty := runtimeMetadataFromChatMessage(ChatMessage{})
	if len(empty) != 0 {
		t.Fatalf("空消息 gui = %v", empty)
	}
}

func TestChatMessageItemFallbackArms(t *testing.T) {
	// threadItemFromSessionMessage 的普通文本回落 + 无 item 消息跳过。
	created := time.Now().UTC().Truncate(time.Second)
	// 无 item_id 的消息：不产 item（走外层占位合并路径）。
	if _, ok := threadItemFromSessionMessage(adapter.SessionMessage{
		Role: "assistant", Content: "plain", Time: created,
	}); ok {
		t.Fatal("无 item_id 应跳过")
	}
	// reason/plan/status 三 kind + 默认 agent_message。
	for _, tc := range []struct {
		kind   string
		meta   map[string]any
		wantK  string
		wantTx string
	}{
		{"reasoning", nil, "reasoning", "思考"},
		{"plan", nil, "plan", "计划"},
		{"status", map[string]any{"level": "info"}, "status", "状态"},
		{"", nil, "agent_message", "正文"},
	} {
		meta := map[string]any{"item_id": "it-1"}
		for k, v := range tc.meta {
			meta[k] = v
		}
		if tc.kind != "" {
			meta["kind"] = tc.kind
		}
		item, ok := threadItemFromSessionMessage(adapter.SessionMessage{
			Role: "assistant", Content: tc.wantTx, Time: created, Metadata: meta,
		})
		if !ok || item.Kind != tc.wantK {
			t.Fatalf("kind=%q item = %+v, %v", tc.kind, item, ok)
		}
		if item.Kind == "reasoning" && item.Reasoning != tc.wantTx {
			t.Fatalf("reasoning 文本 = %q", item.Reasoning)
		}
		if item.Kind != "reasoning" && strings.TrimSpace(item.Text) != tc.wantTx {
			t.Fatalf("kind=%q 文本 = %q", tc.kind, item.Text)
		}
		if item.Kind == "status" && item.Level != "info" {
			t.Fatalf("status level = %q", item.Level)
		}
	}
}
