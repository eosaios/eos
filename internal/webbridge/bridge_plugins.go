package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"fmt"
)

// PluginInstallResult mirrors the core protocol type.
type PluginInstallResult struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Installed       bool     `json:"installed"`
	McpRegistered   bool     `json:"mcp_registered"`
	SkillsInstalled []string `json:"skills_installed"`
	Path            string   `json:"path"`
}

// PluginDetail mirrors the core protocol type.
type PluginDetail struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Version       string   `json:"version"`
	Author        string   `json:"author"`
	Permissions   []string `json:"permissions"`
	Enabled       bool     `json:"enabled"`
	Source        string   `json:"source"`
	Path          string   `json:"path"`
	McpServerName string   `json:"mcp_server_name"`
	Skills        []string `json:"skills"`
}

// PluginInstall 经内核 plugin/install 安装插件（本地路径或 git URL）。
func (s *BridgeService) PluginInstall(source string) (PluginInstallResult, error) {
	var out PluginInstallResult
	rawParams, _ := json.Marshal(map[string]string{"source": source})
	raw, err := gatewayRPC(s, "plugin/install", rawParams)
	if err != nil {
		return out, fmt.Errorf("插件安装失败: %w", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("解析安装结果失败: %w", err)
	}
	return out, nil
}

// PluginInstallConfirm 二次调用（用户已确认权限）。
func (s *BridgeService) PluginInstallConfirm(source string) (PluginInstallResult, error) {
	var out PluginInstallResult
	rawParams, _ := json.Marshal(map[string]interface{}{
		"source":              source,
		"confirm_permissions": true,
	})
	raw, err := gatewayRPC(s, "plugin/install", rawParams)
	if err != nil {
		return out, fmt.Errorf("插件安装失败: %w", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("解析安装结果失败: %w", err)
	}
	return out, nil
}

// PluginRemove 经内核 plugin/remove 卸载插件。
func (s *BridgeService) PluginRemove(name string) error {
	rawParams, _ := json.Marshal(map[string]string{"name": name})
	_, err := gatewayRPC(s, "plugin/remove", rawParams)
	if err != nil {
		return fmt.Errorf("插件卸载失败: %w", err)
	}
	return nil
}

// PluginList 经内核 plugin/list 获取已安装插件列表。
func (s *BridgeService) PluginList() ([]PluginDetail, error) {
	raw, err := gatewayRPC(s, "plugin/list", nil)
	if err != nil {
		return nil, fmt.Errorf("获取插件列表失败: %w", err)
	}
	var out []PluginDetail
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析插件列表失败: %w", err)
	}
	return out, nil
}

func gatewayRPC(s *BridgeService, method string, params []byte) (json.RawMessage, error) {
	g, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	return g.CoreCallRPC(coreCtx(), method, json.RawMessage(params))
}

// PluginSearchResult mirrors the core protocol type.
type PluginSearchResult struct {
	Results  []PluginIndexEntry `json:"results"`
	Total    int                `json:"total"`
	IndexURL string             `json:"index_url"`
}

// PluginIndexEntry mirrors the core protocol type.
type PluginIndexEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Source      string   `json:"source"`
	Permissions []string `json:"permissions"`
	Tags        []string `json:"tags"`
}

// PluginSearch 经内核 plugin/search 搜索远程插件索引。
func (s *BridgeService) PluginSearch(query string) (PluginSearchResult, error) {
	rawParams, _ := json.Marshal(map[string]string{"query": query})
	raw, err := gatewayRPC(s, "plugin/search", rawParams)
	if err != nil {
		return PluginSearchResult{}, fmt.Errorf("搜索插件失败: %w", err)
	}
	var out PluginSearchResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("解析搜索结果失败: %w", err)
	}
	return out, nil
}
