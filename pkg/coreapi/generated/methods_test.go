package generated

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import "testing"

func TestCoreMethodsAndGroups(t *testing.T) {
	methods := CoreMethods()
	if len(methods) < 10 {
		t.Fatalf("methods = %d", len(methods))
	}
	seen := map[string]bool{}
	for _, m := range methods {
		if m == "" {
			t.Fatal("empty method")
		}
		if seen[m] {
			t.Fatalf("dup method %q", m)
		}
		seen[m] = true
	}

	groups := MethodGroups()
	if len(groups) == 0 {
		t.Fatal("empty groups")
	}
	total := 0
	for g, list := range groups {
		if g == "" || len(list) == 0 {
			t.Fatalf("bad group %q", g)
		}
		total += len(list)
	}
	if total == 0 {
		t.Fatal("no methods in groups")
	}
}
