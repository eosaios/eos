package webbridge

import (
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// 命令类结果的结构化流投影：stdout/stderr/exit_code 必须到达界面。
// 回归点：此前 shell 成功时界面只拿到「shell exit 0 (cwd: …)」，真实
// stdout 从未被带出（formatToolResultOutput 只取 display）。
// 本文件与桌面端 internal/app 的同名测试对应：网页模式桥接层是那份
// 投影逻辑的副本，加字段/改投影时两边要一起动。
func TestExtractItemToolResultCommandStreams(t *testing.T) {
	t.Run("shell_exec 成功：stdout 与 exit 0 都带出", func(t *testing.T) {
		result := extractItemToolResult(map[string]any{
			"name":    "shell_exec",
			"status":  "ok",
			"display": "shell exit 0 (cwd: /w)",
			"output": map[string]any{
				"stdout":    "hello\n",
				"stderr":    "",
				"exit_code": float64(0),
			},
			"duration_ms": float64(12),
		})
		if result.Stdout != "hello\n" {
			t.Fatalf("stdout = %q, want hello", result.Stdout)
		}
		if result.ExitCode == nil || *result.ExitCode != 0 {
			t.Fatalf("exit code = %v, want explicit 0", result.ExitCode)
		}
		if result.Status != "completed" || result.DurationMS != 12 {
			t.Fatalf("status/duration = %q/%d", result.Status, result.DurationMS)
		}
	})

	t.Run("git 失败：stderr 与非零退出码", func(t *testing.T) {
		result := extractItemToolResult(map[string]any{
			"name":    "git",
			"status":  "error",
			"display": "git exit 128",
			"output": map[string]any{
				"stdout":    "",
				"stderr":    "fatal: not a git repository",
				"exit_code": float64(128),
			},
			"error": "git failed",
		})
		if result.Stderr != "fatal: not a git repository" {
			t.Fatalf("stderr = %q", result.Stderr)
		}
		if result.ExitCode == nil || *result.ExitCode != 128 {
			t.Fatalf("exit code = %v, want 128", result.ExitCode)
		}
		if result.Status != "failed" || result.Error != "git failed" {
			t.Fatalf("status/error = %q/%q", result.Status, result.Error)
		}
	})

	t.Run("超长输出保留尾部并标记截断", func(t *testing.T) {
		long := strings.Repeat("a", toolStreamTailRunes) + "TAIL"
		result := extractItemToolResult(map[string]any{
			"status": "ok",
			"output": map[string]any{
				"stdout":    long,
				"exit_code": float64(0),
			},
		})
		if !result.Truncated {
			t.Fatal("expected truncated flag")
		}
		if len([]rune(result.Stdout)) != toolStreamTailRunes {
			t.Fatalf("stdout runes = %d, want %d", len([]rune(result.Stdout)), toolStreamTailRunes)
		}
		if !strings.HasSuffix(result.Stdout, "TAIL") {
			t.Fatal("tail must be preserved")
		}
	})

	t.Run("非命令结果不受影响", func(t *testing.T) {
		for _, obj := range []map[string]any{
			{"status": "ok", "display": "read a.go (12 bytes)", "output": "file body"},
			{"status": "ok", "output": map[string]any{"matches": 3}},
			{"status": "ok", "output": map[string]any{"stdout": "", "stderr": ""}},
		} {
			result := extractItemToolResult(obj)
			if result.Stdout != "" || result.Stderr != "" || result.ExitCode != nil || result.Truncated {
				t.Fatalf("non-command result %+v must not gain streams", result)
			}
		}
	})

	t.Run("多字节字符不被截成半个字", func(t *testing.T) {
		long := strings.Repeat("中", toolStreamTailRunes+5)
		tail, truncated := toolStreamTail(long, false)
		if !truncated || strings.Contains(tail, "\uFFFD") {
			t.Fatalf("rune tail broken: truncated=%v", truncated)
		}
	})
}

// 历史会话加载：工具回执带展示摘要（eos_gui_tool_result）与内核原始正文。
// 回归点：此前只用摘要，重新打开会话后命令输出退化成「shell exit 0 (cwd: …)」，
// 实时能看到真实输出而重载后看不到。形状取自真实持久化的 messages.json。
func TestBackfillToolResultsRestoresCommandStreamsFromReceipt(t *testing.T) {
	items := []ThreadItem{
		{ID: "call_7", Kind: "tool_call", ToolName: "shell_exec", ToolArgs: `{"command":"git log --oneline -3"}`},
		{ID: "call_8", Kind: "tool_call", ToolName: "shell_exec", ToolArgs: `{"command":"cat missing.txt"}`},
		{ID: "call_1", Kind: "tool_call", ToolName: "shell_pwd", ToolArgs: `{}`},
		{ID: "call_2", Kind: "tool_call", ToolName: "read_file", ToolArgs: `{"path":"README.md"}`},
	}
	receipts := []adapter.SessionMessage{
		{Role: "tool", Type: "tool", Content: `{"output":{"exit_code":0,"stderr":"","stdout":"cc84395 chore: init fixture\n","truncated_stderr":false,"truncated_stdout":false},"request_id":"call_7","status":"ok","tool":"shell_exec","truncated":false}`,
			Metadata: map[string]any{"tool_call_id": "call_7", coreapi.ToolResultPresentationKey: map[string]any{"display": "shell exit 0 (cwd: /ws)", "duration_ms": float64(57), "error": "", "status": "completed"}}},
		{Role: "tool", Type: "tool", Content: `{"output":{"exit_code":1,"stderr":"cat: missing.txt: No such file or directory\n","stdout":"","truncated_stderr":false,"truncated_stdout":false},"request_id":"call_8","status":"error","tool":"shell_exec","truncated":false}`,
			Metadata: map[string]any{"tool_call_id": "call_8", coreapi.ToolResultPresentationKey: map[string]any{"display": "shell exit 1 (cwd: /ws)\ncat: missing.txt: No such file or directory", "duration_ms": float64(52), "error": "", "status": "failed"}}},
		// 非命令类：正文是普通文本，不受影响
		{Role: "tool", Type: "tool", Content: "cwd: /ws",
			Metadata: map[string]any{"tool_call_id": "call_1", coreapi.ToolResultPresentationKey: map[string]any{"display": "cwd: /ws", "duration_ms": float64(2), "error": "", "status": "completed"}}},
		// 正文是 JSON 但 output 是字符串：同样不受影响
		{Role: "tool", Type: "tool", Content: `{"output":"# walk\n","status":"ok","tool":"read_file"}`,
			Metadata: map[string]any{"tool_call_id": "call_2", coreapi.ToolResultPresentationKey: map[string]any{"display": "Read 7 bytes from README.md", "duration_ms": float64(7), "error": "", "status": "completed"}}},
	}
	backfillToolResults(receipts, items)

	ok := items[0].ToolResult
	if ok == nil || ok.Stdout != "cc84395 chore: init fixture\n" || ok.ExitCode == nil || *ok.ExitCode != 0 {
		t.Fatalf("successful command streams not restored: %+v", ok)
	}
	if ok.Status != "completed" || ok.DurationMS != 57 || ok.Output != "shell exit 0 (cwd: /ws)" {
		t.Fatalf("presentation fields lost: %+v", ok)
	}
	failed := items[1].ToolResult
	if failed == nil || failed.Stderr != "cat: missing.txt: No such file or directory\n" || failed.ExitCode == nil || *failed.ExitCode != 1 || failed.Status != "failed" {
		t.Fatalf("failed command streams not restored: %+v", failed)
	}
	for idx, want := range map[int]string{2: "cwd: /ws", 3: "Read 7 bytes from README.md"} {
		got := items[idx].ToolResult
		if got == nil || got.Output != want || got.Stdout != "" || got.Stderr != "" || got.ExitCode != nil {
			t.Fatalf("non-command result %d changed: %+v", idx, got)
		}
	}
}

func TestCommandCwd(t *testing.T) {
	cases := map[string]string{
		"shell exit 0 (cwd: /Users/tina/home/eos-project)":       "/Users/tina/home/eos-project",
		"shell exit 128 (cwd: .)":                                ".",
		"shell exit 1 (cwd: /w)\ncat: missing.txt: No such file": "/w",
		"shell exit 0":                "",
		"bg task t1 exited 0\nlogs":   "",
		"read a.go (1842 bytes)":      "",
		"cwd: /not-the display shape": "",
	}
	for display, want := range cases {
		if got := commandCwd(display); got != want {
			t.Errorf("commandCwd(%q) = %q, want %q", display, got, want)
		}
	}
	result := extractItemToolResult(map[string]any{
		"status": "ok", "display": "shell exit 0 (cwd: /ws)",
		"output": map[string]any{"stdout": "", "stderr": "", "exit_code": float64(0)},
	})
	if result.Cwd != "/ws" {
		t.Fatalf("cwd not carried on the item result: %+v", result)
	}
}

// 可读输出要把命令失败原因带到界面：display 只有退出码，真正原因在
// output.stderr；bg_task 的日志在 output.output。这两条在桌面端已生效，
// 网页模式副本此前只取 display，失败命令只剩「shell exit 128」。
func TestFormatToolResultOutputKeepsCommandFailureReason(t *testing.T) {
	if got := formatToolResultOutput(map[string]any{
		"name": "shell_exec", "display": "shell exit 128",
		"output": map[string]any{"stdout": "", "stderr": "fatal: not a git repository\n"},
	}); got != "shell exit 128\nfatal: not a git repository" {
		t.Fatalf("stderr not appended: %q", got)
	}
	// display 已经包含 stderr 原文时不重复拼接。
	if got := formatToolResultOutput(map[string]any{
		"display": "shell exit 1\ncat: nope",
		"output":  map[string]any{"stderr": "cat: nope"},
	}); got != "shell exit 1\ncat: nope" {
		t.Fatalf("stderr duplicated: %q", got)
	}
	if got := formatToolResultOutput(map[string]any{
		"name": "bg_task", "display": "bg task t1 exited 0",
		"output": map[string]any{"output": "line one\nline two"},
	}); got != "bg task t1 exited 0\nline one\nline two" {
		t.Fatalf("bg_task logs missing: %q", got)
	}
	if got := formatToolResultOutput(map[string]any{"status": "ok", "output": "plain body"}); got != "plain body" {
		t.Fatalf("string output lost: %q", got)
	}
}
