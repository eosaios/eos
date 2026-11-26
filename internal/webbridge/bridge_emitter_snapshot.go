package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// bridge_emitter_snapshot.go — shell-updated 全量快照的合并发射。
//
// 快照的构造要做多次内核 RPC，并会与会话事件消费争抢状态锁。此前每次请求各起
// 一个 goroutine：谁先构造完谁先发，一份较早开始构造的快照可能晚到，把前端刚
// 收到的流式文本覆盖回更短的旧值；突发时还会重复构造许多份。
//
// 现在同一时刻最多构造一份：构造期间到达的请求按会话合并（同一会话只保留最新
// 一次），构造完成后按请求先后依次补发。后请求的快照不会跑到先请求的前面。

import (
	"strings"
	"sync"
)

// shellSnapshotQueue 是 shell-updated 的合并队列。零值可用。
type shellSnapshotQueue struct {
	mu       sync.Mutex
	inFlight bool
	order    []string          // 待发会话，按首次请求的先后
	source   map[string]string // 会话 → 最新一次请求的 source
}

// offer 登记一次请求；返回 true 表示调用方需要启动（唯一的）构造循环。
func (q *shellSnapshotQueue) offer(sessionID, source string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.source == nil {
		q.source = make(map[string]string)
	}
	if _, queued := q.source[sessionID]; !queued {
		q.order = append(q.order, sessionID)
	}
	q.source[sessionID] = source
	if q.inFlight {
		return false
	}
	q.inFlight = true
	return true
}

// take 取出下一条待发请求；没有待发请求时结束构造循环并返回 false。
func (q *shellSnapshotQueue) take() (sessionID, source string, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.order) == 0 {
		q.inFlight = false
		return "", "", false
	}
	sessionID, q.order = q.order[0], q.order[1:]
	source = q.source[sessionID]
	delete(q.source, sessionID)
	return sessionID, source, true
}

func (s *BridgeService) emitShellUpdatedForSessionWithSource(sessionID, source string) {
	emitter := s.currentEmitter()
	if emitter == nil {
		return
	}
	if !s.shellSnapshots.offer(sessionID, source) {
		return
	}
	go func() {
		for {
			sessionID, source, ok := s.shellSnapshots.take()
			if !ok {
				return
			}
			payload := s.loadBootstrap(source)
			if strings.TrimSpace(sessionID) != "" {
				payload = s.bootstrapForSessionWithSource(sessionID, source)
			}
			emitter(shellUpdatedEventName, payload)
		}
	}()
}
