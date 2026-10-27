package webbridge

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// 版本目录解析：把 workspace root 映射到全局版本存储目录。
// 新版布局 ~/.eos/versions/workspaces/<sha256前8位>，legacy 布局 <root>/.eos/versions。
// existingVersionWorkspaceRoot 优先返回已存在的布局；两者都不存在时返回新版路径，
// 供首次创建场景使用（由调用方负责后续创建），不是错误兜底。

func resolveVersionWorkspaceRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		root, _ = os.Getwd()
	}
	if strings.TrimSpace(root) == "" {
		return ""
	}
	if !filepath.IsAbs(root) {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}
	return filepath.Clean(root)
}

func globalVersionsBaseDir() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".eos", "versions")
	}
	if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
		return filepath.Join(filepath.Clean(wd), ".eos", "versions")
	}
	return filepath.Join(".eos", "versions")
}

func workspaceVersionNamespaceID(root string) string {
	normalized := strings.ToLower(filepath.ToSlash(resolveVersionWorkspaceRoot(root)))
	if normalized == "" {
		return "default"
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:8])
}

func versionWorkspaceRoot(root string) string {
	return filepath.Join(globalVersionsBaseDir(), "workspaces", workspaceVersionNamespaceID(root))
}

func legacyVersionWorkspaceRoot(root string) string {
	root = resolveVersionWorkspaceRoot(root)
	if root == "" {
		return filepath.Join(".eos", "versions")
	}
	return filepath.Join(root, ".eos", "versions")
}

func existingVersionWorkspaceRoot(root string) string {
	current := versionWorkspaceRoot(root)
	if info, err := os.Stat(current); err == nil && info.IsDir() {
		return current
	}
	legacy := legacyVersionWorkspaceRoot(root)
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		return legacy
	}
	return current
}
