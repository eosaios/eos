package cli

// internal/cli/web.go — eos web 子命令：把桌面前端跑在浏览器里。
// 复用 internal/webbridge（自 eos-app 桥接层移植）：本地 HTTP 服务静态
// 前端 + BridgeService 反射分发 + WS 事件流，内核 sidecar 由桥自启。

import (
	"errors"
	"strings"

	"github.com/eosaios/eos/internal/webbridge"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"

	"github.com/spf13/cobra"
)

const defaultWebListenAddr = "127.0.0.1:8788"

func newWebCmd() *cobra.Command {
	var (
		listen    string
		workspace string
		uiDir     string
		noOpen    bool
	)
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Run the EOS desktop workbench UI in a local browser.",
		Long: "Start a local HTTP server that serves the EOS workbench UI (the desktop\n" +
			"frontend) in your browser and bridges it to the eos-core sidecar over\n" +
			"WebSocket. Only reachable from 127.0.0.1.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			corePath, coreManifest := resolveWebCoreBinary()
			return webbridge.Run(cmd.Context(), webbridge.ServerOptions{
				ListenAddr:       strings.TrimSpace(listen),
				UIDir:            strings.TrimSpace(uiDir),
				StartupWorkspace: strings.TrimSpace(workspace),
				CorePath:         corePath,
				CoreManifestPath: coreManifest,
				NoOpenBrowser:    noOpen,
			})
		},
	}
	cmd.Flags().StringVar(&listen, "listen", defaultWebListenAddr, "HTTP listen address (loopback only is enforced by default value)")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root path (default: ~/.eos/workspace, matching the desktop app)")
	cmd.Flags().StringVar(&uiDir, "ui-dir", "", "Frontend dist directory (default: EOS_WEB_UI_DIR or auto-detect eos-app frontend/dist)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Do not open the browser automatically")
	return cmd
}

// resolveWebCoreBinary 用 CLI 侧 sidecar 解析器（EOS_CORE_PATH /
// EOS_CORE_MANIFEST / EOS_CORE_BIN_DIR / 内嵌产物）定位内核，结果交给
// webbridge 启动 stdio 网关。解析失败不在此报错：交由 webbridge 的自身
// 搜索（EOS_GUI_* / exe 同级 core/）兜底，仍失败才向用户报启动错误。
func resolveWebCoreBinary() (string, string) {
	resolved, err := sidecar.ResolveBinary(sidecar.ResolveOptions{})
	if err != nil {
		if !errors.Is(err, sidecar.ErrCoreBinaryNotFound) {
			return "", ""
		}
		return "", ""
	}
	return resolved.Path, resolved.ManifestPath
}
