package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
)

// WorkspaceGitRepoCard 是提交/推送操作台里单个仓库的卡片：仓库根、显示名、
// 是否主仓库 + 与顶栏徽标同构的概览。
type WorkspaceGitRepoCard struct {
	Root    string              `json:"root"`
	Name    string              `json:"name,omitempty"`
	Primary bool                `json:"primary"`
	Summary WorkspaceGitSummary `json:"summary"`
}

// WorkspaceGitRepos 是 git 操作台的数据源。Ok=false 表示内核过旧（无
// git/repos 方法）或查询失败：前端静默隐藏操作台，只保留顶栏徽标；
// Ok=true 且 Repos 为空表示当前工作区没有 git 仓库。
type WorkspaceGitRepos struct {
	Ok    bool                   `json:"ok"`
	Error string                 `json:"error,omitempty"`
	Repos []WorkspaceGitRepoCard `json:"repos"`
}

// GetWorkspaceGitRepos 枚举工作区相关仓库（主仓库 + 一级子仓库）及各自
// 概览。变更统计来自各仓库自身的 git status（.gitignore 由 git 生效）。
func (s *BridgeService) GetWorkspaceGitRepos(workspaceRoot string) WorkspaceGitRepos {
	if s == nil {
		return WorkspaceGitRepos{}
	}
	gateway := s.runtimeGatewayClient()
	if gateway == nil {
		return WorkspaceGitRepos{}
	}
	result, err := gateway.CoreGitReposRPC(coreCtx(), strings.TrimSpace(workspaceRoot))
	if err != nil {
		return WorkspaceGitRepos{Error: err.Error()}
	}
	repos := make([]WorkspaceGitRepoCard, 0, len(result.Repos))
	for _, repo := range result.Repos {
		changes := make([]WorkspaceGitChange, 0, len(repo.Summary.Changes))
		for _, change := range repo.Summary.Changes {
			changes = append(changes, WorkspaceGitChange{
				Path:   strings.TrimSpace(change.Path),
				State:  strings.TrimSpace(change.State),
				Staged: change.Staged,
			})
		}
		repos = append(repos, WorkspaceGitRepoCard{
			Root:    strings.TrimSpace(repo.Root),
			Name:    strings.TrimSpace(repo.Name),
			Primary: repo.Primary,
			Summary: WorkspaceGitSummary{
				Ok:       true,
				Branch:   strings.TrimSpace(repo.Summary.Branch),
				Upstream: strings.TrimSpace(repo.Summary.Upstream),
				Ahead:    int(repo.Summary.Ahead),
				Behind:   int(repo.Summary.Behind),
				Changes:  changes,
			},
		})
	}
	return WorkspaceGitRepos{Ok: true, Repos: repos}
}

