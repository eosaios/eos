package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// runtime 事件 sink 余臂批测：Notify 校验族、订阅注册/关闭/取消、
// publish 满缓冲丢弃、过滤匹配（类型/会话/轮次/代理多源）、close 幂等。

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func eventNotification(eventType, sessionID, turnID string, payload map[string]any) protocoljsonrpc.Notification {
	env := map[string]any{
		"version": "v1", "event_type": eventType, "session_id": sessionID,
		"turn_id": turnID, "source": "runtime", "payload": payload,
	}
	return protocoljsonrpc.Notification{
		Method: protocoljsonrpc.NotificationEvent,
		Params: mustMarshal(env),
	}
}

func TestEventSinkNotifyValidation(t *testing.T) {
	sink := newRuntimeJSONRPCEventSink()
	ctx := context.Background()

	// nil sink 全直通。
	var nilSink *runtimeJSONRPCEventSink
	if err := nilSink.Notify(ctx, protocoljsonrpc.Notification{}); err != nil {
		t.Fatalf("nil sink = %v", err)
	}
	// 非 event 方法静默忽略。
	if err := sink.Notify(ctx, protocoljsonrpc.Notification{Method: "other", Params: []byte("{}")}); err != nil {
		t.Fatalf("非 event 方法 = %v", err)
	}
	// 坏 envelope JSON。
	if err := sink.Notify(ctx, protocoljsonrpc.Notification{
		Method: protocoljsonrpc.NotificationEvent, Params: []byte("{bad"),
	}); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
	// ctx 已取消。
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sink.Notify(canceled, eventNotification("turn.completed", "s", "t", nil)); err == nil {
		t.Fatal("取消 ctx 应报错")
	}
}

func TestEventSinkSubscribeLifecycle(t *testing.T) {
	sink := newRuntimeJSONRPCEventSink()
	ctx := context.Background()

	// nil sink：关闭通道 + 空 unsubscribe。
	var nilSink *runtimeJSONRPCEventSink
	ch, unsub := nilSink.Subscribe(ctx, runtimeJSONRPCEventFilter{}, 0)
	if ch == nil {
		t.Fatal("nil sink 通道非 nil")
	}
	if _, ok := <-ch; ok {
		t.Fatal("nil sink 通道应已关闭")
	}
	unsub()

	// 正常订阅 → close 后再订阅拿关闭通道。
	ch1, unsub1 := sink.Subscribe(ctx, runtimeJSONRPCEventFilter{}, 8)
	if ch1 == nil {
		t.Fatal("订阅通道")
	}
	unsub1() // 显式退订关通道
	if _, ok := <-ch1; ok {
		t.Fatal("退订后通道应关闭")
	}
	unsub1() // 幂等

	sink.close()
	ch2, _ := sink.Subscribe(ctx, runtimeJSONRPCEventFilter{}, 8)
	if _, ok := <-ch2; ok {
		t.Fatal("关闭后订阅应立即拿关闭通道")
	}
	sink.close() // 幂等
	var nilClose *runtimeJSONRPCEventSink
	nilClose.close()

	// ctx 取消自动退订。
	sink2 := newRuntimeJSONRPCEventSink()
	subCtx, cancel := context.WithCancel(context.Background())
	ch3, _ := sink2.Subscribe(subCtx, runtimeJSONRPCEventFilter{}, 8)
	cancel()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch3:
			if !ok {
				return // 通道被关：自动退订生效
			}
		case <-deadline:
			t.Fatal("ctx 取消未触发退订")
		}
	}
}

func TestEventSinkPublishDropOnFullBuffer(t *testing.T) {
	sink := newRuntimeJSONRPCEventSink()
	ctx := context.Background()
	// 缓冲 1：第二条同订阅事件被丢弃（publish 非阻塞原则）。
	ch, _ := sink.Subscribe(ctx, runtimeJSONRPCEventFilter{}, 1)
	if err := sink.Notify(ctx, eventNotification("turn.completed", "s1", "t1", nil)); err != nil {
		t.Fatal(err)
	}
	if err := sink.Notify(ctx, eventNotification("turn.completed", "s1", "t1", nil)); err != nil {
		t.Fatal(err) // 不报错，只计数丢弃
	}
	if len(ch) != 1 {
		t.Fatalf("缓冲应只保留 1 条: %d", len(ch))
	}
	// publish nil sink 直通。
	var nilSink *runtimeJSONRPCEventSink
	if err := nilSink.publish(ctx, Event{}); err != nil {
		t.Fatal(err)
	}
}

