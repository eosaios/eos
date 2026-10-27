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

// 本用例使用与 `eos` 相同的解析规则定位内核；对着本地构建的内核跑时
// 设置 EOS_CORE_PATH 即可，无需改测试代码。

// TestE2EFileMkdirRelativePathApproval verifies that a relative write path
// (e.g. a model-supplied "test") is resolved against the request workspace root
// and allowed by the workspace-write sandbox after approval, rather than being
// denied as "outside all writable roots". This reproduces the user scenario
// where creating a directory via FileMkdir stalled after approval.
func TestE2EFileMkdirRelativePathApproval(t *testing.T) {
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
		Title:         "e2e mkdir",
	})
	if err != nil {
		t.Fatalf("Sessions().Create() error = %v", err)
	}

	// Direct tool/execute with a RELATIVE path "test". file_mkdir is medium
	// risk under the default approval policy, so the first call must return
	// approval_required and an approval_id (pending inserted, sandbox not yet
	// checked). workspace_root is supplied so the later approval path can
	// resolve the relative write path.
	var toolResult coreapi.ToolResult
	if err := client.Process().Call(ctx, jsonrpc.MethodToolExecute, map[string]any{
		"name":           "file_mkdir",
		"args":           map[string]any{"path": "test"},
		"workspace_root": tmpDir,
		"session_id":     sess.ID,
		"request_id":     "req_mkdir_1",
	}, &toolResult); err != nil {
		t.Fatalf("tool/execute error = %v", err)
	}

	if toolResult.Status != "approval_required" {
		t.Fatalf("tool/execute status = %q, want approval_required (display=%q output=%s)",
			toolResult.Status, toolResult.Display, string(toolResult.Output))
	}
	var pending struct {
		ApprovalID string `json:"approval_id"`
	}
	if err := json.Unmarshal(toolResult.Output, &pending); err != nil || pending.ApprovalID == "" {
		t.Fatalf("missing approval_id in output: %s (err=%v)", string(toolResult.Output), err)
	}

	// Approve. approval/respond 的同步返回是 ApprovalReceipt（accepted /
	// decision / approval_id）——实际工具执行结果通过事件总线异步送达
	// （tool.approval_approved → 恢复的 turn 事件），不在 respond 的返回值里。
	var receipt struct {
		ApprovalID string `json:"approval_id"`
		Decision   string `json:"decision"`
		Accepted   bool   `json:"accepted"`
	}
	if err := client.Process().Call(ctx, jsonrpc.MethodApprovalRespond, map[string]any{
		"approval_id": pending.ApprovalID,
		"decision":    "accept",
	}, &receipt); err != nil {
		t.Fatalf("approval/respond error = %v", err)
	}
	if !receipt.Accepted || receipt.ApprovalID != pending.ApprovalID || receipt.Decision != "accept" {
		t.Fatalf("approval/respond receipt = %+v, want accepted=true decision=accept approval_id=%s",
			receipt, pending.ApprovalID)
	}

	// 异步执行：相对路径 "test" 解析到 workspace root 下并真正建目录，
	// 轮询等待内核完成恢复执行。
	created := filepath.Join(tmpDir, "test")
	deadline := time.Now().Add(20 * time.Second)
	var info os.FileInfo
	for {
		var err error
		info, err = os.Stat(created)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected directory %s to be created after approval: %v", created, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !info.IsDir() {
		t.Fatalf("%s exists but is not a directory", created)
	}
}