// GitOpResult 是无返回值 git 操作（暂存/放弃合并）的统一结果。
type GitOpResult struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// GitCommitOpResult 是 git/commit 的结果：成功带新提交 hash 与分支。
type GitCommitOpResult struct {
	GitOpResult
	Hash   string `json:"hash,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// GitPushOpResult 是 git/push 的结果。Status 取值：pushed /
// merged_then_pushed / conflict / merge_pending。
type GitPushOpResult struct {
	GitOpResult
	Status    string   `json:"status,omitempty"`
	Branch    string   `json:"branch,omitempty"`
	Conflicts []string `json:"conflicts,omitempty"`
}

// GitSuggestMessageOpResult 是 git/suggest_message 的结果：AI 生成的
// Conventional Commits 提交信息（用户确认前可编辑）。
type GitSuggestMessageOpResult struct {
	GitOpResult
	Message string `json:"message,omitempty"`
}

// GitStage 暂存（all=true 全部；否则按 paths）或取消暂存（unstage=true）。
func (s *BridgeService) GitStage(workspaceRoot string, paths []string, all, unstage bool) GitOpResult {
	if s == nil {
		return GitOpResult{}
	}
	gateway := s.runtimeGatewayClient()
	if gateway == nil {
		return GitOpResult{}
	}
	if err := gateway.CoreGitStageRPC(coreCtx(), strings.TrimSpace(workspaceRoot), paths, all, unstage); err != nil {
		return GitOpResult{Error: err.Error()}
	}
	return GitOpResult{Ok: true}
}

// GitCommit 以给定 message 提交暂存区。merge 进行中时该提交即冲突解决
// 的收尾合并提交。
func (s *BridgeService) GitCommit(workspaceRoot, message string) GitCommitOpResult {
	if s == nil {
		return GitCommitOpResult{}
	}
	gateway := s.runtimeGatewayClient()
	if gateway == nil {
		return GitCommitOpResult{}
	}
	result, err := gateway.CoreGitCommitRPC(coreCtx(), strings.TrimSpace(workspaceRoot), message)
	if err != nil {
		return GitCommitOpResult{GitOpResult: GitOpResult{Error: err.Error()}}
	}
	return GitCommitOpResult{
		GitOpResult: GitOpResult{Ok: true},
		Hash:        strings.TrimSpace(result.Hash),
		Branch:      strings.TrimSpace(result.Branch),
	}
}

// GitPush 推送：内核先 fetch，落后上游时自动 merge，冲突时返回 conflict
// 与未解决文件列表（merge 保持现场，等 AI 自动解决或人工处理）。
func (s *BridgeService) GitPush(workspaceRoot string) GitPushOpResult {
	if s == nil {
		return GitPushOpResult{}
	}
	gateway := s.runtimeGatewayClient()
	if gateway == nil {
		return GitPushOpResult{}
	}
	result, err := gateway.CoreGitPushRPC(coreCtx(), strings.TrimSpace(workspaceRoot))
	if err != nil {
		return GitPushOpResult{GitOpResult: GitOpResult{Error: err.Error()}}
	}
	conflicts := make([]string, 0, len(result.Conflicts))
	for _, conflict := range result.Conflicts {
		conflicts = append(conflicts, strings.TrimSpace(conflict))
	}
	return GitPushOpResult{
		GitOpResult: GitOpResult{Ok: true},
		Status:      strings.TrimSpace(result.Status),
		Branch:      strings.TrimSpace(result.Branch),
		Conflicts:   conflicts,
	}
}

// GitAbortMerge 放弃进行中的 merge，回到合并前状态（冲突面板的逃生口）。
func (s *BridgeService) GitAbortMerge(workspaceRoot string) GitOpResult {
	if s == nil {
		return GitOpResult{}
	}
	gateway := s.runtimeGatewayClient()
	if gateway == nil {
		return GitOpResult{}
	}
	if err := gateway.CoreGitMergeAbortRPC(coreCtx(), strings.TrimSpace(workspaceRoot)); err != nil {
		return GitOpResult{Error: err.Error()}
	}
	return GitOpResult{Ok: true}
}

// SuggestGitCommitMessage 由内核用全局默认模型一次性生成提交信息
// （Conventional Commits 格式，不带 eos 署名——署名由壳层按设置拼接）。
func (s *BridgeService) SuggestGitCommitMessage(workspaceRoot string) GitSuggestMessageOpResult {
	if s == nil {
		return GitSuggestMessageOpResult{}
	}
	gateway := s.runtimeGatewayClient()
	if gateway == nil {
		return GitSuggestMessageOpResult{}
	}
	result, err := gateway.CoreGitSuggestMessageRPC(coreCtx(), strings.TrimSpace(workspaceRoot))
	if err != nil {
		return GitSuggestMessageOpResult{GitOpResult: GitOpResult{Error: err.Error()}}
	}
	return GitSuggestMessageOpResult{
		GitOpResult: GitOpResult{Ok: true},
		Message:     strings.TrimSpace(result.Message),
	}
}
