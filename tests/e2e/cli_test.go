// Package e2e 是 EOS 真实二进制端到端测试。
//
// 硬性要求（CASE-CATALOG CLI-*）：
//   - 必须运行真实 eos 子进程。调用内部 RunPrintMode / 直接驱动函数
//     一律不计入这些用例。
//   - 确定性模型由本地脚本服务提供；不接触真实凭据或付费模型。
//   - stdout / stderr 分开捕获，退出码必须断言。
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/testsupport"
)

var (
	buildOnce sync.Once
	buildPath string
	buildErr  error
)

// eosBinary 构建并缓存真实 eos 二进制。
//
// 真实运行二进制而不是调内部函数：只有这样才能覆盖参数解析、stdout/stderr
// 分流、退出码和协议污染这些用户实际会遇到的边界。
func eosBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "eos-e2e-bin-")
		if err != nil {
			buildErr = err
			return
		}
		out := filepath.Join(dir, "eos")
		if runtime.GOOS == "windows" {
			out += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = repoRoot(t)
		if combined, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build: %v\n%s", err, combined)
			return
		}
		buildPath = out
	})
	if buildErr != nil {
		t.Fatalf("构建 eos 二进制失败：%v", buildErr)
	}
	return buildPath
}

// repoRoot 从本文件位置推导 eos-cli 仓库根，不写死开发机路径。
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("e2e: runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// cliFixture 是一次子进程 E2E 需要的隔离环境。
type cliFixture struct {
	env   *testsupport.Env
	model *testsupport.ModelServer
	// Token 是测试用假 token，只通过子进程 env 下发。
	Token string
}

// newCLIFixture 建立隔离环境与脚本模型服务。
func newCLIFixture(t *testing.T, scripts ...*testsupport.ModelScript) *cliFixture {
	t.Helper()
	env := testsupport.NewEnv(t)
	if len(scripts) == 0 {
		scripts = []*testsupport.ModelScript{testsupport.NewBackgroundScript()}
	}
	model := testsupport.NewModelServer(t, scripts...)
	return &cliFixture{env: env, model: model, Token: model.Token}
}

// cmdEnv 组装子进程环境。
//
// 模型通过 EOS_API_BASE / EOS_API_KEY / EOS_MODEL 三个环境变量下发 ——
// 这是真实内核在无钥匙串环境下的受支持配置通道（core 的报错信息也明确
// 指向它），不需要写配置文件或依赖 OS 钥匙串。
func (f *cliFixture) cmdEnv(extra map[string]string) []string {
	env := map[string]string{
		"HOME":                       f.env.Home,
		"EOS_HOME":                   filepath.Join(f.env.Home, ".eos"),
		"EOS_WORKSPACE_ROOT":         f.env.WorkspaceRoot,
		"EOS_SANDBOX_WORKSPACE_ROOT": f.env.WorkspaceRoot,
		"EOS_CORE_STORE_DIR":         f.env.StoreDir,
		"EOS_LOG_LEVEL":              "error",
		"EOS_SANDBOX_MODE":           "workspace-write",
		"EOS_APPROVAL_MODE":          "on-request",
		"EOS_API_BASE":               f.model.BaseURL,
		"EOS_API_KEY":                f.Token,
		"EOS_MODEL":                  "eos-test-model",
	}
	for k, v := range extra {
		env[k] = v
	}
	return testsupport.FlattenEnv(env)
}

// result 是子进程运行的完整结果。
type result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// runCLI 运行一次 eos 子进程并完整捕获结果。
func (f *cliFixture) runCLI(t *testing.T, args []string, stdin string, extraEnv map[string]string, timeout time.Duration) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, eosBinary(t), args...)
	cmd.Dir = f.env.WorkspaceRoot
	cmd.Env = f.cmdEnv(extraEnv)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	res := result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: elapsed}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		} else {
			t.Fatalf("运行 eos 子进程失败：%v", err)
		}
	}
	return res
}

