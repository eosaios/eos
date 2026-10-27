package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// stdio_client 进程生命周期批测：真实脚本子进程驱动 Start/退出监看/
// 重启链/Close 双路径，外加 derivedEnv / mergeStdioEnv / streamAdapter
// 纯函数矩阵。显式 CorePath 走 resolver 的免 manifest 分支（requireFile
// 即过），脚本行为=吃 stdin、收到 EOF 后数秒退出。

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// newScriptCore 写一个假 eos-core 脚本：stdin 读到 EOF 后短睡退出。
func newScriptCore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "eos-core")
	script := "#!/bin/sh\nwhile read line; do :; done\nsleep 2\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write script core: %v", err)
	}
	return path
}

func newLifecycleClient(t *testing.T) *StdioClient {
	t.Helper()
	t.Setenv("EOS_CORE_PATH", "")
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")
	client := NewStdioClient(StdioClientOptions{
		CorePath:         newScriptCore(t),
		CoreLogDir:       t.TempDir(),
		Workspace:        t.TempDir(),
		SandboxMode:      "workspace-write",
		StoreDir:         filepath.Join(t.TempDir(), "store"),
		VerifyChecksum:   false,
		RequireSignature: false,
	})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestStdioClientProcessLifecycleAndRestart(t *testing.T) {
	client := newLifecycleClient(t)
	ctx := context.Background()

	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// 双重启动错误臂。
	if err := client.Start(ctx); err == nil || !strings.Contains(err.Error(), "already started") {
		t.Fatalf("double start = %v", err)
	}
	// 解析产物回读：显式 CorePath 透传。
	if rb := client.ResolvedBinary(); rb.Path == "" {
		t.Fatalf("ResolvedBinary = %+v", rb)
	}
	// 日志文件应已建（CoreLogDir 生效）。
	logPath := filepath.Join(client.opts.CoreLogDir, "core", "eos-core.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("core log 未创建: %v", err)
	}

	// 杀进程 → 退出监看收口 done + ProcessState 就绪。
	if client.cmd == nil || client.cmd.Process == nil {
		t.Fatal("进程未启动")
	}
	_ = client.cmd.Process.Kill()
	select {
	case <-client.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done 未在杀进程后关闭")
	}
	st := client.ProcessState()
	if st == nil {
		t.Fatal("退出后 ProcessState 应非 nil")
	}
	if st.Success() {
		t.Fatal("被杀进程 Success 应为 false")
	}
	if st.ExitCode() == 0 {
		t.Fatalf("被杀进程 ExitCode = %d", st.ExitCode())
	}

	// ensureStarted → restart 重启链：新进程重新拉起。
	if err := client.ensureStarted(ctx); err != nil {
		t.Fatalf("ensureStarted restart: %v", err)
	}
	client.mu.Lock()
	newCmd := client.cmd
	client.mu.Unlock()
	if newCmd == nil || newCmd.Process == nil {
		t.Fatal("重启后进程缺失")
	}
	// 二次 ensureStarted（未退出）幂等直达。
	if err := client.ensureStarted(ctx); err != nil {
		t.Fatalf("ensureStarted idempotent: %v", err)
	}

	// Close 优雅路径：关流 → 脚本读 EOF 退出 → done。
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-client.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Close 后 Done 未关闭")
	}
}

