package cli

// internal/cli/bridge.go — eos bridge 子命令组。
// 当前实现 manifest 子命令：输出描述 EOS 接入信息的 JSON 清单，供 IDE /
// 平台侧宿主自动发现如何启动并连接 EOS。schema 见 internal/docs/serve/IDE_BRIDGE.md。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"

	"github.com/spf13/cobra"
)

func newBridgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bridge",
		Short: "IDE / host integration helpers.",
		Long:  "Generate bridge artifacts so IDEs and host platforms can auto-discover how to start and connect to EOS.",
	}
	cmd.AddCommand(newBridgeManifestCmd())
	return cmd
}

func newBridgeManifestCmd() *cobra.Command {
	var (
		workspace    string
		accessMode   string
		approvalMode string
		transport    string
	)
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Emit a JSON manifest describing how to connect to EOS.",
		Long: "Print a JSON manifest to stdout with the launch command, protocol version,\n" +
			"supported methods, capabilities, and default session parameters. Hosts consume\n" +
			"this to auto-discover EOS integration. Schema: internal/docs/serve/IDE_BRIDGE.md.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBridgeManifest(cmd.Context(), bridgeManifestOptions{
				Workspace:    strings.TrimSpace(workspace),
				AccessMode:   strings.TrimSpace(accessMode),
				ApprovalMode: strings.TrimSpace(approvalMode),
				Transport:    strings.TrimSpace(transport),
			})
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root path (default: current directory)")
	cmd.Flags().StringVar(&accessMode, "access-mode", "", "Default access mode recorded in the manifest")
	cmd.Flags().StringVar(&approvalMode, "approval-mode", "", "Default approval mode recorded in the manifest")
	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport recorded in the manifest (stdio)")
	return cmd
}

type bridgeManifestOptions struct {
	Workspace    string
	AccessMode   string
	ApprovalMode string
	Transport    string
}

// BridgeManifest 是 eos bridge manifest 的 JSON 输出结构。
// 字段语义见 internal/docs/serve/IDE_BRIDGE.md。
type BridgeManifest struct {
	Command         string            `json:"command"`
	Transport       string            `json:"transport"`
	ProtocolVersion string            `json:"protocol_version"`
	ServerName      string            `json:"server_name"`
	Methods         []string          `json:"methods"`
	Capabilities    map[string]any    `json:"capabilities,omitempty"`
	Defaults        map[string]string `json:"defaults"`
}

func runBridgeManifest(ctx context.Context, opts bridgeManifestOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// 启动一次 sidecar 拿内核 initialize 结果（protocol_version / capabilities /
	// methods），与内核实际能力保持一致，不靠壳层硬编码。
	selected, err := startServeEngine(ctx, serveOptionEnv(serveOptions{
		Workspace:    opts.Workspace,
		AccessMode:   opts.AccessMode,
		ApprovalMode: opts.ApprovalMode,
	}))
	if err != nil {
		return err
	}
	defer selected.Close()

	manifest := buildManifest(selected.Initialize, opts)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(manifest)
}

// buildManifest 是纯函数：用内核 initialize 结果 + 选项装配 manifest。
// methods 优先用内核返回的（与内核实际能力一致），降级到 AllCoreMethods()。
// command 按 transport + workspace 拼装成可直接启动的命令行。
func buildManifest(init coreapijsonrpc.InitializeResult, opts bridgeManifestOptions) BridgeManifest {
	transport := strings.TrimSpace(opts.Transport)
	if transport == "" {
		transport = "stdio"
	}
	methods := init.Methods
	if len(methods) == 0 {
		methods = jsonrpc.AllCoreMethods()
	}
	defaults := map[string]string{}
	if w := strings.TrimSpace(opts.Workspace); w != "" {
		defaults["workspace"] = w
	}
	if v := strings.TrimSpace(opts.AccessMode); v != "" {
		defaults["access_mode"] = v
	}
	if v := strings.TrimSpace(opts.ApprovalMode); v != "" {
		defaults["approval_mode"] = v
	}
	return BridgeManifest{
		Command:         buildBridgeCommand(transport, opts.Workspace),
		Transport:       transport,
		ProtocolVersion: init.ProtocolVersion,
		ServerName:      firstNonEmptyBridge(init.ServerName, "eos-core"),
		Methods:         methods,
		Capabilities:    init.Capabilities,
		Defaults:        defaults,
	}
}

// buildBridgeCommand 拼装宿主可直接执行的启动命令。
func buildBridgeCommand(transport, workspace string) string {
	parts := []string{"eos", "serve", "--transport", transport}
	if w := strings.TrimSpace(workspace); w != "" {
		parts = append(parts, "--workspace", fmt.Sprintf("%q", w))
	}
	return strings.Join(parts, " ")
}

func firstNonEmptyBridge(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
