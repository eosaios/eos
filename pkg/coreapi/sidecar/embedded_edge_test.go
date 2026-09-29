package sidecar

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"strings"
	"testing"
)

func TestDefaultEmbeddedCacheDir(t *testing.T) {
	dir, err := defaultEmbeddedCacheDir()
	if err != nil {
		// UserCacheDir 可能失败，允许
		return
	}
	if !strings.Contains(dir, "eos") {
		t.Fatalf("dir = %q", dir)
	}
}

func TestMaterializeEmbeddedNoCore(t *testing.T) {
	// 未注入内嵌内核时返回错误
	if _, err := materializeEmbedded("linux", "amd64"); err == nil {
		t.Fatal("no embedded core")
	}
}
