package ui

// app_git_commit_test.go — 提交提醒文案组装与直派守卫的单元测试。

import (
	"strings"
	"testing"
)

func TestGitCommitHintText(t *testing.T) {
	cases := []struct {
		name  string
		lang  string
		dirty int
		ahead int
		want  string
	}{
		{name: "dirty only zh", lang: "zh", dirty: 3, ahead: 0, want: "工作区有 3 个未提交文件，输入 /commit 让我提交推送"},
		{name: "ahead only zh", lang: "zh", dirty: 0, ahead: 2, want: "2 个提交未推送，输入 /commit 让我提交推送"},
		{name: "both zh", lang: "zh", dirty: 3, ahead: 2, want: "工作区有 3 个未提交文件、2 个提交未推送，输入 /commit 让我提交推送"},
		{name: "both en", lang: "en", dirty: 1, ahead: 1, want: "1 uncommitted file(s) in the workspace, 1 commit(s) not pushed — type /commit and I will commit and push"},
	}
	for _, tc := range cases {
		if got := gitCommitHintText(tc.lang, tc.dirty, tc.ahead); got != tc.want {
			t.Errorf("%s: gitCommitHintText() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDispatchGitCommitRequestBlockedWhileProcessing(t *testing.T) {
	app := newTestAppModel(t)
	app.state.Processing = true
	if cmd := app.dispatchGitCommitRequest(); cmd != nil {
		t.Fatal("dispatch must be a no-op while a turn is processing")
	}
}

func TestSendMessageTextIgnoresBlankDraft(t *testing.T) {
	app := newTestAppModel(t)
	if cmd := app.sendMessageText("   ", false); cmd != nil {
		t.Fatal("blank programmatic draft must not dispatch")
	}
}

// 基线语义（2026-10-04 误弹修正）：提示只看「计数相对本轮开始基线净增加」。
// 纯提问轮（基线 15 → 结束仍 15）不提示；基线缺失不提示；AI 提交推送后
// dirty 降、ahead 升按 push-only 提示。
func TestGitCommitHintRequiresBaselineGrowth(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	hintCount := func() int {
		n := 0
		for _, h := range app.history {
			if h.kind == "system" && strings.Contains(h.content, "/commit") {
				n++
			}
		}
		return n
	}

	// 基线缺失（-1）：工作区有遗留脏改动也不提示。
	app.gitBaselineDirty = -1
	app.gitBaselineAhead = -1
	_, _ = app.handleGitCommitHintMsg(GitCommitHintMsg{OK: true, Branch: "main", Dirty: 15})
	if hintCount() != 0 {
		t.Fatal("missing baseline must not hint")
	}

	// 纯提问轮：基线 15 → 结束仍 15，不提示（旧逻辑此处会误弹）。
	app.gitBaselineDirty = 15
	app.gitBaselineAhead = 0
	_, _ = app.handleGitCommitHintMsg(GitCommitHintMsg{OK: true, Branch: "main", Dirty: 15})
	if hintCount() != 0 {
		t.Fatal("turn without growth must not hint")
	}

	// 本轮真有修改：15 → 18，提示一条。
	_, _ = app.handleGitCommitHintMsg(GitCommitHintMsg{OK: true, Branch: "main", Dirty: 18})
	if hintCount() != 1 {
		t.Fatal("grown dirty count must hint once")
	}

	// AI 提交未推成：dirty 降、ahead 升 → 按 push-only 提示。
	app2 := newTestAppModel(t)
	app2.gitBaselineDirty = 15
	app2.gitBaselineAhead = 0
	_, _ = app2.handleGitCommitHintMsg(GitCommitHintMsg{OK: true, Branch: "main", Dirty: 0, Ahead: 2})
	n := 0
	for _, h := range app2.history {
		if h.kind == "system" && strings.Contains(h.content, "未推送") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("commit-without-push turn must hint push-only once, got %d", n)
	}
}