// CLI-01（P0）入口合法性：--help / --version / 错误 flag / 错误子命令。
//
// 精确预期：
//   - 合法入口退出码 0；
//   - 错误入口退出码非 0，且 stderr 有可读信息；
//   - 任何入口都不得触发模型请求（脚本服务必须 0 次命中）。
func TestCLI01_EntrypointsExitCodes(t *testing.T) {
	f := newCLIFixture(t, testsupport.NewTextScript("main", nil, "不应被调用"))
	bin := eosBinary(t)

	cases := []struct {
		name     string
		args     []string
		wantExit int
		wantErr  bool
	}{
		{name: "version", args: []string{"--version"}, wantExit: 0},
		{name: "help", args: []string{"--help"}, wantExit: 0},
		{name: "bad_flag", args: []string{"--definitely-not-a-flag"}, wantExit: 1, wantErr: true},
		{name: "bad_subcommand", args: []string{"definitely-not-a-subcommand"}, wantExit: 1, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := f.runCLI(t, tc.args, "", nil, 60*time.Second)
			if res.ExitCode != tc.wantExit {
				t.Errorf("exit = %d, want %d (stdout=%q stderr=%q)",
					res.ExitCode, tc.wantExit, trunc(res.Stdout, 200), trunc(res.Stderr, 300))
			}
			if tc.wantErr && strings.TrimSpace(res.Stderr) == "" {
				t.Errorf("错误入口 stderr 为空，用户得不到任何诊断信息")
			}
			if !tc.wantErr && tc.name == "version" {
				if !strings.Contains(res.Stdout, "eos v") {
					t.Errorf("--version 输出不含版本号：%q", trunc(res.Stdout, 200))
				}
			}
		})
	}

	// 入口检查不得触发模型调用。
	if n := f.model.ScriptUseCount("main"); n != 0 {
		t.Errorf("入口检查触发了 %d 次模型请求，help/version 不应碰模型", n)
	}
	_ = bin
}

// CLI-02（P0）--print text：stdout 是完整回答，stderr 只有允许的元信息。
//
// 精确预期：
//   - 退出码 0；
//   - stdout 精确等于 "你好，世界🌍"（允许结尾换行）；
//   - stderr 不含 ANSI 控制序列，且只含允许的元信息行；
//   - stdout 不得混入协议或日志内容。
func TestCLI02_PrintTextExactOutput(t *testing.T) {
	f := newCLIFixture(t, testsupport.NewTextScript("main", nil, "你好，", "世界🌍"))
	res := f.runCLI(t, []string{"--print", "打招呼"}, "", nil, 120*time.Second)

	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stdout=%q stderr=%q)", res.ExitCode, trunc(res.Stdout, 300), trunc(res.Stderr, 400))
	}
	got := strings.TrimRight(res.Stdout, "\n")
	if got != "你好，世界🌍" {
		t.Errorf("stdout = %q, want %q", got, "你好，世界🌍")
	}
	if strings.Contains(res.Stdout, "Content-Length") || strings.Contains(res.Stdout, `{"id":`) {
		t.Errorf("stdout 混入了协议帧：%q", trunc(res.Stdout, 300))
	}
	if strings.Contains(testsupport.TrimANSI(res.Stderr), "\x1b") {
		t.Errorf("stderr 含 ANSI 控制序列：%q", trunc(res.Stderr, 300))
	}
	// stderr 只允许元信息，不允许出现正文（正文必须只在 stdout）。
	if strings.Contains(res.Stderr, "你好，世界🌍") {
		t.Errorf("正文同时出现在 stderr，输出被重复：%q", trunc(res.Stderr, 300))
	}
	if f.model.ScriptUseCount("main") == 0 {
		t.Errorf("脚本模型服务未被调用，测试没有真正跑到模型链路")
	}
}

// CLI-03（P0）--print --output-format json：stdout 只有一个 JSON 文档。
//
// 精确预期：
//   - 退出码 0；
//   - stdout 整体可被 json.Unmarshal 成对象；
//   - content 精确等于 fixture 回答；
//   - token 字段语义正确且为正数（来自脚本模型返回的 usage）。
func TestCLI03_PrintJSONDocument(t *testing.T) {
	f := newCLIFixture(t, testsupport.NewTextScript("main", nil, "你好，世界🌍"))
	res := f.runCLI(t, []string{"--print", "打招呼", "--output-format", "json"}, "", nil, 120*time.Second)

	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", res.ExitCode, trunc(res.Stderr, 400))
	}
	trimmed := strings.TrimSpace(res.Stdout)
	var doc map[string]any
	dec := json.NewDecoder(strings.NewReader(trimmed))
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("stdout 不是单个 JSON 文档：%v\nstdout=%q", err, trunc(res.Stdout, 400))
	}
	if dec.More() {
		t.Errorf("stdout 含多个 JSON 文档，机器消费端无法确定边界：%q", trunc(res.Stdout, 400))
	}
	if doc["error"] != nil {
		t.Fatalf("成功路径却返回 error 字段：%v", doc["error"])
	}
	if got, _ := doc["content"].(string); got != "你好，世界🌍" {
		t.Errorf("content = %q, want 你好，世界🌍", got)
	}
	total, ok := doc["total_tokens"].(float64)
	if !ok || total <= 0 {
		t.Errorf("total_tokens = %v, want 正数（脚本模型返回了 usage）", doc["total_tokens"])
	}
	if strings.TrimSpace(res.Stderr) != "" {
		t.Logf("stderr 非空（记录）：%q", trunc(res.Stderr, 200))
	}
}

