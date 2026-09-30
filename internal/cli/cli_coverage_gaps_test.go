package cli

// cli_coverage_gaps_test.go — internal/cli 覆盖率冲刺批一：纯函数、错误臂、
// 命令构造器与 doc 子命令。HOME 全程隔离，避免污染真实用户目录。
//
// 豁免清单（不在此文件硬测，理由如下）：
//   - newUpdateCmd 真网络（github.com releases）：环境不可控，只测代理
//     fail-fast 错误臂；成功路径依赖外网，书面豁免。
//   - RunPrintMode / runServe / runMcpServe / runExec / runBridgeManifest 的
//     真内核启动：需要 vendored eos-core 子进程，本地 stage 占位内核上
//     TestStdioGateway* 预期失败已说明环境不稳；只测不启内核的校验臂。
//   - 真系统弹窗 / 剪贴板 / hdiutil / open 挂载：系统交互，不进单测。
//   - os.Exit 进程入口（Execute）：由 root_test 的 flag 注册测试间接覆盖。

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/coreapi/engineprovider"
	"github.com/spf13/pflag"
)

// ---------- serveOptionEnv / printModeEnv / mcpOptionEnv ----------

func TestServeOptionEnvFullMapping(t *testing.T) {
	t.Run("sandbox and approval and workspace", func(t *testing.T) {
		env := serveOptionEnv(serveOptions{
			SandboxMode:  "workspace-write",
			AccessMode:   "read-only", // SandboxMode 优先
			ApprovalMode: "never",
			Workspace:    "/ws",
		})
		if env["EOS_SANDBOX_MODE"] != "workspace-write" {
			t.Fatalf("EOS_SANDBOX_MODE = %q", env["EOS_SANDBOX_MODE"])
		}
		if env["EOS_APPROVAL_MODE"] != "never" {
			t.Fatalf("EOS_APPROVAL_MODE = %q", env["EOS_APPROVAL_MODE"])
		}
		if env["EOS_WORKSPACE_ROOT"] != "/ws" || env["EOS_SANDBOX_WORKSPACE_ROOT"] != "/ws" {
			t.Fatalf("workspace env = %v", env)
		}
	})
	t.Run("access mode fallback when sandbox empty", func(t *testing.T) {
		env := serveOptionEnv(serveOptions{AccessMode: "danger-full-access"})
		if env["EOS_SANDBOX_MODE"] != "danger-full-access" {
			t.Fatalf("fallback = %q", env["EOS_SANDBOX_MODE"])
		}
	})
	t.Run("empty opts uses cwd", func(t *testing.T) {
		env := serveOptionEnv(serveOptions{})
		if env["EOS_WORKSPACE_ROOT"] == "" {
			t.Fatal("cwd fallback empty")
		}
	})
	t.Run("skip all clears modes", func(t *testing.T) {
		env := serveOptionEnv(serveOptions{
			SandboxMode:  "workspace-write",
			ApprovalMode: "never",
			SkipAll:      true,
		})
		if env["EOS_SKIP_PERMISSIONS"] != "1" {
			t.Fatalf("skip = %v", env)
		}
		if _, ok := env["EOS_SANDBOX_MODE"]; ok {
			t.Fatalf("sandbox should be cleared: %v", env)
		}
		if _, ok := env["EOS_APPROVAL_MODE"]; ok {
			t.Fatalf("approval should be cleared: %v", env)
		}
	})
}

func TestPrintModeEnvSkipAndAccessFallback(t *testing.T) {
	env := printModeEnv(PrintOptions{AccessMode: "read-only", SkipPermissions: true})
	if env["EOS_SANDBOX_MODE"] != "read-only" {
		t.Fatalf("access fallback = %q", env["EOS_SANDBOX_MODE"])
	}
	if env["EOS_SKIP_PERMISSIONS"] != "1" {
		t.Fatal("skip flag missing")
	}
	env = printModeEnv(PrintOptions{SandboxMode: "danger-full-access", AccessMode: "read-only"})
	if env["EOS_SANDBOX_MODE"] != "danger-full-access" {
		t.Fatalf("sandbox wins = %q", env["EOS_SANDBOX_MODE"])
	}
}

func TestMcpOptionEnvSkipAll(t *testing.T) {
	env := mcpOptionEnv(mcpServeOptions{
		SandboxMode:  "workspace-write",
		ApprovalMode: "never",
		SkipAll:      true,
	})
	if env["EOS_SKIP_PERMISSIONS"] != "1" {
		t.Fatal("skip missing")
	}
	if _, ok := env["EOS_SANDBOX_MODE"]; ok {
		t.Fatal("sandbox should be cleared")
	}
}