func TestEventSinkFilterMatching(t *testing.T) {
	sink := newRuntimeJSONRPCEventSink()
	ctx := context.Background()

	// 事件类型过滤：命中 + 不命中。
	chHit, _ := sink.Subscribe(ctx, runtimeJSONRPCEventFilter{EventTypes: []string{"turn.completed"}}, 8)
	chMiss, _ := sink.Subscribe(ctx, runtimeJSONRPCEventFilter{EventTypes: []string{"item.delta"}}, 8)
	_ = sink.Notify(ctx, eventNotification("turn.completed", "s1", "t1", nil))
	if len(chHit) != 1 || len(chMiss) != 0 {
		t.Fatalf("类型过滤命中 = %d/%d", len(chHit), len(chMiss))
	}

	// 会话过滤：event 字段与 payload.session_id 双源。
	sink2 := newRuntimeJSONRPCEventSink()
	chS, _ := sink2.Subscribe(ctx, runtimeJSONRPCEventFilter{SessionID: "s-want"}, 8)
	_ = sink2.Notify(ctx, eventNotification("turn.completed", "s-want", "t1", nil))
	_ = sink2.Notify(ctx, eventNotification("turn.completed", "s-other", "t1", nil))
	if len(chS) != 1 {
		t.Fatalf("会话过滤 = %d", len(chS))
	}

	// 轮次过滤：TurnID 与 RequestID 双源。
	sink3 := newRuntimeJSONRPCEventSink()
	chT, _ := sink3.Subscribe(ctx, runtimeJSONRPCEventFilter{TurnID: "t-want"}, 8)
	_ = sink3.Notify(ctx, eventNotification("turn.completed", "s1", "t-want", nil))
	if len(chT) != 1 {
		t.Fatalf("TurnID 命中 = %d", len(chT))
	}
	// RequestID 兜底：EventType 空 + Kind 回落 + payload turn_id 源。
	sink4 := newRuntimeJSONRPCEventSink()
	chR, _ := sink4.Subscribe(ctx, runtimeJSONRPCEventFilter{TurnID: "t-req"}, 8)
	env := map[string]any{
		"version": "v1", "source": "runtime",
		"payload": map[string]any{"turn_id": "t-req", "session_id": "s1"},
	}
	_ = sink4.Notify(ctx, protocoljsonrpc.Notification{
		Method: protocoljsonrpc.NotificationEvent, Params: mustMarshal(env),
	})
	if len(chR) != 1 {
		t.Fatalf("payload turn_id 命中 = %d", len(chR))
	}

	// 代理过滤：AgentID 不匹配拒收。
	sink5 := newRuntimeJSONRPCEventSink()
	chA, _ := sink5.Subscribe(ctx, runtimeJSONRPCEventFilter{AgentID: "a-want"}, 8)
	_ = sink5.Notify(ctx, eventNotification("turn.completed", "s1", "t1", nil))
	if len(chA) != 0 {
		t.Fatalf("AgentID 过滤 = %d", len(chA))
	}
}

func TestNormalizeFilterTrims(t *testing.T) {
	f := normalizeRuntimeJSONRPCEventFilter(runtimeJSONRPCEventFilter{
		SessionID: " s ", TurnID: " t ", AgentID: " a ",
	})
	if f.SessionID != "s" || f.TurnID != "t" || f.AgentID != "a" {
		t.Fatalf("trim = %+v", f)
	}
	// 订阅事件类型集：带会话/轮次/代理维度=全量订阅（nil），纯类型=状态同步集。
	if got := runtimeRPCSubscriptionEventTypes(runtimeJSONRPCEventFilter{SessionID: "s"}); got != nil {
		t.Fatalf("带会话维度应全量订阅 = %v", got)
	}
	if got := runtimeRPCSubscriptionEventTypes(runtimeJSONRPCEventFilter{}); len(got) == 0 {
		t.Fatal("纯类型过滤应展开状态同步事件集")
	}
}