// CLI-04（P0）stream-json：必须证明首条业务增量在终态之前到达。
//
// 精确预期：
//   - 退出码 0；
//   - stdout 逐行都是合法 JSON（不能混进日志或 ANSI）；
//   - 存在 item.delta 事件，且其出现位置在 turn.completed 之前；
//   - turn.completed 恰好一条；
//   - 所有事件共享同一 turn_id / item_id 关联。
func TestCLI04_StreamJSONIncrementalBeforeTerminal(t *testing.T) {
	// 用屏障制造“长时间流”：模型先发首条增量，然后停在屏障上，
	// 测试在放行之前就检查 stdout 是否已经收到增量。
	barrier := testsupport.NewBarrier()
	f := newCLIFixture(t, &testsupport.ModelScript{
		Name:               "stream",
		TextDeltas:         []string{"首段", "次段"},
		BarrierAfterDeltas: barrier,
		BarrierAfterN:      1,
		DisconnectAfterN:   -1,
	}, testsupport.NewBackgroundScript())

	// 后台运行子进程，边跑边读 stdout。
	cmd := exec.Command(eosBinary(t), "--print", "打招呼", "--output-format", "stream-json")
	cmd.Dir = f.env.WorkspaceRoot
	cmd.Env = f.cmdEnv(nil)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start eos: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	lines := make(chan string, 256)
	go func() {
		defer close(lines)
		dec := json.NewDecoder(stdout)
		for {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return
			}
			lines <- string(raw)
		}
	}()

	// 模型请求到达并停在屏障上 —— 此时内核不应已经完成。
	barrier.Await(t, 60*time.Second)

	// 在放行之前，给 stdout 一点时间拿到首段增量。
	var preTerminal []string
	deadline := time.Now().Add(20 * time.Second)
collecting:
	for time.Now().Before(deadline) {
		select {
		case line, ok := <-lines:
			if !ok {
				break collecting
			}
			preTerminal = append(preTerminal, line)
			if strings.Contains(line, `"item.delta"`) {
				break collecting
			}
		case <-time.After(200 * time.Millisecond):
		}
	}

	if len(preTerminal) == 0 {
		t.Fatalf("放行前 stdout 没有任何输出，无法证明流式增量先于终态到达")
	}
	sawDelta := false
	for _, l := range preTerminal {
		if strings.Contains(l, `"item.delta"`) {
			sawDelta = true
		}
		if strings.Contains(l, `"turn.completed"`) {
			t.Fatalf("模型还在屏障上时就出现了 turn.completed，增量不是真流式")
		}
	}
	if !sawDelta {
		t.Fatalf("放行前只收到 %d 行且没有 item.delta：%v", len(preTerminal), preTerminal)
	}
	t.Logf("放行前已收到 %d 行（首段增量已到达）", len(preTerminal))

	// 放行后应能收到终态。
	barrier.Release()

	collected := append([]string{}, preTerminal...)
	drainDeadline := time.After(60 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				goto done
			}
			collected = append(collected, line)
		case <-drainDeadline:
			t.Fatalf("放行后 60s 内 stdout 未关闭（终态缺失）")
		}
	}
