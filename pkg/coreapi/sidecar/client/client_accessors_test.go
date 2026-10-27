package client

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"testing"
	"time"
)

func TestClientNilSafety(t *testing.T) {
	var c *Client
	if c.Engine() != nil {
		t.Fatal("nil engine")
	}
	if c.Process() != nil {
		t.Fatal("nil process")
	}
	init := c.Initialize()
	if init.ServerName != "" || len(init.Methods) != 0 {
		t.Fatalf("nil init = %+v", init)
	}
	if c.HasMethod("x") {
		t.Fatal("nil hasmethod")
	}
	if c.MissingMethods() != nil {
		t.Fatal("nil missing")
	}
	if err := c.Close(); err != nil {
		t.Fatal("nil close")
	}
	ch := c.Wait()
	select {
	case err := <-ch:
		if err == nil {
			t.Fatal("nil wait")
		}
	case <-time.After(time.Second):
		t.Fatal("wait timeout")
	}
}

func TestClientZeroValueAccessors(t *testing.T) {
	c := &Client{}
	if c.Engine() != nil {
		t.Fatal("zero engine")
	}
	if c.Process() != nil {
		t.Fatal("zero process")
	}
	if c.HasMethod("x") {
		t.Fatal("zero hasmethod")
	}
	// 无 methods → 返回全部 required
	if len(c.MissingMethods()) != len(RequiredMethods) {
		t.Fatalf("missing = %v", c.MissingMethods())
	}
	if err := c.Close(); err != nil {
		t.Fatal("zero close")
	}
	ch := c.Wait()
	select {
	case err := <-ch:
		if err == nil {
			t.Fatal("zero wait")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	// 有 init methods
	c2 := &Client{}
	c2.init = InitializeResult{Methods: []string{"a", "b"}}
	if !c2.HasMethod("a") || c2.HasMethod("z") {
		t.Fatal("hasmethod")
	}
	if len(c2.MissingMethods()) == 0 && len(RequiredMethods) > 2 {
		// required 里应有缺失
		t.Fatal("should miss some")
	}
}
