package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func (svc *AttachmentService) PrepareForSession(paths []string, workspace string) ([]AttachmentRef, []adapter.Attachment, error) {
	compact := compactPaths(paths)
	refs := make([]AttachmentRef, 0, len(compact))
	runtimeAttachments := make([]adapter.Attachment, 0, len(compact))
	for _, item := range compact {
		ref, runtimeAttachment, err := svc.PrepareOneForSession(item, workspace)
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, ref)
		runtimeAttachments = append(runtimeAttachments, runtimeAttachment)
	}
	return refs, runtimeAttachments, nil
}

func (svc *AttachmentService) PrepareOneForSession(path string, workspace string) (AttachmentRef, adapter.Attachment, error) {
	if svc.bridge == nil {
		return AttachmentRef{}, adapter.Attachment{}, errors.New("bridge service is not available")
	}
	original := strings.TrimSpace(path)
	if original == "" {
		return AttachmentRef{}, adapter.Attachment{}, errors.New("attachment path is required")
	}
	source := original
	if !filepath.IsAbs(source) && strings.TrimSpace(workspace) != "" {
		source = filepath.Join(workspace, source)
	}
	source = filepath.Clean(source)
	info, err := os.Stat(source)
	if err != nil {
		return AttachmentRef{}, adapter.Attachment{}, err
	}
	if info.IsDir() {
		return AttachmentRef{}, adapter.Attachment{}, errors.New("attachment path must be a file: " + source)
	}
	if info.Size() > maxGeneralAttachmentBytes {
		return AttachmentRef{}, adapter.Attachment{}, fmt.Errorf("attachment exceeds %dMB limit: %s", maxGeneralAttachmentBytes/(1024*1024), filepath.Base(source))
	}

	target := source
	copied := false
	if strings.TrimSpace(workspace) != "" && !pathInsideWorkspace(source, workspace) {
		copiedPath, err := copyAttachmentIntoWorkspace(source, workspace)
		if err != nil {
			return AttachmentRef{}, adapter.Attachment{}, err
		}
		target = copiedPath
		copied = true
	}
	mimeType := detectAttachmentMIME(target)
	kind := attachmentKindFromPath(target, mimeType)
	workspacePath := ""
	if strings.TrimSpace(workspace) != "" {
		if rel, err := filepath.Rel(filepath.Clean(workspace), filepath.Clean(target)); err == nil && !strings.HasPrefix(rel, "..") {
			workspacePath = filepath.ToSlash(rel)
		}
	}
	name := filepath.Base(target)
	ref := AttachmentRef{
		Name:          name,
		Path:          target,
		MIME:          mimeType,
		Kind:          kind,
		WorkspacePath: workspacePath,
		OriginalPath:  original,
		Copied:        copied,
	}
	return ref, adapter.Attachment{
		Name: name,
		Path: target,
		MIME: mimeType,
		Kind: kind,
	}, nil
}
