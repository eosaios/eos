package state

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestThinkingGlobalState(t *testing.T) {
	// init 默认 true
	if !Thinking() {
		t.Fatal("default thinking")
	}
	SetThinking(false)
	if Thinking() {
		t.Fatal("set false")
	}
	snap := GetSnapshot()
	if snap.Thinking {
		t.Fatal("snapshot")
	}
	SetThinking(true)
	if !GetSnapshot().Thinking {
		t.Fatal("snapshot true")
	}
}
