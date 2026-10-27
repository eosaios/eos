package webbridge

import (
	"errors"
	"strings"
)

func (w *WorkspaceService) ListRemoteWorkspaces() []WorkspaceCard {
	s := w.bridge
	if s == nil {
		return []WorkspaceCard{}
	}
	return s.loadRemoteWorkspaces(s.activeWorkspaceValue())
}

func (w *WorkspaceService) OpenRemoteWorkspace(idOrPath string) (BootstrapState, error) {
	s := w.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	item, err := s.openRemoteWorkspaceRPC(idOrPath)
	if err != nil {
		return s.LoadBootstrap(), err
	}
	state, err := w.SelectWorkspace(item.LocalPath)
	if err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return state, nil
}

func (w *WorkspaceService) ForgetRemoteWorkspace(idOrPath string) (BootstrapState, error) {
	s := w.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.forgetRemoteWorkspaceRPC(idOrPath); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (w *WorkspaceService) ClearRemoteWorkspaceCache(idOrPath string) (BootstrapState, error) {
	s := w.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	if err := s.clearRemoteWorkspaceCacheRPC(idOrPath); err != nil {
		return s.LoadBootstrap(), err
	}
	s.stateMu.Lock()
	s.emitShellUpdated()
	s.stateMu.Unlock()
	return s.LoadBootstrap(), nil
}

func (w *WorkspaceService) StartRemoteRepoFlow(req RemoteRepoFlowRequest) (BootstrapState, error) {
	s := w.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	repoURL := strings.TrimSpace(req.RepoURL)
	if repoURL == "" {
		return s.LoadBootstrap(), errors.New("远程仓库 URL 不能为空")
	}
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		goal = "打开仓库，检查状态，并等待我的下一步指令。"
	}
	parts := []string{
		"请在当前本地会话里临时处理这个远程仓库，不要把当前工作区强制切换过去。",
		"仓库：" + repoURL,
	}
	if platform := strings.TrimSpace(req.Platform); platform != "" {
		parts = append(parts, "平台："+platform)
	}
	if branch := strings.TrimSpace(req.Branch); branch != "" {
		parts = append(parts, "分支："+branch)
	}
	parts = append(parts,
		"请使用 remote_repo_* 工具完成 clone/open、checkout、提交/推送和 PR/MR 等远程仓库操作；需要授权、推送、创建 PR/MR 或清理缓存时走 GUI 审批。",
		"目标："+goal,
	)

	s.stateMu.RLock()
	sessionID := strings.TrimSpace(s.currentSessionID)
	workspace := strings.TrimSpace(s.activeWorkspace)
	s.stateMu.RUnlock()
	if sessionID == "" {
		bootstrap := s.LoadBootstrap()
		sessionID = strings.TrimSpace(bootstrap.CurrentSessionID)
	}
	return s.chatService().SendChatWithReasoning(sessionID, workspace, strings.Join(parts, "\n"), nil, "")
}
