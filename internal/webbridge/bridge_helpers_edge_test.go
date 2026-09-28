package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// ---------- attachment helpers ----------

func TestAttachmentKindAndImagePath(t *testing.T) {
	cases := []struct {
		path, mime, want string
	}{
		{"a.png", "image/png", "image"},
		{"a.txt", "image/webp", "image"},
		{"a.pdf", "", "pdf"},
		{"a.bin", "application/pdf", "pdf"},
		{"a.docx", "", "document"},
		{"a.bin", "application/msword", "document"},
		{"a.xlsx", "", "spreadsheet"},
		{"a.csv", "", "spreadsheet"},
		{"a.bin", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "spreadsheet"},
		{"a.txt", "text/plain", "file"},
	}
	for _, tc := range cases {
		if got := attachmentKindFromPath(tc.path, tc.mime); got != tc.want {
			t.Fatalf("kind(%q,%q) = %q, want %q", tc.path, tc.mime, got, tc.want)
		}
	}

	for _, p := range []string{"a.PNG", "b.jpg", "c.jpeg", "d.webp", "e.gif", "f.bmp"} {
		if !isImageAttachmentPath(p) {
			t.Fatalf("%q should be image", p)
		}
	}
	if isImageAttachmentPath("a.txt") || isImageAttachmentPath("") {
		t.Fatal("non-image")
	}
}

func TestDetectAttachmentImageMIME(t *testing.T) {
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0)
	if typ, ok := detectAttachmentImageMIME(png); !ok || typ != "image/png" {
		t.Fatalf("png = %q %v", typ, ok)
	}
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0}
	if typ, ok := detectAttachmentImageMIME(jpeg); !ok || typ != "image/jpeg" {
		t.Fatalf("jpeg = %q %v", typ, ok)
	}
	if typ, ok := detectAttachmentImageMIME([]byte("GIF87a....")); !ok || typ != "image/gif" {
		t.Fatalf("gif87 = %q %v", typ, ok)
	}
	if typ, ok := detectAttachmentImageMIME([]byte("GIF89a....")); !ok || typ != "image/gif" {
		t.Fatalf("gif89 = %q %v", typ, ok)
	}
	webp := []byte("RIFF\x00\x00\x00\x00WEBP")
	if typ, ok := detectAttachmentImageMIME(webp); !ok || typ != "image/webp" {
		t.Fatalf("webp = %q %v", typ, ok)
	}
	if typ, ok := detectAttachmentImageMIME([]byte("BMxx")); !ok || typ != "image/bmp" {
		t.Fatalf("bmp = %q %v", typ, ok)
	}
	if _, ok := detectAttachmentImageMIME([]byte("plain")); ok {
		t.Fatal("plain should not match")
	}
	if _, ok := detectAttachmentImageMIME(nil); ok {
		t.Fatal("empty")
	}
}

func TestNormalizeImportedImageMIMEAndDataURL(t *testing.T) {
	for mime, wantExt := range map[string]string{
		"image/png":  ".png",
		"IMAGE/JPEG": ".jpg",
		"image/jpg":  ".jpg",
		"image/webp": ".webp",
		"image/gif":  ".gif",
		"image/bmp":  ".bmp",
		"image/x-ms-bmp": ".bmp",
	} {
		got, ext, ok := normalizeImportedImageMIME(mime)
		if !ok || ext != wantExt || got == "" {
			t.Fatalf("normalize(%q) = %q %q %v", mime, got, ext, ok)
		}
	}
	if _, _, ok := normalizeImportedImageMIME("text/plain"); ok {
		t.Fatal("non-image")
	}

	mime, body, ok := splitAttachmentDataURL("data:image/png;base64,AAAA")
	if !ok || mime != "image/png" || body != "AAAA" {
		t.Fatalf("data url = %q %q %v", mime, body, ok)
	}
	mime, body, ok = splitAttachmentDataURL("data:image/jpeg,raw")
	if !ok || mime != "image/jpeg" || body != "raw" {
		t.Fatalf("data url no b64 = %q %q %v", mime, body, ok)
	}
	if _, _, ok := splitAttachmentDataURL("not-a-data-url"); ok {
		t.Fatal("plain should fail")
	}
	if _, body, ok := splitAttachmentDataURL("plain"); ok || body != "plain" {
		t.Fatalf("plain fallback = %q %v", body, ok)
	}
}

