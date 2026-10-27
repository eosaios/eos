package webbridge

import "testing"

func TestRequestCompletedTextUsesPayloadText(t *testing.T) {
	payload := map[string]any{
		"text": "<think>用户发来了你好</think>\n你好！这是最终正文。",
	}
	got := requestCompletedText(payload)
	want := "<think>用户发来了你好</think>\n你好！这是最终正文。"
	if got != want {
		t.Fatalf("requestCompletedText()=%q, want %q", got, want)
	}
}

func TestRequestCompletedTextDoesNotCompareAgainstBuffer(t *testing.T) {
	payload := map[string]any{
		"text": "<think>\n用户只是",
	}

	got := requestCompletedText(payload)
	want := "<think>\n用户只是"
	if got != want {
		t.Fatalf("requestCompletedText()=%q, want %q", got, want)
	}
}
