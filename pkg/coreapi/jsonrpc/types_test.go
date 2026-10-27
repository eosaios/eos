package jsonrpc

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"testing"

	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

func TestNotifierFunc(t *testing.T) {
	called := false
	var got protocoljsonrpc.Notification
	var n Notifier = NotifierFunc(func(ctx context.Context, notification protocoljsonrpc.Notification) error {
		called = true
		got = notification
		return nil
	})
	if err := n.Notify(context.Background(), protocoljsonrpc.Notification{Method: "x"}); err != nil {
		t.Fatal(err)
	}
	if !called || got.Method != "x" {
		t.Fatalf("notify = %+v", got)
	}

	boom := errors.New("boom")
	var n2 Notifier = NotifierFunc(func(context.Context, protocoljsonrpc.Notification) error {
		return boom
	})
	if err := n2.Notify(context.Background(), protocoljsonrpc.Notification{}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
