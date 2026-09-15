package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/ui"

	"github.com/spf13/cobra"
)

var (
	printQuery         string
	outputFormat       string
	continueChat       bool
	resumeSession      string
	cliModel           string
	cliMaxTurns        int
	cliAllowedTools    string
	cliDisallowedTools string
	cliAccessMode      string
	cliApprovalMode    string
	cliSandboxMode     string
	cliSkipPermissions bool
	cliShowVersion     bool
	// cliDefaultWorkspace --default：工作区取默认工作区（~/.eos/workspace）
	// 而非当前执行 shell 所在目录。
	cliDefaultWorkspace bool
)

// rootLang 根据环境变量 EOS_LANG 返回界面语言。
// 当前仅支持 zh/en，默认返回 zh。
func rootLang() string {
	lang := os.Getenv("EOS_LANG")
	if lang == "en" {
		return "en"
	}
	if lang == "zh" {
		return "zh"
	}
	return "zh"
}

// rootShort 返回根命令的短描述（用于 help 等场景）。
func rootShort() string {
	return ui.T("cli.root.short", rootLang())
}

// rootLong 返回根命令的长描述（用于 help 等场景）。
func rootLong() string {
	return ui.T("cli.root.long", rootLang())
}

// rootCmd 表示根命令：不带子命令时启动交互式 TUI。
var rootCmd = &cobra.Command{
	Use:   "eos",
	Short: rootShort(),
	Long:  rootLong(),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return applyDefaultWorkspaceFlag(cmd)
	},
	Run: func(cmd *cobra.Command, args []string) {
		slog.Info("cli.start", "lang", rootLang())

		if cliShowVersion {
			printVersion()
			return
		}

		// Handle --print mode
		if printQuery != "" {
			access, approval := mergeConfigPermissions(cmd.Flags(), "sandbox-mode", cliAccessMode, cliApprovalMode)
			modes := resolveModeConfig(access, approval, cliSandboxMode, cliSkipPermissions)
			if err := RunPrintMode(PrintOptions{
				Query:           printQuery,
				OutputFormat:    outputFormat,
				AccessMode:      modes.AccessMode,
				ApprovalMode:    modes.ApprovalMode,
				SandboxMode:     modes.SandboxMode,
				SkipPermissions: modes.SkipAllChecks,
				ModelOverride:   cliModel,
			}); err != nil {
				os.Exit(1)
			}
			return
		}

		// Build TUI options from CLI flags. 权限模式先经 resolveModeConfig 归一
		// （--access-mode 与 --sandbox-mode 共用内核 kebab-case 词表，显式
		// --access-mode 优先）——TUI 路径此前透传原始值且 EOS_ACCESS_MODE 无
		// 内核消费者，导致 --access-mode 在 TUI 下不生效。
		resolved := func() resolvedModeConfig {
			access, approval := mergeConfigPermissions(cmd.Flags(), "sandbox-mode", cliAccessMode, cliApprovalMode)
			return resolveModeConfig(access, approval, cliSandboxMode, cliSkipPermissions)
		}()
		opts := ui.TUIOptions{
			ModelOverride:   cliModel,
			MaxTurns:        cliMaxTurns,
			AccessMode:      resolved.AccessMode,
			ApprovalMode:    resolved.ApprovalMode,
			SandboxMode:     resolved.SandboxMode,
			SkipPermissions: resolved.SkipAllChecks,
		}
		if continueChat {
			opts.SessionID = "latest"
		}
		if resumeSession != "" {
			opts.SessionID = resumeSession
		}
		if cliAllowedTools != "" {
			for _, t := range strings.Split(cliAllowedTools, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					opts.AllowedTools = append(opts.AllowedTools, t)
				}
			}
		}
		if cliDisallowedTools != "" {
			for _, t := range strings.Split(cliDisallowedTools, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					opts.DisallowedTools = append(opts.DisallowedTools, t)
				}
			}
		}

		ui.StartInteractiveTUIWithOptions(opts)
	},
}

