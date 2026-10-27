package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// 更新代理配置键（写在全局配置 ~/.eos.json，与 eos-cli 共用同一组键：
// CLI 通过 eos config set update_proxy 写入，桌面端通过设置页写入）。
const (
	updateProxyEnabledKey = "update_proxy_enabled"
	updateProxyURLKey     = "update_proxy_url"
)

// updateProxyRaw 读取全局配置的更新代理配置（开关关/未配置 = 直连，
// 遵循环境 HTTP_PROXY 约定——与 Go 默认传输行为一致）。
func (s *BridgeService) updateProxyRaw() (enabled bool, url string, err error) {
	if s == nil {
		return false, "", nil
	}
	doc, err := loadJSONDocument(s.configPathReadOnly())
	if err != nil {
		return false, "", fmt.Errorf("read update proxy config: %w", err)
	}
	return loadJSONBool(doc, updateProxyEnabledKey, false), loadJSONString(doc, updateProxyURLKey, ""), nil
}

// updateHTTPClient 按代理地址构造 HTTP 客户端：空地址返回 nil（沿用默认
// 客户端 → 环境代理/直连），显式地址在此校验并 fail-fast——「开关开启但
// 地址写错」在构造期报错，不拖到请求时才失败。
func updateHTTPClient(proxyURL string) (*http.Client, error) {
	trimmed := strings.TrimSpace(proxyURL)
	if trimmed == "" {
		return nil, nil
	}
	proxyEndpoint, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("更新代理地址非法: %w", err)
	}
	if proxyEndpoint.Scheme != "http" && proxyEndpoint.Scheme != "https" {
		return nil, fmt.Errorf("更新代理地址仅支持 http/https: %s", trimmed)
	}
	if proxyEndpoint.Host == "" {
		return nil, fmt.Errorf("更新代理地址缺少主机名: %s", trimmed)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyEndpoint)
	return &http.Client{Transport: transport}, nil
}

// validateUpdateProxyURL 验证更新代理地址（设置保存时调用）。
func validateUpdateProxyURL(raw string) error {
	_, err := updateHTTPClient(raw)
	return err
}