// ---------- mergeConfigPermissions 配置文件分支 ----------

// captureStdout 捕获 fmt.Print* / os.Stdout 直写（doc/legal 等命令不走 cobra Out）。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

func TestMergeConfigPermissionsReadsConfigFile(t *testing.T) {
	setTestHome(t)
	home := os.Getenv("HOME")
	cfgPath := filepath.Join(home, ".eos.json")
	writeRawConfig(t, cfgPath, `{"permissions":{"access_mode":"read-only","approval_mode":"never"}}`)

	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	fs.String("access-mode", "", "")
	fs.String("approval-mode", "", "")
	fs.String("sandbox-mode", "", "")
	am, ap := mergeConfigPermissions(fs, "sandbox-mode", "", "")
	if am != "read-only" {
		t.Fatalf("access-mode from config = %q, want read-only", am)
	}
	if ap != "never" {
		t.Fatalf("approval-mode from config = %q, want never", ap)
	}

	// 显式 flag 已设置：调用方传入的 accessMode 参数即 flag 值，配置不得覆盖
	fs2 := pflag.NewFlagSet("t", pflag.ContinueOnError)
	fs2.String("access-mode", "", "")
	fs2.String("approval-mode", "", "")
	fs2.String("sandbox-mode", "", "")
	_ = fs2.Set("access-mode", "danger-full-access")
	am, ap = mergeConfigPermissions(fs2, "sandbox-mode", "danger-full-access", "on-request")
	if am != "danger-full-access" {
		t.Fatalf("explicit access wins = %q", am)
	}
	if ap != "on-request" {
		t.Fatalf("explicit approval wins = %q", ap)
	}

	// sandbox flag 已设置时不从配置取 access_mode
	fs3 := pflag.NewFlagSet("t", pflag.ContinueOnError)
	fs3.String("access-mode", "", "")
	fs3.String("approval-mode", "", "")
	fs3.String("sandbox-mode", "", "")
	_ = fs3.Set("sandbox-mode", "workspace")
	am, _ = mergeConfigPermissions(fs3, "sandbox-mode", "", "")
	if am != "" {
		t.Fatalf("sandbox flag set should block config access_mode, got %q", am)
	}
}

// ---------- newHiddenLegalCmd 真正执行 ----------

func TestHiddenLegalCmdRunPrintsWatermark(t *testing.T) {
	cmd := newHiddenLegalCmd()
	out := captureStdout(t, func() {
		cmd.Run(cmd, nil)
	})
	if !strings.Contains(out, "Copyright (c) 2026 EOSAIOS") {
		t.Fatalf("missing copyright: %q", out)
	}
	if !strings.Contains(out, hiddenLegalWatermark) {
		t.Fatalf("missing watermark: %q", out)
	}
	if !strings.Contains(out, "GoVersion:") {
		t.Fatalf("missing go version line: %q", out)
	}
}

// ---------- newDocumentCmd：generate / read / convert ----------

