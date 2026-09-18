package headless

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// turn.go — turn 启动、事件订阅与统一事件泵。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
)

// TurnStartResult 是 turn/start 的异步结果（turn/start 立即返回，turn 在
// 内核后台线程运行，正文经事件流送达）。
type TurnStartResult struct {
	Turn coreapi.Turn
	Err  error
}

// StartTurnAsync 异步发起 turn/start。请求级记忆注入开关与 TUI 共用同一
// 全局配置（~/.eos.json memory_injection_enabled，默认开），壳层只透传，
// 注入裁决在内核。
func StartTurnAsync(ctx context.Context, engine coreapi.Engine, req coreapi.StartTurnRequest) <-chan TurnStartResult {
	ch := make(chan TurnStartResult, 1)
	go func() {
		if engine == nil {
			ch <- TurnStartResult{Err: fmt.Errorf("core engine unavailable")}
			return
		}
		cfg, _ := config.Load()
		useMemory := config.MemoryInjectionEnabled(&cfg)
		req.UseMemory = &useMemory
		turn, err := engine.Turns().Start(ctx, req)
		ch <- TurnStartResult{Turn: turn, Err: err}
	}()
	return ch
}

// SubscribeTurnEvents 复用 engine 的事件总线，filter 当前 session + turn ID。
// 返回的 channel 在 sidecar 进程退出或 ctx cancel 时自动关闭。
func SubscribeTurnEvents(ctx context.Context, engine coreapi.Engine, sessionID, turnID string) <-chan protocol.Envelope {
	if engine == nil {
		ch := make(chan protocol.Envelope)
		close(ch)
		return ch
	}
	ch, err := engine.Events().Subscribe(ctx, coreapi.EventFilter{SessionID: sessionID, TurnID: turnID})
	if err != nil {
		// Subscribe 失败：返回已关闭的 channel，调用方在 turn.Start 失败时能直接感知。
		closed := make(chan protocol.Envelope)
		close(closed)
		return closed
	}
	return ch
}

// AwaitTurn 驱动一次已启动的 turn 到终止：消费 turn/start 异步结果与事件流，
// 每个事件经 NormalizeEvent 后交给 sink。sink 返回 stop=true 时提前结束
//（审批快速返回等场景）；返回 err 时立即透传。事件流自然关闭且 start 已
// 返回视为静默完成（nil）。ctx 取消返回 ctx.Err()。
func AwaitTurn(
	ctx context.Context,
	startDone <-chan TurnStartResult,
	events <-chan protocol.Envelope,
	sink func(eventType protocol.EventType, payload map[string]any, raw protocol.EventType) (stop bool, err error),
) error {
	eventsCh := events
	startDoneCh := startDone
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-startDoneCh:
			startDoneCh = nil
			if result.Err != nil {
				return result.Err
			}
			// turn/start is non-blocking; the turn runs on a background thread
			// and delivers results via events (item.delta / item.completed /
			// request.completed). No fallback timer needed — request.done or
			// request.failed is the termination signal.
		case ev, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				if startDoneCh == nil {
					return nil
				}
				continue
			}
			eventType, payload, raw := NormalizeEvent(ev)
			stop, err := sink(eventType, payload, raw)
			if err != nil || stop {
				return err
			}
		}
	}
}

// NormalizeEvent 把内核 Envelope 归一化为（归一事件类型, payload, 原始类型）。
// 归一化改写事件名时在 payload 里保留 original_event_type；request.failed
// 无错误文案时按原始事件补 cancelled/interrupted 语义。
func NormalizeEvent(ev protocol.Envelope) (protocol.EventType, map[string]any, protocol.EventType) {
	payload := protocol.ClonePayload(ev.Payload)
	if payload == nil {
		payload = map[string]any{}
	}
	rawEventType := ev.EventType
	eventType := protocol.NormalizeEventType(rawEventType)
	if rawEventType != eventType {
		payload["original_event_type"] = string(rawEventType)
	}
	if eventType == protocol.EventTypeRequestFailed && PayloadText("", payload, "error", "summary", "message", "text") == "" {
		switch rawEventType {
		case protocol.EventTypeTurnCancelled:
			payload["error"] = "request cancelled"
		case protocol.EventTypeTurnInterrupted:
			payload["error"] = "request interrupted"
		}
	}
	return eventType, payload, rawEventType
}

// FailureMessage 取 request.failed 类事件的错误文案，无文案时按原始事件
// 归类为 cancelled / interrupted / failed。
func FailureMessage(rawEventType protocol.EventType, payload map[string]any) string {
	msg := PayloadText("", payload, "error", "summary", "message", "text")
	if msg != "" {
		return msg
	}
	switch rawEventType {
	case protocol.EventTypeTurnCancelled:
		return "request cancelled"
	case protocol.EventTypeTurnInterrupted:
		return "request interrupted"
	default:
		return "request failed"
	}
}

// PayloadText 从事件 payload 里取第一个非空字符串字段（无命中返回 fallback
// 的 Trim 空串）。
func PayloadText(fallback string, data map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := data[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return strings.TrimSpace(fallback)
}

// NewTurnID 生成带前缀的 turn id（时间纳秒保证唯一）。
func NewTurnID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
