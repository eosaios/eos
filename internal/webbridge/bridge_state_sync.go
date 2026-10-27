package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/fsnotify/fsnotify"
)

const shellStateSyncDebounce = 200 * time.Millisecond

func (s *BridgeService) startStateSynchronizers() {
	if s == nil || s.stopCh == nil {
		return
	}
	s.startRuntimeStateSynchronizer()
	s.startFileStateSynchronizer()
}

func (s *BridgeService) startRuntimeStateSynchronizer() {
	if s.runtimeGatewayClient() == nil {
		return
	}
	sources, unsubscribe := s.subscribeRuntimeStateEvents()
	go s.runShellSyncDebouncer(sources)
	go func() {
		<-s.stopCh
		unsubscribe()
	}()
}

// subscribeRuntimeStateEvents 通过 CoreSubscribeEventsRPC / event/subscribe 接收 runtime event。
// 不再保留 legacy SubscribeStateChanges fallback。不改变前端事件名、debounce timing。
func (s *BridgeService) subscribeRuntimeStateEvents() (<-chan string, func()) {
	out := make(chan string, 64)

	ctx := context.Background()
	events, unsubscribeRPC, err := s.subscribeRuntimeEventsRPC(ctx, "", "", "", 64)
	if err != nil {
		slog.Warn("bridge.state_sync.events_rpc_unavailable", "error", err.Error())
		// JSON-RPC event/subscribe 不可用时返回空 channel，不阻塞 state sync
		return out, func() {}
	}

	go func() {
		defer close(out)
		for event := range events {
			source := runtimeEventShellSource(event)
			select {
			case out <- source:
			default:
			}
		}
	}()
	return out, unsubscribeRPC
}

// runtimeEventShellSource 将 JSON-RPC event 转换为与 legacy state change 一致的 shell source 名称。
func runtimeEventShellSource(event adapter.Event) string {
	topic := strings.TrimSpace(event.Type)
	if topic == "" {
		topic = strings.TrimSpace(event.EventType)
	}
	logRuntimeStateSyncEvent(topic, event)
	return stateChangeShellSource(topic)
}

func logRuntimeStateSyncEvent(topic string, event adapter.Event) {
	normalized := strings.TrimSpace(topic)
	if normalized == "" {
		return
	}
	if normalized != "turn.error" && normalized != "Error" && normalized != "request.failed" {
		return
	}
	payload := event.Payload
	if len(payload) == 0 {
		payload = event.Data
	}
	slog.Warn("bridge.runtime_state_sync.error_event",
		"topic", normalized,
		"session_id", strings.TrimSpace(event.SessionID),
		"turn_id", strings.TrimSpace(event.TurnID),
		"request_id", strings.TrimSpace(event.RequestID),
		"agent_id", strings.TrimSpace(event.AgentID),
		"message", strings.TrimSpace(event.EffectiveMessage()),
		"payload", strings.TrimSpace(fmt.Sprintf("%v", payload)))
}

func (s *BridgeService) startFileStateSynchronizer() {
	sources := make(chan string, 64)
	go s.runShellSyncDebouncer(sources)
	go func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			slog.Warn("bridge.state_sync.fsnotify_unavailable", "error", err.Error())
			return
		}
		defer watcher.Close()

		paths := newStatePathWatcher(watcher)
		paths.reconcile(s.stateWatchDirectories())

		for {
			select {
			case <-s.stopCh:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !isStateWatchOp(event.Op) {
					continue
				}
				paths.reconcile(s.stateWatchDirectories())
				if isStateWatchEventPath(event.Name) {
					// automation.json 被 AI（「在对话中创建」流程）直接写入时，
					// 除 bootstrap 刷新外还需重建定时调度。
					s.maybeReloadAutomationFromFile(event.Name)
					enqueueShellSync(sources, "event.filesystem")
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if err != nil {
					slog.Debug("bridge.state_sync.fsnotify_error", "error", err.Error())
				}
			}
		}
	}()
}

func (s *BridgeService) runShellSyncDebouncer(sources <-chan string) {
	timer := time.NewTimer(time.Hour)
	stopShellSyncTimer(timer)
	pending := map[string]struct{}{}

	for {
		select {
		case <-s.stopCh:
			stopShellSyncTimer(timer)
			return
		case source, ok := <-sources:
			if !ok {
				stopShellSyncTimer(timer)
				return
			}
			source = strings.TrimSpace(source)
			if source == "" {
				source = "event.state"
			}
			pending[source] = struct{}{}
			resetShellSyncTimer(timer)
		case <-timer.C:
			if len(pending) > 0 {
				batch := shellSyncBatchSource(pending)
				pending = map[string]struct{}{}
				s.stateMu.RLock()
				running := make([]string, 0, len(s.runningConversations))
				for key := range s.runningConversations {
					if strings.TrimSpace(key) != "" {
						running = append(running, key)
					}
				}
				s.stateMu.RUnlock()
				if len(running) > 0 {
					for _, sessionID := range running {
						s.emitShellUpdatedForSessionWithSource(sessionID, batch)
					}
				} else {
					s.emitShellUpdatedForSessionWithSource("", batch)
				}
			}
		}
	}
}

func resetShellSyncTimer(timer *time.Timer) {
	stopShellSyncTimer(timer)
	timer.Reset(shellStateSyncDebounce)
}

func stopShellSyncTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func enqueueShellSync(sources chan<- string, source string) {
	select {
	case sources <- source:
	default:
	}
}

func stateChangeShellSource(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "event.runtime.state"
	}
	return "event.runtime.state." + topic
}

func shellSyncBatchSource(sources map[string]struct{}) string {
	if len(sources) == 0 {
		return "event.state"
	}
	keys := make([]string, 0, len(sources))
	for source := range sources {
		keys = append(keys, source)
	}
	sort.Strings(keys)
	if len(keys) == 1 {
		return keys[0]
	}
	return "event.state.batch"
}