// Execute 执行根命令入口（由 main 调用）。
func Execute() error {
	slog.Info("cli.execute")
	return rootCmd.Execute()
}

// applyDefaultWorkspaceFlag 是 --default 的统一落点：把进程工作目录切换到
// 默认工作区（~/.eos/workspace，与内核 conversation_default_workspace 同一
// 事实源）。所有以 cwd 为工作区来源的路径——TUI 启动（rememberKnownWorkspace /
// 工作区信任 / SetForeground）、print / exec / serve / mcp 的
// EOS_WORKSPACE_ROOT env 注入与会话根解析——因此一致落到默认工作区，
// 效果等同用户 cd 到默认工作区后再启动 eos。
//
// --default 与显式 --workspace 同时给出是自相矛盾的配置，fail-fast 拒绝
// （AGENTS.md 原则 1），不做静默优先级兜底。
func applyDefaultWorkspaceFlag(cmd *cobra.Command) error {
	if !cliDefaultWorkspace {
		return nil
	}
	if f := cmd.Flags().Lookup("workspace"); f != nil && strings.TrimSpace(f.Value.String()) != "" {
		return fmt.Errorf("--default cannot be combined with --workspace")
	}
	if err := config.EnsureDefaultWorkspaceDir(); err != nil {
		return fmt.Errorf("--default: create default workspace: %w", err)
	}
	if err := os.Chdir(config.DefaultWorkspacePath()); err != nil {
		return fmt.Errorf("--default: switch to default workspace: %w", err)
	}
	return nil
}

func init() {
	// 全局 flags（配置读写统一走 internal/config 的 ~/.eos.json，
	// 不设 cobra/viper 配置线——历史上 viper 读的 ~/.eos.yaml 无任何消费方）。
	rootCmd.PersistentFlags().StringVarP(&printQuery, "print", "p", "", "Run a single query in headless mode and print the result")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output-format", "text", "Output format for print mode: text, json, stream-json")
	rootCmd.PersistentFlags().BoolVarP(&continueChat, "continue", "c", false, "Continue the most recent conversation")
	rootCmd.PersistentFlags().StringVar(&resumeSession, "resume", "", "Resume a specific conversation by session ID")
	rootCmd.PersistentFlags().StringVar(&cliModel, "model", "", "Override the model for this session")
	rootCmd.PersistentFlags().IntVar(&cliMaxTurns, "max-turns", 0, "Maximum number of turns (0=unlimited)")
	rootCmd.PersistentFlags().StringVar(&cliAllowedTools, "allowed-tools", "", "Comma-separated list of allowed tools")
	rootCmd.PersistentFlags().StringVar(&cliDisallowedTools, "disallowed-tools", "", "Comma-separated list of disallowed tools")
	rootCmd.PersistentFlags().StringVar(&cliAccessMode, "access-mode", "", "Sandbox access mode: read-only, workspace-write, or danger-full-access")
	rootCmd.PersistentFlags().StringVar(&cliApprovalMode, "approval-mode", "", "Approval mode: untrusted, on-request, or never (on-failure is accepted as an alias of on-request)")
	rootCmd.PersistentFlags().StringVar(&cliSandboxMode, "sandbox-mode", "workspace", "Alias of --access-mode (workspace=workspace-write, full_access=danger-full-access)")
	rootCmd.PersistentFlags().BoolVar(&cliSkipPermissions, "dangerously-skip-permissions", false, "Full-access preset: --access-mode danger-full-access --approval-mode never")
	rootCmd.PersistentFlags().BoolVar(&cliDefaultWorkspace, "default", false, "Use the default workspace (~/.eos/workspace) instead of the current directory")
	rootCmd.Flags().BoolVar(&cliShowVersion, "version", false, "Print the EOS version and exit")

	rootCmd.AddCommand(newDocumentCmd())
	rootCmd.AddCommand(newExecCmd())
	rootCmd.AddCommand(newUpdateCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newBridgeCmd())
	rootCmd.AddCommand(newMcpCmd())
	rootCmd.AddCommand(newWebCmd())
	rootCmd.AddCommand(newHiddenLegalCmd())
}
