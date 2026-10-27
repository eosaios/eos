package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"errors"
	"os"
	"sort"
	"strings"
	"time"
)

// workspaceIgnoredDirNames 是工作区文件树默认不展示的目录名集合：
// 版本库元数据、依赖缓存与构建产物，体量大且对内容查看无意义。
var workspaceIgnoredDirNames = map[string]struct{}{
	".git": {}, ".hg": {}, ".svn": {},
	"node_modules": {}, "vendor": {}, "target": {},
	"dist": {}, "build": {}, "out": {}, "bin": {}, "obj": {},
	".venv": {}, "venv": {}, "__pycache__": {},
	".idea": {}, ".vscode": {}, ".next": {}, ".cache": {},
}

type WorkspaceFilesService struct {
	bridge *BridgeService
}

func NewWorkspaceFilesService(bridge *BridgeService) *WorkspaceFilesService {
	return &WorkspaceFilesService{bridge: bridge}
}

func (s *BridgeService) workspaceFilesService() *WorkspaceFilesService {
	if s == nil {
		return NewWorkspaceFilesService(nil)
	}
	s.serviceMu.Lock()
	defer s.serviceMu.Unlock()
	if s.workspaceFilesSvc == nil {
		s.workspaceFilesSvc = NewWorkspaceFilesService(s)
	}
	return s.workspaceFilesSvc
}

// ListWorkspaceDirectory 列出工作区内目录的一层内容，供右侧预览面板的
// 文件树懒展开使用。relPath 为空或 "." 表示工作区根目录；路径经
// resolveWorkspaceFilePath 沙箱校验，工作区外路径一律拒绝。
func (svc *WorkspaceFilesService) ListWorkspaceDirectory(relPath string) (DirectoryListing, error) {
	s := svc.bridge
	if s == nil {
		return DirectoryListing{}, errors.New("bridge service is not available")
	}
	trimmed := strings.TrimSpace(relPath)
	if trimmed == "" {
		trimmed = "."
	}
	target, err := s.resolveWorkspaceFilePath(trimmed)
	if err != nil {
		return DirectoryListing{}, err
	}
	dirEntries, err := os.ReadDir(target)
	if err != nil {
		return DirectoryListing{}, err
	}
	listing := DirectoryListing{
		Path:    target,
		Entries: make([]DirectoryEntry, 0, len(dirEntries)),
	}
	for _, dirEntry := range dirEntries {
		name := dirEntry.Name()
		if dirEntry.IsDir() {
			if _, ignored := workspaceIgnoredDirNames[name]; ignored {
				continue
			}
		}
		info, err := dirEntry.Info()
		if err != nil {
			// 目录扫描与文件系统变更并发，条目在 Stat 前被删除属正常竞态，跳过。
			continue
		}
		listing.Entries = append(listing.Entries, DirectoryEntry{
			Name:    name,
			IsDir:   dirEntry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
	}
	sortDirectoryEntries(listing.Entries)
	return listing, nil
}

func sortDirectoryEntries(entries []DirectoryEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

// joinWorkspaceTreePath 计算文件树子节点的相对路径（slash 分隔，供下一次
// ListWorkspaceDirectory / PreviewWorkspaceFile 使用）。
func joinWorkspaceTreePath(parent string, name string) string {
	if parent == "" || parent == "." {
		return name
	}
	return parent + "/" + name
}
