package webbridge

import (
	"slices"
	"strings"
	"time"
)

// isBinaryBytes reports whether data looks like binary content by checking for
// a NUL byte. This is a generic byte heuristic shared by the workspace diff
// collector (skip binary files in diffs) and the attachment service (decide
// whether attachment content is previewable), so it lives here rather than in
// either domain file.
func isBinaryBytes(data []byte) bool {
	return slices.Contains(data, byte(0))
}

func isDialogCancelledError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "cancelled by user")
}

func fallbackText(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

// #endregion

func parseTime(value string) time.Time {
	if value == "" {
		return time.Now()
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now()
	}
	return parsed
}

func normalizeExecutionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "plan", "计划优先", "先出计划":
		return "plan"
	default:
		return "auto"
	}
}