func TestSafeAttachmentFilename(t *testing.T) {
	if got := safeAttachmentFilename("a.png", ".png"); got != "a.png" {
		t.Fatalf("safe = %q", got)
	}
	// 非法字符替换
	got := safeAttachmentFilename(`a<b>c:d"e|f?g*h`, ".png")
	if strings.ContainsAny(got, `<>:"/\|?*`) {
		t.Fatalf("illegal chars kept: %q", got)
	}
	// 空名回落
	if got := safeAttachmentFilename("", ".jpg"); got != "clipboard-image.jpg" {
		t.Fatalf("empty name = %q", got)
	}
	if got := safeAttachmentFilename(".", ".png"); got != "clipboard-image.png" {
		t.Fatalf("dot name = %q", got)
	}
	// 无扩展名时补 .png
	if got := safeAttachmentFilename("x", ""); !strings.HasSuffix(got, ".png") {
		t.Fatalf("default ext = %q", got)
	}
	// 超长截断
	long := strings.Repeat("a", 200) + ".png"
	if got := safeAttachmentFilename(long, ".png"); len([]rune(strings.TrimSuffix(got, ".png"))) > 120 {
		t.Fatalf("too long = %q", got)
	}
	// 控制字符
	if got := safeAttachmentFilename("a\x01b", ".png"); strings.ContainsRune(got, 0x01) {
		t.Fatalf("control kept = %q", got)
	}
}

func TestMakeAttachmentsAndImagePaths(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "a.png")
	txtPath := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(pngPath, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txtPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	atts := makeAttachments([]string{pngPath, txtPath, ""})
	if len(atts) != 2 {
		t.Fatalf("atts = %d", len(atts))
	}
	if atts[0].Kind != "image" {
		t.Fatalf("png kind = %q", atts[0].Kind)
	}

	imgs := imagePathsFromAttachments([]AttachmentRef{
		{Path: pngPath}, {Path: txtPath}, {Path: ""},
	})
	if len(imgs) != 1 || imgs[0] != pngPath {
		t.Fatalf("image paths = %v", imgs)
	}

	pngPath2 := filepath.Join(dir, "c.png")
	if err := os.WriteFile(pngPath2, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatal(err)
	}
	rt := imagePathsFromRuntimeAttachments([]adapter.Attachment{
		{Kind: "image", Path: pngPath},
		{Kind: "Image", Path: pngPath2},
		{Kind: "file", Path: txtPath},
		{Kind: "image", Path: "  "},
	})
	if len(rt) != 2 {
		t.Fatalf("runtime images = %v", rt)
	}
}

func TestCopyAttachmentIntoWorkspace(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "in.png")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 空 workspace 原样返回
	got, err := copyAttachmentIntoWorkspace(src, "")
	if err != nil || got != src {
		t.Fatalf("empty ws = %q %v", got, err)
	}

	ws := t.TempDir()
	got, err = copyAttachmentIntoWorkspace(src, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, filepath.Join(ws, ".eos", "attachments")) {
		t.Fatalf("target = %q", got)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != "data" {
		t.Fatalf("copied = %q %v", data, err)
	}

	// 源缺失
	if _, err := copyAttachmentIntoWorkspace(filepath.Join(srcDir, "nope.png"), ws); err == nil {
		t.Fatal("missing source")
	}
}

