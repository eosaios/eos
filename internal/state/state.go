package state

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"sync/atomic"
)

// 这里是跨模块共享的轻量全局状态层：
// - UI 需要展示/切换的状态（例如 executionMode、thinking）
// - runtime 需要读取的状态（例如 executionMode）
//
// 目标是避免 UI 与 runtime 各自维护一份状态并在调用链路中手工传参导致的分叉。
//
// 约束：
// - 只放"少量、读多写少、且必须跨模块共享"的状态
// - 不做持久化（持久化由 UI settings 负责）
// - 不提供订阅/回调，保持简单
var (
	// thinking: UI 层的"思考显示"开关（当前不影响模型参数）
	thinking atomic.Bool
)

func init() {
	thinking.Store(true)
}

// Thinking 返回 UI 的思考显示开关。
func Thinking() bool {
	return thinking.Load()
}

// SetThinking 设置 UI 的思考显示开关。
func SetThinking(v bool) {
	thinking.Store(v)
}

// Snapshot 用于一次性读取多个状态，避免跨模块读取时产生不必要的重复调用。
type Snapshot struct {
	Thinking bool
}

// GetSnapshot 返回当前全局状态快照。
// 这里不保证强一致（例如每个字段读到的时刻不同），但满足 UI 展示与调用参数读取的需求。
func GetSnapshot() Snapshot {
	return Snapshot{
		Thinking: Thinking(),
	}
}
