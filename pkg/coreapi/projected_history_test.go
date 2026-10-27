package coreapi

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPreserveProjectedToolReceipts(t *testing.T) {
	original := "  {\"output\":{\"content\":\"实际正文 <>& 🌍\\n\",\"large\":9007199254740993},\"status\":\"ok\"}\n"
	key := func(turn string) map[string]any {
		return map[string]any{"turn_id": turn, "tool_call_id": "same-call", ToolResultPresentationKey: map[string]any{"display": "display summary"}}
	}
	req := SaveSessionMessagesRequest{SessionID: "session", WorkspaceRoot: "/workspace", Messages: []SessionMessage{
		{Role: "user", Content: "original user"},
		{Role: "tool", Content: "display summary", Metadata: key("turn-1")},
		{Role: "tool", Content: "second summary", Metadata: key("turn-2")},
	}}
	before := append([]SessionMessage(nil), req.Messages...)
	got, err := PreserveProjectedToolReceipts(context.Background(), req, func(_ context.Context, load LoadSessionMessagesRequest) ([]SessionMessage, error) {
		if !reflect.DeepEqual(load, LoadSessionMessagesRequest{SessionID: "session", WorkspaceRoot: "/workspace"}) {
			t.Fatalf("load scope = %#v", load)
		}
		return []SessionMessage{
			{Role: "tool", Content: original, ImagePaths: []string{"/fixture/image.png"}, Metadata: map[string]any{"turn_id": "turn-1", "tool_call_id": "same-call"}},
			{Role: "tool", Content: "second exact result", Metadata: map[string]any{"turn_id": "turn-2", "tool_call_id": "same-call"}},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := req
	want.Messages = append([]SessionMessage(nil), req.Messages...)
	want.Messages[1].Content = original
	want.Messages[1].ImagePaths = []string{"/fixture/image.png"}
	want.Messages[2].Content = "second exact result"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("save = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(req.Messages, before) {
		t.Fatal("caller mutated")
	}
}

func TestPreserveProjectedToolReceiptsRejectsMissingAmbiguousOrUnreadable(t *testing.T) {
	msg := SessionMessage{Role: "tool", Content: "summary", Metadata: map[string]any{"turn_id": "turn", "tool_call_id": "call", ToolResultPresentationKey: map[string]any{"display": "summary"}}}
	canonical := SessionMessage{Role: "tool", Content: "exact", Metadata: map[string]any{"turn_id": "turn", "tool_call_id": "call"}}
	for _, tc := range []struct {
		name     string
		messages []SessionMessage
		loadErr  error
	}{
		{name: "missing"}, {name: "different_turn", messages: []SessionMessage{{Role: "tool", Content: "wrong", Metadata: map[string]any{"turn_id": "other", "tool_call_id": "call"}}}},
		{name: "ambiguous", messages: []SessionMessage{canonical, canonical}},
		{name: "unreadable", loadErr: errors.New("core disconnected")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := SaveSessionMessagesRequest{SessionID: "session", Messages: []SessionMessage{msg}}
			got, err := PreserveProjectedToolReceipts(context.Background(), req, func(context.Context, LoadSessionMessagesRequest) ([]SessionMessage, error) {
				return tc.messages, tc.loadErr
			})
			if err == nil || !reflect.DeepEqual(got, SaveSessionMessagesRequest{}) {
				t.Fatalf("result=%#v error=%v", got, err)
			}
			if !reflect.DeepEqual(req.Messages, []SessionMessage{msg}) {
				t.Fatal("failed save mutated input")
			}
		})
	}
}

func TestPreserveProjectedToolReceiptsLeavesUnmarkedSaveContract(t *testing.T) {
	req := SaveSessionMessagesRequest{Messages: []SessionMessage{{Role: "tool", Content: "legacy raw message", Metadata: map[string]any{"tool_call_id": "old"}}, {Role: "assistant", Metadata: map[string]any{ToolResultPresentationKey: "not a tool"}}}}
	got, err := PreserveProjectedToolReceipts(context.Background(), req, func(context.Context, LoadSessionMessagesRequest) ([]SessionMessage, error) {
		t.Fatal("unmarked replacement must not load or merge history")
		return nil, nil
	})
	if err != nil || !reflect.DeepEqual(got, req) {
		t.Fatalf("result=%#v error=%v", got, err)
	}
}
