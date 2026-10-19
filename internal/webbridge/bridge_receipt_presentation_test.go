package webbridge

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestToolReceiptPresentationSurvivesCanonicalPersistence(t *testing.T) {
	src := chatMessageWithItems("assistant", "receipt-assistant")
	src.turnID = "receipt-turn"
	src.Items = []ThreadItem{{ID: "receipt-call", Kind: "tool_call", ToolName: "read_file", ToolArgs: `{"path":"fixture.txt"}`, Status: "completed", ToolResult: &ItemToolResult{Status: "completed", Output: "Read 42 bytes", DurationMS: 7}}}
	messages := sessionMessagesFromChatMessage(src)
	// Simulate the adapter retaining opaque core content, then a real JSON wire
	// round trip. Display text and status must still match the original UI item.
	messages[1].Content = `{"output":{"content":"FULL_MODEL_CONTENT"},"status":"ok"}`
	if _, ok := messages[1].Metadata[coreapi.ToolResultPresentationKey]; !ok {
		t.Fatal("missing presentation annotation")
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatal(err)
	}
	restored := chatMessagesFromRuntime(messages)
	if len(restored) != 1 || len(restored[0].Items) != 1 || !reflect.DeepEqual(restored[0].Items[0].ToolResult, src.Items[0].ToolResult) {
		t.Fatalf("UI receipt changed: %#v", restored)
	}
}
