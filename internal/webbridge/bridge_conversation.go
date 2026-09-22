package webbridge

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/protocol"
)

func turnRollbacksFromMessages(messages []ChatMessage) ([]TurnRollback, error) {
	rollbacks := []TurnRollback{}
	for _, message := range messages {
		if !strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
			continue
		}
		if message.Rollback != nil {
			rollback := *message.Rollback
			rollback.Files = nonNilSlice(append([]RollbackFileSnapshot(nil), message.Rollback.Files...))
			rollbacks = append(rollbacks, rollback)
			continue
		}
		if message.ChangeSet != nil && len(message.ChangeSet.Files) > 0 {
			return nil, errors.New("这轮文件变更缺少安全回退快照，无法回退")
		}
	}
	return rollbacks, nil
}

func (s *BridgeService) runConversation(ctx context.Context, sessionID, assistantMessageID, input string, attachments []adapter.Attachment, resume bool) {
	defer s.conversationWG.Done()
	// 用 WithCancelCause 而非 WithCancel：静默看门狗（修复点 #5a）触发时通过
	// cancel(cause) 把 errTurnWatchdogTripped 传给 finishConversation，让它显示
	// 「会话无响应」而非笼统的「流式响应异常结束」。
	runtimeCtx, cancelWithCause := context.WithCancelCause(ctx)
	defer cancelWithCause(context.Canceled)
	defer func() { s.finishConversation(sessionID, assistantMessageID, runtimeCtx) }()

	s.stateMu.RLock()
	workspace := ""
	if session := s.sessions[strings.TrimSpace(sessionID)]; session != nil {
		workspace = strings.TrimSpace(session.WorkspacePath)
	}
	s.stateMu.RUnlock()

	if workspace != "" {
		if err := s.activateWorkspaceRPC(workspace); err != nil {
			slog.Warn("bridge.core_rpc.write_failed", "domain", "activate-workspace", "workspace", workspace, "error", err)
		}
		if err := s.setWorkspaceCurrentSessionRPC(workspace, sessionID); err != nil {
			slog.Warn("bridge.core_rpc.write_failed", "domain", "set-current-session", "workspace", workspace, "session_id", sessionID, "error", err)
		}
		if err := s.resumeWorkspaceSessionRPC(workspace, sessionID); err != nil {
			slog.Warn("bridge.core_rpc.write_failed", "domain", "resume-session", "workspace", workspace, "session_id", sessionID, "error", err)
		}
	}
	s.stateMu.Lock()
	if session := s.ensureSessionByIDLocked(sessionID); session != nil {
		s.appendRuntimeEventLocked(session, assistantMessageID, "workspace", "正在确认工作区", workspace, "running")
		session.UpdatedAt = time.Now()
		if persistErr := s.persistSessionLocked(session); persistErr != nil {
			session.NeedsAttention = true
			s.pushNotificationLocked(s.t("notification.request_failed.title"), persistErr.Error(), "danger")
		}
	}
	s.stateMu.Unlock()
	s.emitShellUpdatedForSession(sessionID)

	turnID := newID("turn")
	if gateway := s.runtimeGatewayClient(); gateway != nil && strings.TrimSpace(turnID) != "" {
		s.attachRunningTurn(sessionID, assistantMessageID, conversationGatewayTurnHandle(gateway, sessionID, nil, turnID))
	}
	// resume 走 turn/resume（不发送用户输入，内核按已提交历史续写）；普通对话
	// 走 turn/start。
	var streamHandle conversationStreamHandle
	var err error
	if resume {
		streamHandle, err = s.invokeConversationResumeStream(runtimeCtx, sessionID, turnID)
	} else {
		streamHandle, err = s.invokeConversationTurnStream(runtimeCtx, sessionID, input, attachments, turnID)
	}
	if streamHandle.SessionID != "" {
		sessionID = streamHandle.SessionID
	}
	source := "runtime"
	if err != nil {
		s.stateMu.Lock()
		session := s.ensureSessionByIDLocked(sessionID)
		session.Running = false
		session.NeedsAttention = true
		session.UpdatedAt = time.Now()
		s.appendRuntimeEventLocked(session, assistantMessageID, "error", "请求启动失败", err.Error(), "failed")
		s.setMessageStatus(session, assistantMessageID, requestFailureMessage(err.Error()), "error", "failed")
		if persistErr := s.persistSessionLocked(session); persistErr != nil {
			s.pushNotificationLocked(s.t("notification.request_failed.title"), persistErr.Error(), "danger")
		}
		s.pushNotificationLocked(s.t("notification.request_failed.title"), err.Error(), "danger")
		s.stateMu.Unlock()
		s.emitShellUpdatedForSession(sessionID)
		return
	}
	s.attachRunningTurn(sessionID, assistantMessageID, streamHandle)
	stream := streamHandle.Events

	// handleStreamEvent 处理单个 turn 事件（原 for-range 循环体）。抽出成闭包让
	// 外层 streamEventWithWatchdog 能把它喂给静默看门狗的 select（修复点 #5a）。
	handleStreamEvent := func(event adapter.Event) {
		kind := strings.TrimSpace(event.EventType)
		message := normalizeConversationEventMessage(kind, event.EffectiveMessage())
		// turn 进行中的事件分类：
		// - item_*（item_started/delta/completed）：走轻量增量 delta emit，零 RPC。
		// - turn.completed：turn 结束，走一次全量 loadBootstrap 同步最终状态（含 tool results）。
		// - 其它 lifecycle（turn.started/tool.*/agent.* 等）：turn 进行中不触发全量 loadBootstrap
		//   （否则会覆盖 delta 的增量 items，导致顺序错乱）。这些事件的信息已通过 delta 到达
		//   前端，或由 state-sync debouncer 低频同步。
		//
		// 事件类型走 protocol 包常量（与内核协议单一真相源），不再硬编码字符串——
		// 避免壳层与内核事件名漂移。
		isItemEvent := kind == string(protocol.EventTypeTurnItemStarted) ||
			kind == string(protocol.EventTypeTurnItemDelta) ||
			kind == string(protocol.EventTypeTurnItemCompleted)
		isTurnCompleted := kind == string(protocol.EventTypeTurnCompleted) ||
			kind == string(protocol.EventTypeTurnCancelled) ||
			kind == string(protocol.EventTypeTurnError)
		s.stateMu.Lock()
		session := s.ensureSessionByIDLocked(sessionID)
		assistantMessage := findSessionMessageByID(session, assistantMessageID)
		result := s.handleConversationEventLocked(conversationEventFrame{
			session:            session,
			sessionID:          sessionID,
			assistantMessageID: assistantMessageID,
			input:              input,
			source:             source,
			kind:               kind,
			message:            message,
			event:              event,
			assistantCompleted: assistantMessage != nil && strings.EqualFold(strings.TrimSpace(assistantMessage.State), "completed"),
		})
		session.UpdatedAt = time.Now()
		if result.persist {
			if persistErr := s.persistSessionLocked(session); persistErr != nil {
				session.NeedsAttention = true
				s.pushNotificationLocked(s.t("notification.request_failed.title"), persistErr.Error(), "danger")
			}
		}
		// 锁内提取增量 payload 并直接 emit（Wails EventProcessor 是 goroutine-safe 的，
		// 不发 RPC 不阻塞，可在锁内调）。item 事件走这条轻量路径。
		if isItemEvent && assistantMessage != nil {
			s.emitItemDeltaLocked(sessionID, assistantMessageID, kind, event, assistantMessage)
		}
		s.stateMu.Unlock()
		// turn 进行中的 lifecycle 事件（非 item、非 turn 完成）默认不触发全量
		// loadBootstrap——否则会用 Go 内存的 items 覆盖前端的 delta 增量，导致顺序错乱。
		// 例外：审批/问询/计划问题事件必须立即同步，让浮层卡片秒到（不能等 200ms
		// debouncer，否则会出现"看到等待确认文案但卡片不显示"的窗口）。
		// turn 完成/取消/出错时无论如何都全量同步最终状态。
		if !isTurnCompleted && !result.emitBootstrap {
			return
		}
		s.emitShellUpdatedForSession(sessionID)
	}

	// 静默看门狗（修复点 #5a）：在 select 里消费 stream，任意事件重置 timer；
	// stream 静默超过 turnWatchdogSilenceTimeout 则强制收尾，避免 Rust 侧卡死时
	// bridge 永远卡在 range、UI 永久 loading。
	// 例外：审批/问询挂起中流静默是用户决策时间（可能远超 35 分钟），shouldDeferTrip
	// 查询本会话 pending prompt 数，非 0 则顺延不判卡死。
	if tripped := s.streamEventWithWatchdog(stream, handleStreamEvent, cancelWithCause, streamHandle.Interrupt, func() bool {
		s.stateMu.Lock()
		defer s.stateMu.Unlock()
		return s.pendingPromptCountLocked(sessionID) > 0
	}); tripped {
		slog.Warn("bridge.turn_watchdog_forced_finish", "session_id", sessionID)
	}
}

