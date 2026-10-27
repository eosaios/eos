package client_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
	sidecarclient "github.com/eosaios/eos/pkg/coreapi/sidecar/client"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// TestE2EVendoredFileMkdirAllowSession reproduces the "本次都允许" (allow_session)
// stall: the TUI maps allow_session to decision="acceptForSession", which Rust's
// approval_is_approved must treat as approved. Without the fix the tool is
// denied and the turn stalls after approval.
func TestE2EVendoredFileMkdirAllowSession(t *testing.T) {
	coreBinary := resolveE2ECoreBinary(t)
	t.Logf("driving real core: %s", coreBinary)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tmpDir := t.TempDir()

	client, err := sidecarclient.Start(ctx, sidecarclient.Options{
		BinaryPath: coreBinary,
		Env: map[string]string{
			"EOS_SANDBOX_MODE":           "workspace-write",
			"EOS_WORKSPACE_ROOT":         tmpDir,
			"EOS_SANDBOX_WORKSPACE_ROOT": tmpDir,
		},
	})
	if err != nil {
		t.Fatalf("sidecarclient.Start() error = %v", err)
	}
	defer client.Close()

	sess, err := client.Engine().Sessions().Create(ctx, coreapi.CreateSessionRequest{
		WorkspaceRoot: tmpDir,
		Title:         "e2e allow_session",
	})
	if err != nil {
		t.Fatalf("Sessions().Create() error = %v", err)
	}

	var toolResult coreapi.ToolResult
	if err := client.Process().Call(ctx, jsonrpc.MethodToolExecute, map[string]any{
		"name":           "file_mkdir",
		"args":           map[string]any{"path": "test"},
		"workspace_root": tmpDir,
		"session_id":     sess.ID,
		"request_id":     "req_mkdir_sess",
	}, &toolResult); err != nil {
		t.Fatalf("tool/execute error = %v", err)
	}
	if toolResult.Status != "approval_required" {
		t.Fatalf("tool/execute status = %q, want approval_required (display=%q)", toolResult.Status, toolResult.Display)
	}
	var pending struct {
		ApprovalID string `json:"approval_id"`
	}
	if err := json.Unmarshal(toolResult.Output, &pending); err != nil || pending.ApprovalID == "" {
		t.Fatalf("missing approval_id: %s", string(toolResult.Output))
	}

	// Respond exactly like the TUI does for allow_session: decision="acceptForSession".
	var receipt struct {
		ApprovalID string `json:"approval_id"`
		Decision   string `json:"decision"`
		Accepted   bool   `json:"accepted"`
	}
	if err := client.Process().Call(ctx, jsonrpc.MethodApprovalRespond, map[string]any{
		"approval_id": pending.ApprovalID,
		"decision":    "acceptForSession",
	}, &receipt); err != nil {
		t.Fatalf("approval/respond error = %v", err)
	}
	if !receipt.Accepted {
		t.Fatalf("approval/respond(acceptForSession) receipt = %+v, want accepted=true", receipt)
	}

	created := filepath.Join(tmpDir, "test")
	deadline := time.Now().Add(30 * time.Second)
	var info os.FileInfo
	for {
		var err error
		info, err = os.Stat(created)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected directory %s after allow_session: %v", created, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !info.IsDir() {
		t.Fatalf("%s exists but is not a directory", created)
	}
}
