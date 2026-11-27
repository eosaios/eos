package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"testing"
	"time"
)

// item_completed 携带该项的权威全文：订阅缓冲满了也必须送到。它被丢掉而终态照常
// 到达，表现就是一轮「正常结束」却少了内容。
func TestItemCompletedIsDeliveredWhenBufferIsFull(t *testing.T) {
	for _, eventType := range []string{"turn.item_completed", "turn.item_started"} {
		sink := newRuntimeJSONRPCEventSink()
		ch, unsub := sink.Subscribe(context.Background(), runtimeJSONRPCEventFilter{}, 1)
		if err := sink.publish(context.Background(), Event{EventType: "turn.item_delta"}); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = sink.publish(context.Background(), Event{EventType: eventType, RequestID: "item-1"})
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("%s: critical publish blocked the caller", eventType)
		}
		<-ch // 腾出缓冲
		select {
		case ev := <-ch:
			if ev.EventType != eventType {
				t.Fatalf("event = %q, want %s", ev.EventType, eventType)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s was dropped on a full buffer", eventType)
		}
		unsub()
	}
}
