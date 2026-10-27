package cli

// internal/cli/mcp.go — eos mcp 子命令组。
// 当前实现 serve 子命令：把 EOS 作为标准 MCP Server 暴露（stdio / sse）。
// MVP 能力（tools/list + tools/call）见 internal/docs/mcp/SERVER.md。

import (
	"context"
	"fmt"
	"os"
	"strings"

	mcpserver "github.com/eosaios/eos/internal/mcp/server"
	"github.com/eosaios/eos/pkg/coreapi/engineprovider"

	"github.com/spf13/cobra"
)

func newMcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run EOS as a standard MCP server, or manage MCP integrations.",
		Long: "EOS as Model Context Protocol server. `eos mcp serve` exposes EOS tools\n" +
			"to external agents/hosts over stdio or sse. See internal/docs/mcp/SERVER.md.",
	}
	cmd.AddCommand(newMcpServeCmd())
	return cmd
}

func newMcpServeCmd() *cobra.Command {
	var (
		workspace      string
		accessMode     string
		approvalMode   string
		sandboxMode    string
		skipPermission bool
		transport      string
		listen         string
		modelOverride  string
		chatTimeout    int
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run EOS as a standard MCP server (stdio or sse).",
		Long: "Expose EOS via the Model Context Protocol: atomic tools (tools/list + tools/call),\n" +
			"agent delegation (eos_chat / eos_task_*), session management and the approval loop\n" +
			"(eos_approval_respond). Default session is created per connection; callers may\n" +
			"override via _meta.session_id or per-tool session_id. See internal/docs/mcp/SERVER.md.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			access, approval := mergeConfigPermissions(cmd.Flags(), "sandbox-mode", accessMode, approvalMode)
			modes := resolveModeConfig(access, approval, sandboxMode, skipPermission)
			return runMcpServe(cmd.Context(), mcpServeOptions{
				Workspace:      strings.TrimSpace(workspace),
				AccessMode:     modes.AccessMode,
				ApprovalMode:   modes.ApprovalMode,
				SandboxMode:    modes.SandboxMode,
				SkipAll:        modes.SkipAllChecks,
				Transport:      strings.TrimSpace(transport),
				Listen:         strings.TrimSpace(listen),
				ModelOverride:  strings.TrimSpace(modelOverride),
				ChatTimeoutSec: chatTimeout,
			})
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root path (default: current directory)")
	cmd.Flags().StringVar(&accessMode, "access-mode", "", "Sandbox access mode: read-only, workspace-write, or danger-full-access")
	cmd.Flags().StringVar(&approvalMode, "approval-mode", "", "Approval mode: untrusted, on-request, or never (on-failure is accepted as an alias of on-request)")
	cmd.Flags().StringVar(&sandboxMode, "sandbox-mode", "workspace", "Alias of --access-mode (workspace=workspace-write, full_access=danger-full-access)")
	cmd.Flags().BoolVar(&skipPermission, "dangerously-skip-permissions", false, "Full-access preset: --access-mode danger-full-access --approval-mode never")
	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport: stdio or sse")
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8765", "SSE listen address (only used when --transport sse)")
	cmd.Flags().StringVar(&modelOverride, "model", "", "Model override (entry name / model ID / plan label) applied to each session's first eos_chat")
	cmd.Flags().IntVar(&chatTimeout, "chat-timeout", 600, "Default eos_chat / eos_task_wait blocking timeout in seconds (per-call timeout_secs overrides)")
	return cmd
}

type mcpServeOptions struct {
	Workspace      string
	AccessMode     string
	ApprovalMode   string
	SandboxMode    string
	SkipAll        bool
	Transport      string
	Listen         string
	ModelOverride  string
	ChatTimeoutSec int
}

func runMcpServe(ctx context.Context, opts mcpServeOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	transport := strings.ToLower(strings.TrimSpace(opts.Transport))
	if transport == "" {
		transport = "stdio"
	}
	if transport != "stdio" && transport != "sse" {
		return fmt.Errorf("unsupported transport %q: only stdio and sse are supported", opts.Transport)
	}

	selected, err := startMcpEngine(ctx, mcpOptionEnv(opts))
	if err != nil {
		return err
	}
	defer selected.Close()

	mcpSrv, _, err := mcpserver.New(ctx, mcpserver.Options{
		Engine:          selected.Engine,
		WorkspaceRoot:   opts.Workspace,
		ChatTimeoutSecs: opts.ChatTimeoutSec,
		ModelOverride:   opts.ModelOverride,
	})
	if err != nil {
		return fmt.Errorf("mcp serve: %w", err)
	}

	switch transport {
	case "stdio":
		return mcpserver.ServeStdio(ctx, mcpSrv)
	case "sse":
		addr := strings.TrimSpace(opts.Listen)
		if addr == "" {
			addr = "127.0.0.1:8765"
		}
		fmt.Fprintf(os.Stderr, "eos mcp serve (sse) listening on %s\n", addr)
		return mcpserver.ServeSSE(ctx, mcpSrv, addr)
	}
	return fmt.Errorf("unsupported transport %q", opts.Transport)
}

// startMcpEngine 启动 mcp serve 用的 sidecar。信任内核全部 methods。
func startMcpEngine(ctx context.Context, env map[string]string) (engineprovider.Selection, error) {
	selection, err := engineprovider.Select(ctx, engineprovider.Options{
		Mode:            engineprovider.ModeAuto,
		Sidecar:         productionSidecarProcessOptions(env),
		RequiredMethods: nil,
	})
	if err != nil {
		return engineprovider.Selection{}, fmt.Errorf("start eos-core sidecar (mcp serve): %w", err)
	}
	return selection, nil
}

// mcpOptionEnv 把 mcp serve flags 透传到 eos-core 子进程环境变量，与 serve/exec 一致。
func mcpOptionEnv(opts mcpServeOptions) map[string]string {
	env := map[string]string{}
	// 沙箱轴只经 EOS_SANDBOX_MODE 单通道下发（内核不读 EOS_ACCESS_MODE）；
	// AccessMode/SandboxMode 已由 resolveModeConfig 归一为内核 kebab-case 规范值。
	if v := strings.TrimSpace(opts.SandboxMode); v != "" {
		env["EOS_SANDBOX_MODE"] = v
	} else if v := strings.TrimSpace(opts.AccessMode); v != "" {
		env["EOS_SANDBOX_MODE"] = v
	}
	if v := strings.TrimSpace(opts.ApprovalMode); v != "" {
		env["EOS_APPROVAL_MODE"] = v
	}
	if ws := strings.TrimSpace(opts.Workspace); ws != "" {
		env["EOS_WORKSPACE_ROOT"] = ws
		env["EOS_SANDBOX_WORKSPACE_ROOT"] = ws
	} else if cwd, err := os.Getwd(); err == nil {
		if cwd := strings.TrimSpace(cwd); cwd != "" {
			env["EOS_WORKSPACE_ROOT"] = cwd
			env["EOS_SANDBOX_WORKSPACE_ROOT"] = cwd
		}
	}
	if opts.SkipAll {
		env["EOS_SKIP_PERMISSIONS"] = "1"
		delete(env, "EOS_APPROVAL_MODE")
		delete(env, "EOS_SANDBOX_MODE")
	}
	return env
}
