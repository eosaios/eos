package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxFileSize 默认最大文件大小限制 (10MB)
	MaxFileSize = 10 * 1024 * 1024

	// BinaryCheckBytes 读取字节数用于二进制检测
	BinaryCheckBytes = 512

	// MaxBinaryNullRatio 二进制文件空字节比例阈值
	MaxBinaryNullRatio = 0.03
)

// 文件签名（魔术字节）用于二进制文件检测
var binarySignatures = [][]byte{
	// PDF
	{0x25, 0x50, 0x44, 0x46},
	// ZIP (including JAR, DOCX, XLSX, etc.)
	{0x50, 0x4B, 0x03, 0x04},
	{0x50, 0x4B, 0x05, 0x06},
	{0x50, 0x4B, 0x07, 0x08},
	// RAR
	{0x52, 0x61, 0x72, 0x21},
	// 7z
	{0x37, 0x7A, 0xBC, 0xAF},
	// PNG
	{0x89, 0x50, 0x4E, 0x47},
	// JPEG
	{0xFF, 0xD8, 0xFF},
	// GIF
	{0x47, 0x49, 0x46},
	// WebP
	{0x52, 0x49, 0x46, 0x46},
	// BMP
	{0x42, 0x4D},
	// TIFF (little endian)
	{0x49, 0x49, 0x2A, 0x00},
	// TIFF (big endian)
	{0x4D, 0x4D, 0x00, 0x2A},
	// ICO
	{0x00, 0x00, 0x01, 0x00},
	// ELF (Linux executable)
	{0x7F, 0x45, 0x4C, 0x46},
	// PE (Windows executable)
	{0x4D, 0x5A},
	// Mach-O (macOS executable)
	{0xFE, 0xED, 0xFA, 0xCF},
	{0xFE, 0xED, 0xFA, 0xCE},
	{0xCE, 0xFA, 0xED, 0xFE},
	{0xCF, 0xfa, 0xed, 0xfe},
	// WAV
	{0x52, 0x49, 0x46, 0x46},
	// AVI
	{0x52, 0x49, 0x46, 0x46},
	// MP3
	{0x49, 0x44, 0x33},
	{0xFF, 0xFB},
	{0xFF, 0xFA},
	{0xFF, 0xF3},
	// MP4
	{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70},
	{0x00, 0x00, 0x00, 0x1C, 0x66, 0x74, 0x79, 0x70},
	{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70},
	// SQLite database
	{0x53, 0x51, 0x4C, 0x69},
}

// 已知文本文件扩展名
var textExtensions = map[string]bool{
	".txt":           true,
	".md":            true,
	".markdown":      true,
	".go":            true,
	".js":            true,
	".ts":            true,
	".jsx":           true,
	".tsx":           true,
	".py":            true,
	".html":          true,
	".htm":           true,
	".css":           true,
	".scss":          true,
	".sass":          true,
	".less":          true,
	".json":          true,
	".yaml":          true,
	".yml":           true,
	".xml":           true,
	".toml":          true,
	".ini":           true,
	".conf":          true,
	".cfg":           true,
	".sh":            true,
	".bash":          true,
	".zsh":           true,
	".fish":          true,
	".bat":           true,
	".ps1":           true,
	".psm1":          true,
	".java":          true,
	".c":             true,
	".cpp":           true,
	".cc":            true,
	".cxx":           true,
	".h":             true,
	".hpp":           true,
	".hxx":           true,
	".rs":            true,
	".rb":            true,
	".php":           true,
	".cs":            true,
	".swift":         true,
	".kt":            true,
	".kts":           true,
	".lua":           true,
	".pl":            true,
	".pm":            true,
	".r":             true,
	".sql":           true,
	".eos":           true,
	".vbs":           true,
	".dockerfile":    true,
	".env":           true,
	".gitignore":     true,
	".gitattributes": true,
	".gitmodules":    true,
	".editorconfig":  true,
	".eslintrc":      true,
	".prettierrc":    true,
	".tsbuildinfo":   true,
	"makefile":       true,
	"cmakelists.txt": true,
	".proto":         true,
	".graphql":       true,
	".gql":           true,
	".vue":           true,
	".svelte":        true,
}

// CheckFileSize 检查文件大小是否超过限制
// 返回: 文件大小, 是否超过限制, 错误
func CheckFileSize(path string, maxSize int64) (int64, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false, err
	}
	size := info.Size()
	exceeds := size > maxSize
	if exceeds {
		slog.Warn("file.check_size.exceeded",
			"component", ComponentTool,
			"path", path,
			"size", size,
			"max_size", maxSize,
		)
	}
	return size, exceeds, nil
}

// IsBinaryFile 检查文件是否为二进制文件
// 通过以下方式检测：
// 1. 检查文件扩展名
// 2. 检查魔术字节签名
// 3. 检查空字节比例
func IsBinaryFile(path string) bool {
	// 首先检查扩展名
	if isBinaryByExtension(path) {
		slog.Debug("file.is_binary.extension",
			"component", ComponentTool,
			"path", path,
		)
		return true
	}

	// 检查魔术字节
	if isBinaryBySignature(path) {
		slog.Debug("file.is_binary.signature",
			"component", ComponentTool,
			"path", path,
		)
		return true
	}

	// 检查空字节比例
	if isBinaryByNullBytes(path) {
		slog.Debug("file.is_binary.null_bytes",
			"component", ComponentTool,
			"path", path,
		)
		return true
	}

	return false
}

