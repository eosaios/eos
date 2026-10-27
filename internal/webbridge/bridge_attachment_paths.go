package webbridge

import (
	"errors"
	"path/filepath"
	"strings"
)

func (s *BridgeService) resolveWorkspaceFilePath(path string) (string, error) {
	raw := strings.TrimSpace(path)
	if raw == "" {
		return "", errors.New(s.t("error.attachment.path_required"))
	}
	roots := s.workspacePreviewRoots()
	if len(roots) == 0 {
		return "", errors.New(s.t("error.attachment.preview_workspace_required"))
	}
	candidates := []string{}
	cleaned := filepath.Clean(filepath.FromSlash(raw))
	if filepath.IsAbs(cleaned) {
		candidates = append(candidates, cleaned)
	} else {
		for _, root := range roots {
			candidates = append(candidates, filepath.Join(root, cleaned))
		}
	}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if !pathWithinAnyRoot(abs, roots) {
			continue
		}
		return filepath.Clean(abs), nil
	}
	return "", errors.New(s.t("error.attachment.preview_text_workspace_only"))
}

func (s *BridgeService) workspacePreviewRoots() []string {
	s.stateMu.RLock()
	roots := []string{}
	if workspace := strings.TrimSpace(s.activeWorkspace); workspace != "" {
		roots = append(roots, workspace)
	}
	if session := s.sessions[strings.TrimSpace(s.currentSessionID)]; session != nil {
		if workspace := strings.TrimSpace(session.WorkspacePath); workspace != "" {
			roots = append(roots, workspace)
		}
	}
	s.stateMu.RUnlock()
	for _, item := range s.worktreesReadOnly() {
		if item.Active && strings.TrimSpace(item.Path) != "" {
			roots = append(roots, item.Path)
		}
	}
	if len(roots) == 0 {
		roots = append(roots, s.defaultWorkspacePathReadOnly())
	}
	out := make([]string, 0, len(roots))
	seen := map[string]struct{}{}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		key := strings.ToLower(filepath.Clean(abs))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, filepath.Clean(abs))
	}
	return out
}

// pathWithinAnyRoot 判定 path 是否落在任一白名单根内。
//
// 两边都按 EvalSymlinks 归一后再比较：根目录本身也可能是符号链接
// （macOS 的 /var → /private/var、symlinked home 等），若只解析目标不解析
// 根，「目标已解析、根未解析」会导致合法路径被误判为白名单外。根是可信
// 配置，解析根不弱化防穿越语义（目标仍以完整解析后的真实路径参与比较）。
func pathWithinAnyRoot(path string, roots []string) bool {
	targets := []string{filepath.Clean(path)}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		targets = append(targets, filepath.Clean(resolved))
	}
	for _, root := range roots {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rootCandidates := []string{filepath.Clean(rootAbs)}
		if resolvedRoot, err := filepath.EvalSymlinks(rootAbs); err == nil {
			rootCandidates = append(rootCandidates, filepath.Clean(resolvedRoot))
		}
		for _, rootClean := range rootCandidates {
			for _, target := range targets {
				if pathWithinRoot(strings.ToLower(target), strings.ToLower(rootClean)) {
					return true
				}
			}
		}
	}
	return false
}

func pathWithinRoot(target, root string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// languageFromPath 的返回值与前端 shiki 白名单（workbench-code-highlight.tsx
// 的 languageAliases + loadHighlighter）一一对齐：这里映射出的语言前端必须
// 能高亮，否则退化为纯文本展示。
func languageFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "tsx"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".css":
		return "css"
	case ".json", ".jsonc":
		return "json"
	case ".md", ".markdown":
		return "markdown"
	case ".html", ".htm":
		return "html"
	case ".yml", ".yaml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".rs":
		return "rust"
	case ".py", ".pyi":
		return "python"
	case ".java":
		return "java"
	case ".sql":
		return "sql"
	case ".sh", ".bash", ".zsh":
		return "bash"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	default:
		return "text"
	}
}

func pathInsideWorkspace(path string, workspace string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	if path == "" || workspace == "" {
		return false
	}
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
