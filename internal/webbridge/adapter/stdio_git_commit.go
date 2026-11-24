package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"

	"github.com/eosaios/eos/pkg/coreapi"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// CoreGitCommitMessageRPC 让内核按辅助任务路由生成提交信息：由内核决定用谁的
// 模型、由谁付费。ctx 取消只结束等待；内核会把已发出的调用收尾。
func (g *StdioGateway) CoreGitCommitMessageRPC(ctx context.Context, request coreapi.GitSuggestMessageRequest) (coreapi.GitSuggestMessageResult, error) {
	var out coreapi.GitSuggestMessageResult
	err := g.client.Call(ctx, protocoljsonrpc.MethodGitSuggestMessage, request, &out)
	return out, err
}

// CoreGitSuggestRoutingSupportedRPC 报告内核是否支持提交信息的辅助任务路由。
// 旧内核会忽略 routing 并回落到全局默认模型，调用方必须先问这一位。
func (g *StdioGateway) CoreGitSuggestRoutingSupportedRPC(ctx context.Context) (bool, error) {
	if supported := g.gitSuggestRouting.Load(); supported != nil {
		return *supported, nil
	}
	var out struct {
		Capabilities map[string]any `json:"capabilities"`
	}
	if err := g.client.Call(ctx, protocoljsonrpc.MethodInitialize, nil, &out); err != nil {
		return false, err
	}
	supported := out.Capabilities["git_suggest_message_routing"] == true
	g.gitSuggestRouting.Store(&supported)
	return supported, nil
}
