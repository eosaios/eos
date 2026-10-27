package i18n

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestTFallsBackAndFormats(t *testing.T) {
	// zh 命中
	if T("help.title", "zh") == "help.title" {
		t.Fatal("zh hit")
	}
	// en 命中
	if T("help.title", "en") == "help.title" {
		t.Fatal("en hit")
	}
	// 未知语言回落 zh
	if T("help.title", "fr") != T("help.title", "zh") {
		t.Fatal("unknown lang → zh")
	}
	// 缺失 key：非 en 先回落 en，再回落 key 本身
	if got := T("no.such.key.xyz", "zh"); got != "no.such.key.xyz" {
		t.Fatalf("missing key = %q", got)
	}
	if got := T("no.such.key.xyz", "en"); got != "no.such.key.xyz" {
		t.Fatalf("missing en = %q", got)
	}
	// 带参数格式化
	got := T("setup.step.model", "zh", "Demo")
	if got == "setup.step.model" {
		t.Fatal("format")
	}
}
