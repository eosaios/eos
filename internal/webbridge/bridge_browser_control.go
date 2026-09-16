package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	coreapi "github.com/eosaios/eos/pkg/coreapi"
)

// browserEventName 前端订阅名：内核 browser.* 事件原样转发（壳层只透传
// 不裁决——D2 控制权状态机的单一事实源在内核）。
const browserEventName = "eos:bridge:browser-event"

// BrowserEventPayload 转发给前端的浏览器事件载荷。
type BrowserEventPayload struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
}

// BrowserControlTakeover 人主动接管浏览器控制权（任何时候可抢；内核
// 裁决 AI 后续浏览器工具 fail-fast 并发布事件）。
func (s *BridgeService) BrowserControlTakeover(reason string, note string, timeoutMs int64) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	req := coreapi.BrowserControlTakeoverRequest{
		Reason: strings.TrimSpace(reason),
		Note:   strings.TrimSpace(note),
	}
	if timeoutMs > 0 {
		v := timeoutMs
		req.TimeoutMS = &v
	}
	if err := gateway.CoreBrowserControlTakeoverRPC(coreCtx(), req); err != nil {
		return nil, fmt.Errorf("接管控制权失败: %w", err)
	}
	return map[string]interface{}{"taken_over": true}, nil
}

// BrowserControlResume 交还 AI 控制权（唤醒等待中的 request_human）。
func (s *BridgeService) BrowserControlResume() (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	if err := gateway.CoreBrowserControlResumeRPC(coreCtx()); err != nil {
		return nil, fmt.Errorf("交还控制权失败: %w", err)
	}
	return map[string]interface{}{"resumed": true}, nil
}

// BrowserFocus 「在外部窗口打开/置顶」：url 非空 = 在目标 profile（缺省
// external 有头实例）新开 tab 打开；空 = 置顶默认 profile 会话 tab。
func (s *BridgeService) BrowserFocus(url string, profile string) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	req := coreapi.BrowserFocusRequest{
		URL:    strings.TrimSpace(url),
		Profile: strings.TrimSpace(profile),
	}
	if err := gateway.CoreBrowserFocusRPC(coreCtx(), req); err != nil {
		return nil, fmt.Errorf("打开外部浏览器失败: %w", err)
	}
	return map[string]interface{}{"focused": true}, nil
}

// BrowserSetDefaultProfile 切换默认 profile（新会话生效；旧浏览器保留）。
func (s *BridgeService) BrowserSetDefaultProfile(profile string) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	if err := gateway.CoreBrowserSetDefaultProfileRPC(coreCtx(), strings.TrimSpace(profile)); err != nil {
		return nil, fmt.Errorf("切换 profile 失败: %w", err)
	}
	return map[string]interface{}{"profile": profile}, nil
}

// BrowserTabNew 面板「+」新开 tab：会话绑定切到新 tab，旧 tab 保留给人看。
// 壳层纯透传，tab 归属由内核裁决。
func (s *BridgeService) BrowserTabNew(url string) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	tab, err := gateway.CoreBrowserTabNewRPC(coreCtx(), strings.TrimSpace(url))
	if err != nil {
		return nil, fmt.Errorf("新建标签页失败: %w", err)
	}
	return map[string]interface{}{"index": tab.Index, "url": tab.URL, "title": tab.Title}, nil
}

// BrowserTabSwitch 面板标签条点击：会话绑定切到所选 tab。
func (s *BridgeService) BrowserTabSwitch(index int) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	tab, err := gateway.CoreBrowserTabSwitchRPC(coreCtx(), index)
	if err != nil {
		return nil, fmt.Errorf("切换标签页失败: %w", err)
	}
	return map[string]interface{}{"index": tab.Index, "url": tab.URL, "title": tab.Title}, nil
}

// BrowserTabClose 面板标签条 ×：index 为空 = 关闭会话绑定 tab。
func (s *BridgeService) BrowserTabClose(index *int) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	if err := gateway.CoreBrowserTabCloseRPC(coreCtx(), index); err != nil {
		return nil, fmt.Errorf("关闭标签页失败: %w", err)
	}
	return map[string]interface{}{"closed": true}, nil
}

// BrowserNavigate 面板地址栏导航：内核懒启动浏览器并在最近活跃 tab 打开
// url（壳层只透传，目标 tab 由内核裁决）。
func (s *BridgeService) BrowserNavigate(url string) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(url)
	if trimmed == "" {
		return nil, fmt.Errorf("地址不能为空")
	}
	if err := gateway.CoreBrowserNavigateRPC(coreCtx(), trimmed); err != nil {
		return nil, fmt.Errorf("打开地址失败: %w", err)
	}
	return map[string]interface{}{"navigated": true, "url": trimmed}, nil
}