done:
	_ = cmd.Wait()

	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("exit = %v, want 0 (stderr=%q)", cmd.ProcessState, trunc(stderrBuf.String(), 300))
	}

	// 逐行校验：每行合法 JSON、类型已知、关联 ID 一致。
	var turnIDs, itemIDs []string
	deltaCount, terminalCount := 0, 0
	for i, line := range collected {
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("第 %d 行不是合法 JSON：%v\nline=%q", i+1, err, trunc(line, 300))
		}
		typ, _ := ev["type"].(string)
		if typ == "" {
			t.Errorf("第 %d 行缺少 type：%q", i+1, trunc(line, 200))
		}
		if strings.ContainsAny(line, "\x1b") {
			t.Errorf("第 %d 行混入 ANSI 控制序列", i+1)
		}
		if id, ok := ev["turn_id"].(string); ok && id != "" {
			turnIDs = append(turnIDs, id)
		}
		switch typ {
		case "item.delta":
			deltaCount++
			if d, _ := ev["delta"].(string); d != "" {
				itemIDs = append(itemIDs, fmt.Sprintf("%v", ev["item_id"]))
			}
		case "turn.completed":
			terminalCount++
		}
	}
	if deltaCount == 0 {
		t.Errorf("stream-json 没有 item.delta 事件")
	}
	if terminalCount != 1 {
		t.Errorf("turn.completed 出现 %d 次，want 1", terminalCount)
	}
	if len(turnIDs) > 1 {
		uniques := map[string]bool{}
		for _, id := range turnIDs {
			uniques[id] = true
		}
		if len(uniques) > 1 {
			t.Errorf("同一进程的事件用了多个 turn_id：%v", uniques)
		}
	}
	if len(itemIDs) > 0 && itemIDs[0] == "" {
		t.Errorf("item.delta 缺少 item_id 关联")
	}
	t.Logf("stream-json 事件行数=%d delta=%d terminal=%d turn=%v",
		len(collected), deltaCount, terminalCount, turnIDs)
}

// CLI-05（P0）stdin 输入：中文、多行、空内容。
//
// 精确预期：
//   - 多行中文输入完整抵达模型（断言脚本服务收到的真实请求内容）；
//   - 空 stdin 非零退出且有可读错误。
func TestCLI05_StdinPrompt(t *testing.T) {
	const marker = "第一行中文\n第二行 emoji 🌍"
	f := newCLIFixture(t, testsupport.NewTextScript("main", nil, "收到"))
	bin := eosBinary(t)

	t.Run("multiline_stdin", func(t *testing.T) {
		res := f.runCLI(t, []string{"exec", "-"}, marker, nil, 120*time.Second)
		if res.ExitCode != 0 {
			t.Fatalf("exit = %d, want 0 (stderr=%q)", res.ExitCode, trunc(res.Stderr, 400))
		}
		reqs := f.model.WaitForRequests(t, 1, 30*time.Second)
		body := string(reqs[0].RawBody)
		for _, want := range []string{"第一行中文", "第二行"} {
			if !strings.Contains(body, want) {
				t.Errorf("模型请求里缺少 stdin 输入片段 %q（输入被截断或损坏）", want)
			}
		}
		if !strings.Contains(body, "🌍") {
			t.Errorf("模型请求里缺少 emoji，UTF-8 在 stdin 传输中被破坏")
		}
	})

	t.Run("empty_stdin", func(t *testing.T) {
		// 空输入必须失败，而不是静默成功或永久等待。
		cmd := exec.CommandContext(context.Background(), bin, "exec", "-")
		cmd.Dir = f.env.WorkspaceRoot
		cmd.Env = f.cmdEnv(nil)
		cmd.Stdin = strings.NewReader("")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("空 stdin 竟然成功（exit=0），output=%q", trunc(string(out), 200))
		}
		if !strings.Contains(strings.ToLower(string(out)), "empty") &&
			!strings.Contains(strings.ToLower(string(out)), "空") {
			t.Errorf("空 stdin 错误信息不可读：%q", trunc(string(out), 300))
		}
	})
}

