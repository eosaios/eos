package headless

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"testing"
)

func TestCwdWorkspaceRoot(t *testing.T) {
	if cwdWorkspaceRoot() == "" {
		t.Fatal("cwd empty")
	}
}

func TestResolveActiveModelNameNilEngine(t *testing.T) {
	name, err := ResolveActiveModelName(context.Background(), nil)
	if err != nil || name != "" {
		t.Fatalf("nil engine = %q %v", name, err)
	}
}
