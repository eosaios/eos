package webbridge

import (
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

// isPromptResolvedEvent 判定事件是否为内核侧审批/问询"已处理"通知。
//
// 这些事件表示某个 approval_id / inquiry_id 在内核侧已经 resolved（用户决策、
// 内核自动拒绝、或别处处理）。Go 壳层收到后要从 s.prompts 里清掉对应项，
// 否则前端浮层会继续显示一个内核早已处理的卡片。
//
// 注意：用户的"点按钮"路径走 ResolvePrompt RPC，已经 delete(s.prompts, ...)；
// 这里的监听主要兜住"内核侧自行决定"（无人值守自动 deny 等）和"并发入口"两种场景。
func isPromptResolvedEvent(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "tool.approval_approved",
		"tool.approval_rejected",
		"approval.responded",
		"inquiry.responded":
		return true
	default:
		return false
	}
}

// settlePromptLocked 收口 prompt 的消息流状态变更：删除 prompt、自消息里把等待状态行
// 收尾为 completed，推进消息级状态，并翻转 ToolCall item 的 Approval.State（单一数据源）。
// 审批/问询的"已允许/已拒绝/已取消/已处理"都走这一条路径。
//
// 改写后立即通过 conversation-delta 增量通道推送两条 delta：
//  1. status item 的最终状态（等待确认→已允许）——消息流状态行翻转。
//  2. ToolCall item 的完整快照（Approval.State 翻转）——浮层卡片据此收起/变色。
//
// 这是"点允许后等待确认立即变绿"的权威路径，绕开全量快照 + 时间戳守卫竞态。
// 必须在 stateMu 锁内调用（emitConversationDelta 是 goroutine-safe，可在锁内调用）。
func (s *BridgeService) settlePromptLocked(
	session *sessionState,
	prompt *promptState,
	text, level, messageState string,
) *sessionState {
	return s.settlePromptWithStateLocked(session, prompt, text, level, messageState, approvalStateForResolution(prompt, level))
}

// settlePromptWithStateLocked 是 settlePromptLocked 的显式目标状态变体：item 的
// Approval.State 由调用方直接给定（停止撤回固定 cancelled、完全访问固定
// approved/answered），文案/状态行语义与 settlePromptLocked 完全一致。
func (s *BridgeService) settlePromptWithStateLocked(
	session *sessionState,
	prompt *promptState,
	text, level, messageState, approvalState string,
) *sessionState {
	if prompt == nil {
		return nil
	}
	promptID := strings.TrimSpace(prompt.ID)
	delete(s.prompts, promptID)
	if session == nil {
		session = s.ensureSessionByIDLocked(prompt.SessionID)
	}
	if session == nil {
		return nil
	}
	session.UpdatedAt = time.Now()
	statusKey := promptStatusKey(promptID)
	s.setMessageStatusWithItemStateKey(
		session,
		prompt.AssistantMessageID,
		statusKey,
		text,
		level,
		messageState,
		"completed",
	)
	// 增量推送 status item 状态翻转。
	s.emitPromptStatusDeltaLocked(session, prompt.AssistantMessageID, statusKey)
	// 单一数据源：翻转 ToolCall item 的 Approval.State，并 delta 推送完整 item 快照。
	// 前端浮层据此把 pending 卡片收起/变色，消息流里工具卡片也同步显示审批结果。
	s.settleItemApprovalLocked(session, prompt, approvalState)
	return session
}

// settleItemApprovalLocked 翻转 ToolCall item 上的 Approval.State 并 delta 推送。
// callID 来自 prompt.CallID（handleConversationApprovalLocked/RequestUserInput 写入）。
// approvalState 是目标状态（approved/denied/cancelled/answered/processed），由调用方
// 按决策语义显式给定——settlePromptLocked 用 approvalStateForResolution 推导，停止
// 撤回等固定语义路径直接传目标值。
func (s *BridgeService) settleItemApprovalLocked(session *sessionState, prompt *promptState, approvalState string) {
	if session == nil || prompt == nil {
		return
	}
	callID := strings.TrimSpace(prompt.CallID)
	if callID == "" {
		return
	}
	msg := findSessionMessageByID(session, prompt.AssistantMessageID)
	if msg == nil {
		return
	}
	idx := findItemIndex(msg.Items, callID)
	if idx < 0 {
		return
	}
	item := &msg.Items[idx]
	if item.Approval == nil {
		return
	}
	item.Approval.State = approvalState
	item.Approval.ResolvedAt = time.Now().Format(time.RFC3339)
	// delta 推送完整 item 快照（含翻转后的 Approval）。
	s.emitConversationDelta(ConversationDeltaPayload{
		SessionID: session.ID,
		MessageID: prompt.AssistantMessageID,
		ItemID:    item.ID,
		Kind:      item.Kind,
		Status:    item.Status,
		Item:      item,
	})
}

