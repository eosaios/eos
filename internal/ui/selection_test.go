package ui

// selection_test.go — 框选复制的纯逻辑测试（wrapPlainLines / extractSelection /
// normalizeSelection）。鼠标事件接线见 app_model_test 层或手工验证。

import (
	"strings"
	"testing"
)

func TestWrapPlainLines_SingleShortLine(t *testing.T) {
	got := wrapPlainLines("hello", 10)
	if len(got) != 1 || got[0] != "hello" {
		t.Fatalf("got %#v, want [hello]", got)
	}
}

func TestWrapPlainLines_WrapsLongLine(t *testing.T) {
	got := wrapPlainLines("abcdef", 3)
	want := []string{"abc", "def"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestWrapPlainLines_PreservesBlankLines(t *testing.T) {
	got := wrapPlainLines("a\n\nb", 10)
	want := []string{"a", "", "b"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWrapPlainLines_StripsANSI(t *testing.T) {
	got := wrapPlainLines("\x1b[31mred\x1b[0m text", 20)
	if len(got) != 1 || got[0] != "red text" {
		t.Fatalf("got %#v, want [red text]", got)
	}
}

func TestExtractSelection_SingleLine(t *testing.T) {
	text := "hello world"
	got := extractSelection(text, 20, selectionCoord{line: 0, col: 6}, selectionCoord{line: 0, col: 11})
	if got != "world" {
		t.Fatalf("got %q, want world", got)
	}
}

func TestExtractSelection_ReversedOrderNormalized(t *testing.T) {
	text := "hello world"
	got := extractSelection(text, 20, selectionCoord{line: 0, col: 11}, selectionCoord{line: 0, col: 6})
	if got != "world" {
		t.Fatalf("got %q, want world (order normalized)", got)
	}
}

func TestExtractSelection_AcrossLines(t *testing.T) {
	text := "line one\nline two"
	got := extractSelection(text, 20,
		selectionCoord{line: 0, col: 5}, // "one" starts at col5
		selectionCoord{line: 1, col: 4}) // "line" ends at col4
	// expected: "one\nline"
	if got != "one\nline" {
		t.Fatalf("got %q, want %q", got, "one\nline")
	}
}

func TestExtractSelection_AcrossWrappedLines(t *testing.T) {
	text := "abcdefgh" // width 4 → lines [abcd, efgh]
	got := extractSelection(text, 4,
		selectionCoord{line: 0, col: 2},
		selectionCoord{line: 1, col: 2})
	// line0 col2..end = "cd"; line1 col0..2 = "ef" → "cd\nef"
	if got != "cd\nef" {
		t.Fatalf("got %q, want %q", got, "cd\nef")
	}
}

func TestExtractSelection_ClampsOutOfRange(t *testing.T) {
	text := "ab"
	got := extractSelection(text, 20,
		selectionCoord{line: 0, col: 0},
		selectionCoord{line: 0, col: 99})
	if got != "ab" {
		t.Fatalf("got %q, want ab", got)
	}
}

func TestExtractSelection_EmptySelection(t *testing.T) {
	text := "abc"
	got := extractSelection(text, 20,
		selectionCoord{line: 0, col: 1},
		selectionCoord{line: 0, col: 1})
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestNormalizeSelection(t *testing.T) {
	a, b := normalizeSelection(selectionCoord{line: 3, col: 1}, selectionCoord{line: 1, col: 5})
	if a.line != 1 || b.line != 3 {
		t.Fatalf("normalize failed: got (%d,%d)->(%d,%d)", a.line, a.col, b.line, b.col)
	}
	// 同行按列排序
	a, b = normalizeSelection(selectionCoord{line: 1, col: 9}, selectionCoord{line: 1, col: 2})
	if a.line != 1 || a.col != 2 || b.col != 9 {
		t.Fatalf("same-line normalize failed: (%d,%d)->(%d,%d)", a.line, a.col, b.line, b.col)
	}
}

func TestApplySelectionHighlight(t *testing.T) {
	t.Skip("applySelectionHighlight 位于 shell 包，见 views/shell/selection_test.go")
}
