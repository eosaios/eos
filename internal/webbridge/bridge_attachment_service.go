package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxWorkspaceFilePreviewBytes = 512 * 1024

type AttachmentService struct {
	bridge *BridgeService
}

func NewAttachmentService(bridge *BridgeService) *AttachmentService {
	return &AttachmentService{bridge: bridge}
}

func (s *BridgeService) attachmentService() *AttachmentService {
	if s == nil {
		return NewAttachmentService(nil)
	}
	if s.attachmentSvc == nil {
		s.attachmentSvc = NewAttachmentService(s)
	}
	return s.attachmentSvc
}

// OpenAttachmentDialog web 模式不支持原生文件选择对话框（浏览器沙箱无文件系统
// 访问）。附件走 ImportAttachment（base64 上传）或 PreviewWorkspaceFile（工作区内文件）。
func (svc *AttachmentService) OpenAttachmentDialog() (FileDialogResult, error) {
	return FileDialogResult{}, errors.New("native file dialog is not available in web mode")
}

func (svc *AttachmentService) ImportAttachment(name string, mime string, base64Data string) (AttachmentRef, error) {
	s := svc.bridge
	if s == nil {
		return AttachmentRef{}, errors.New("bridge service is not available")
	}
	raw := strings.TrimSpace(base64Data)
	if dataMIME, body, ok := splitAttachmentDataURL(raw); ok {
		if inferredMIME, _, inferredOK := normalizeImportedImageMIME(dataMIME); inferredOK {
			mime = inferredMIME
		}
		raw = body
	}
	normalizedMIME, ext, ok := normalizeImportedImageMIME(mime)
	if !ok {
		return AttachmentRef{}, errors.New("only png, jpeg, webp, gif, and bmp images are supported")
	}
	if len(raw) > maxImportedAttachmentBase64Len {
		return AttachmentRef{}, errors.New("image exceeds 20MB limit")
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return AttachmentRef{}, errors.New("image data is not valid base64")
	}
	if len(data) == 0 {
		return AttachmentRef{}, errors.New("image data is empty")
	}
	if len(data) > maxImportedAttachmentBytes {
		return AttachmentRef{}, errors.New("image exceeds 20MB limit")
	}

	safeName := safeAttachmentFilename(name, ext)
	dir := filepath.Join(attachmentCacheDir(), fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return AttachmentRef{}, err
	}
	target := filepath.Join(dir, safeName)
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return AttachmentRef{}, err
	}

	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return AttachmentRef{Name: safeName, Path: target, MIME: normalizedMIME, Kind: "image"}, nil
}

// PreviewAttachment 返回附件图片的轻量预览描述：只回传路由 url（见
// attachment_image_route.go 的注释——base64 大负载会卡死 WebView2 消息
// 通道），图片字节由 <img src> 走 HTTP 路由加载。
func (svc *AttachmentService) PreviewAttachment(path string) (AttachmentPreview, error) {
	if svc.bridge == nil {
		return AttachmentPreview{}, errors.New("bridge service is not available")
	}
	target := strings.TrimSpace(path)
	if target == "" {
		return AttachmentPreview{}, errors.New("attachment path is required")
	}
	// 复用路由的白名单校验（存在/扩展名/根目录/大小），保证返回的 url
	// 一定可加载；MIME 靠魔数嗅探而非扩展名。
	resolved, err := svc.bridge.resolveAttachmentImagePath(target)
	if err != nil {
		return AttachmentPreview{}, err
	}
	head := make([]byte, 16)
	file, err := os.Open(resolved)
	if err != nil {
		return AttachmentPreview{}, err
	}
	defer file.Close()
	if _, err := io.ReadFull(file, head); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return AttachmentPreview{}, err
	}
	mime, ok := detectAttachmentImageMIME(head)
	if !ok {
		return AttachmentPreview{}, errors.New("only png, jpeg, webp, gif, and bmp images are supported for preview")
	}
	return AttachmentPreview{
		Name: filepath.Base(resolved),
		Path: resolved,
		MIME: mime,
		URL:  AttachmentImageURL(resolved),
	}, nil
}

func (svc *AttachmentService) PreviewWorkspaceFile(path string, line int) (WorkspaceFilePreview, error) {
	s := svc.bridge
	if s == nil {
		return WorkspaceFilePreview{}, errors.New("bridge service is not available")
	}
	target, err := s.resolveWorkspaceFilePath(path)
	if err != nil {
		return WorkspaceFilePreview{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return WorkspaceFilePreview{}, err
	}
	if info.IsDir() {
		return WorkspaceFilePreview{}, errors.New("cannot preview a directory")
	}
	file, err := os.Open(target)
	if err != nil {
		return WorkspaceFilePreview{}, err
	}
	defer file.Close()
	// LimitReader(max+1) 一次读出：超出上限时截断展示并置 Truncated，
	// 而不是整读大文件再判长（避免预览 GB 级日志时把进程内存打爆）。
	data, err := io.ReadAll(io.LimitReader(file, maxWorkspaceFilePreviewBytes+1))
	if err != nil {
		return WorkspaceFilePreview{}, err
	}
	truncated := len(data) > maxWorkspaceFilePreviewBytes
	if truncated {
		data = data[:maxWorkspaceFilePreviewBytes]
		// 截断点回退到最后一个换行（含）：保留完整行，行级截断不会劈开
		// 多字节 UTF-8 字符，高亮渲染也保持整行语义。无换行的超长单行按字节截。
		if idx := bytes.LastIndexByte(data, '\n'); idx >= 0 {
			data = data[:idx+1]
		}
	}
	if isBinaryBytes(data) {
		return WorkspaceFilePreview{}, errors.New("cannot preview a binary file")
	}
	if line < 0 {
		line = 0
	}
	return WorkspaceFilePreview{
		Name:      filepath.Base(target),
		Path:      target,
		Language:  languageFromPath(target),
		Content:   string(data),
		Line:      line,
		Truncated: truncated,
	}, nil
}
