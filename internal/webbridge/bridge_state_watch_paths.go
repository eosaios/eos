package webbridge

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// 状态文件监听：watcher 装配 + 监听目录聚合。
// 版本目录解析见 bridge_state_watch_versions.go，
// 路径 helper 见 bridge_state_watch_helpers.go，
// 事件过滤见 bridge_state_watch_filter.go。

type statePathWatcher struct {
	watcher *fsnotify.Watcher
	paths   map[string]struct{}
}

func newStatePathWatcher(watcher *fsnotify.Watcher) *statePathWatcher {
	return &statePathWatcher{
		watcher: watcher,
		paths:   map[string]struct{}{},
	}
}

func (w *statePathWatcher) reconcile(paths []string) {
	if w == nil || w.watcher == nil {
		return
	}
	next := map[string]struct{}{}
	for _, path := range paths {
		path = cleanExistingDir(path)
		if path == "" {
			continue
		}
		next[path] = struct{}{}
		if _, ok := w.paths[path]; ok {
			continue
		}
		if err := w.watcher.Add(path); err != nil {
			slog.Debug("bridge.state_sync.watch_add_failed", "path", path, "error", err.Error())
			continue
		}
		w.paths[path] = struct{}{}
	}
	for path := range w.paths {
		if _, ok := next[path]; ok {
			continue
		}
		_ = w.watcher.Remove(path)
		delete(w.paths, path)
	}
}

func (s *BridgeService) stateWatchDirectories() []string {
	out := map[string]struct{}{}
	addDir := func(path string) {
		if dir := cleanExistingDir(path); dir != "" {
			out[dir] = struct{}{}
		}
	}
	addFileParent := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		addDir(filepath.Dir(path))
	}

	if s != nil {
		addFileParent(s.coreConfigPathReadOnly())
		addFileParent(ResolveWorkspaceSettingsPath(""))

		workspaceSet := map[string]struct{}{}
		addWorkspace := func(path string) {
			path = strings.TrimSpace(path)
			if path != "" {
				workspaceSet[path] = struct{}{}
			}
		}
		addWorkspace(s.activeWorkspaceValue())
		addWorkspace(s.startupWorkspace)
		addWorkspace(s.defaultWorkspacePathReadOnly())
		addWorkspace(s.lastWorkspacePathReadOnly())
		snapshot := s.runtimeSnapshotReadOnly()
		addWorkspace(snapshot.ForegroundWorkspace)
		for _, workspace := range snapshot.Workspaces {
			addWorkspace(workspace.Path)
		}

		for workspace := range workspaceSet {
			addDir(workspace)
			eosDir := filepath.Join(workspace, ".eos")
			addDir(eosDir)
			addDir(filepath.Join(eosDir, "sessions"))
			addDir(filepath.Join(eosDir, "worktrees"))
			addDir(existingVersionWorkspaceRoot(workspace))
			addFileParent(ResolveWorkspaceSettingsPath(workspace))

		}

		for _, skill := range s.skillsReadOnly() {
			addPathOrParent(out, skill.BaseDir)
			addPathOrParent(out, skill.Location)
		}
		for _, plugin := range s.pluginsReadOnly() {
			addPathOrParent(out, pluginSourcePath(plugin.Source))
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		addDir(filepath.Join(home, ".eos"))
	}

	paths := make([]string, 0, len(out))
	for path := range out {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
