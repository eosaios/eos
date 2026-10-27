package webbridge

import (
	"os"
	"path/filepath"
	"strings"
)

// 状态监听路径 helper：路径清理、目录存在性校验、plugin source 解析。

func addPathOrParent(out map[string]struct{}, path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		if dir := cleanExistingDir(path); dir != "" {
			out[dir] = struct{}{}
		}
		return
	}
	if err == nil {
		if dir := cleanExistingDir(filepath.Dir(path)); dir != "" {
			out[dir] = struct{}{}
		}
	}
}

func cleanExistingDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		cleaned = filepath.Clean(path)
	}
	info, err := os.Stat(cleaned)
	if err != nil || !info.IsDir() {
		return ""
	}
	return cleaned
}

func pluginSourcePath(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	if value, ok := strings.CutPrefix(source, "directory:"); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