// BrowserLiveStart 开启内嵌实时视口流（面板打开时调用；帧经
// browser.frame 事件到达前端，内核逐帧 ack 维持流控）。
func (s *BridgeService) BrowserLiveStart(maxWidth int, maxHeight int, quality int) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	req := coreapi.BrowserLiveStartRequest{}
	if maxWidth > 0 {
		v := uint32(maxWidth)
		req.MaxWidth = &v
	}
	if maxHeight > 0 {
		v := uint32(maxHeight)
		req.MaxHeight = &v
	}
	if quality > 0 {
		v := uint32(quality)
		req.Quality = &v
	}
	if err := gateway.CoreBrowserLiveStartRPC(coreCtx(), req); err != nil {
		return nil, fmt.Errorf("开启实时视口失败: %w", err)
	}
	return map[string]interface{}{"started": true}, nil
}

// BrowserLiveStop 停止内嵌实时视口流（面板关闭即停）。
func (s *BridgeService) BrowserLiveStop() (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	if err := gateway.CoreBrowserLiveStopRPC(coreCtx()); err != nil {
		return nil, fmt.Errorf("停止实时视口失败: %w", err)
	}
	return map[string]interface{}{"stopped": true}, nil
}

// BrowserInput 人在内嵌视口的输入注入（单事件单调用；前端负责 DOM →
// 统一载荷的归一与坐标映射，内核只透传给 CDP）。
func (s *BridgeService) BrowserInput(request map[string]interface{}) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("输入事件序列化失败: %w", err)
	}
	var req coreapi.BrowserInputRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("输入事件格式错误: %w", err)
	}
	if strings.TrimSpace(req.Kind) == "" {
		return nil, fmt.Errorf("输入事件缺少 kind")
	}
	if err := gateway.CoreBrowserInputRPC(coreCtx(), req); err != nil {
		return nil, fmt.Errorf("注入输入失败: %w", err)
	}
	return map[string]interface{}{"ok": true}, nil
}

// BrowserHistory 地址栏历史导航：back / forward / reload。
func (s *BridgeService) BrowserHistory(action string) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(action)
	switch trimmed {
	case "back", "forward", "reload":
	default:
		return nil, fmt.Errorf("不支持的历史操作: %s", trimmed)
	}
	if err := gateway.CoreBrowserHistoryRPC(coreCtx(), trimmed); err != nil {
		return nil, fmt.Errorf("历史导航失败: %w", err)
	}
	return map[string]interface{}{"ok": true, "action": trimmed}, nil
}

// BrowserCopySelection 读取页面当前选中文本（Cmd+C 系统剪贴板桥的读选区半边）。
// 空选区返回空 text，前端据此决定是否写系统剪贴板。
func (s *BridgeService) BrowserCopySelection() (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	result, err := gateway.CoreBrowserCopySelectionRPC(coreCtx())
	if err != nil {
		return nil, fmt.Errorf("读取选区失败: %w", err)
	}
	return map[string]interface{}{"text": result.Text}, nil
}

// BrowserPickStart 开启元素选取模式（真实浏览器 hover 高亮 + 点击捕获；
// browser.pick.selected 事件回传结构化引用）。
func (s *BridgeService) BrowserPickStart() (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	if err := gateway.CoreBrowserPickStartRPC(coreCtx()); err != nil {
		return nil, fmt.Errorf("开启选取模式失败: %w", err)
	}
	return map[string]interface{}{"picking": true}, nil
}

// BrowserPickStop 退出选取模式。
func (s *BridgeService) BrowserPickStop() (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	if err := gateway.CoreBrowserPickStopRPC(coreCtx()); err != nil {
		return nil, fmt.Errorf("退出选取模式失败: %w", err)
	}
	return map[string]interface{}{"picking": false}, nil
}

// BrowserProfiles 列出 profile 注册表。
// BrowserCredentialsImport 设置页「从外部浏览器导入登录态」：endpoint 空 =
// 自动模式（拷贝默认 Chrome cookie 到临时调试实例后导入，macOS/Linux）；
// 非空 = 手动模式（浏览器以 --remote-debugging-port 启动后填 host:port）。
// 导入耗时可到几十秒（冷启动+Keychain 授权），前端需异步等待并给反馈。
func (s *BridgeService) BrowserCredentialsImport(endpoint string, profile string, domains []string, dryRun bool) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	req := coreapi.BrowserCredentialsImportRequest{
		Endpoint: strings.TrimSpace(endpoint),
		Profile:  strings.TrimSpace(profile),
		Domains:  domains,
		DryRun:   dryRun,
	}
	result, err := gateway.CoreBrowserCredentialsImportRPC(coreCtx(), req)
	if err != nil {
		return nil, fmt.Errorf("导入登录态失败: %w", err)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("序列化导入结果失败: %w", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("解析导入结果失败: %w", err)
	}
	return out, nil
}

