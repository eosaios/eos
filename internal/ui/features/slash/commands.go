package slash

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "strings"

type Group string

const (
	GroupGeneral Group = "general"
	GroupProject Group = "project"
	GroupRuntime Group = "runtime"
	GroupConfig  Group = "config"
)

// Command 描述一个 slash command 的元数据。
type Command struct {
	Name          string
	Aliases       []string
	Group         Group
	DescriptionZH string
	DescriptionEN string
	Usage         string
	Hidden        bool
}

type GroupedCommands struct {
	Group    Group
	Label    string
	Commands []Command
}

var Commands = []Command{
	{Name: "/help", Group: GroupGeneral, DescriptionZH: "查看命令帮助与快捷键", DescriptionEN: "Show command help and shortcuts"},
	{Name: "/init", Group: GroupGeneral, DescriptionZH: "生成仓库级 EOS.md 指南文件", DescriptionEN: "Generate a repository EOS.md guide"},
	{Name: "/init-verifiers", Group: GroupProject, DescriptionZH: "自动检测项目类型并建议验证工具", DescriptionEN: "Detect project type and suggest verifier tools"},
	{Name: "/clear", Group: GroupGeneral, DescriptionZH: "清空当前会话内容区域", DescriptionEN: "Clear the current conversation view"},
	{Name: "/exit", Group: GroupGeneral, DescriptionZH: "退出应用", DescriptionEN: "Exit the application"},
	{Name: "/lang", Group: GroupGeneral, Usage: "/lang zh|en", DescriptionZH: "切换界面语言", DescriptionEN: "Switch the UI language"},
	{Name: "/status", Group: GroupGeneral, DescriptionZH: "显示当前模型、模式、工作区和上下文用量", DescriptionEN: "Show current model, mode, workspace, and context usage"},
	{Name: "/fast", Group: GroupGeneral, DescriptionZH: "切换快速模型模式", DescriptionEN: "Toggle fast model mode"},

	{Name: "/workspace", Aliases: []string{"/worktree"}, Group: GroupProject, Usage: "/workspace [list|add|remove|use <path>]", DescriptionZH: "管理工作区与切换活跃根目录", DescriptionEN: "Manage workspaces and switch the active root"},
	{Name: "/session", Aliases: []string{"/sessions"}, Group: GroupProject, Usage: "/session [list|save|export <id> [path]]", DescriptionZH: "查看、保存或导出会话", DescriptionEN: "List, save, or export sessions"},
	{Name: "/resume", Group: GroupProject, Usage: "/resume [id]", DescriptionZH: "恢复最近或指定会话", DescriptionEN: "Resume the latest or a specific session"},
	{Name: "/history", Aliases: []string{"/versions"}, Group: GroupProject, DescriptionZH: "打开版本历史面板", DescriptionEN: "Open the version history panel"},
	{Name: "/diff", Group: GroupProject, Usage: "/diff [path]", DescriptionZH: "查看待提交 diff 或指定文件差异", DescriptionEN: "Show the pending diff or a file diff"},
	{Name: "/review", Group: GroupProject, Usage: "/review [path]", DescriptionZH: "汇总改动、诊断和审查入口", DescriptionEN: "Summarize changes, diagnostics, and review entry points"},
	{Name: "/verify", Group: GroupProject, Usage: "/verify [目标说明]", DescriptionZH: "发起一次标准化验证请求", DescriptionEN: "Start a standardized verification request"},
	{Name: "/git", Group: GroupProject, Usage: "/git status|branches|log|show|diff [args...]", DescriptionZH: "查看 Git 状态、分支、日志和差异", DescriptionEN: "Inspect Git status, branches, history, and diff"},
	{Name: "/commit", Group: GroupProject, DescriptionZH: "让 AI 检查当前改动并按仓库规范提交推送", DescriptionEN: "Ask AI to inspect changes, then commit and push per repo conventions"},
	{Name: "/remote", Group: GroupProject, Usage: "/remote [status]", DescriptionZH: "查看当前远程仓库上下文与本地隔离目录", DescriptionEN: "Show the current remote repository context and local sandbox path"},
	{Name: "/compact", Group: GroupProject, DescriptionZH: "压缩当前上下文", DescriptionEN: "Compact the current context"},
	{Name: "/export", Group: GroupProject, Usage: "/export [markdown|json] [path]", DescriptionZH: "导出当前会话为 Markdown 或 JSON", DescriptionEN: "Export current session as Markdown or JSON"},
	{Name: "/rename", Group: GroupProject, Usage: "/rename <title>", DescriptionZH: "重命名当前会话", DescriptionEN: "Rename the current session"},
	{Name: "/share", Group: GroupProject, DescriptionZH: "分享会话到剪贴板或文件", DescriptionEN: "Share session to clipboard or file"},

	{Name: "/memory", Group: GroupRuntime, DescriptionZH: "打开记忆面板", DescriptionEN: "Open the memory panel"},
	{Name: "/context", Aliases: []string{"/ctx"}, Group: GroupRuntime, DescriptionZH: "打开上下文面板", DescriptionEN: "Open the context panel"},
	{Name: "/tasks", Group: GroupRuntime, DescriptionZH: "打开任务面板", DescriptionEN: "Open the tasks panel"},
	{Name: "/goal", Group: GroupRuntime, Usage: "/goal [set <目标> [预算tokens]|get|pause|resume|clear]", DescriptionZH: "目标模式：设定目标后 agent 持续工作直到完成", DescriptionEN: "Goal mode: agent keeps working toward the goal until complete"},
	{Name: "/plan", Group: GroupRuntime, Usage: "/plan [auto|plan]", DescriptionZH: "查看当前计划/待办，或切换计划执行模式", DescriptionEN: "Inspect plan/todos or switch planning mode"},
	{Name: "/plan-style", Group: GroupRuntime, Usage: "/plan-style [concise|detailed|custom:<text>]", DescriptionZH: "查看或设置计划提示风格", DescriptionEN: "Inspect or change the planner prompt style"},
	{Name: "/permissions", Group: GroupRuntime, Usage: "/permissions [auto|plan|access <mode>|approval <mode>]", DescriptionZH: "查看或切换执行/访问/审批模式", DescriptionEN: "Inspect or change execution, access, and approval modes"},
	{Name: "/doctor", Group: GroupRuntime, DescriptionZH: "输出运行、工具和诊断摘要", DescriptionEN: "Print an execution, tools, and diagnostics summary", Hidden: true},
	{Name: "/stats", Group: GroupRuntime, DescriptionZH: "显示 Token 用量和工具调用统计", DescriptionEN: "Show token usage and tool call statistics", Hidden: true},

	{Name: "/model", Aliases: []string{"/models"}, Group: GroupConfig, Usage: "/model [use <name>]", DescriptionZH: "查看模型面板或切换当前模型", DescriptionEN: "Open model management or switch the active model"},
	{Name: "/config", Aliases: []string{"/settings"}, Group: GroupConfig, DescriptionZH: "打开配置面板", DescriptionEN: "Open the settings panel"},
	{Name: "/mcp", Group: GroupConfig, DescriptionZH: "打开 MCP 面板", DescriptionEN: "Open the MCP panel"},
	{Name: "/lsp", Group: GroupConfig, DescriptionZH: "打开 LSP 状态面板", DescriptionEN: "Open the LSP status panel"},
	{Name: "/rules", Group: GroupConfig, DescriptionZH: "打开规则面板", DescriptionEN: "Open the rules panel"},
	{Name: "/skills", Group: GroupConfig, Usage: "/skills [reload]", DescriptionZH: "列出或重载可用 skills", DescriptionEN: "List or reload available skills"},
	{Name: "/plugin", Group: GroupConfig, DescriptionZH: "列出已注册插件", DescriptionEN: "List registered plugins"},
	{Name: "/reload-plugins", Group: GroupConfig, DescriptionZH: "重载插件扩展与目录发现", DescriptionEN: "Reload plugin extensions and discovery", Hidden: true},
	{Name: "/cost", Group: GroupConfig, DescriptionZH: "打开成本统计面板", DescriptionEN: "Open the cost panel"},
	{Name: "/theme", Group: GroupConfig, Usage: "/theme [dark|light|nord|...]", DescriptionZH: "切换 TUI 配色主题", DescriptionEN: "Switch TUI color theme"},
}

