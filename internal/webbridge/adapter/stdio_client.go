package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// StdioClientOptions configures how the stdio JSON-RPC client connects to an
// external `eos-core --stdio` process.
type StdioClientOptions struct {
	// CorePath is the explicit path to eos-core. When empty, the sidecar
	// resolver uses EOS_CORE_PATH, EOS_CORE_MANIFEST, EOS_CORE_BIN_DIR, and
	// bundled manifest search roots.
	CorePath string
	// ManifestPath is an explicit sidecar manifest path.
	ManifestPath string
	// CoreBinDir is an optional root containing <target>/manifest.json.
	CoreBinDir string
	// VerifyChecksum and RequireSignature mirror the CLI sidecar release gates.
	VerifyChecksum      bool
	RequireSignature    bool
	AllowDevPlaceholder bool
	PublicKeyPath       string

	// Deprecated: AppServerPath is kept only so older tests/callers compile.
	// New code should use CorePath.
	AppServerPath string
	// ExtraArgs are appended after the mandatory "--stdio".
	ExtraArgs []string
	// Workspace is passed to eos-core through EOS_WORKSPACE_ROOT.
	Workspace string
	// SandboxMode is passed through EOS_SANDBOX_MODE. Empty defaults to workspace.
	SandboxMode string
	// StoreDir is passed through EOS_CORE_STORE_DIR.
	StoreDir string
	// Env is merged after os.Environ and after derived EOS_* defaults.
	Env map[string]string
	// CoreLogDir is where eos-core stderr is captured (eos-core.log).
	// Empty disables core log capture.
	CoreLogDir string
}

// StdioResolvedBinary captures which eos-core artifact the GUI actually
// resolved before starting the stdio transport.
type StdioResolvedBinary struct {
	Path         string
	ManifestPath string
	Source       string
	Target       string
}

// StdioClient launches an external `eos-core --stdio` process
// and communicates with it over Content-Length framed JSON-RPC via stdin/stdout.
type StdioClient struct {
	opts   StdioClientOptions
	cmd    *exec.Cmd
	stream *streamAdapter
	client *stdioRPCClient
	done   chan struct{}
	exited bool
	// cmdProcessState：Wait 收割后的进程状态快照（锁内传递，防直读竞态）。
	cmdProcessState *os.ProcessState
	restartMu       sync.Mutex
	resolvedBinary  StdioResolvedBinary
	mu              sync.Mutex
}

// streamAdapter bridges io.Reader/io.Writer from cmd.StdinPipe/StdoutPipe
// to the jsonrpc Stream type.
type streamAdapter struct {
	reader *bufio.Reader
	writer io.WriteCloser
	closer io.Closer
}

func (s *streamAdapter) Read(p []byte) (int, error) {
	if s == nil || s.reader == nil {
		return 0, io.ErrClosedPipe
	}
	return s.reader.Read(p)
}

func (s *streamAdapter) Write(p []byte) (int, error) {
	if s == nil || s.writer == nil {
		return 0, io.ErrClosedPipe
	}
	return s.writer.Write(p)
}

func (s *streamAdapter) Close() error {
	if s == nil {
		return nil
	}
	var first error
	if s.writer != nil {
		first = s.writer.Close()
	}
	if s.closer != nil {
		if err := s.closer.Close(); first == nil {
			first = err
		}
	}
	return first
}

// NewStdioClient creates a new stdio JSON-RPC client but does NOT start the
// process. Call Start() to launch and begin communicating.
func NewStdioClient(opts StdioClientOptions) *StdioClient {
	return &StdioClient{
		opts: opts,
		done: make(chan struct{}),
	}
}

// Start launches the eos-core process, wires the stdio JSON-RPC stream, and
// begins the notification read loop.
func (sc *StdioClient) Start(ctx context.Context) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.cmd != nil {
		return errors.New("stdio client already started")
	}
	return sc.startProcessLocked(ctx)
}

