package i18n

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestWebBridgeT(t *testing.T) {
	// zh/en 命中
	if T("memory.note_saved", "zh") == "memory.note_saved" {
		t.Fatal("zh")
	}
	if T("memory.note_saved", "en") == "memory.note_saved" {
		t.Fatal("en")
	}
	// 未知语言回落 zh
	if T("memory.note_saved", "fr") != T("memory.note_saved", "zh") {
		t.Fatal("fallback zh")
	}
	// 缺失 key
	if T("no.such", "zh") != "no.such" {
		t.Fatal("missing")
	}
	if T("no.such", "en") != "no.such" {
		t.Fatal("missing en")
	}
	// 格式化（若有带参 key，调用不 panic 即可）
	_ = T("memory.note_saved", "zh", "x")
}
