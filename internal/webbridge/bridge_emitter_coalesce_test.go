package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"path/filepath"
	"testing"
	"time"
)

// 整条链路：突发 60 次请求只触发 1–2 份快照（首份 + 构造期间合并出的一份）。
// 此前每次请求各起一个 goroutine 各建一份，突发时重复构造许多份，而且谁先建完
// 谁先发，一份较早开始构造的旧快照可能晚到，把前端刚收到的流式文本覆盖回更短的旧值。
func TestShellUpdatedBurstEmitsCoalescedSnapshots(t *testing.T) {
	// 快照构造会经内核网关读设置；空服务没有网关。仍把 HOME 指到临时目录，
	// 确保这个测试在任何情况下都碰不到真实的 ~/.eos.json。
	t.Setenv("HOME", t.TempDir())
	t.Setenv("EOS_CONFIG_PATH", filepath.Join(t.TempDir(), "eos.json"))
	s := &BridgeService{}
	emitted := make(chan struct{}, 128)
	s.emitEvent = func(name string, data any) {
		if name == shellUpdatedEventName {
			emitted <- struct{}{}
		}
	}
	for i := 0; i < 60; i++ {
		s.emitShellUpdatedForSessionWithSource("", "event.test")
	}
	time.Sleep(300 * time.Millisecond) // 等全部构造与发射结束
	close(emitted)
	count := 0
	for range emitted {
		count++
	}
	if count < 1 || count > 2 {
		t.Fatalf("emitted %d snapshots for a 60-request burst, want 1..2", count)
	}
}
