package ui

// app_git_commit.go — git 提交提醒与 /commit 直派。
//
// 产品语义（零噪音）：
//   - turn 结束且工作区有未提交/未推送变更 → 一条系统提示（状态栏 ●/↑ 标记
//     由 git summary 节流刷新另路维护）；计数相对上次提示无变化不重复提示；
//   - /commit 把内置指令直派给 AI（不经输入框、不自动触发），由 agent 走
//     工具审批链执行提交推送；
//   - 设置 git_commit_reminder 可整体关闭提醒（nil = 旧配置默认开）。

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/i18n"
)

// gitCommitInstruction 是 /commit 直派给 AI 的内置指令
// （以用户消息呈现在会话流里，保证可解释）。
func (m *AppModel) gitCommitInstruction() string {
	return m.localize(
		"请检查当前工作区的改动，按仓库提交规范帮我提交并推送。",
		"Please inspect the current workspace changes and commit and push them following the repository's commit conventions.",
	)
}

// dispatchGitCommitRequest 把提交推送指令直派给 AI 执行。AI 处理中时拒绝，
// 避免与进行中的回合互相踩踏。
func (m *AppModel) dispatchGitCommitRequest() tea.Cmd {
	if m.state.Processing {
		m.appendSystem(i18n.T("git.commit.busy", m.state.Language), "warning")
		return nil
	}
	return m.sendMessageText(m.gitCommitInstruction(), false)
}

// GitCommitHintMsg 是 turn 结束后的 git 概览查询结果（提醒决策用）。
// OK=false 表示设置关闭或查询失败，一律不提示。
type GitCommitHintMsg struct {
	OK     bool
	Branch string
	Dirty  int
	Ahead  int
}

// GitBaselineMsg 是 turn 开始时的工作区计数快照结果（提交提醒的对照基线）。
// OK=false 表示非 git / 查询失败——基线缺失，本轮结束不提示。
type GitBaselineMsg struct {
	OK    bool
	Dirty int
	Ahead int
}

// snapshotGitBaseline 在 AI turn 开始时快照工作区计数，作为本轮结束评估
// 提交提醒的对照基线（2026-10-04 语义修正：只有计数相对本轮开始净增加才
// 提示，纯提问/只读轮不弹「本轮修改了 N 个文件」）。
func (m *AppModel) snapshotGitBaseline() tea.Cmd {
	if m == nil || m.adapter == nil {
		return nil
	}
	adapter := m.adapter
	return func() tea.Msg {
		ctx := context.Background()
		result, err := adapter.GitSummary(ctx, "")
		if err != nil {
			return GitBaselineMsg{OK: false}
		}
		return GitBaselineMsg{
			OK:    true,
			Dirty: len(result.Changes),
			Ahead: int(result.Ahead),
		}
	}
}

// handleGitBaselineMsg 落盘本轮基线；查询失败按缺失（-1）处理。
func (m *AppModel) handleGitBaselineMsg(msg GitBaselineMsg) (tea.Model, tea.Cmd) {
	if msg.OK {
		m.gitBaselineDirty = msg.Dirty
		m.gitBaselineAhead = msg.Ahead
	} else {
		m.gitBaselineDirty = -1
		m.gitBaselineAhead = -1
	}
	return m, nil
}

// scheduleGitCommitReminder turn 结束后拉一次设置与 git 概览，
// 由 handleGitCommitHintMsg 决定是否提示。
func (m *AppModel) scheduleGitCommitReminder() tea.Cmd {
	if m == nil || m.adapter == nil {
		return nil
	}
	adapter := m.adapter
	return func() tea.Msg {
		// 开关裁决读 CLI config（内核 Settings 不落盘，持久化在 config）。
		if cfg, _ := config.Load(); !config.GitCommitReminderEnabled(&cfg) {
			return GitCommitHintMsg{OK: false}
		}
		ctx := context.Background()
		result, err := adapter.GitSummary(ctx, "")
		if err != nil {
			return GitCommitHintMsg{OK: false}
		}
		return GitCommitHintMsg{
			OK:     true,
			Branch: result.Branch,
			Dirty:  len(result.Changes),
			Ahead:  int(result.Ahead),
		}
	}
}

// handleGitCommitHintMsg 依据 turn 结束时的 git 概览决定是否提示，
// 并顺带刷新状态栏 git 项。提示条件：计数相对本轮开始基线净增加
// （gitBaselineDirty < 0 = 基线缺失，不提示）。
func (m *AppModel) handleGitCommitHintMsg(msg GitCommitHintMsg) (tea.Model, tea.Cmd) {
	if m.shell != nil {
		m.shell.SetGitSummary(msg.Branch, msg.Dirty, msg.Ahead)
	}
	if !msg.OK || m.state.Processing {
		return m, m.finalizeUpdate(nil)
	}
	if m.gitBaselineDirty < 0 {
		return m, m.finalizeUpdate(nil)
	}
	if msg.Dirty <= m.gitBaselineDirty && msg.Ahead <= m.gitBaselineAhead {
		return m, m.finalizeUpdate(nil)
	}
	m.appendSystem(gitCommitHintText(m.state.Language, msg.Dirty, msg.Ahead), "info")
	return m, m.finalizeUpdate(nil)
}

// gitCommitHintText 组装提醒文案（parts 风格对齐桌面端徽标）。
func gitCommitHintText(lang string, dirty, ahead int) string {
	var parts []string
	if dirty > 0 {
		parts = append(parts, i18n.T("git.hint.dirty", lang, dirty))
	}
	if ahead > 0 {
		parts = append(parts, i18n.T("git.hint.ahead", lang, ahead))
	}
	return strings.Join(parts, i18n.T("git.hint.join", lang)) + i18n.T("git.hint.action", lang)
}