// settleSessionPromptsCancelledLocked 把会话内全部待审 prompt 收口为 cancelled：
// 删 prompt + 状态行收尾 + item Approval.State 翻 cancelled + delta。用户停止会话 =
// 撤回全部等待中的确认，横幅（item 投影）必须随之收起；此前停止路径只裸删
// s.prompts，item 永远 pending，浮层卡死且点「允许/拒绝」因 prompt 已删而幂等
// 空转（ResolvePrompt 扑空直接成功返回）。必须在 stateMu 锁内调用。
func (s *BridgeService) settleSessionPromptsCancelledLocked(session *sessionState) {
	if session == nil {
		return
	}
	var pending []*promptState
	for _, prompt := range s.prompts {
		if prompt != nil && strings.TrimSpace(prompt.SessionID) == session.ID {
			pending = append(pending, prompt)
		}
	}
	for _, prompt := range pending {
		s.settlePromptWithStateLocked(
			session,
			prompt,
			s.t("approval.resolved.cancelled"),
			"warning",
			"failed",
			"cancelled",
		)
	}
}

// approvalStateForResolution 把决策的 level + prompt 类型映射成 Approval.State 值。
func approvalStateForResolution(prompt *promptState, level string) string {
	if prompt != nil && prompt.Kind == "request_user_input" {
		return "answered"
	}
	switch strings.TrimSpace(strings.ToLower(level)) {
	case "success":
		return "approved"
	case "warning":
		return "denied"
	default:
		return "processed"
	}
}

// emitPromptStatusDeltaLocked 推送单个 status item 的状态变更到前端。
// statusKey 是该 status item 的确定性 id（promptStatusKey 派生）。在 stateMu 锁内调用：
// 从消息里按 key 取出最新 item 快照，构造 delta payload 直发，零 RPC、不阻塞。
func (s *BridgeService) emitPromptStatusDeltaLocked(session *sessionState, assistantMessageID, statusKey string) {
	statusKey = strings.TrimSpace(statusKey)
	if session == nil || statusKey == "" {
		return
	}
	msg := findSessionMessageByID(session, assistantMessageID)
	if msg == nil {
		return
	}
	idx := findStatusItemIndex(msg.Items, statusKey)
	if idx < 0 {
		return
	}
	item := msg.Items[idx]
	s.emitConversationDelta(ConversationDeltaPayload{
		SessionID: session.ID,
		MessageID: assistantMessageID,
		ItemID:    item.ID,
		Kind:      item.Kind,
		Status:    item.Status,
		Item:      &item,
	})
}

// handleConversationResolutionLocked 处理"已 resolved"事件：从 s.prompts 删除
// 对应条目，并把消息流的"等待确认…"状态行同步为决策结果。返回值指示
// runConversation 是否需要在锁外立即触发一次全量同步，让前端浮层即时收起卡片。
//
// 必须在 stateMu 锁内调用（读写 s.prompts）。
func (s *BridgeService) handleConversationResolutionLocked(frame conversationEventFrame) conversationEventResult {
	approvalID := resolutionApprovalID(frame.event)
	if approvalID == "" {
		return conversationEventResult{}
	}
	prompt, ok := s.prompts[approvalID]
	if !ok {
		// 已经被 ResolvePrompt RPC 删除，或不在本壳层管辖——幂等，不报错。
		return conversationEventResult{}
	}
	// 内核侧 resolved（切 never 模式 auto-approve、无人值守 auto-deny、CLI 旁路决策）时，
	// "等待确认…"状态行必须同步更新为决策结果——否则该审批的 ResolvePrompt 扑空
	//（prompts 已删 → ErrNotExist），状态行永远残留，用户分不清"运行中"与"卡死"。
	// 这里同样要把消息整体状态与 status item 分离：消息继续 streaming，但审批状态
	// 行本身已经完成，必须收尾为 completed，避免视觉上还像 pending。
	text, level := s.resolutionStatusTextAndLevel(frame.kind)
	s.settlePromptLocked(frame.session, prompt, text, level, "streaming")
	// resolved 必须立即同步给前端：浮层要秒收起，不能等 200ms debouncer。
	return conversationEventResult{emitBootstrap: true}
}

