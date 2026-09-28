package engineprovider

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"errors"
	"os"
	"testing"
)

func TestResolveMode(t *testing.T) {
	for _, v := range []string{"", "auto", "rust", "AUTO", "  rust  "} {
		mode, err := ResolveMode(v)
		if err != nil || mode != ModeAuto {
			t.Fatalf("ResolveMode(%q) = %v %v", v, mode, err)
		}
	}
	if _, err := ResolveMode("legacy"); err == nil {
		t.Fatal("legacy should fail")
	}
	// 环境变量回落
	t.Setenv(EnvCoreEngine, "rust")
	mode, err := ResolveMode("")
	if err != nil || mode != ModeAuto {
		t.Fatalf("env = %v %v", mode, err)
	}
	t.Setenv(EnvCoreEngine, "nope")
	if _, err := ResolveMode(""); err == nil {
		t.Fatal("bad env")
	}
	_ = os.Getenv(EnvCoreEngine)
}

func TestMissingMethods(t *testing.T) {
	if MissingMethods([]string{"a"}, nil) != nil {
		t.Fatal("no required")
	}
	got := MissingMethods([]string{"a", " b "}, []string{"a", "b", "c", ""})
	if len(got) != 1 || got[0] != "c" {
		t.Fatalf("missing = %v", got)
	}
	if len(MissingMethods(nil, []string{"x"})) != 1 {
		t.Fatal("empty available")
	}
}

func TestSelectionClose(t *testing.T) {
	s := Selection{}
	if err := s.Close(); err != nil {
		t.Fatal("nil close")
	}
	called := false
	s2 := Selection{close: func() error {
		called = true
		return errors.New("x")
	}}
	if err := s2.Close(); err == nil || !called {
		t.Fatalf("close = %v", err)
	}
}
