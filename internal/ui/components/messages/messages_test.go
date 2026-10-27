package messages

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/eosaios/eos/internal/ui/styles"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSIForTest(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

func testStyles() *styles.Styles {
	return styles.NewStyles(styles.GetTheme("dark"))
}

func TestTruncateInlinePreservesUTF8(t *testing.T) {
	text := "这是一个用于测试中文截断是否安全的字符串"
	got := truncateInline(text, 8)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateInline produced invalid utf8: %q", got)
	}
	if !strings.Contains(got, "...(+") {
		t.Fatalf("truncateInline=%q, want truncation marker", got)
	}
}

func TestBubbleWidthUsesWiderLayout(t *testing.T) {
	if got := bubbleWidth(132); got <= 96 {
		t.Fatalf("bubbleWidth(132)=%d, want > 96", got)
	}
}

func TestRenderAgentDispatchShowsLinearRoute(t *testing.T) {
	msg := &AgentDispatchMessage{
		AgentName:  "verification",
		AgentID:    "subagent_verification_3",
		SourceName: "assistant",
		Event:      "dispatch",
		Task:       "验证输出是否完整",
	}
	rendered := stripANSIForTest(msg.Render(testStyles(), 132))
	for _, want := range []string{"[dispatch]", "assistant -> verification", "subagent_verification_3"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered=%q, want substring %q", rendered, want)
		}
	}
}

func TestToolResultShowsExplicitTruncationNotice(t *testing.T) {
	msg := &ToolCallMessage{
		Name:   "read",
		Status: "success",
		Result: strings.Repeat("测", 1300),
	}
	rendered := stripANSIForTest(msg.Render(testStyles(), 132))
	if !strings.Contains(rendered, "[truncated: showing first 1200 of 1300 chars]") {
		t.Fatalf("rendered=%q, want explicit truncation notice", rendered)
	}
	if !utf8.ValidString(rendered) {
		t.Fatalf("rendered contains invalid utf8")
	}
}