// startProcessLocked launches eos-core, wires the transport, and starts the
// exit monitor. Caller must hold sc.mu.
func (sc *StdioClient) startProcessLocked(ctx context.Context) error {
	resolved, err := sc.resolveCoreBinary()
	if err != nil {
		return err
	}
	sc.resolvedBinary = resolved
	binaryPath := resolved.Path

	args := []string{"--stdio"}
	args = append(args, sc.opts.ExtraArgs...)

	// Keep the sidecar process alive after startup completes. The startup ctx is
	// only for the initial transport handshake; binding it to CommandContext
	// would kill eos-core as soon as the caller cancels the startup timeout.
	sc.cmd = exec.Command(binaryPath, args...)
	// eos-core 是控制台程序，GUI 壳层拉起必须隐藏控制台，否则桌面端启动
	// 即弹命令行窗口（RELEASE_PROCESS 坑位 #9）。走 eos-cli 共享实现，勿内联。
	sidecar.HideConsole(sc.cmd)
	sc.cmd.Env = mergeStdioEnv(os.Environ(), sc.derivedEnv())

	stdin, err := sc.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdio client stdin pipe: %w", err)
	}
	stdout, err := sc.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdio client stdout pipe: %w", err)
	}

	// Capture eos-core stderr to the core log file instead of discarding it.
	var coreLogPath string
	if dir := strings.TrimSpace(sc.opts.CoreLogDir); dir != "" {
		coreLogPath = filepath.Join(dir, "core", "eos-core.log")
	}
	if coreLogPath == "" {
		sc.cmd.Stderr = nil
	} else {
		_ = os.MkdirAll(filepath.Dir(coreLogPath), 0755)
		coreLog, err := os.OpenFile(coreLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			slog.Warn("stdio_client.core_log_open_failed", "path", coreLogPath, "error", err.Error())
			sc.cmd.Stderr = nil
		} else {
			sc.cmd.Stderr = coreLog
			logDone := sc.done
			go func() {
				// Keep the file handle alive until the process exits.
				// done 同样闭包捕获：restart 会替换 sc.done（无锁读竞态）。
				<-logDone
				_ = coreLog.Close()
			}()
		}
	}

	if err := sc.cmd.Start(); err != nil {
		return fmt.Errorf("stdio client start: %w", err)
	}

	br := bufio.NewReader(stdout)
	sa := &streamAdapter{
		reader: br,
		writer: stdin,
		closer: stdin,
	}
	sc.stream = sa

	sc.client = newStdioRPCClient(sa)
	// The transport client must outlive the startup timeout; individual RPCs
	// provide their own request-scoped contexts.
	if err := sc.client.Start(context.Background()); err != nil {
		_ = sc.cmd.Process.Kill()
		return fmt.Errorf("stdio client stream start: %w", err)
	}

	// Monitor process exit
	// cmd 闭包捕获：监看若在运行期读 sc.cmd，与 Close()/restart() 置 nil 的
	// 窗口竞争（eos-app -race 实测同源代码）。捕获后 Wait 作用于已启动进程。
	monitorCmd := sc.cmd
	monitorDone := sc.done
	go func() {
		_ = monitorCmd.Wait()
		// ProcessState 经锁内快照传递：cmd.ProcessState 由 Wait 写（无锁），
		// 外侧 ProcessState() 直读该字段构成数据竞争——快照后双锁同步。
		sc.mu.Lock()
		sc.cmdProcessState = monitorCmd.ProcessState
		close(monitorDone)
		sc.exited = true
		client := sc.client
		sc.mu.Unlock()
		if client != nil {
			client.close()
		}
		slog.Warn("stdio_client.process_exited", "will_reconnect_on_next_call", true)
	}()

	return nil
}

// restart relaunches eos-core after the previous process exited. Safe to call
// concurrently; only the first caller performs the restart.
func (sc *StdioClient) restart(ctx context.Context) error {
	sc.restartMu.Lock()
	defer sc.restartMu.Unlock()
	// Re-check under restartMu: another caller may have already restarted.
	sc.mu.Lock()
	if sc.cmd != nil && !sc.exited {
		sc.mu.Unlock()
		return nil
	}
	// Clean up the dead process state.
	if sc.client != nil {
		sc.client.close()
	}
	sc.client = nil
	sc.stream = nil
	sc.cmd = nil
	sc.exited = false
	sc.done = make(chan struct{})
	err := sc.startProcessLocked(ctx)
	sc.mu.Unlock()
	if err != nil {
		slog.Warn("stdio_client.reconnect_failed", "error", err)
	}
	return err
}

