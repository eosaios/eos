package config

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func DefaultWorkspacePath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".eos", "workspace")
	}
	return filepath.Join(home, ".eos", "workspace")
}

func EnsureDefaultWorkspaceDir() error {
	path := strings.TrimSpace(DefaultWorkspacePath())
	if path == "" {
		return nil
	}
	return os.MkdirAll(path, 0o755)
}

func ResolveWorkspacePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("workspace path required")
	}
	if raw == "~" || strings.HasPrefix(raw, "~"+string(os.PathSeparator)) || strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return "", errors.New("failed to resolve home path")
		}
		rest := strings.TrimPrefix(raw, "~")
		rest = strings.TrimPrefix(rest, "/")
		rest = strings.TrimPrefix(rest, "\\")
		raw = filepath.Join(home, rest)
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	return NormalizeWorkspacePath(abs), nil
}

func NormalizeWorkspacePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(filepath.FromSlash(path))
	if vol := filepath.VolumeName(path); vol != "" {
		rest := strings.TrimPrefix(path, vol)
		path = strings.ToUpper(vol) + rest
	}
	return path
}

func PathsEqual(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	if strings.EqualFold(filepath.VolumeName(left), filepath.VolumeName(right)) {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func NormalizeKnownWorkspaces(paths []string) []string {
	defaultWorkspace := NormalizeWorkspacePath(DefaultWorkspacePath())
	out := make([]string, 0, len(paths)+1)
	seen := make(map[string]struct{}, len(paths)+1)
	add := func(path string) {
		path = NormalizeWorkspacePath(path)
		if path == "" {
			return
		}
		key := strings.ToLower(path)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, path)
	}
	add(defaultWorkspace)
	for _, path := range paths {
		add(path)
	}
	slices.SortFunc(out, func(a, b string) int {
		if PathsEqual(a, defaultWorkspace) {
			return -1
		}
		if PathsEqual(b, defaultWorkspace) {
			return 1
		}
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	return out
}

func RememberWorkspace(cfg *Config, path string, foreground bool) bool {
	if cfg == nil {
		return false
	}
	path = NormalizeWorkspacePath(path)
	if path == "" {
		return false
	}
	if PathsEqual(path, DefaultWorkspacePath()) {
		_ = EnsureDefaultWorkspaceDir()
	}
	next := NormalizeKnownWorkspaces(append(cfg.KnownWorkspaces, path))
	changed := !sameWorkspaceSlice(cfg.KnownWorkspaces, next)
	cfg.KnownWorkspaces = next
	if foreground {
		if !PathsEqual(cfg.LastWorkspace, path) {
			changed = true
		}
		cfg.LastWorkspace = path
	}
	return changed
}

func ForgetWorkspace(cfg *Config, path string) bool {
	if cfg == nil {
		return false
	}
	target := NormalizeWorkspacePath(path)
	defaultWorkspace := NormalizeWorkspacePath(DefaultWorkspacePath())
	if target == "" || PathsEqual(target, defaultWorkspace) {
		next := NormalizeKnownWorkspaces(cfg.KnownWorkspaces)
		changed := !sameWorkspaceSlice(cfg.KnownWorkspaces, next)
		cfg.KnownWorkspaces = next
		if cfg.LastWorkspace != "" && PathsEqual(cfg.LastWorkspace, target) {
			cfg.LastWorkspace = defaultWorkspace
			changed = true
		}
		return changed
	}
	next := make([]string, 0, len(cfg.KnownWorkspaces))
	for _, path := range cfg.KnownWorkspaces {
		if PathsEqual(path, target) {
			continue
		}
		next = append(next, path)
	}
	next = NormalizeKnownWorkspaces(next)
	changed := !sameWorkspaceSlice(cfg.KnownWorkspaces, next)
	cfg.KnownWorkspaces = next
	if cfg.LastWorkspace != "" && PathsEqual(cfg.LastWorkspace, target) {
		cfg.LastWorkspace = defaultWorkspace
		changed = true
	}
	return changed
}

func ResolveForegroundWorkspace(cfg Config, preferred string) string {
	preferred = NormalizeWorkspacePath(preferred)
	last := NormalizeWorkspacePath(cfg.LastWorkspace)
	defaultWorkspace := NormalizeWorkspacePath(DefaultWorkspacePath())
	switch {
	case preferred != "":
		return preferred
	case last != "":
		return last
	default:
		return defaultWorkspace
	}
}

func NormalizeWorkspaceState(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	changed := false
	nextKnown := NormalizeKnownWorkspaces(cfg.KnownWorkspaces)
	if !sameWorkspaceSlice(cfg.KnownWorkspaces, nextKnown) {
		cfg.KnownWorkspaces = nextKnown
		changed = true
	}
	last := NormalizeWorkspacePath(cfg.LastWorkspace)
	if last == "" {
		last = NormalizeWorkspacePath(DefaultWorkspacePath())
	}
	if !PathsEqual(cfg.LastWorkspace, last) {
		cfg.LastWorkspace = last
		changed = true
	}
	if !containsWorkspace(cfg.KnownWorkspaces, last) {
		cfg.KnownWorkspaces = NormalizeKnownWorkspaces(append(cfg.KnownWorkspaces, last))
		changed = true
	}
	return changed
}

func containsWorkspace(paths []string, target string) bool {
	for _, path := range paths {
		if PathsEqual(path, target) {
			return true
		}
	}
	return false
}

func sameWorkspaceSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !PathsEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}