// emitItemDeltaLocked 在持有 stateMu 锁时构造并推送轻量增量事件。
// item_started/completed 带完整 ThreadItem（前端 upsert）；item_delta 带增量文本（前端追加）。
// 必须在锁内调用（读取 assistantMessage.Items 的最新状态），emit 本身不阻塞。
func (s *BridgeService) emitItemDeltaLocked(sessionID, assistantMessageID, kind string, event adapter.Event, assistantMessage *ChatMessage) {
	payload := payloadForItem(event)
	switch kind {
	case "turn.item_started", "turn.item_completed":
		// 带完整 item 快照：从 assistantMessage.Items 找到刚 upsert 的那个。
		item, ok := extractThreadItem(payload["item"])
		if !ok {
			return
		}
		idx := findItemIndex(assistantMessage.Items, item.ID)
		if idx < 0 {
			return
		}
		current := assistantMessage.Items[idx] // 拷贝（含 handleConversationEventLocked 累积的结果）
		s.emitConversationDelta(ConversationDeltaPayload{
			SessionID: sessionID,
			MessageID: assistantMessageID,
			ItemID:    current.ID,
			Kind:      current.Kind,
			Status:    current.Status,
			Item:      &current,
		})
	case "turn.item_delta":
		itemID, _ := payload["item_id"].(string)
		deltaType, _ := payload["delta_type"].(string)
		delta, _ := payload["delta"].(string)
		if strings.TrimSpace(delta) == "" {
			return
		}
		idx := findItemIndex(assistantMessage.Items, itemID)
		if idx < 0 {
			return
		}
		current := assistantMessage.Items[idx]
		s.emitConversationDelta(ConversationDeltaPayload{
			SessionID: sessionID,
			MessageID: assistantMessageID,
			ItemID:    current.ID,
			Kind:      current.Kind,
			DeltaType: deltaType,
			Delta:     delta,
			Status:    current.Status,
		})
	}
}