func ParseCommand(input string) (cmd string, args []string, isCmd bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", nil, false
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil, false
	}

	cmd = parts[0]
	if len(parts) > 1 {
		args = parts[1:]
	}

	return cmd, args, true
}

func (c Command) DisplayText() string {
	if strings.TrimSpace(c.Usage) != "" {
		return c.Usage
	}
	return c.Name
}

func (c Command) Description(lang string) string {
	if strings.EqualFold(strings.TrimSpace(lang), "en") {
		return c.DescriptionEN
	}
	return c.DescriptionZH
}

func (g Group) Label(lang string) string {
	switch g {
	case GroupProject:
		if strings.EqualFold(lang, "en") {
			return "Project Workflow"
		}
		return "工程流程"
	case GroupRuntime:
		if strings.EqualFold(lang, "en") {
			return "Work Status"
		}
		return "工作状态"
	case GroupConfig:
		if strings.EqualFold(lang, "en") {
			return "Settings and Connections"
		}
		return "设置与连接"
	default:
		if strings.EqualFold(lang, "en") {
			return "General"
		}
		return "通用"
	}
}

func VisibleCommands() []Command {
	out := make([]Command, 0, len(Commands))
	for _, cmd := range Commands {
		if cmd.Hidden {
			continue
		}
		out = append(out, cmd)
	}
	return out
}

func FindCommand(name string) *Command {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	for i := range Commands {
		cmd := &Commands[i]
		if strings.EqualFold(cmd.Name, name) {
			return cmd
		}
		for _, alias := range cmd.Aliases {
			if strings.EqualFold(alias, name) {
				return cmd
			}
		}
	}
	return nil
}

func NormalizeCommand(name string) string {
	if cmd := FindCommand(name); cmd != nil {
		return cmd.Name
	}
	return ""
}

func GetSuggestions(prefix string) []Command {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return VisibleCommands()
	}

	visible := VisibleCommands()
	out := make([]Command, 0, len(visible))
	for _, cmd := range visible {
		if commandMatches(cmd, prefix) {
			out = append(out, cmd)
		}
	}
	return out
}

func GroupedVisibleCommands(lang string) []GroupedCommands {
	order := []Group{GroupGeneral, GroupProject, GroupRuntime, GroupConfig}
	byGroup := map[Group][]Command{}
	for _, cmd := range VisibleCommands() {
		byGroup[cmd.Group] = append(byGroup[cmd.Group], cmd)
	}

	out := make([]GroupedCommands, 0, len(order))
	for _, group := range order {
		items := byGroup[group]
		if len(items) == 0 {
			continue
		}
		out = append(out, GroupedCommands{
			Group:    group,
			Label:    group.Label(lang),
			Commands: items,
		})
	}
	return out
}

func commandMatches(cmd Command, prefix string) bool {
	if strings.HasPrefix(strings.ToLower(cmd.Name), prefix) || strings.Contains(strings.ToLower(cmd.Name), prefix) {
		return true
	}
	for _, alias := range cmd.Aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if strings.HasPrefix(alias, prefix) || strings.Contains(alias, prefix) {
			return true
		}
	}
	return false
}
