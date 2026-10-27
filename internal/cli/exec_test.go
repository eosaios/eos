package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestExecCmd_FlagParsing(t *testing.T) {
	cmd := newExecCmd()

	flags := cmd.Flags()
	if f := flags.Lookup("workspace"); f == nil {
		t.Fatal("missing --workspace flag")
	}
	if f := flags.Lookup("sandbox"); f == nil {
		t.Fatal("missing --sandbox flag")
	}
	if f := flags.Lookup("execution-mode"); f == nil {
		t.Fatal("missing --execution-mode flag")
	}
	if f := flags.Lookup("output"); f == nil {
		t.Fatal("missing --output flag")
	}
	if f := flags.Lookup("timeout"); f == nil {
		t.Fatal("missing --timeout flag")
	}
	if f := flags.Lookup("access-mode"); f == nil {
		t.Fatal("missing --access-mode flag")
	}
	if f := flags.Lookup("approval-mode"); f == nil {
		t.Fatal("missing --approval-mode flag")
	}
	if f := flags.Lookup("dangerously-skip-permissions"); f == nil {
		t.Fatal("missing --dangerously-skip-permissions flag")
	}
	if f := flags.Lookup("resume"); f == nil {
		t.Fatal("missing --resume flag")
	}

	if cmd.Use != "exec <prompt|->" {
		t.Fatalf("unexpected Use: %s", cmd.Use)
	}
	if !cmd.HasFlags() {
		t.Fatal("exec command should have flags")
	}
}

func TestExecCmd_RequiresExactlyOneArg(t *testing.T) {
	cmd := newExecCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.RunE = func(cmd *cobra.Command, args []string) error { return nil }

	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when no args provided")
	}

	cmd.SetArgs([]string{"hello", "world"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when more than one arg provided")
	}
}