// ensureStarted reconnects eos-core if the previous process has exited.
func (sc *StdioClient) ensureStarted(ctx context.Context) error {
	sc.mu.Lock()
	exited := sc.exited
	sc.mu.Unlock()
	if !exited {
		return nil
	}
	return sc.restart(ctx)
}

func (sc *StdioClient) resolveCoreBinary() (StdioResolvedBinary, error) {
	opts := sidecar.ResolveOptions{
		BinaryPath:          firstNonEmpty(sc.opts.CorePath, sc.opts.AppServerPath),
		ManifestPath:        sc.opts.ManifestPath,
		RootDir:             sc.opts.CoreBinDir,
		VerifyChecksum:      sc.opts.VerifyChecksum,
		RequireSignature:    sc.opts.RequireSignature,
		AllowDevPlaceholder: sc.opts.AllowDevPlaceholder,
		PublicKeyPath:       sc.opts.PublicKeyPath,
	}
	resolved, err := sidecar.ResolveBinary(opts)
	if err != nil {
		return StdioResolvedBinary{}, fmt.Errorf("resolve eos-core sidecar: %w", err)
	}
	return StdioResolvedBinary{
		Path:         resolved.Path,
		ManifestPath: resolved.ManifestPath,
		Source:       resolved.Source,
		Target:       resolved.Target,
	}, nil
}

func (sc *StdioClient) derivedEnv() map[string]string {
	env := map[string]string{}
	// Production GUI should not inherit a fake provider selection from the parent environment.
	env["EOS_MODEL_PROVIDER"] = ""
	workspace := strings.TrimSpace(sc.opts.Workspace)
	if workspace != "" {
		env["EOS_WORKSPACE_ROOT"] = workspace
		env["EOS_SANDBOX_WORKSPACE_ROOT"] = workspace
	}
	sandboxMode := strings.TrimSpace(sc.opts.SandboxMode)
	if sandboxMode == "" {
		// 没有 workspace 时不能默认 workspace-write：内核会因
		// "workspace-write requires at least one writable root" fail-fast
		// 拒绝启动（无 workspace = 无可写根 = 自相矛盾配置）。生产路径
		// （bridge 层）总会传 workspace 回退，这里只兜直接构造 StdioClient
		// 且不带 workspace 的场景（测试、CLI），降级为安全的 read-only。
		if workspace == "" {
			sandboxMode = "read-only"
		} else {
			sandboxMode = "workspace"
		}
	}
	env["EOS_SANDBOX_MODE"] = sandboxMode
	// workspace 模式下放行网络：用户已授权工作区内的文件写入和命令执行，
	// npm/pip/git/curl 等正常命令需要网络访问。read-only/danger 模式不受影响
	// （danger 本身全放行，read-only 不会执行命令）。
	if sandboxMode == "workspace" || sandboxMode == "workspace_write" {
		env["EOS_SANDBOX_ALLOW_NETWORK"] = "true"
	}
	if storeDir := strings.TrimSpace(sc.opts.StoreDir); storeDir != "" {
		env["EOS_CORE_STORE_DIR"] = storeDir
	} else if workspace != "" {
		env["EOS_CORE_STORE_DIR"] = filepath.Join(workspace, ".eos", "core")
	}
	for k, v := range sc.opts.Env {
		k = strings.TrimSpace(k)
		if k != "" {
			env[k] = v
		}
	}
	return env
}