// CLI-08（P0）模型故障：401 / 429 / 500 / 断流都必须非零退出且不得伪装成功。
func TestCLI08_ModelFailuresAreNotSuccess(t *testing.T) {
	cases := []struct {
		name   string
		script *testsupport.ModelScript
	}{
		{name: "http_401", script: testsupport.NewErrorScript("fail", nil, 401, `{"error":{"message":"unauthorized","type":"auth"}}`)},
		{name: "http_429", script: testsupport.NewErrorScript("fail", nil, 429, `{"error":{"message":"rate limited","type":"rate"}}`)},
		{name: "http_500", script: testsupport.NewErrorScript("fail", nil, 500, `{"error":{"message":"server error","type":"server"}}`)},
		{name: "stream_cut", script: &testsupport.ModelScript{
			Name: "fail", TextDeltas: []string{"只发了一半"}, DisconnectAfterN: 1,
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCLIFixture(t, tc.script, testsupport.NewBackgroundScript())
			res := f.runCLI(t, []string{"--print", "打招呼", "--output-format", "json"}, "", nil, 120*time.Second)

			if res.ExitCode == 0 {
				t.Errorf("exit = 0，模型失败却报告成功 (stdout=%q)", trunc(res.Stdout, 300))
			}
			// 不得出现“成功终态”。
			for _, bad := range []string{`"type":"turn.completed"`} {
				if strings.Contains(res.Stdout, bad) {
					t.Errorf("失败路径却出现成功终态 %s（stdout=%q）", bad, trunc(res.Stdout, 300))
				}
			}
			// json 模式必须给出可解析的错误对象，而不是空输出。
			trimmed := strings.TrimSpace(res.Stdout)
			if tc.name != "stream_cut" && trimmed != "" {
				var doc map[string]any
				if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
					t.Errorf("失败时 stdout 不是合法 JSON：%v (%q)", err, trunc(trimmed, 200))
				} else if doc["error"] == nil {
					t.Errorf("失败时 JSON 里没有 error 字段：%q", trunc(trimmed, 200))
				}
			}
			if strings.TrimSpace(res.Stderr) == "" && strings.TrimSpace(res.Stdout) == "" {
				t.Errorf("失败时 stdout 与 stderr 都为空，用户得不到任何诊断")
			}
		})
	}
}

// CLI-07（P0）内核不可用：必须非零退出 + 明确错误，且不得给成功 JSON。
func TestCLI07_CoreUnavailableFailsLoudly(t *testing.T) {
	f := newCLIFixture(t, testsupport.NewTextScript("main", nil, "不应出现"))

	t.Run("core_missing", func(t *testing.T) {
		res := f.runCLI(t, []string{"--print", "打招呼", "--output-format", "json"}, "", map[string]string{
			"EOS_CORE_PATH": filepath.Join(f.env.Root, "definitely-not-a-core"),
		}, 120*time.Second)
		if res.ExitCode == 0 {
			t.Errorf("内核不存在时 exit = 0（stdout=%q）", trunc(res.Stdout, 300))
		}
		if strings.Contains(res.Stdout, `"content"`) {
			t.Errorf("内核缺失却返回了成功 JSON：%q", trunc(res.Stdout, 300))
		}
		// DEFECT/CLI-07：当前实现内核缺失时 stdout 与 stderr 全空。
		// 非零退出码本身不够 —— 运维和编排器无法区分“参数错 / 内核缺失 /
		// 权限不足 / 内核崩溃”。这里要求至少给出可定位的诊断。
		if strings.TrimSpace(res.Stderr) == "" && strings.TrimSpace(res.Stdout) == "" {
			t.Errorf("DEFECT/CLI-07: 内核缺失时 exit=%d 但 stdout 与 stderr 全空（各 0 字节），"+
				"调用方拿不到任何可定位的诊断信息。复现: EOS_CORE_PATH=<不存在的路径> eos --print x --output-format json",
				res.ExitCode)
		}
	})

	t.Run("core_not_executable", func(t *testing.T) {
		// 一个存在但不可执行的文件：必须明确失败，不得静默回退。
		notExec := filepath.Join(f.env.Root, "not-exec")
		if err := os.WriteFile(notExec, []byte("not a binary"), 0o644); err != nil {
			t.Fatalf("准备不可执行文件: %v", err)
		}
		res := f.runCLI(t, []string{"--print", "打招呼"}, "", map[string]string{
			"EOS_CORE_PATH": notExec,
		}, 120*time.Second)
		if res.ExitCode == 0 {
			t.Errorf("内核不可执行时 exit = 0（stdout=%q）", trunc(res.Stdout, 300))
		}
	})

	t.Run("core_exits_on_startup", func(t *testing.T) {
		// 一个能执行但立刻非零退出的“内核”：握手必然失败。
		fake := filepath.Join(f.env.Root, "fake-core")
		script := "#!/bin/sh\necho 'fake core refuses to start' >&2\nexit 7\n"
		if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
			t.Fatalf("准备假内核: %v", err)
		}
		res := f.runCLI(t, []string{"--print", "打招呼", "--output-format", "json"}, "", map[string]string{
			"EOS_CORE_PATH": fake,
		}, 120*time.Second)
		if res.ExitCode == 0 {
			t.Errorf("内核启动即退出时 exit = 0（stdout=%q）", trunc(res.Stdout, 300))
		}
		if strings.Contains(res.Stdout, `"content"`) {
			t.Errorf("内核崩溃却返回成功 JSON：%q", trunc(res.Stdout, 300))
		}
	})
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
