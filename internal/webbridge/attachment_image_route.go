package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

// 附件图片的 HTTP 加载路由。
//
// 为什么不走绑定方法返回 base64：截图类图片经 JSON-RPC 回传时负载可达
// 数 MB，WebView2 的消息通道对超大响应会静默丢弃，前端 promise 永远
// pending——表现为预览面板「正在加载…」卡死。图片改由资产服务器的
// HTTP 路由按需拉取（原生 <img src>，浏览器负责流式解码与缓存），
// PreviewAttachment 绑定方法只回传轻量 url。

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// AttachmentImageRoutePath 是资产服务器上附件图片路由的固定路径。
// 前端 <img src> 与 AttachmentPreview.URL 都指向它。
const AttachmentImageRoutePath = "/eos/attachment-image"

// AttachmentImageURL 构造附件图片的可加载 url（path 需是绝对路径）。
func AttachmentImageURL(path string) string {
	return AttachmentImageRoutePath + "?path=" + url.QueryEscape(filepath.ToSlash(path))
}

// AttachmentImageMiddleware 拦截附件图片路由，其余请求原样交给默认
// 静态资源 handler。roots 提供允许的根目录（附件缓存目录 + 各工作区根），
// 每次请求实时读取以跟随工作区切换。
func (s *BridgeService) AttachmentImageMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == AttachmentImageRoutePath {
			s.serveAttachmentImage(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// attachmentImageRoots 汇总图片路由允许的根目录：
// 附件缓存目录（截图/粘贴图导入处）+ 各工作区根（发送后附件被复制进
// 工作区 .eos/attachments）。webview 源是本机回环资产服务器，仍收敛
// 白名单避免把任意本地文件暴露成可枚举的 img url。
func (s *BridgeService) attachmentImageRoots() []string {
	roots := s.workspacePreviewRoots()
	return append(roots, attachmentCacheDir())
}

func (s *BridgeService) serveAttachmentImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	target, err := s.resolveAttachmentImagePath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "attachment image not found", http.StatusNotFound)
		return
	}
	// ServeFile 自带 Range/If-Modified-Since/ETag（mtime+size），重复悬停
	// 与预览面板刷新都吃 304 缓存。
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, target)
}

// resolveAttachmentImagePath 校验并解析图片绝对路径：必须存在、是普通
// 文件、扩展名为图片、大小在上限内、且（含符号链接解析后）落在允许
// 根目录内。
func (s *BridgeService) resolveAttachmentImagePath(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", errors.New("path is required")
	}
	cleaned := filepath.Clean(filepath.FromSlash(target))
	if !filepath.IsAbs(cleaned) {
		return "", errors.New("path must be absolute")
	}
	if !isImageAttachmentPath(cleaned) {
		return "", errors.New("path is not an image")
	}
	// EvalSymlinks 把符号链接解析到真实路径后再做白名单判定，
	// 防止「白名单内的链接指向白名单外的文件」绕过。
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return "", err
	}
	roots := s.attachmentImageRoots()
	if !pathWithinAnyRoot(resolved, roots) {
		return "", errors.New("path is outside attachment roots")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("path is a directory")
	}
	if info.Size() > maxImportedAttachmentBytes {
		return "", errors.New("image exceeds size limit")
	}
	return resolved, nil
}
