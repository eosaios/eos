package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

// 构造期间到达的请求按会话合并，构造完成后按请求先后补发。
func TestShellSnapshotQueueCoalescesAndKeepsRequestOrder(t *testing.T) {
	var q shellSnapshotQueue
	if !q.offer("a", "s1") {
		t.Fatal("first request must start the build loop")
	}
	if q.offer("a", "s2") || q.offer("b", "s3") || q.offer("a", "s4") {
		t.Fatal("requests during a build must be merged, not start another loop")
	}
	var got [][2]string
	for {
		sessionID, source, ok := q.take()
		if !ok {
			break
		}
		got = append(got, [2]string{sessionID, source})
	}
	// a 只发一次且取最新的 source；b 排在 a 之后。
	want := [][2]string{{"a", "s4"}, {"b", "s3"}}
	if len(got) != len(want) {
		t.Fatalf("emitted %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("emitted %v, want %v", got, want)
		}
	}
}

// 队列排空后结束构造循环；之后的请求要能重新启动它。
func TestShellSnapshotQueueRestartAfterDrain(t *testing.T) {
	var q shellSnapshotQueue
	q.offer("a", "s1")
	if _, _, ok := q.take(); !ok {
		t.Fatal("pending request expected")
	}
	if _, _, ok := q.take(); ok {
		t.Fatal("queue should be empty")
	}
	if !q.offer("b", "s2") {
		t.Fatal("a request after the loop ended must start a new loop")
	}
	sessionID, _, ok := q.take()
	if !ok || sessionID != "b" {
		t.Fatalf("take = %q, %v", sessionID, ok)
	}
}