func mergeStdioEnv(base []string, overrides map[string]string) []string {
	out := append([]string(nil), base...)
	if len(overrides) == 0 {
		return out
	}
	index := map[string]int{}
	for i, item := range out {
		if key, _, ok := strings.Cut(item, "="); ok {
			index[strings.ToUpper(key)] = i
		}
	}
	for key, value := range overrides {
		env := key + "=" + value
		upper := strings.ToUpper(key)
		if i, ok := index[upper]; ok {
			out[i] = env
		} else {
			index[upper] = len(out)
			out = append(out, env)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// Call sends a JSON-RPC request and decodes the response into out.
func (sc *StdioClient) Call(ctx context.Context, method string, params any, out any) error {
	if err := sc.ensureStarted(ctx); err != nil {
		return err
	}
	sc.mu.Lock()
	client := sc.client
	sc.mu.Unlock()
	if client == nil {
		return errors.New("stdio client not started")
	}
	return client.Call(ctx, method, params, out)
}

func (sc *StdioClient) SetNotificationHandler(handler protocoljsonrpc.NotificationHandler) {
	sc.mu.Lock()
	client := sc.client
	sc.mu.Unlock()
	if client != nil {
		client.SetNotificationHandler(handler)
	}
}

// ResolvedBinary reports which eos-core artifact was selected for this client.
func (sc *StdioClient) ResolvedBinary() StdioResolvedBinary {
	if sc == nil {
		return StdioResolvedBinary{}
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.resolvedBinary
}

// Close terminates the app-server process and cleans up resources.
func (sc *StdioClient) Close() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.stream != nil {
		_ = sc.stream.Close()
		sc.stream = nil
	}
	if sc.client != nil {
		sc.client.close()
		sc.client = nil
	}
	if sc.cmd != nil && sc.cmd.Process != nil {
		done := sc.done
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			_ = sc.cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
			}
		}
		sc.cmd = nil
	}
	return nil
}

// Done returns a channel that closes when the app-server process exits.
func (sc *StdioClient) Done() <-chan struct{} {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.done
}

// ProcessState returns the underlying process state (nil if not exited).
func (sc *StdioClient) ProcessState() *stdioProcessState {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cmdProcessState == nil {
		return nil
	}
	return &stdioProcessState{ps: sc.cmdProcessState}
}

// stdioProcessState is a thin wrapper around os.ProcessState.
type stdioProcessState struct {
	ps *os.ProcessState
}

func (s *stdioProcessState) ExitCode() int {
	if s == nil || s.ps == nil {
		return -1
	}
	return s.ps.ExitCode()
}

func (s *stdioProcessState) Success() bool {
	if s == nil || s.ps == nil {
		return false
	}
	return s.ps.Success()
}

// stdioRPCClient is the internal JSON-RPC client wrapping the stream transport.
type stdioRPCClient struct {
	stream              *streamAdapter
	sc                  *protocoljsonrpc.StreamClient
	notificationHandler protocoljsonrpc.NotificationHandler
	started             bool
	mu                  sync.Mutex
}

func newStdioRPCClient(sa *streamAdapter) *stdioRPCClient {
	return &stdioRPCClient{stream: sa}
}

func (rc *stdioRPCClient) Start(ctx context.Context) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.started {
		return nil
	}

	stream := protocoljsonrpc.NewStream(rc.stream, rc.stream)
	rc.sc = protocoljsonrpc.NewStreamClient(stream, protocoljsonrpc.WithNotificationHandler(rc.handleNotification))
	if err := rc.sc.Start(ctx); err != nil {
		return err
	}
	rc.started = true
	return nil
}

func (rc *stdioRPCClient) SetNotificationHandler(handler protocoljsonrpc.NotificationHandler) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.notificationHandler = handler
}

func (rc *stdioRPCClient) handleNotification(ctx context.Context, notification protocoljsonrpc.Notification) error {
	rc.mu.Lock()
	handler := rc.notificationHandler
	rc.mu.Unlock()
	if handler == nil {
		return nil
	}
	return handler(ctx, notification)
}

// defaultCoreCallTimeout 是请求-响应级内核 RPC 的兜底超时，仅在调用方未显式设置
// deadline 时生效。内核走 stdio 本地回环，健康状态下所有请求-响应都在秒级内返回；
// 历史上 coreCtx() 返回无 deadline 的 context.Background()，一旦内核请求排队卡死，
// 调用方会永久 park 在等待响应上（表现为 UI 无响应且无任何错误反馈）。流式事件
// 走 notification 通道（runtime_rpc_events.go），不经过 Call，不受此超时影响。
const defaultCoreCallTimeout = 30 * time.Second

func (rc *stdioRPCClient) Call(ctx context.Context, method string, params any, out any) error {
	rc.mu.Lock()
	sc := rc.sc
	rc.mu.Unlock()
	if sc == nil {
		return errors.New("stdio rpc client not started")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCoreCallTimeout)
		defer cancel()
	}
	return sc.Call(ctx, method, params, out)
}

func (rc *stdioRPCClient) close() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.sc != nil {
		_ = rc.sc.Close()
		rc.sc = nil
	}
}
