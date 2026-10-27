package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// 会话级静默看门狗超时（修复点 #5a）。
//
// 设计依据（AGENTS.md §1：明确安全边界，非掩盖 bug 的兜底）：
// 这是 bridge 的最后安全网——即使 Rust 侧因修复点 #1/#2/#3 之外的某条路径卡死、
// 终态事件（turn.completed / turn.cancelled / turn.error）永不发出，bridge 也不会
// 永远卡在 `for event := range stream`，UI 永久 loading。35 分钟 = Rust 侧
// TOOL_HARD_TIMEOUT_SECS（30min）+ GRACE_PERIOD_SECS（5min）+ 余量，让 Rust 先有机会
// 自行收敛；只有真正静默超过此值才判定为「会话无响应」强制收尾。
// 这是「自上次事件以来无任何进展」的超时，不是「turn 总时长」——只要 Rust 还在发
// 任意事件（哪怕是 tool.executing），timer 就被重置。静默 35 分钟正是「卡死」特征。
const turnWatchdogSilenceTimeout = 35 * time.Minute

// errTurnWatchdogTripped 是 finishConversation 用来区分「watchdog 强制收尾」与
// 普通「流异常关闭」的哨兵错误。通过自定义 context cancel 传递（cancel(cause)）。
var errTurnWatchdogTripped = errors.New("turn watchdog: stream silent past timeout")

// turnWatchdogTimer 封装「静默超时」timer 的生命周期：每收到任意事件就 Reset，
// 触发时停止等待。把 timer 与其底层 ticker 分离，方便测试注入短超时。
type turnWatchdogTimer struct {
	timer   *time.Timer
	timeout time.Duration
}

func newTurnWatchdogTimer(timeout time.Duration) *turnWatchdogTimer {
	return &turnWatchdogTimer{
		timer:   time.NewTimer(timeout),
		timeout: timeout,
	}
}

// reset 在每收到一个 stream 事件后调用，把「静默计时」归零。
func (w *turnWatchdogTimer) reset() {
	if w == nil || w.timer == nil {
		return
	}
	if !w.timer.Stop() {
		// 排空可能已经在 channel 里的旧触发值，避免下一轮 select 立即误触发。
		select {
		case <-w.timer.C:
		default:
		}
	}
	w.timer.Reset(w.timeout)
}

func (w *turnWatchdogTimer) stop() {
	if w == nil || w.timer == nil {
		return
	}
	w.timer.Stop()
}

// streamEventWithWatchdog 在带静默看门狗保护下消费 turn 事件流。
//
// 与原 `for event := range stream` 的区别：每个事件到达后 Reset timer；timer 触发
// 说明 stream 静默超过 turnWatchdogSilenceTimeout——此时调用 interrupt（尽力而为，
// sidecar 可能已僵死）+ 用 errTurnWatchdogTripped 取消 context，然后停止等待，
// 让 runConversation 走 finishConversation 收尾（Running=false / NeedsAttention=true /
// 消息 failed），而不是永远卡在 range。
//
// 返回 true 表示 watchdog 触发（调用方据此记日志）；false 表示 stream 正常关闭。
// eventHandler 在 stream 收到事件时被同步调用，返回的事件由调用方继续处理。
// shouldDeferTrip 非空时在 timer 触发前被询问：返回 true 表示当前静默是合法等待
// （如审批/问询挂起中，用户决策时间不限长）——不判卡死，重置计时器继续等。
func (s *BridgeService) streamEventWithWatchdog(
	stream <-chan adapter.Event,
	eventHandler func(event adapter.Event),
	cancelWithCause context.CancelCauseFunc,
	interrupt func(context.Context) error,
	shouldDeferTrip func() bool,
) (watchdogTripped bool) {
	watchdog := newTurnWatchdogTimer(turnWatchdogSilenceTimeout)
	defer watchdog.stop()

	for {
		select {
		case event, ok := <-stream:
			if !ok {
				// stream 关闭：正常结束（turn 完成或 Rust 侧主动收尾）。
				return false
			}
			// 任意事件都说明 turn 仍在推进，重置静默计时。
			watchdog.reset()
			eventHandler(event)
		case <-watchdog.timer.C:
			// 审批/问询挂起中：流静默是用户决策时间，不是卡死——顺延计时继续等。
			if shouldDeferTrip != nil && shouldDeferTrip() {
				watchdog.reset()
				continue
			}
			// 静默超时：stream 长时间无事件且未关闭 = 卡死。强制收尾。
			slog.Warn("bridge.turn_watchdog_tripped",
				"silence_timeout", turnWatchdogSilenceTimeout.String())
			// 尽力通知内核停止该 turn（sidecar 整体僵死时会失败，忽略错误）。
			if interrupt != nil {
				interruptCtx, cancelInterrupt := context.WithTimeout(context.Background(), 3*time.Second)
				if err := interrupt(interruptCtx); err != nil {
					slog.Warn("bridge.turn_watchdog_interrupt_failed", "error", err)
				}
				cancelInterrupt()
			}
			// 用 cause 标记是 watchdog 触发，finishConversation 据此显示「会话无响应」。
			if cancelWithCause != nil {
				cancelWithCause(errTurnWatchdogTripped)
			}
			return true
		}
	}
}
