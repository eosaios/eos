package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ScreenshotResult 是 CaptureScreenshot 的返回：截图经内核 screenshot_capture
// 工具落盘后导入为对话附件，前端把 ref 加入 composer。
type ScreenshotResult struct {
	Attachment AttachmentRef `json:"attachment"`
	Width      int           `json:"width"`
	Height     int           `json:"height"`
}

// CaptureScreenshot 截取屏幕/软件窗口并直接导入为附件。
// window 为空时取焦点/最顶层非 EOS 窗口（「上一个打开的软件」语义）；
// macOS 首次使用系统会弹「屏幕录制」授权，拒绝时返回引导错误。
func (s *BridgeService) CaptureScreenshot(window string) (ScreenshotResult, error) {
	zero := ScreenshotResult{Attachment: AttachmentRef{}}

	args := map[string]any{}
	if window != "" {
		args["window"] = window
	}
	params := map[string]any{
		"name": "screenshot_capture",
		"args": args,
	}
	rawParams, _ := json.Marshal(params)

	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return zero, err
	}
	out, err := gateway.CoreToolExecuteRPC(coreCtx(), rawParams)
	if err != nil {
		return zero, fmt.Errorf("截图执行失败: %w", err)
	}

	var toolOut struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Output struct {
			Path   string `json:"path"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"output"`
	}
	if err := json.Unmarshal(out, &toolOut); err != nil {
		return zero, fmt.Errorf("解析截图结果失败: %w", err)
	}
	if toolOut.Status != "ok" {
		return zero, fmt.Errorf("%s", toolOut.Error)
	}
	if toolOut.Output.Path == "" {
		return zero, fmt.Errorf("截图未返回文件路径")
	}

	png, err := os.ReadFile(toolOut.Output.Path)
	if err != nil {
		return zero, fmt.Errorf("读取截图文件失败: %w", err)
	}
	ref, err := s.ImportAttachment(
		filepath.Base(toolOut.Output.Path),
		"image/png",
		base64.StdEncoding.EncodeToString(png),
	)
	if err != nil {
		return zero, fmt.Errorf("截图导入附件失败: %w", err)
	}
	return ScreenshotResult{
		Attachment: ref,
		Width:      toolOut.Output.Width,
		Height:     toolOut.Output.Height,
	}, nil
}
