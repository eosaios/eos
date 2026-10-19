// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

// Package attachments handles local file transport shared by EOS clients.
package attachments

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi"
)

const MaxBytes = 50 * 1024 * 1024

type File struct {
	Name string
	Path string
	MIME string
	Kind string
}

// Store saves an uploaded file in a private, unique cache directory.
func Store(cacheDir, name string, data []byte) (File, error) {
	if len(data) > MaxBytes {
		return File{}, fmt.Errorf("attachment exceeds 50MB limit")
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		name = "attachment"
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return File{}, err
	}
	dir, err := os.MkdirTemp(cacheDir, "import-")
	if err != nil {
		return File{}, err
	}
	target := filepath.Join(dir, name)
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return File{}, err
	}
	return Describe(target, data), nil
}

// Describe identifies a file without changing its contents or extension.
func Describe(path string, data []byte) File {
	typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if typ == "" {
		typ = http.DetectContentType(data)
	}
	kind := "file"
	switch {
	case strings.HasPrefix(typ, "image/"):
		kind = "image"
	case strings.Contains(typ, "pdf"):
		kind = "pdf"
	case strings.Contains(typ, "word"):
		kind = "document"
	case strings.Contains(typ, "spreadsheet"), strings.HasSuffix(strings.ToLower(path), ".csv"), strings.HasSuffix(strings.ToLower(path), ".tsv"):
		kind = "spreadsheet"
	}
	return File{Name: filepath.Base(path), Path: path, MIME: typ, Kind: kind}
}

// Encode reads a bounded file and encodes the model attachment payload.
// coreapi.Attachment.Path carries base64 bytes, never a local filesystem path.
func Encode(file File) (coreapi.Attachment, error) {
	info, err := os.Stat(file.Path)
	if err != nil {
		return coreapi.Attachment{}, err
	}
	if !info.Mode().IsRegular() {
		return coreapi.Attachment{}, fmt.Errorf("attachment must be a regular file: %s", file.Path)
	}
	src, err := os.Open(file.Path)
	if err != nil {
		return coreapi.Attachment{}, err
	}
	defer src.Close()
	info, err = src.Stat()
	if err != nil {
		return coreapi.Attachment{}, err
	}
	if !info.Mode().IsRegular() {
		return coreapi.Attachment{}, fmt.Errorf("attachment must be a regular file: %s", file.Path)
	}
	if info.Size() > MaxBytes {
		return coreapi.Attachment{}, fmt.Errorf("attachment exceeds 50MB limit: %s", file.Path)
	}
	data, err := io.ReadAll(io.LimitReader(src, MaxBytes+1))
	if err != nil {
		return coreapi.Attachment{}, err
	}
	if len(data) > MaxBytes {
		return coreapi.Attachment{}, fmt.Errorf("attachment exceeds 50MB limit: %s", file.Path)
	}
	description := Describe(file.Path, data)
	if file.Name != "" {
		description.Name = file.Name
	}
	if file.MIME != "" {
		description.MIME = file.MIME
	}
	if file.Kind != "" {
		description.Kind = file.Kind
	}
	return coreapi.Attachment{Name: description.Name, Path: base64.StdEncoding.EncodeToString(data), MIME: description.MIME, Kind: description.Kind}, nil
}
