package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

// 内嵌浏览器实时帧的 HTTP 加载路由（与 eos-app 桌面端逐行镜像）。
//
// screencast JPEG 帧不走事件通道（大 base64 会撑爆消息通道）：
// pump 截获 browser.frame 只缓存帧 + 转发轻载荷（meta/ts），前端
// <img src> 指向本路由按需拉取。

import (
	"encoding/base64"
	"net/http"
	"strings"
)

// BrowserFrameRoutePath 是浏览器帧路由的固定路径。
const BrowserFrameRoutePath = "/eos/browser-frame"

// browserFrameCache 最新一帧（pump 写、HTTP 读；atomic 换指针无锁）。
type browserFrameCache struct {
	jpeg []byte
	ts   int64
}

// captureBrowserFrame 解析 browser.frame 载荷：缓存 JPEG、剥离 dataUrl
// 返回轻载荷。载荷异常时原样返回（降级走旧通道）。
func (s *BridgeService) captureBrowserFrame(payload map[string]any) map[string]any {
	dataURL, _ := payload["dataUrl"].(string)
	const prefix = "data:image/jpeg;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return payload
	}
	jpeg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, prefix))
	if err != nil {
		return payload
	}
	ts, _ := payload["ts"].(float64)
	s.browserFrame.Store(&browserFrameCache{jpeg: jpeg, ts: int64(ts)})
	light := map[string]any{"ts": payload["ts"]}
	if meta, ok := payload["meta"]; ok {
		light["meta"] = meta
	}
	return light
}

// serveBrowserFrame 输出最新帧（no-store：帧是逐次更新的同一 URL 语义）。
func (s *BridgeService) serveBrowserFrame(w http.ResponseWriter, r *http.Request) {
	frame := s.browserFrame.Load()
	if frame == nil || len(frame.jpeg) == 0 {
		http.Error(w, "no frame", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(frame.jpeg)
}
