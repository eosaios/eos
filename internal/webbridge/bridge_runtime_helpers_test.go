package webbridge

import (
	"strings"
	"testing"
)

func TestIsAutoSessionPlaceholderTitle(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{title: "", want: true},
		{title: "   ", want: true},
		{title: "新对话", want: true},
		{title: "New Chat", want: true},
		{title: "new chat", want: true},
		{title: "Untitled Chat", want: false},
		{title: "修复箭头闪烁", want: false},
	}
	for _, tc := range cases {
		if got := isAutoSessionPlaceholderTitle(tc.title); got != tc.want {
			t.Fatalf("isAutoSessionPlaceholderTitle(%q) = %v, want %v", tc.title, got, tc.want)
		}
	}
}

func TestSummarizeSessionTitleInputExtractsTaskSummary(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			input: "你分析一下现在软件使用上有哪些问题，然后在根目录下创建一个单独的文件夹",
			want:  "分析软件问题",
		},
		{
			input: "把设置的菜单里面图标看着比例不太合理，优化一下",
			want:  "优化设置菜单图标",
		},
		{
			input: "帮我修一下完全访问切换还会弹授权的问题",
			want:  "修复完全访问切换弹授权",
		},
	}
	for _, tc := range cases {
		if got := summarizeSessionTitleInput(tc.input); got != tc.want {
			t.Fatalf("summarizeSessionTitleInput(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPrepareOutgoingMessagesLockedTitlesUntitledSessionImmediately(t *testing.T) {
	service := &BridgeService{}
	session := &sessionState{ID: "s1", Title: "New Chat"}

	assistantID := service.prepareOutgoingMessagesLocked(session, "分析当前目录并输出报告", nil)

	if strings.TrimSpace(assistantID) == "" {
		t.Fatal("assistantID is empty")
	}
	if session.Title != "分析目录并写报告" {
		t.Fatalf("session.Title = %q, want summarized title from first input", session.Title)
	}
	if len(session.Messages) != 2 {
		t.Fatalf("len(session.Messages) = %d, want 2", len(session.Messages))
	}
}

func TestPrepareOutgoingMessagesLockedKeepsCustomSessionTitle(t *testing.T) {
	service := &BridgeService{}
	session := &sessionState{ID: "s1", Title: "已有标题"}

	service.prepareOutgoingMessagesLocked(session, "新的用户输入", nil)

	if session.Title != "已有标题" {
		t.Fatalf("session.Title = %q, want existing custom title preserved", session.Title)
	}
}
