package webbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// Requires an explicitly selected paired Core and only uses a disposable workspace.
func TestLiveTranscriptProjection(t *testing.T) {
	binary := os.Getenv("EOS_PROJECTION_TEST_CORE")
	if binary == "" {
		t.Skip("set EOS_PROJECTION_TEST_CORE to run the live transcript fixture")
	}
	dir := t.TempDir()
	work := filepath.Join(dir, "workspace")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(work, "approval-fixture.txt")
	if err := os.WriteFile(sentinel, []byte("synthetic approval fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "fixture expected model JSON", http.StatusBadRequest)
			return
		}
		lastUser, lastTool := -1, -1
		for i, row := range req.Messages {
			switch row.Role {
			case "user":
				lastUser = i
			case "tool":
				lastTool = i
			}
		}
		if lastUser < 0 {
			http.Error(w, "fixture requires a user turn", http.StatusBadRequest)
			return
		}
		input := req.Messages[lastUser].Content
		delta, finish := map[string]any{"content": "saved next reply"}, "stop"
		switch input {
		case "cancel this fixture":
			entered <- struct{}{}
			select {
			case <-release:
				delta["content"] = "late cancelled response"
			case <-r.Context().Done():
				return
			}
		case "approve fixture cleanup":
			if lastTool > lastUser {
				delta["content"] = "saved approval continuation"
			} else {
				delta = map[string]any{"tool_calls": []any{map[string]any{
					"index": 0, "id": "projection-approval", "type": "function",
					"function": map[string]string{"name": "file_delete", "arguments": `{"path":"approval-fixture.txt"}`},
				}}}
				finish = "tool_calls"
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, row := range []map[string]any{
			{"delta": delta},
			{"delta": map[string]any{}, "finish_reason": finish},
		} {
			row["index"] = 0
			data, _ := json.Marshal(map[string]any{"choices": []any{row}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := adapter.NewStdioClient(adapter.StdioClientOptions{
		CorePath: binary, Workspace: work, StoreDir: filepath.Join(dir, "store"), CoreLogDir: dir,
		Env: map[string]string{
			"HOME": dir, "EOS_CONFIG_PATH": filepath.Join(dir, "eos.json"),
			"EOS_API_BASE": server.URL + "/v1", "EOS_API_KEY": "fixture", "EOS_MODEL": "projection-fixture",
			"EOS_KEYRING": "filevault-only", "EOS_MEMORIES_USE": "0", "EOS_MEMORIES_GENERATE": "0",
			"EOS_APPROVAL_MODE": "on-request", "EOS_SANDBOX_MODE": "workspace-write",
		},
	})
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	gateway := adapter.NewStdioGateway(client)
	initialize, err := gateway.CoreInitializeRPC(ctx)
	if err != nil {
		t.Fatal(err)
	}
	supportsProjection := slices.Contains(initialize.Methods, "session/messages/project")
	t.Logf("Core advertises scoped projection: %t", supportsProjection)
	state, err := gateway.CoreCreateSessionRPC(ctx, work, "projection fixture", "gui", nil)
	if err != nil {
		t.Fatal(err)
	}
	bridge := &BridgeService{runtimeGateway: gateway}
	project := func(id, content string) {
		t.Helper()
		view := &sessionState{ID: state.ID, WorkspacePath: work, Messages: []ChatMessage{{
			ID: "preview", Role: "assistant", State: "streaming", Content: content, turnID: id,
		}}}
		if err := bridge.persistSessionForTurnLocked(view, id); err != nil {
			t.Fatal(err)
		}
	}
	start := func(id, input string) <-chan adapter.Event {
		t.Helper()
		events, _, err := gateway.CoreStartTurnStreamWithRequestRPC(ctx, coreapi.StartTurnRequest{
			SessionID: state.ID, TurnID: id, Input: input,
		})
		if err != nil {
			t.Fatal(err)
		}
		return events
	}
	load := func() []adapter.SessionMessage {
		t.Helper()
		messages, err := gateway.CoreLoadSessionMessagesRPC(ctx, work, state.ID)
		if err != nil {
			t.Fatal(err)
		}
		return messages
	}
	await := func(condition func() bool) {
		t.Helper()
		for !condition() {
			select {
			case <-ctx.Done():
				t.Fatal("timed out waiting for fixture state")
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	has := func(text string) bool {
		for _, message := range load() {
			if message.Role == "assistant" && strings.Contains(message.Content, text) {
				return true
			}
		}
		return false
	}
	_ = start("fixture-cancel", "cancel this fixture")
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("cancellation fixture did not start")
	}
	project("fixture-cancel", "provisional old turn")
	if has("provisional old turn") != supportsProjection {
		t.Fatal("live projection did not respect the negotiated Core capability")
	}
	if err := gateway.CoreInterruptTurnRPC(ctx, state.ID, "fixture-cancel"); err != nil {
		t.Fatal(err)
	}
	if supportsProjection {
		stopped := false
		for _, message := range load() {
			if message.Content == "provisional old turn" {
				gui, _ := message.Metadata["eos_gui"].(map[string]any)
				stopped = gui["state"] == "failed"
			}
		}
		if !stopped {
			t.Fatal("cancelled partial response did not survive with a terminal state")
		}
	}
	_ = start("fixture-next", "new turn after cancellation")
	await(func() bool { return has("saved next reply") })
	before := load()
	if supportsProjection {
		params, err := json.Marshal(coreapi.StartTurnRequest{
			SessionID: state.ID, TurnID: "fixture-cancel", Input: "duplicate identifier",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := gateway.CoreCallRPC(ctx, "turn/start", params); err == nil ||
			!strings.Contains(err.Error(), "already been used") {
			t.Fatalf("Core accepted a reused turn identity: %v", err)
		}
	}
	project("fixture-cancel", "stale cancellation replay")
	if !reflect.DeepEqual(load(), before) || has("late cancelled response") {
		t.Fatal("late cancelled turn replaced the committed reply")
	}

	_ = start("fixture-approval", "approve fixture cleanup")
	var approvalID string
	await(func() bool {
		pending, err := gateway.CoreApprovalListRPC(ctx, coreapi.PendingApprovalListRequest{SessionID: state.ID})
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range pending.Approvals {
			if entry.TurnID == "fixture-approval" && entry.ToolName == "file_delete" {
				approvalID = entry.ApprovalID
			}
		}
		// The approval becomes actionable when Core has committed the waiting turn.
		for _, m := range load() {
			if approvalID != "" && m.Metadata["approval_id"] == approvalID {
				return true
			}
		}
		return false
	})
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatal("fixture changed before approval", err)
	}
	project("fixture-approval", "approval view before resume")
	if err := gateway.CoreRespondApprovalRPC(ctx, approvalID, coreapi.ApprovalDecision("accept")); err != nil {
		t.Fatal(err)
	}
	await(func() bool { return has("saved approval continuation") })
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("approved fixture action did not run: %v", err)
	}
	after := load()
	project("fixture-approval", "late pre-approval view")
	if !reflect.DeepEqual(load(), after) || !has("saved next reply") {
		t.Fatal("late approval view altered either committed turn")
	}
	t.Log("verified cancel, subsequent turn, approval continuation and both late projection replays")
}