// BrowserProfileUpsert 设置页：创建/更新浏览器 profile（headless=nil 不更新；
// 内置无头视口 / external 有头外部窗口）。
func (s *BridgeService) BrowserProfileUpsert(name string, headless *bool, note string) (map[string]interface{}, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("profile 名不能为空")
	}
	params := map[string]interface{}{"name": trimmed}
	if headless != nil {
		params["headless"] = *headless
	}
	if trimmedNote := strings.TrimSpace(note); trimmedNote != "" {
		params["note"] = trimmedNote
	}
	profiles, err := gateway.CoreBrowserProfileUpsertRPC(coreCtx(), params)
	if err != nil {
		return nil, fmt.Errorf("保存 profile 失败: %w", err)
	}
	return map[string]interface{}{"profiles": profiles}, nil
}

func (s *BridgeService) BrowserProfiles() ([]coreapi.BrowserProfileRecord, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return nil, err
	}
	out, err := gateway.CoreBrowserProfilesRPC(coreCtx())
	if err != nil {
		return nil, fmt.Errorf("获取 profile 列表失败: %w", err)
	}
	return out, nil
}

// BrowserPickUploadFile 打开系统文件选择框，返回所选路径（供
// browser_upload_file 工具使用）。filter 后缀如 "png,jpg,pdf"。
func (s *BridgeService) BrowserPickUploadFile(filter string) (map[string]interface{}, error) {
	result, err := s.OpenAttachmentDialog()
	if err != nil {
		return nil, fmt.Errorf("打开文件选择失败: %w", err)
	}
	if result.Cancelled || len(result.Paths) == 0 {
		return map[string]interface{}{"cancelled": true}, nil
	}
	return map[string]interface{}{
		"path":      result.Paths[0],
		"cancelled": false,
	}, nil
}

// startBrowserEventPump 订阅内核 browser.* 事件并转发给前端
// （takeover 卡片/选取器/下载提示/预览流的统一数据源）。
func (s *BridgeService) startBrowserEventPump() {
	ctx := context.Background()
	events, unsubscribe, err := s.subscribeRuntimeEventsRPC(ctx, "", "", "", 128)
	if err != nil {
		// JSON-RPC event/subscribe 不可用时静默跳过（内核未起/版本旧）
		slog.Warn("bridge.browser_events.rpc_unavailable", "error", err.Error())
		return
	}
	go func() {
		defer unsubscribe()
		for event := range events {
			topic := strings.TrimSpace(event.Type)
			if topic == "" {
				topic = strings.TrimSpace(event.EventType)
			}
			if !strings.HasPrefix(topic, "browser.") {
				continue
			}
			payload := event.Payload
			if len(payload) == 0 {
				payload = event.Data
			}
			// upload.needed：弹原生文件选择框并回填（取消也回填空——
			// 让工具立刻明确失败而非等超时）
			if topic == "browser.upload.needed" {
				s.handleUploadNeeded(payload)
			}
			// frame：缓存帧本体、转发轻载荷（meta/ts）——大 base64 不走
			// 消息通道（见 browser_frame_route.go）
			if topic == "browser.frame" {
				payload = s.captureBrowserFrame(payload)
			}
			s.emitBrowserEvent(BrowserEventPayload{
				Type:    topic,
				Payload: payload,
			})
		}
	}()
}

func (s *BridgeService) emitBrowserEvent(payload BrowserEventPayload) {
	emitter := s.currentEmitter()
	if emitter == nil {
		return
	}
	emitter(browserEventName, payload)
}

// handleUploadNeeded 处理 browser.upload.needed：原生选择框 →
// browser/upload_provide 回填。
func (s *BridgeService) handleUploadNeeded(payload map[string]any) {
	requestID, _ := payload["request_id"].(string)
	if strings.TrimSpace(requestID) == "" {
		slog.Warn("bridge.browser_events.upload_needed_missing_id")
		return
	}
	go func() {
		paths := []string{}
		if result, err := s.BrowserPickUploadFile(""); err == nil && !result["cancelled"].(bool) {
			if p, ok := result["path"].(string); ok && p != "" {
				paths = []string{p}
			}
		}
		gateway, err := requireRuntimeGateway(s)
		if err != nil {
			slog.Warn("bridge.browser_events.upload_provide_unavailable", "error", err.Error())
			return
		}
		if err := gateway.CoreBrowserUploadProvideRPC(coreCtx(), requestID, paths); err != nil {
			slog.Warn("bridge.browser_events.upload_provide_failed", "error", err.Error())
		}
	}()
}