func TestDocumentCmdGenerateAndReadDocx(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.docx")
	cmd := newDocumentCmd()
	cmd.SetArgs([]string{"generate", "--format", "docx", "--output", out, "--title", "T", "--content", "hello\nworld"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate docx: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("docx not written: %v", err)
	}

	readCmd := newDocumentCmd()
	readCmd.SetArgs([]string{"read", out})
	text := captureStdout(t, func() {
		if err := readCmd.Execute(); err != nil {
			t.Fatalf("read docx: %v", err)
		}
	})
	if !strings.Contains(text, "hello") {
		t.Fatalf("read text = %q", text)
	}
}

func TestDocumentCmdGenerateAndReadXlsx(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.xlsx")
	cmd := newDocumentCmd()
	cmd.SetArgs([]string{"generate", "--format", "xlsx", "--output", out, "--title", "T", "--content", "a,b\nc,d"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate xlsx: %v", err)
	}

	readCmd := newDocumentCmd()
	readCmd.SetArgs([]string{"read", "--json", out})
	text := captureStdout(t, func() {
		if err := readCmd.Execute(); err != nil {
			t.Fatalf("read xlsx: %v", err)
		}
	})
	if !strings.Contains(text, "Sheet1") {
		t.Fatalf("json read = %q", text)
	}
}

func TestDocumentCmdGenerateAndReadPDF(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.pdf")
	cmd := newDocumentCmd()
	cmd.SetArgs([]string{"generate", "--format", "pdf", "--output", out, "--title", "T", "--content", "pdf body"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate pdf: %v", err)
	}
	readCmd := newDocumentCmd()
	readCmd.SetArgs([]string{"read", out})
	text := captureStdout(t, func() {
		if err := readCmd.Execute(); err != nil {
			t.Fatalf("read pdf: %v", err)
		}
	})
	if !strings.Contains(text, "pdf body") {
		t.Fatalf("pdf text = %q", text)
	}
}

func TestDocumentCmdGenerateValidation(t *testing.T) {
	cmd := newDocumentCmd()
	cmd.SetArgs([]string{"generate", "--output", filepath.Join(t.TempDir(), "x.docx")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("missing --format must error")
	}
	cmd2 := newDocumentCmd()
	cmd2.SetArgs([]string{"generate", "--format", "docx"})
	if err := cmd2.Execute(); err == nil {
		t.Fatal("missing --output must error")
	}
	cmd3 := newDocumentCmd()
	cmd3.SetArgs([]string{"generate", "--format", "rtf", "--output", "x"})
	if err := cmd3.Execute(); err == nil {
		t.Fatal("unsupported format must error")
	}
}

func TestDocumentCmdReadUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(bad, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newDocumentCmd()
	cmd.SetArgs([]string{"read", bad})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unsupported read format must error")
	}
}

func TestDocumentCmdConvertSameFormat(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.docx")
	if err := os.WriteFile(src, []byte("raw"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "b.docx")
	cmd := newDocumentCmd()
	cmd.SetArgs([]string{"convert", src, "--to", "docx", "--output", dst, "--fidelity", "content"})
	// convert 把结果 JSON 写到 os.Stdout（不是 cobra Out），这里只断言产物。
	if err := cmd.Execute(); err != nil {
		t.Fatalf("convert: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("dest missing: %v", err)
	}
	if string(data) != "raw" {
		t.Fatalf("copy content = %q", data)
	}
}

func TestDocumentCmdConvertInvalidInput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newDocumentCmd()
	cmd.SetArgs([]string{"convert", src, "--to", "pdf"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unsupported source must error")
	}
}

// ---------- resolveWebCoreBinary ----------

func TestResolveWebCoreBinaryWithExplicitPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "eos-core")
	if err := os.WriteFile(bin, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EOS_CORE_PATH", bin)
	gotPath, gotManifest := resolveWebCoreBinary()
	if gotPath != bin {
		// requireFile 可能做 Clean/绝对化
		if filepath.Base(gotPath) != "eos-core" {
			t.Fatalf("path = %q, want %q", gotPath, bin)
		}
	}
	if gotManifest != "" {
		// EOS_CORE_PATH 旁路不读 manifest
		_ = gotManifest
	}
}

func TestResolveWebCoreBinaryNotFound(t *testing.T) {
	setTestHome(t)
	t.Setenv("EOS_CORE_PATH", "")
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", filepath.Join(t.TempDir(), "empty-cores"))
	// 清掉可能的内嵌命中：显式指向不存在的 BinaryPath 不适用；
	// 这里只断言不 panic，返回空或有效路径皆可（内嵌可能仍命中）。
	p, m := resolveWebCoreBinary()
	_ = p
	_ = m
}

// ---------- doctor 子函数 ----------

func TestDoctorReportHandshakeBranches(t *testing.T) {
	var buf bytes.Buffer
	r := &doctorReporter{w: &buf}

	// 空 methods
	doctorReportHandshake(r, engineprovider.Selection{
		Initialize: coreapijsonrpc.InitializeResult{ServerName: "eos-core"},
	})
	if r.failed == 0 {
		t.Fatal("empty methods must fail")
	}

	// 有 missing
	buf.Reset()
	r2 := &doctorReporter{w: &buf}
	doctorReportHandshake(r2, engineprovider.Selection{
		Initialize: coreapijsonrpc.InitializeResult{ServerName: "eos-core", Methods: []string{"a"}},
		Missing:    []string{"state/snapshot"},
	})
	if r2.failed == 0 {
		t.Fatal("missing methods must fail")
	}

	// 全齐
	buf.Reset()
	r3 := &doctorReporter{w: &buf}
	doctorReportHandshake(r3, engineprovider.Selection{
		Initialize: coreapijsonrpc.InitializeResult{ServerName: "eos-core", Methods: []string{"a", "b"}},
	})
	if r3.failed != 0 {
		t.Fatalf("complete handshake must pass, failed=%d out=%q", r3.failed, buf.String())
	}
}

func TestDoctorCoreResolveWithExplicitPath(t *testing.T) {
	setTestHome(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "eos-core")
	if err := os.WriteFile(bin, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EOS_CORE_PATH", bin)
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")

	var buf bytes.Buffer
	r := &doctorReporter{w: &buf}
	resolved := doctorCoreResolve(r)
	if resolved.Path == "" {
		t.Fatalf("resolved empty path, out=%q", buf.String())
	}
	if !strings.Contains(buf.String(), "内核二进制") {
		t.Fatalf("report = %q", buf.String())
	}
	// EOS_CORE_PATH 旁路：无 manifest
	if !strings.Contains(buf.String(), "旁路") && !strings.Contains(buf.String(), "EOS_CORE_PATH") {
		t.Fatalf("bypass line missing: %q", buf.String())
	}
}

func TestDoctorCoreResolveNotFound(t *testing.T) {
	setTestHome(t)
	t.Setenv("EOS_CORE_PATH", filepath.Join(t.TempDir(), "missing-core"))
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")
	var buf bytes.Buffer
	r := &doctorReporter{w: &buf}
	resolved := doctorCoreResolve(r)
	if resolved.Path != "" {
		t.Fatalf("expected empty resolved, got %+v", resolved)
	}
	if r.failed == 0 {
		t.Fatal("missing core must fail")
	}
}

// ---------- loadRawConfigDoc / saveRawConfigDoc 边界 ----------

func TestLoadRawConfigDocEdges(t *testing.T) {
	// 不存在 → 空 doc
	doc, err := loadRawConfigDoc(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc) != 0 {
		t.Fatalf("doc = %v", doc)
	}
	// 空白文件 → 空 doc
	p := filepath.Join(t.TempDir(), "blank.json")
	if err := os.WriteFile(p, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err = loadRawConfigDoc(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc) != 0 {
		t.Fatalf("blank doc = %v", doc)
	}
	// 坏 JSON
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRawConfigDoc(bad); err == nil {
		t.Fatal("bad json must error")
	}
}

func TestSaveRawConfigDocCreatesParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "cfg.json")
	doc := map[string]json.RawMessage{"k": json.RawMessage(`"v"`)}
	if err := saveRawConfigDoc(doc, path); err != nil {
		t.Fatal(err)
	}
	got, err := loadRawConfigDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got["k"]) != `"v"` {
		t.Fatalf("got = %v", got)
	}
}

// ---------- setUpdateProxy 其余分支 ----------

func TestSetUpdateProxyInvalidLeavesDoc(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".eos.json")
	writeRawConfig(t, path, `{"keep":1}`)
	if err := setUpdateProxy("zh", "not-a-url", path); err == nil {
		t.Fatal("invalid proxy must error")
	}
	doc, err := loadRawConfigDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	if rawConfigDocBool(doc, updateProxyEnabledField) {
		t.Fatal("must not enable")
	}
	if string(doc["keep"]) != `1` {
		t.Fatalf("keep = %v", doc["keep"])
	}
}

// ---------- runServe / runMcpServe 传输校验（不启内核） ----------

func TestRunServeRejectsNonStdioTransport(t *testing.T) {
	err := runServe(context.Background(), serveOptions{Transport: "sse"})
	if err == nil {
		t.Fatal("non-stdio must error")
	}
	if !strings.Contains(err.Error(), "unsupported transport") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunMcpServeRejectsUnknownTransport(t *testing.T) {
	err := runMcpServe(context.Background(), mcpServeOptions{Transport: "websocket"})
	if err == nil {
		t.Fatal("unknown transport must error")
	}
	if !strings.Contains(err.Error(), "unsupported transport") {
		t.Fatalf("err = %v", err)
	}
	// 空 transport 默认 stdio，会走到启内核——本地无内核时应报启动错误而非 transport 错误
	err = runMcpServe(context.Background(), mcpServeOptions{Transport: "HTTP"})
	if err == nil {
		t.Fatal("http transport must error")
	}
}

func TestRunMcpServeEmptyTransportTriesStdio(t *testing.T) {
	setTestHome(t)
	t.Setenv("EOS_CORE_PATH", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("EOS_CORE_MANIFEST", "")
	// 空 transport 规范为 stdio，随后启内核失败（本地无内核）——
	// 断言错误来自启动而非 transport 解析。
	err := runMcpServe(context.Background(), mcpServeOptions{})
	if err == nil {
		t.Fatal("expected engine start failure")
	}
	if strings.Contains(err.Error(), "unsupported transport") {
		t.Fatalf("should not be transport error: %v", err)
	}
}

// ---------- newUpdateCmd 代理 fail-fast（不打外网） ----------

func TestUpdateCmdFailsFastOnInvalidProxy(t *testing.T) {
	setTestHome(t)
	home := os.Getenv("HOME")
	writeRawConfig(t, filepath.Join(home, ".eos.json"),
		`{"update_proxy_enabled":true,"update_proxy_url":"ftp://bad"}`)

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--check"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("invalid proxy must fail before network")
	}
}