// isBinaryByExtension 通过扩展名判断是否为二进制文件
func isBinaryByExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))

	// 如果是已知文本扩展名，不是二进制
	if textExtensions[ext] || textExtensions[base] {
		return false
	}

	// 已知二进制扩展名
	binaryExts := map[string]bool{
		".exe":    true,
		".dll":    true,
		".so":     true,
		".dylib":  true,
		".a":      true,
		".lib":    true,
		".o":      true,
		".obj":    true,
		".bin":    true,
		".dat":    true,
		".db":     true,
		".sqlite": true,
		".pdf":    true,
		".docx":   true,
		".xlsx":   true,
		".zip":    true,
		".tar":    true,
		".gz":     true,
		".rar":    true,
		".7z":     true,
		".bz2":    true,
		".xz":     true,
		".png":    true,
		".jpg":    true,
		".jpeg":   true,
		".gif":    true,
		".webp":   true,
		".bmp":    true,
		".ico":    true,
		".svg":    true, // SVG 是文本，但通常被视为图像
		".tiff":   true,
		".tif":    true,
		".mp3":    true,
		".mp4":    true,
		".wav":    true,
		".flac":   true,
		".ogg":    true,
		".wma":    true,
		".m4a":    true,
		".aac":    true,
		".avi":    true,
		".mkv":    true,
		".mov":    true,
		".wmv":    true,
		".flv":    true,
		".webm":   true,
		".class":  true,
		".jar":    true,
		".war":    true,
		".ear":    true,
		".dex":    true,
		".apk":    true,
		".ipa":    true,
		".deb":    true,
		".rpm":    true,
		".msi":    true,
		".dmg":    true,
		".iso":    true,
		".img":    true,
		".vmdk":   true,
		".qcow2":  true,
		".woff":   true,
		".woff2":  true,
		".eot":    true,
		".ttf":    true,
		".otf":    true,
		".pem":    true,
		".crt":    true,
		".key":    true,
		".der":    true,
		".p12":    true,
		".pfx":    true,
		".pyc":    true,
		".pyo":    true,
		".pyd":    true,
		".node":   true,
	}

	return binaryExts[ext]
}

// isBinaryBySignature 通过魔术字节检测二进制文件
func isBinaryBySignature(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("file.is_binary.close.error",
				"component", ComponentTool,
				"path", path,
				"error", err,
			)
		}
	}()

	buf := make([]byte, BinaryCheckBytes)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return false
	}
	buf = buf[:n]

	for _, sig := range binarySignatures {
		if bytes.HasPrefix(buf, sig) {
			return true
		}
	}

	return false
}

// isBinaryByNullBytes 通过空字节比例检测二进制文件
func isBinaryByNullBytes(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("file.is_binary.close.error",
				"component", ComponentTool,
				"path", path,
				"error", err,
			)
		}
	}()

	buf := make([]byte, BinaryCheckBytes)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return false
	}
	buf = buf[:n]

	nullCount := 0
	for _, b := range buf {
		if b == 0 {
			nullCount++
		}
	}

	ratio := float64(nullCount) / float64(n)
	return ratio > MaxBinaryNullRatio
}

// ValidateFileForRead 验证文件是否适合读取
// 检查：文件存在、是否为二进制、大小限制
// 返回: 是否有效、文件大小、错误消息
func ValidateFileForRead(path string, maxSize int64) (bool, int64, string) {
	// 检查文件是否存在
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, "file not found"
		}
		return false, 0, "cannot access file"
	}

	// 检查是否为目录
	if info.IsDir() {
		return false, 0, "path is a directory, not a file"
	}

	// 检查二进制文件
	if IsBinaryFile(path) {
		return false, info.Size(), "binary file detected, cannot read"
	}

	// 检查文件大小
	size := info.Size()
	if size > maxSize {
		return false, size, "file too large"
	}

	return true, size, ""
}

// ReadFileLimited 读取文件内容，带大小限制
// 如果文件超过限制，返回截断内容和错误
func ReadFileLimited(path string, maxSize int64) (string, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}

	size := info.Size()
	if size == 0 {
		return "", 0, nil
	}

	if size <= maxSize {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", size, err
		}
		return string(content), size, nil
	}

	// 文件太大，只读取前 maxSize 字节
	file, err := os.Open(path)
	if err != nil {
		return "", size, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("file.read_limited.close.error",
				"component", ComponentTool,
				"path", path,
				"error", err,
			)
		}
	}()

	buf := make([]byte, maxSize)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", size, err
	}

	slog.Warn("file.read_limited.truncated",
		"component", ComponentTool,
		"path", path,
		"size", size,
		"read", n,
		"max_size", maxSize,
	)

	return string(buf[:n]), size, &FileTooLargeError{
		Path:     path,
		Size:     size,
		MaxSize:  maxSize,
		ReadSize: int64(n),
	}
}

// FileTooLargeError 文件过大错误
type FileTooLargeError struct {
	Path     string
	Size     int64
	MaxSize  int64
	ReadSize int64
}

// Error 实现 error 接口
func (e *FileTooLargeError) Error() string {
	return "file too large: " + e.Path
}

// IsFileTooLarge 检查错误是否为文件过大错误
func IsFileTooLarge(err error) bool {
	_, ok := err.(*FileTooLargeError)
	return ok
}
