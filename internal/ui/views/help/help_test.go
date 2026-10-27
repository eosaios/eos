package help

import (
	"regexp"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/ui/styles"
)

var ansiPatternHelpTest = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSIHelpTest(s string) string {
	return ansiPatternHelpTest.ReplaceAllString(s, "")
}

func TestHelpContentRowsStartAfterSectionHeaders(t *testing.T) {
	h := NewHelpView(styles.NewStyles(styles.DefaultDarkTheme()), "zh")
	lines := strings.Split(stripANSIHelpTest(h.renderContent("zh")), "\n")

	for _, prefix := range []string{"F2", "/help", "/workspace"} {
		line := findHelpLine(t, lines, prefix)
		if !strings.HasPrefix(line, prefix) {
			t.Fatalf("line for %q should start at column 0, got %q", prefix, line)
		}
	}
}

func TestHelpContentWrapsLongCommandUsageBeforeDescription(t *testing.T) {
	h := NewHelpView(styles.NewStyles(styles.DefaultDarkTheme()), "zh")
	lines := strings.Split(stripANSIHelpTest(h.renderContent("zh")), "\n")

	i := findHelpLineIndex(t, lines, "/workspace")
	if strings.Contains(lines[i], "管理工作区") {
		t.Fatalf("long /workspace usage should not push the description rightward on the same line, got %q", lines[i])
	}
	if i+1 >= len(lines) {
		t.Fatalf("missing wrapped description line after %q", lines[i])
	}

	wantPrefix := strings.Repeat(" ", commandColumnWidth) + "- 管理工作区"
	if !strings.HasPrefix(lines[i+1], wantPrefix) {
		t.Fatalf("wrapped description line = %q, want prefix %q", lines[i+1], wantPrefix)
	}
}

func findHelpLine(t *testing.T, lines []string, prefix string) string {
	t.Helper()
	return lines[findHelpLineIndex(t, lines, prefix)]
}

func findHelpLineIndex(t *testing.T, lines []string, prefix string) int {
	t.Helper()
	for i, line := range lines {
		if strings.Contains(line, prefix) {
			return i
		}
	}
	t.Fatalf("missing line containing %q in:\n%s", prefix, strings.Join(lines, "\n"))
	return -1
}