func TestStdioClientStartFailuresAndCloseKill(t *testing.T) {
	t.Setenv("EOS_CORE_PATH", "")
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")

	// 不存在的 CorePath → resolver 错误臂。
	client := NewStdioClient(StdioClientOptions{CorePath: "/definitely/missing/eos-core"})
	if err := client.Start(context.Background()); err == nil {
		t.Fatal("缺失 CorePath Start 应报错")
	}
	// Start 失败后再 Close 不应 panic（cmd nil 路径）。
	if err := client.Close(); err != nil {
		t.Fatalf("Close after failed start: %v", err)
	}

	// 不可执行文件 → cmd.Start 错误臂。
	dir := t.TempDir()
	notExec := filepath.Join(dir, "eos-core")
	if err := os.WriteFile(notExec, []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	client2 := NewStdioClient(StdioClientOptions{CorePath: notExec})
	if err := client2.Start(context.Background()); err == nil {
		t.Fatal("不可执行文件 Start 应报错")
	}

	// Close 强杀路径：脚本忽略 stdin EOF（不吃 stdin，长睡），
	// Close 关流后 500ms 内进程不退 → Kill 兜底。
	longSleep := filepath.Join(dir, "eos-core-sleep")
	if err := os.WriteFile(longSleep, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	client3 := NewStdioClient(StdioClientOptions{CorePath: longSleep})
	if err := client3.Start(context.Background()); err != nil {
		t.Fatalf("sleep core Start: %v", err)
	}
	start := time.Now()
	if err := client3.Close(); err != nil {
		t.Fatalf("Close kill path: %v", err)
	}
	select {
	case <-client3.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("强杀后 Done 未关闭")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Close 耗时异常: %v", elapsed)
	}
}

func TestStdioClientRestartFailureArm(t *testing.T) {
	t.Setenv("EOS_CORE_PATH", "")
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")

	// 先正常启动，杀掉后把 CorePath 指向坏路径再 restart → 失败臂。
	script := newScriptCore(t)
	client := NewStdioClient(StdioClientOptions{CorePath: script})
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	_ = client.cmd.Process.Kill()
	select {
	case <-client.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done 未关闭")
	}
	client.mu.Lock()
	client.opts.CorePath = "/definitely/missing/eos-core"
	client.mu.Unlock()
	if err := client.ensureStarted(context.Background()); err == nil {
		t.Fatal("坏路径 restart 应报错")
	}
}

func TestDerivedEnvMatrix(t *testing.T) {
	// workspace + 显式 sandbox：透传 + workspace（下划线/单词形态）放行网络。
	// 注：bridge 生产传的 "workspace-write"（连字符）不落 ALLOW_NETWORK env——
	// 内核 SandboxPolicy::derive 按 mode 自派生 allow_network（壳层不手组装
	// Policy），该 env 只服务 CLI 的 workspace/workspace_write 形态。
	env := (&StdioClient{opts: StdioClientOptions{
		Workspace: "/ws", SandboxMode: "workspace", StoreDir: "/store",
		Env: map[string]string{" EOS_EXTRA ": " v "},
	}}).derivedEnv()
	if env["EOS_WORKSPACE_ROOT"] != "/ws" || env["EOS_SANDBOX_WORKSPACE_ROOT"] != "/ws" {
		t.Fatalf("workspace env = %v", env)
	}
	if env["EOS_SANDBOX_MODE"] != "workspace" {
		t.Fatalf("sandbox mode = %q", env["EOS_SANDBOX_MODE"])
	}
	if env["EOS_SANDBOX_ALLOW_NETWORK"] != "true" {
		t.Fatalf("workspace 模式应放行网络: %v", env)
	}
	if env["EOS_CORE_STORE_DIR"] != "/store" {
		t.Fatalf("store dir = %q", env["EOS_CORE_STORE_DIR"])
	}
	if env["EOS_EXTRA"] != " v " {
		t.Fatalf("自定义 env 透传 = %q", env["EOS_EXTRA"])
	}
	if env["EOS_MODEL_PROVIDER"] != "" {
		t.Fatalf("provider 应清空: %q", env["EOS_MODEL_PROVIDER"])
	}
	// workspace_write（下划线）同放行；workspace-write（连字符）不设该 env。
	env = (&StdioClient{opts: StdioClientOptions{Workspace: "/ws", SandboxMode: "workspace_write"}}).derivedEnv()
	if env["EOS_SANDBOX_ALLOW_NETWORK"] != "true" {
		t.Fatalf("workspace_write 应放行网络: %v", env)
	}
	env = (&StdioClient{opts: StdioClientOptions{Workspace: "/ws", SandboxMode: "workspace-write"}}).derivedEnv()
	if _, ok := env["EOS_SANDBOX_ALLOW_NETWORK"]; ok {
		t.Fatal("连字符形态由内核自派生，不应设 ALLOW_NETWORK")
	}

	// 无 sandbox + workspace：默认 workspace 模式 + store 落 workspace。
	env = (&StdioClient{opts: StdioClientOptions{Workspace: "/ws"}}).derivedEnv()
	if env["EOS_SANDBOX_MODE"] != "workspace" {
		t.Fatalf("默认 sandbox = %q", env["EOS_SANDBOX_MODE"])
	}
	if env["EOS_CORE_STORE_DIR"] != filepath.Join("/ws", ".eos", "core") {
		t.Fatalf("默认 store = %q", env["EOS_CORE_STORE_DIR"])
	}

	// 无 workspace 无 sandbox：降级 read-only。
	env = (&StdioClient{}).derivedEnv()
	if env["EOS_SANDBOX_MODE"] != "read-only" {
		t.Fatalf("无 workspace 默认 = %q", env["EOS_SANDBOX_MODE"])
	}
	if _, ok := env["EOS_SANDBOX_ALLOW_NETWORK"]; ok {
		t.Fatal("read-only 不应放行网络")
	}

	// 空白键的自定义 env 跳过。
	env = (&StdioClient{opts: StdioClientOptions{Env: map[string]string{"   ": "x"}}}).derivedEnv()
	if _, ok := env[""]; ok {
		t.Fatal("空白键不应写入")
	}
}

func TestMergeStdioEnvMatrix(t *testing.T) {
	base := []string{"PATH=/usr/bin", "eos_model_provider=fake"}
	out := mergeStdioEnv(base, map[string]string{
		"EOS_MODEL_PROVIDER": "",  // 大小写不敏感替换既有键
		"EOS_NEW_KEY":        "v", // 新键追加
	})
	if len(out) != 3 {
		t.Fatalf("合并长度 = %d: %v", len(out), out)
	}
	if out[1] != "EOS_MODEL_PROVIDER=" {
		t.Fatalf("既有键替换 = %q", out[1])
	}
	if out[2] != "EOS_NEW_KEY=v" {
		t.Fatalf("新键追加 = %q", out[2])
	}
	// 空 overrides 原样返回副本。
	out2 := mergeStdioEnv([]string{"A=1"}, nil)
	if len(out2) != 1 || out2[0] != "A=1" {
		t.Fatalf("空 overrides = %v", out2)
	}
	// 无 "=" 的基项被跳过（不索引）。
	out3 := mergeStdioEnv([]string{"MALFORMED", "A=1"}, map[string]string{"B": "2"})
	if len(out3) != 3 {
		t.Fatalf("畸形基项处理 = %v", out3)
	}
}

func TestStreamAdapterArms(t *testing.T) {
	// nil receiver 三臂。
	var nilAdapter *streamAdapter
	if _, err := nilAdapter.Read(make([]byte, 1)); err != io.ErrClosedPipe {
		t.Fatalf("nil Read = %v", err)
	}
	if _, err := nilAdapter.Write([]byte("x")); err != io.ErrClosedPipe {
		t.Fatalf("nil Write = %v", err)
	}
	if err := nilAdapter.Close(); err != nil {
		t.Fatalf("nil Close = %v", err)
	}
	// reader/writer 缺位臂。
	if _, err := (&streamAdapter{}).Read(make([]byte, 1)); err != io.ErrClosedPipe {
		t.Fatalf("nil reader = %v", err)
	}
	if _, err := (&streamAdapter{}).Write([]byte("x")); err != io.ErrClosedPipe {
		t.Fatalf("nil writer = %v", err)
	}
	if err := (&streamAdapter{}).Close(); err != nil {
		t.Fatalf("empty Close = %v", err)
	}
	// 真实 reader/writer 往返。
	pr, pw := io.Pipe()
	sa := &streamAdapter{reader: bufio.NewReader(pr), writer: pw, closer: pw}
	go func() { _, _ = pw.Write([]byte("hello\n")) }()
	line, err := sa.reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "hello" {
		t.Fatalf("read back = %q, %v", line, err)
	}
	_ = sa.Close()
}

func TestStdioClientNilAndEdgeArms(t *testing.T) {
	// ResolvedBinary nil receiver 臂。
	var nilClient *StdioClient
	if rb := nilClient.ResolvedBinary(); rb.Path != "" {
		t.Fatalf("nil ResolvedBinary = %+v", rb)
	}
	// ProcessState 未退出臂（cmd 在、ProcessState nil）。
	client := newLifecycleClient(t)
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st := client.ProcessState(); st != nil {
		t.Fatalf("运行中 ProcessState 应 nil: %+v", st)
	}
	// 进程存活时 restart 幂等直达臂。
	if err := client.restart(context.Background()); err != nil {
		t.Fatalf("存活进程 restart 应 no-op: %v", err)
	}
	// stdioProcessState nil 臂。
	var nilState *stdioProcessState
	if nilState.ExitCode() != -1 || nilState.Success() {
		t.Fatal("nil stdioProcessState 臂不符")
	}
	// Call 的 ensureStarted 错误臂：真死进程（勿手工置 exited=true——
	// 「进程活着却标记退出」违反不变量，退出监看 goroutine 会读到被
	// restart 置 nil 的 cmd 而 panic；生产 exited 只在 Wait 返回后置位）。
	_ = client.cmd.Process.Kill()
	select {
	case <-client.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done 未关闭")
	}
	client.mu.Lock()
	client.opts.CorePath = "/definitely/missing/eos-core"
	client.mu.Unlock()
	if err := client.Call(context.Background(), "x", nil, nil); err == nil {
		t.Fatal("坏路径 Call 应经 ensureStarted 报错")
	}
}

func TestStdioClientCoreLogOpenFailure(t *testing.T) {
	t.Setenv("EOS_CORE_PATH", "")
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")
	// CoreLogDir 挂在普通文件之下：OpenFile 必败 → 告警臂 + stderr 丢弃。
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewStdioClient(StdioClientOptions{
		CorePath:   newScriptCore(t),
		CoreLogDir: filepath.Join(blocker, "sub"),
	})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start（日志打开失败不阻塞）: %v", err)
	}
}

func TestStdioRPCClientUnitArms(t *testing.T) {
	pr, pw := io.Pipe()
	_ = pw.Close()
	_ = pr.Close()
	rc := newStdioRPCClient(&streamAdapter{reader: bufio.NewReader(pr), writer: pw})

	// 未启动 Call：not started 臂。
	if err := rc.Call(context.Background(), "m", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "not started") {
		t.Fatalf("未启动 Call = %v", err)
	}
	// 关闭流上 Start 惰性成功（读循环事后退出）；随后 nil ctx Call 走
	// Background 兜底臂，死流上快速失败。
	if err := rc.Start(context.Background()); err != nil {
		t.Fatalf("惰性 Start = %v", err)
	}
	// 二次 Start 幂等直达臂。
	if err := rc.Start(context.Background()); err != nil {
		t.Fatalf("二次 Start 应幂等: %v", err)
	}
	if err := rc.Call(nil, "m", nil, nil); err == nil {
		t.Fatal("死流 Call 应报错")
	}
	// handleNotification nil handler 臂 + 透传臂。
	if err := rc.handleNotification(context.Background(), protocoljsonrpc.Notification{}); err != nil {
		t.Fatalf("nil handler = %v", err)
	}
	called := false
	rc.SetNotificationHandler(func(context.Context, protocoljsonrpc.Notification) error {
		called = true
		return nil
	})
	_ = rc.handleNotification(context.Background(), protocoljsonrpc.Notification{})
	if !called {
		t.Fatal("handler 未透传")
	}
}
