package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

// server_static.go 服务前端静态产物并解析 UI 目录。
//
// v1 不做 go:embed：前端构建产物（eos-app-src/frontend/dist）由目录解析
// 定位。解析顺序：显式 --ui-dir → EOS_WEB_UI_DIR → 相对工作目录的候选路径。
// 全部落空则启动失败并列出已搜索路径（fail-fast，不提供降级页面）。

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// webRuntimeMarker 是注入 index.html 的 web 运行时标记。前端
// hasWebRuntime() 据此选择桌面桥（callBackend 经 runtime.js shim 到本服务），
// 而不是预览 mock 桥。
const webRuntimeMarker = `<script>window.__EOS_WEB_RUNTIME__ = true;</script>`

// resolveWebUIDir 定位前端构建产物目录。返回已解析目录或带搜索路径清单的错误。
func resolveWebUIDir(explicit string) (string, error) {
	if dir := strings.TrimSpace(explicit); dir != "" {
		if err := validateUIDir(dir); err != nil {
			return "", fmt.Errorf("web ui dir %q: %w", dir, err)
		}
		return dir, nil
	}
	if dir := strings.TrimSpace(os.Getenv("EOS_WEB_UI_DIR")); dir != "" {
		if err := validateUIDir(dir); err != nil {
			return "", fmt.Errorf("EOS_WEB_UI_DIR %q: %w", dir, err)
		}
		return dir, nil
	}
	candidates := webUIDirCandidates()
	for _, dir := range candidates {
		if validateUIDir(dir) == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("web UI dist not found; set --ui-dir or EOS_WEB_UI_DIR to eos-app frontend/dist (searched: %s)",
		strings.Join(candidates, ", "))
}

func webUIDirCandidates() []string {
	var candidates []string
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "eos-app-src", "frontend", "dist"),
			filepath.Join(wd, "frontend", "dist"),
			filepath.Join(wd, "..", "eos-app-src", "frontend", "dist"),
		)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "eos-app-src", "frontend", "dist"),
			filepath.Join(exeDir, "frontend", "dist"),
		)
	}
	return candidates
}

func validateUIDir(dir string) error {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	if err != nil {
		return fmt.Errorf("index.html not found: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("index.html is a directory")
	}
	return nil
}

// handleStatic 服务前端产物。"GET /" 注入 web 运行时标记；带扩展名的
// 资源路径按普通静态文件处理（SPA 前端无客户端路由，不存在的路径 404）。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		// r.URL.Path 服务端请求恒以 / 开头；再拼前导斜杠会得到 "//x"，
		// 被 path.Clean 折叠后与原串不等 → 所有资产被当可疑路径 404。
		// 净化必须用 path.Clean（URL 恒为正斜杠语义）：filepath.Clean 在
		// Windows 会把 / 规范成 \，同样导致全部资产 404（CI 实测抓到）。
		rel := r.URL.Path
		if rel == "" {
			rel = "/"
		}
		if rel != path.Clean(rel) || strings.Contains(rel, "\x00") {
			http.NotFound(w, r)
			return
		}
		full := filepath.Join(s.uiDir, filepath.FromSlash(path.Clean(rel)))
		if !strings.HasPrefix(full, filepath.Clean(s.uiDir)+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, full)
		return
	}
	raw, err := os.ReadFile(filepath.Join(s.uiDir, "index.html"))
	if err != nil {
		http.Error(w, "index.html unreadable: "+err.Error(), http.StatusInternalServerError)
		return
	}
	html := injectWebRuntimeMarker(string(raw))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(html))
}

// injectWebRuntimeMarker 把 web 运行时标记插到 <head> 最前面（任何模块
// 脚本执行前生效），页面没有 <head> 时原样返回（fail-fast，产物非法）。
func injectWebRuntimeMarker(html string) string {
	if i := strings.Index(html, "<head>"); i >= 0 {
		return html[:i+len("<head>")] + webRuntimeMarker + html[i+len("<head>"):]
	}
	return html
}

func runtimeGOOS() string {
	return runtime.GOOS
}
