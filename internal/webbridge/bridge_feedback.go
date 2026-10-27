package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"os/exec"

	"github.com/eosaios/eos/pkg/executil"
	"runtime"
	"strings"
)

// OpenExternalURL 在系统默认浏览器中打开外部链接（意见反馈等入口）。
// 仅允许 https 协议（防 file:// / javascript: 注入）。
func (s *BridgeService) OpenExternalURL(url string) error {
	if url == "" {
		return fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("仅支持 https 链接: %s", url)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return executil.StartDetached(cmd)
}