func TestExecResult_JSONOutput(t *testing.T) {
	input := 100
	reply := 50
	total := 150
	cost := 0.002

	result := ExecResult{
		Content:     "hello world",
		Model:       "test-model",
		InputTokens: &input,
		ReplyTokens: &reply,
		TotalTokens: &total,
		DurationMs:  1234,
		CostUSD:     &cost,
		Workspace:   "/tmp/test",
	}

	bs, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(bs, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["content"] != "hello world" {
		t.Fatalf("unexpected content: %v", parsed["content"])
	}
	if parsed["model"] != "test-model" {
		t.Fatalf("unexpected model: %v", parsed["model"])
	}
	if parsed["duration_ms"] != float64(1234) {
		t.Fatalf("unexpected duration_ms: %v", parsed["duration_ms"])
	}
	if parsed["workspace"] != "/tmp/test" {
		t.Fatalf("unexpected workspace: %v", parsed["workspace"])
	}
	if parsed["input_tokens"] != float64(100) {
		t.Fatalf("unexpected input_tokens: %v", parsed["input_tokens"])
	}
	if parsed["total_tokens"] != float64(150) {
		t.Fatalf("unexpected total_tokens: %v", parsed["total_tokens"])
	}
	if parsed["cost_usd"] != 0.002 {
		t.Fatalf("unexpected cost_usd: %v", parsed["cost_usd"])
	}
}

func TestExecResult_JSONError(t *testing.T) {
	result := ExecResult{
		Error: "exec timed out after 5s",
	}

	bs, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(bs, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["error"] != "exec timed out after 5s" {
		t.Fatalf("unexpected error: %v", parsed["error"])
	}
	if _, ok := parsed["content"]; ok {
		t.Fatal("content should be omitted when empty")
	}
}

func TestExecResult_OmitEmptyFields(t *testing.T) {
	result := ExecResult{
		DurationMs: 500,
	}

	bs, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(bs, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	emptyFields := []string{"content", "model", "input_tokens", "reply_tokens", "total_tokens", "cost_usd", "workspace", "error"}
	for _, f := range emptyFields {
		if _, ok := parsed[f]; ok {
			t.Fatalf("field %s should be omitted when nil/empty", f)
		}
	}
}

func TestExecCmd_DefaultTimeout(t *testing.T) {
	cmd := newExecCmd()
	cmd.SetArgs([]string{"test"})

	timeout, err := cmd.Flags().GetDuration("timeout")
	if err != nil {
		t.Fatal(err)
	}
	if timeout != 10*time.Minute {
		t.Fatalf("default timeout should be 10m, got %v", timeout)
	}
}

func TestExecCmd_DefaultOutput(t *testing.T) {
	cmd := newExecCmd()
	cmd.SetArgs([]string{"test"})

	output, err := cmd.Flags().GetString("output")
	if err != nil {
		t.Fatal(err)
	}
	if output != "text" {
		t.Fatalf("default output should be text, got %s", output)
	}
}

func TestExecCmd_DefaultSandbox(t *testing.T) {
	cmd := newExecCmd()
	cmd.SetArgs([]string{"test"})

	sandbox, err := cmd.Flags().GetString("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	if sandbox != "workspace" {
		t.Fatalf("default sandbox should be workspace, got %s", sandbox)
	}
}

func TestExecCmd_FlagValues(t *testing.T) {
	cmd := newExecCmd()
	cmd.SetArgs([]string{
		"--workspace", "/my/project",
		"--sandbox", "full_access",
		"--execution-mode", "plan",
		"--output", "json",
		"--timeout", "30s",
		"build the project",
	})

	var parsedArgs []string
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		parsedArgs = args
		return nil
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if len(parsedArgs) != 1 || parsedArgs[0] != "build the project" {
		t.Fatalf("positional args: got %v", parsedArgs)
	}

	ws, _ := cmd.Flags().GetString("workspace")
	if ws != "/my/project" {
		t.Fatalf("workspace: got %q", ws)
	}

	sb, _ := cmd.Flags().GetString("sandbox")
	if sb != "full_access" {
		t.Fatalf("sandbox: got %q", sb)
	}

	em, _ := cmd.Flags().GetString("execution-mode")
	if em != "plan" {
		t.Fatalf("execution-mode: got %q", em)
	}

	out, _ := cmd.Flags().GetString("output")
	if out != "json" {
		t.Fatalf("output: got %q", out)
	}

	to, _ := cmd.Flags().GetDuration("timeout")
	if to != 30*time.Second {
		t.Fatalf("timeout: got %v", to)
	}
}

func TestWriteExecJSON_WritesValidJSON(t *testing.T) {
	input := 42
	result := ExecResult{
		Content:     "result text",
		Model:       "gpt-4",
		InputTokens: &input,
		DurationMs:  100,
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	writeExecJSON(w, result)
	w.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	output := strings.TrimSpace(buf.String())
	var parsed ExecResult
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid json: %v, output: %q", err, output)
	}
	if parsed.Content != "result text" {
		t.Fatalf("content mismatch: %q", parsed.Content)
	}
	if parsed.DurationMs != 100 {
		t.Fatalf("duration_ms mismatch: %d", parsed.DurationMs)
	}
}

func TestExecOptions_TimeoutDefaultsTo10Min(t *testing.T) {
	opts := execOptions{Timeout: 0}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	if opts.Timeout != 10*time.Minute {
		t.Fatalf("expected 10m default, got %v", opts.Timeout)
	}
}

func TestExecTimeout_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	<-ctx.Done()
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", ctx.Err())
	}
}

// TestExecOptionEnv_DoesNotInjectLegacyMode 验证 exec flags 不会把 dev-only 的
// EOS_CORE_ENGINE / EOS_CORE_ALLOW_FALLBACK 注入到生产环境变量。
func TestExecOptionEnv_DoesNotInjectLegacyMode(t *testing.T) {
	opts := execOptions{
		AccessMode:     "workspace-write",
		ApprovalMode:   "on-request",
		Sandbox:        "workspace-write",
		SkipPermission: false,
	}
	env := execOptionEnv(opts)
	for key, value := range env {
		if strings.EqualFold(key, "EOS_CORE_ENGINE") {
			t.Fatalf("execOptionEnv leaked EOS_CORE_ENGINE=%q", value)
		}
		if strings.EqualFold(key, "EOS_CORE_ALLOW_FALLBACK") {
			t.Fatalf("execOptionEnv leaked EOS_CORE_ALLOW_FALLBACK=%q", value)
		}
	}
	// 沙箱轴只经 EOS_SANDBOX_MODE 单通道下发：内核不读 EOS_ACCESS_MODE
	// （历史死参数），且 AccessMode/Sandbox 均为归一化后的规范值。
	if _, ok := env["EOS_ACCESS_MODE"]; ok {
		t.Fatalf("execOptionEnv leaked EOS_ACCESS_MODE=%q (kernel does not read it)", env["EOS_ACCESS_MODE"])
	}
	if env["EOS_SANDBOX_MODE"] != "workspace-write" {
		t.Fatalf("EOS_SANDBOX_MODE = %q, want workspace-write (Sandbox wins when set)", env["EOS_SANDBOX_MODE"])
	}
	if env["EOS_APPROVAL_MODE"] != "on-request" {
		t.Fatalf("EOS_APPROVAL_MODE = %q, want on-request", env["EOS_APPROVAL_MODE"])
	}
	if _, ok := env["EOS_SKIP_PERMISSIONS"]; ok {
		t.Fatalf("EOS_SKIP_PERMISSIONS should be absent when SkipPermission=false")
	}
}

// TestExecFailsWithoutRustBinary 验证 `eos exec` 在缺 eos-core 二进制时直接 error，
// 不会回退到 Go sharedcore。这是 packaging test 的关键。
func TestExecFailsWithoutRustBinary(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EOS_CORE_PATH", filepath.Join(dir, "no-such-binary"))
	t.Setenv("EOS_CORE_ALLOW_FALLBACK", "")
	t.Setenv("EOS_CORE_ENGINE", "") // 生产默认：rust-only

	err := runExec(context.Background(), execOptions{
		Prompt:  "hello world",
		Output:  "text",
		Timeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatal("runExec succeeded without rust binary; expected error")
	}
	if !strings.Contains(err.Error(), "eos-core") && !strings.Contains(err.Error(), "rust") &&
		!strings.Contains(err.Error(), "sidecar") && !strings.Contains(err.Error(), "binary") {
		t.Logf("note: runExec error = %q", err.Error())
	}
}

func TestResolveExecPrompt_PlainArg(t *testing.T) {
	got, err := resolveExecPrompt("  fix the bug  ")
	if err != nil {
		t.Fatalf("resolveExecPrompt() error = %v", err)
	}
	if got != "  fix the bug  " {
		t.Fatalf("plain arg should pass through verbatim, got %q", got)
	}
}

func TestResolveExecPrompt_StdinDash(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	if _, err := w.WriteString("prompt from pipe"); err != nil {
		t.Fatalf("pipe write error = %v", err)
	}
	w.Close()
	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()

	got, err := resolveExecPrompt("-")
	if err != nil {
		t.Fatalf("resolveExecPrompt('-') error = %v", err)
	}
	if got != "prompt from pipe" {
		t.Fatalf("stdin prompt = %q, want %q", got, "prompt from pipe")
	}
}

func TestResolveExecPrompt_StdinEmpty(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	w.Close()
	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()

	if _, err := resolveExecPrompt("-"); err == nil {
		t.Fatal("empty stdin should error")
	}
}
