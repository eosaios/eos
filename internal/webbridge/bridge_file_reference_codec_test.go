package webbridge

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func TestFileReferencesSurviveSessionSaveAndReload(t *testing.T) {
	message := ChatMessage{ID: "user-files", Role: "user", Content: "检查这些文件", State: "completed", CreatedAt: "2026-10-19T00:00:00Z", UpdatedAt: "2026-10-19T00:00:00Z", Attachments: []AttachmentRef{
		{Name: "需求.md", Path: "/tmp/原始目录/需求.md", MIME: "text/markdown", Kind: "file"},
		{Name: "data.xlsx", Path: "/tmp/data.xlsx", MIME: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Kind: "spreadsheet"},
		{Name: "image.png", Path: "/tmp/image.png", MIME: "image/png", Kind: "image"},
	}}
	stored := sessionMessageContentFromChat(message)
	if !strings.Contains(stored.Content, "/tmp/原始目录/需求.md") || !strings.Contains(stored.Content, "/tmp/data.xlsx") || strings.Contains(stored.Content, "/tmp/image.png") {
		t.Fatalf("stored model content = %s", stored.Content)
	}
	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	var decoded adapter.SessionMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	loaded := chatMessageFromRuntime(decoded)
	wantJSON, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("loaded=%+v; want=%+v", loaded, message)
	}
	if !reflect.DeepEqual(sessionMessageContentFromChat(loaded), stored) {
		t.Fatal("saving reloaded history changed file references")
	}
}
