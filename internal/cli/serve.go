package cli

// internal/cli/serve.go — eos serve 子命令：把 eos-core 内核能力通过 stdio
// JSON-RPC 对外暴露为本地工具服务（透传代理，裁决在内核）。
// 协议契约见 internal/docs/serve/API.md。

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/eosaios/eos/internal/serve"
	"github.com/eosaios/eos/pkg/coreapi/engineprovider"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"

	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var (
		workspace      string
		accessMode     string
		approvalMode   string
		sandboxMode    string
		skipPermission bool
		transport      string
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run EOS as a local tool service (stdio JSON-RPC).",
		Long: "Start EOS as a local tool service exposing the eos-core JSON-RPC API over stdio.\n" +
			"All core methods are transparently forwarded to the eos-core sidecar;\n" +
			"all authorization and sandbox decisions are made in the core. See\n" +
			"internal/docs/serve/API.md for the wire contract.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			access, approval := mergeConfigPermissions(cmd.Flags(), "sandbox-mode", accessMode, approvalMode)
			modes := resolveModeConfig(access, approval, sandboxMode, skipPermission)
			return runServe(cmd.Context(), serveOptions{
				Workspace:    strings.TrimSpace(workspace),
				AccessMode:   modes.AccessMode,
				ApprovalMode: modes.ApprovalMode,
				SandboxMode:  modes.SandboxMode,
				SkipAll:      modes.SkipAllChecks,
				Transport:    strings.TrimSpace(transport),
			})
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root path (default: current directory)")
	cmd.Flags().StringVar(&accessMode, "access-mode", "", "Sandbox access mode: read-only, workspace-write, or danger-full-access")
	cmd.Flags().StringVar(&approvalMode, "approval-mode", "", "Approval mode: untrusted, on-request, or never (on-failure is accepted as an alias of on-request)")
	cmd.Flags().StringVar(&sandboxMode, "sandbox-mode", "workspace", "Alias of --access-mode (workspace=workspace-write, full_access=danger-full-access)")
	cmd.Flags().BoolVar(&skipPermission, "dangerously-skip-permissions", false, "Full-access preset: --access-mode danger-full-access --approval-mode never")
	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport: stdio (only stdio is currently supported)")
	return cmd
}

type serveOptions struct {
	Workspace    string
	AccessMode   string
	ApprovalMode string
	SandboxMode  string
	SkipAll      bool
	Transport    string
}

func runServe(ctx context.Context, opts serveOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !strings.EqualFold(strings.TrimSpace(opts.Transport), "stdio") {
		return fmt.Errorf("unsupported transport %q: only stdio is currently supported", opts.Transport)
	}

	selected, err := startServeEngine(ctx, serveOptionEnv(opts))
	if err != nil {
		return err
	}
	defer selected.Close()

	// Engine 实际是 *sidecar.RemoteEngine，取其 ProcessClient 作为透传 Caller。
	// serve 层只依赖 serve.Caller 接口，不耦合 sidecar 具体类型；类型断言只在 CLI 层。
	remote, ok := selected.Engine.(*sidecar.RemoteEngine)
	if !ok || remote == nil {
		return fmt.Errorf("serve: engine is not a *sidecar.RemoteEngine (got %T)", selected.Engine)
	}
	caller := remote.ProcessClient()
	if caller == nil {
		return fmt.Errorf("serve: sidecar process client unavailable")
	}

	router, err := serve.NewRouter(serve.Options{
		Caller:     caller,
		InitResult: selected.Initialize,
	})
	if err != nil {
		return fmt.Errorf("serve: build router: %w", err)
	}

	stream := jsonrpc.NewStream(os.Stdin, os.Stdout)
	return jsonrpc.ServeStream(ctx, router, stream)
}

// startServeEngine 启动 serve 用的 sidecar。RequiredMethods 留空，信任内核
// initialize 返回的全部 methods（serve 是通用透传，不限 TUI 的最小 method 集）。
func startServeEngine(ctx context.Context, env map[string]string) (engineprovider.Selection, error) {
	selection, err := engineprovider.Select(ctx, engineprovider.Options{
		Mode:            engineprovider.ModeAuto,
		Sidecar:         productionSidecarProcessOptions(env),
		RequiredMethods: nil, // 信任内核全部 methods
	})
	if err != nil {
		return engineprovider.Selection{}, fmt.Errorf("start eos-core sidecar (serve): %w", err)
	}
	return selection, nil
}

// serveOptionEnv 把 serve flags 透传到 eos-core 子进程环境变量，与 exec/print 一致。
func serveOptionEnv(opts serveOptions) map[string]string {
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