// resolutionStatusTextAndLevel 把内核 resolved 事件 kind 映射为消息流状态行文案与级别。
// 与 ResolvePrompt 的用户决策路径（resolvedStatusTextAndLevel）语义对齐：
// approved=已允许(success)、rejected=已拒绝(warning)、responded=已处理(info)。
func (s *BridgeService) resolutionStatusTextAndLevel(kind string) (string, string) {
	switch strings.TrimSpace(kind) {
	case "tool.approval_approved":
		return s.t("approval.resolved.allowed"), "success"
	case "tool.approval_rejected":
		return s.t("approval.resolved.denied"), "warning"
	default:
		return s.t("approval.resolved.default"), "info"
	}
}

// resolutionApprovalID 从 resolved 事件的 payload 取出 approval_id / inquiry_id。
// 不同事件用不同字段名，这里按优先级回退：先扫 payload，再用 Envelope.request_id。
func resolutionApprovalID(event adapter.Event) string {
	payload := event.Payload
	if len(payload) == 0 {
		payload = event.Data
	}
	for _, key := range []string{"approval_id", "inquiry_id"} {
		if v := strings.TrimSpace(runtimeStringValue(payload, key)); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(event.EffectiveRequestID()); v != "" {
		return v
	}
	return ""
}

// pendingPromptStatusTextAndLevel 返回 prompt 挂起时状态行的文案与级别。
// 创建（handleConversationApprovalLocked/RequestUserInput）与回滚（unsettlePromptLocked）
// 必须共用同一来源，保证「挂起文案」只有一个真相。
func (s *BridgeService) pendingPromptStatusTextAndLevel(prompt *promptState) (string, string) {
	if prompt != nil && prompt.Source == "request-user-input" {
		return s.t("request_user_input.message_status.text"), "info"
	}
	return s.t("approval.message_status.text"), "warning"
}

// unsettlePromptLocked 是 settlePromptLocked 的逆操作：respond RPC 失败时把 prompt
// 恢复为 pending——重新挂回 s.prompts、状态行翻回挂起文案、ToolCall item 的
// Approval.State 翻回 pending，并 delta 推送，让前端卡片重新出现供用户重试。
// 若内核实际已处理该审批（如响应丢失但内核已落账），resolved 事件路径
// （handleConversationResolutionLocked）会再次收口，状态自愈。必须在 stateMu 锁内调用。
func (s *BridgeService) unsettlePromptLocked(session *sessionState, prompt *promptState) {
	if prompt == nil {
		return
	}
	promptID := strings.TrimSpace(prompt.ID)
	if promptID == "" {
		return
	}
	if session == nil {
		session = s.ensureSessionByIDLocked(prompt.SessionID)
	}
	if session == nil {
		return
	}
	s.prompts[promptID] = prompt
	session.UpdatedAt = time.Now()
	statusKey := promptStatusKey(promptID)
	text, level := s.pendingPromptStatusTextAndLevel(prompt)
	s.setMessageStatusWithItemStateKey(session, prompt.AssistantMessageID, statusKey, text, level, "waiting", "waiting")
	s.emitPromptStatusDeltaLocked(session, prompt.AssistantMessageID, statusKey)
	s.unsettleItemApprovalLocked(session, prompt)
}

// unsettleItemApprovalLocked 把 ToolCall item 的 Approval.State 翻回 pending 并 delta 推送，
// 让前端把该行从「已回答/已允许」恢复为待处理样式。
func (s *BridgeService) unsettleItemApprovalLocked(session *sessionState, prompt *promptState) {
	if session == nil || prompt == nil {
		return
	}
	callID := strings.TrimSpace(prompt.CallID)
	if callID == "" {
		return
	}
	msg := findSessionMessageByID(session, prompt.AssistantMessageID)
	if msg == nil {
		return
	}
	idx := findItemIndex(msg.Items, callID)
	if idx < 0 {
		return
	}
	item := &msg.Items[idx]
	if item.Approval == nil {
		return
	}
	item.Approval.State = "pending"
	item.Approval.ResolvedAt = ""
	// delta 推送完整 item 快照（含翻回 pending 的 Approval），与 settle 路径对称。
	s.emitConversationDelta(ConversationDeltaPayload{
		SessionID: session.ID,
		MessageID: prompt.AssistantMessageID,
		ItemID:    item.ID,
		Kind:      item.Kind,
		Status:    item.Status,
		Item:      item,
	})
}
