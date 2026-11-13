package webbridge

import (
	"reflect"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func TestHistoryKeepsIdenticalRepliesInDistinctUserTurns(t *testing.T) {
	messages := []adapter.SessionMessage{
		{Role: "user", Content: "只回复两个字：就绪"},
		{Role: "assistant", Content: "The user wants 就绪", Metadata: map[string]any{"turn_id": "turn-1", "item_id": "thought-1", "kind": "reasoning"}},
		{Role: "assistant", Content: "就绪", Metadata: map[string]any{"turn_id": "turn-1", "item_id": "answer-1"}},
		{Role: "user", Content: "只回复两个字：就绪"},
		{Role: "assistant", Content: "就绪", Metadata: map[string]any{"turn_id": "turn-2", "item_id": "answer-2"}},
	}
	got := chatMessagesFromRuntime(messages)
	roles := make([]string, len(got))
	for i, message := range got {
		roles[i] = message.Role
	}
	if !reflect.DeepEqual(roles, []string{"user", "assistant", "user", "assistant"}) {
		t.Fatalf("history roles = %v", roles)
	}
	wantFirst := []ThreadItem{
		{ID: "thought-1", Kind: "reasoning", Reasoning: "The user wants 就绪", Status: "completed"},
		{ID: "answer-1", Kind: "agent_message", Text: "就绪", Status: "completed"},
	}
	wantSecond := []ThreadItem{{ID: "answer-2", Kind: "agent_message", Text: "就绪", Status: "completed"}}
	if !reflect.DeepEqual(got[1].Items, wantFirst) || !reflect.DeepEqual(got[3].Items, wantSecond) {
		t.Fatalf("history duplicated or merged distinct turns: first=%+v second=%+v", got[1].Items, got[3].Items)
	}
}
