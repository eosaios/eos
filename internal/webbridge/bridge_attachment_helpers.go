package webbridge

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func copyAttachmentIntoWorkspace(source string, workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return source, nil
	}
	ext := filepath.Ext(source)
	safeName := safeAttachmentFilename(filepath.Base(source), ext)
	targetDir := filepath.Join(workspace, ".eos", "attachments")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(targetDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), safeName))
	data, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	if len(data) > maxGeneralAttachmentBytes {
		return "", fmt.Errorf("附件超过 %dMB 限制: %s", maxGeneralAttachmentBytes/(1024*1024), filepath.Base(source))
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return "", err
	}
	return target, nil
}

func detectAttachmentMIME(path string) string {
	if ext := strings.ToLower(filepath.Ext(path)); ext != "" {
		if typ := mime.TypeByExtension(ext); strings.TrimSpace(typ) != "" {
			return typ
		}
	}
	if data, err := os.ReadFile(path); err == nil {
		if typ, ok := detectAttachmentImageMIME(data); ok {
			return typ
		}
	}
	return ""
}

func attachmentKindFromPath(path string, mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.Contains(mimeType, "pdf") || ext == ".pdf":
		return "pdf"
	case strings.Contains(mimeType, "word") || ext == ".doc" || ext == ".docx":
		return "document"
	case strings.Contains(mimeType, "spreadsheet") || ext == ".xls" || ext == ".xlsx" || ext == ".csv" || ext == ".tsv":
		return "spreadsheet"
	default:
		return "file"
	}
}

func makeAttachments(paths []string) []AttachmentRef {
	out := make([]AttachmentRef, 0, len(paths))
	for _, item := range compactPaths(paths) {
		mimeType := detectAttachmentMIME(item)
		out = append(out, AttachmentRef{
			Name: filepath.Base(item),
			Path: item,
			MIME: mimeType,
			Kind: attachmentKindFromPath(item, mimeType),
		})
	}
	return out
}

func imagePathsFromAttachments(attachments []AttachmentRef) []string {
	paths := make([]string, 0, len(attachments))
	for _, item := range attachments {
		path := strings.TrimSpace(item.Path)
		if path == "" || !isImageAttachmentPath(path) {
			continue
		}
		paths = append(paths, path)
	}
	return compactPaths(paths)
}

func imagePathsFromRuntimeAttachments(attachments []adapter.Attachment) []string {
	paths := make([]string, 0, len(attachments))
	for _, item := range attachments {
		if strings.EqualFold(strings.TrimSpace(item.Kind), "image") && strings.TrimSpace(item.Path) != "" {
			paths = append(paths, item.Path)
		}
	}
	return compactPaths(paths)
}

func isImageAttachmentPath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp":
		return true
	default:
		return false
	}
}

func detectAttachmentImageMIME(data []byte) (string, bool) {
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png", true
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg", true
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "image/gif", true
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp", true
	}
	if len(data) >= 2 && string(data[:2]) == "BM" {
		return "image/bmp", true
	}
	return "", false
}

func normalizeImportedImageMIME(mime string) (string, string, bool) {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return "image/png", ".png", true
	case "image/jpeg", "image/jpg":
		return "image/jpeg", ".jpg", true
	case "image/webp":
		return "image/webp", ".webp", true
	case "image/gif":
		return "image/gif", ".gif", true
	case "image/bmp", "image/x-ms-bmp":
		return "image/bmp", ".bmp", true
	default:
		return "", "", false
	}
}

func splitAttachmentDataURL(value string) (string, string, bool) {
	header, body, found := strings.Cut(strings.TrimSpace(value), ",")
	if !found || !strings.HasPrefix(strings.ToLower(header), "data:") {
		return "", value, false
	}
	mime := header[len("data:"):]
	if semicolon := strings.Index(mime, ";"); semicolon >= 0 {
		mime = mime[:semicolon]
	}
	return mime, body, true
}

func safeAttachmentFilename(name, ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		ext = ".png"
	}
	name = filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "." || name == string(filepath.Separator) {
		name = ""
	}
	if strings.TrimSpace(name) == "" {
		name = "clipboard-image" + ext
	}
	if filepath.Ext(name) != "" {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r < 32:
			builder.WriteByte('-')
		case strings.ContainsRune(`<>:"/\|?*`, r):
			builder.WriteByte('-')
		default:
			builder.WriteRune(r)
		}
	}
	base := strings.Trim(strings.TrimSpace(builder.String()), ". ")
	if base == "" {
		base = "clipboard-image"
	}
	if len([]rune(base)) > 120 {
		base = string([]rune(base)[:120])
	}
	return base + ext
}

func attachmentCacheDir() string {
	if UsesWorkspaceState() {
		return filepath.Join(DevStateDir(), "attachments")
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".eos", "gui-attachments")
	}
	return filepath.Join(os.TempDir(), "eos-gui", "attachments")
}
