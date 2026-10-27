package webbridge

import (
	"os"
	"path/filepath"
	"strings"
)

func tailLines(path string, maxLines int) []string {
	if strings.TrimSpace(path) == "" || maxLines <= 0 {
		return []string{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	out := make([]string, 0, maxLines)
	for index := len(lines) - 1; index >= 0 && len(out) < maxLines; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		out = append([]string{line}, out...)
	}
	return nonNilSlice(out)
}

func defaultExportBundlePath() string {
	return filepath.Join(DefaultLogDir(), "eos-diagnostics.zip")
}
