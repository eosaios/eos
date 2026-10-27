package webbridge

import "strings"

func defaultInputSuggestions() []string {
	return []string{
		// 趣味游戏
		"用 HTML+JS 生成一个俄罗斯方块游戏，单文件可直接在浏览器运行，支持计分和键盘控制",
		"用 HTML+JS 写一个贪吃蛇小游戏，带计分、方向键控制和撞墙结束",
		"做一个 2048 小游戏，单 HTML 文件，方向键操作，支持重新开始",
		"用 HTML+Canvas 做一个打砖块游戏，单文件可运行",
		// 实用 Web 工具
		"用 React + Vite 创建一个待办事项应用，支持新增、完成、删除并用 localStorage 保存",
		"搭建一个 Markdown 在线预览器，左侧编辑右侧实时渲染，单页应用",
		"做一个番茄钟计时器网页，带开始、暂停、25 分钟倒计时和提醒",
		"做一个网页计算器，支持加减乘除、连续运算和清空",
		// 命令行/脚本
		"写一个 Python 脚本，统计指定目录下各类文件的数量和总大小",
		"写一个 Node.js 脚本，把一个文件夹里的图片批量压缩并输出到新目录",
		"用 Python 写一个命令行 JSON 美化工具，支持从管道或文件读取",
		"在当前工作区初始化一个 Go 命令行工具，支持 --help 和至少一个子命令",
		// 数据/可视化
		"用 Python + matplotlib 读取 CSV 并画出柱状图和折线图",
		"写一个 HTML 页面，用 Canvas 实时绘制一条跳动的折线图",
		"生成当前项目目录结构的树状图，输出为 Markdown 格式",
		"用 Python 写一个脚本，统计一段文本的词频并输出 Top 20",
	}
}

func defaultAutomationTemplates() []AutomationTemplateCard {
	return []AutomationTemplateCard{
		{
			ID:          "daily-check",
			Title:       "每日晨间巡检",
			Description: "每天 09:00 汇总工作区状态、会话内变更、通知中心和运行任务，生成一条可执行计划。",
			Prompt:      "先检查当前工作区，再帮我生成今天的执行计划。",
			Schedule:    "0 9 * * *",
			Preset:      true,
		},
		{
			ID:          "weekday-release-check",
			Title:       "工作日下班前检查",
			Description: "工作日 18:00 围绕会话内变更、规则、设置和测试给出一次上线前检查清单。",
			Prompt:      "帮我做一次发布前检查，重点看变更卡片、规则和设置状态。",
			Schedule:    "0 18 * * 1-5",
			Preset:      true,
		},
		{
			ID:          "weekly-report",
			Title:       "每周周报与规划",
			Description: "每周一 09:00 回顾上周进展并规划本周工作。",
			Prompt:      "回顾上周的工作进展（看会话历史、变更和任务），并帮我规划本周的重点工作。",
			Schedule:    "0 9 * * 1",
			Preset:      true,
		},
		{
			ID:          "hourly-check",
			Title:       "每小时巡检",
			Description: "每小时整点轻量同步工作区状态与待办。",
			Prompt:      "简要检查当前工作区状态，提醒我未完成的任务和待处理的通知。",
			Schedule:    "0 * * * *",
			Preset:      true,
		},
		{
			ID:          "automation-design",
			Title:       "自动化设计草案",
			Description: "基于现有技能、插件与任务，梳理一个适合当前项目的自动化方案（手动运行）。",
			Prompt:      "根据当前项目状态，帮我设计一个自动化方案。",
			Preset:      true,
		},
	}
}

func bridgeModeStatus(mode string) string {
	switch strings.TrimSpace(mode) {
	case "rust-stdio", "legacy":
		return "ready"
	default:
		return "baseline"
	}
}

func bridgeModeDetail(mode string) string {
	switch strings.TrimSpace(mode) {
	case "rust-stdio", "legacy":
		return "聊天、Bash、任务、审批、模型与扩展状态已与核心同步。"
	default:
		return "核心桥接接口未就绪。"
	}
}

func migrationBoundaries() []MigrationBoundary {
	return []MigrationBoundary{
		{
			Name:    "桌面壳层与窗口生命周期",
			Scope:   "Wails 窗口创建、自定义标题栏、窗口控制与状态回传",
			Targets: []string{"main.go", "bridge_service.go", "frontend/src/shell.tsx", "frontend/src/styles.css"},
			Notes:   []string{"Task2 已完成标题栏和窗口控制", "Task4 补齐窗口状态与导出链路"},
		},
		{
			Name:    "核心主流程迁移",
			Scope:   "线程、会话、聊天、输入、工作区、任务、会话内确认、会话内变更、Bash 与命令面板",
			Targets: []string{"bridge_service.go", "frontend/src/workbench.tsx", "internal/adapter/core.go", "internal/adapter/runtime_core.go"},
			Notes:   []string{"Task8 将核心工作流页回补到正式工作台", "通过统一状态快照保持桥接同步"},
		},
		{
			Name:    "工具与管理页面回补",
			Scope:   "Models、MCP、LSP、Skills、Plugins、Context、Cost、Rules、Settings 与 Versions 页面",
			Targets: []string{"bridge_pages.go", "frontend/src/workbench.tsx", "internal/adapter/core.go", "internal/adapter/runtime_core.go"},
			Notes:   []string{"Task9 将工具管理页接回真实 bridge / adapter 能力", "Worktree 专用 API 仍待共享核心后续补齐"},
		},
		{
			Name:    "本地原生能力桥接",
			Scope:   "文件对话框、剪贴板、日志、崩溃报告与诊断包导出",
			Targets: []string{"bridge_service.go", "bridge_pages.go", "internal/app/diagnostic_bundle.go", "internal/app/crash_report.go", "internal/app/logging.go"},
			Notes:   []string{"Task4 将原生能力切换到 Wails 路径", "前端通过服务调用消费这些能力"},
		},
	}
}
