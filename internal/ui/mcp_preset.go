package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// recommendedBrowserPresetJSON returns the canned Playwright MCP server preset
// shown in the setup wizard. It previously lived in internal/mcp.browser.go;
// inlined here when the Go gateway layer was removed.

func recommendedBrowserPresetJSON() string {
	return `[
  {
    "name": "playwright",
    "type": "stdio",
    "command": "npx",
    "args": ["-y", "@playwright/mcp@latest"],
    "envs": {},
    "enabled": true
  }
]`
}
