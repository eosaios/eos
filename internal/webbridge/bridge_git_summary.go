package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
)

// WorkspaceGitSummary 是顶栏「未提交/未推送」徽标的数据源：工作区所在
// git 仓库的概览（分支、上游、ahead/behind、未提交明细）。Ok=false 表示
// 非 git 工作区或查询失败，前端静默隐藏徽标（对齐 gitBranchReadOnly 的
// 零噪音语义，不向用户报错）。
type WorkspaceGitSummary struct {
	Ok       bool                 `json:"ok"`
	Branch   string               `json:"branch,omitempty"`
	Upstream string               `json:"upstream,omitempty"`
	Ahead    int                  `json:"ahead"`
	Behind   int                  `json:"behind"`
	Changes  []WorkspaceGitChange `json:"changes"`
}

// WorkspaceGitChange 是一条未提交变更（state 为 porcelain 状态：
// untracked / M / A / D 等；staged 取 porcelain 首字符非空格，即已进
// 暂存区——「M」单字符形态无法区分已/未暂存，以 staged 字段为准）。
type WorkspaceGitChange struct {
	Path   string `json:"path"`
	State  string `json:"state"`
	Staged bool   `json:"staged"`
}

// GetWorkspaceGitSummary 查询指定工作区的 git 概览；workspaceRoot 为空
// 时内核回填前台工作区。壳层纯透传，裁决在内核。
func (s *BridgeService) GetWorkspaceGitSummary(workspaceRoot string) WorkspaceGitSummary {
	if s == nil {
		return WorkspaceGitSummary{}
	}
	gateway := s.runtimeGatewayClient()
	if gateway == nil {
		return WorkspaceGitSummary{}
	}
	result, err := gateway.CoreGitSummaryRPC(coreCtx(), strings.TrimSpace(workspaceRoot))
	if err != nil {
		return WorkspaceGitSummary{}
	}
	changes := make([]WorkspaceGitChange, 0, len(result.Changes))
	for _, change := range result.Changes {
		changes = append(changes, WorkspaceGitChange{
			Path:   strings.TrimSpace(change.Path),
			State:  strings.TrimSpace(change.State),
			Staged: change.Staged,
		})
	}
	return WorkspaceGitSummary{
		Ok:       true,
		Branch:   strings.TrimSpace(result.Branch),
		Upstream: strings.TrimSpace(result.Upstream),
		Ahead:    int(result.Ahead),
		Behind:   int(result.Behind),
		Changes:  changes,
	}
}
