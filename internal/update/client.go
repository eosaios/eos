package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// NewHTTPClient 按代理配置构造更新流程的 HTTP 客户端（校验 URL 并
// fail-fast：非法地址在构造期报错）。proxyURL 为空时返回 nil——调用方
// 沿用 http.DefaultClient（默认走 http.ProxyFromEnvironment：遵循环境
// HTTP_PROXY/HTTPS_PROXY 约定，未配置时直连）。
func NewHTTPClient(proxyURL string) (*http.Client, error) {
	trimmed := strings.TrimSpace(proxyURL)
	if trimmed == "" {
		return nil, nil
	}
	proxyEndpoint, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("update proxy URL 非法: %w", err)
	}
	if proxyEndpoint.Scheme != "http" && proxyEndpoint.Scheme != "https" {
		return nil, fmt.Errorf("update proxy URL 仅支持 http/https: %q", trimmed)
	}
	if proxyEndpoint.Host == "" {
		return nil, fmt.Errorf("update proxy URL 缺少 host: %q", trimmed)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyEndpoint)
	return &http.Client{Transport: transport}, nil
}