func TestDetectAttachmentMIMEAndCacheDir(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "x")
	if err := os.WriteFile(png, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatal(err)
	}
	// 无扩展名靠内容嗅探
	if typ := detectAttachmentMIME(png); typ != "image/png" {
		t.Fatalf("sniff = %q", typ)
	}
	// 有扩展名用 mime.TypeByExtension
	txt := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if typ := detectAttachmentMIME(txt); !strings.Contains(typ, "text") {
		t.Fatalf("ext mime = %q", typ)
	}

	if dir := attachmentCacheDir(); dir == "" {
		t.Fatal("cache dir empty")
	}
}

// ---------- automation / paths ----------

func TestValidateCronScheduleAndNextRun(t *testing.T) {
	// 空 = 手动模板合法
	if err := validateCronSchedule(""); err != nil {
		t.Fatalf("empty schedule: %v", err)
	}
	if err := validateCronSchedule("  "); err != nil {
		t.Fatal("blank schedule")
	}
	if err := validateCronSchedule("*/5 * * * *"); err != nil {
		t.Fatalf("valid cron: %v", err)
	}
	if err := validateCronSchedule("not a cron"); err == nil {
		t.Fatal("invalid cron should error")
	}

	if _, ok := nextCronRun(""); ok {
		t.Fatal("empty next")
	}
	if _, ok := nextCronRun("bad"); ok {
		t.Fatal("bad next")
	}
	next, ok := nextCronRun("0 0 * * *")
	if !ok || next.IsZero() {
		t.Fatalf("next = %v %v", next, ok)
	}
	if !next.After(time.Now().Add(-time.Second)) {
		t.Fatalf("next should be future: %v", next)
	}
}

func TestSlogLoggerPrintf(t *testing.T) {
	// 不 panic
	slogLogger{}.Printf("hello %s", "world")
}

func TestLanguageFromPathExtMatrix(t *testing.T) {
	cases := map[string]string{
		"a.go":    "go",
		"a.ts":    "tsx",
		"a.tsx":   "tsx",
		"a.js":    "javascript",
		"a.mjs":   "javascript",
		"a.css":   "css",
		"a.json":  "json",
		"a.md":    "markdown",
		"a.html":  "html",
		"a.yaml":  "yaml",
		"a.toml":  "toml",
		"a.rs":    "rust",
		"a.py":    "python",
		"a.java":  "java",
		"a.sql":   "sql",
		"a.sh":    "bash",
		"a.c":     "c",
		"a.cpp":   "cpp",
		"a.rb":    "ruby",
		"a.php":   "php",
		"a.unknown": "text",
		"noext":     "text",
	}
	for path, want := range cases {
		if got := languageFromPath(path); got != want {
			t.Fatalf("lang(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestPathWithinRootAndInsideWorkspace(t *testing.T) {
	if !pathWithinRoot("/ws/a", "/ws") {
		t.Fatal("inside")
	}
	if pathWithinRoot("/other/a", "/ws") {
		t.Fatal("outside")
	}
	if !pathWithinRoot("/ws", "/ws") {
		t.Fatal("self")
	}
	if pathWithinRoot("/ws-evil/a", "/ws") {
		t.Fatal("prefix sibling")
	}

	if !pathInsideWorkspace("/ws/a", "/ws") {
		t.Fatal("inside ws")
	}
	if pathInsideWorkspace("/other", "/ws") {
		t.Fatal("outside ws")
	}
	if pathInsideWorkspace("", "/ws") || pathInsideWorkspace("/ws", "") {
		t.Fatal("empty")
	}

	// pathWithinAnyRoot
	ws := t.TempDir()
	inside := filepath.Join(ws, "f.txt")
	if err := os.WriteFile(inside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !pathWithinAnyRoot(inside, []string{ws}) {
		t.Fatal("any root inside")
	}
	if pathWithinAnyRoot(inside, []string{t.TempDir()}) {
		t.Fatal("any root outside")
	}
}
