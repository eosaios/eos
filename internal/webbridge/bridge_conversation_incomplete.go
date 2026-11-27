package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// bridge_conversation_incomplete.go — 一轮「没做完」的两种收尾：
//
//   - 未完成（turn.completed 带 status=incomplete）：模型输出被长度上限截断、
//     被内容安全中止、或达到单轮步数上限。已生成的内容完整保留，消息本身是
//     终态。说明行由内核作为一条 warning 级 status 项统一下发（所有客户端看到
//     同一句）；壳层只识别原因并挂在消息上，让界面提供「继续」。不能按成功展示，
//     也不能当成故障报错。
//   - 内核发起的取消/中断（turn.cancelled / turn.interrupted）：不是本地停止
//     按钮触发的停止。按「已停止」收尾，而不是让流关闭后的兜底逻辑把它报成
//     「流式响应异常结束」。

import "strings"

// incompleteItemIDSep 是内核说明行 item id 里的分隔符：`<turn_id>_incomplete_<reason>`。
// 原因编在 id 里，会话重载后据此还原（文案可变，原因不丢）。
const incompleteItemIDSep = "_incomplete_"

// knownIncompleteReasons 是内核会给出的全部原因。
var knownIncompleteReasons = []string{"max_output_tokens", "content_filter", "iteration_limit"}

// runtimeIncompleteReason 读出 turn.completed 的未完成原因；正常完成返回空串。
func runtimeIncompleteReason(payload map[string]any) string {
	if !strings.EqualFold(strings.TrimSpace(runtimeStringValue(payload, "status")), "incomplete") {
		return ""
	}
	return normalizeIncompleteReason(runtimeStringValue(payload, "reason"))
}

// incompleteReasonFromItemID 从内核说明行的 item id 还原原因；不是说明行返回空串。
func incompleteReasonFromItemID(itemID string) string {
	_, reason, found := strings.Cut(strings.TrimSpace(itemID), incompleteItemIDSep)
	if !found {
		return ""
	}
	return normalizeIncompleteReason(reason)
}

// normalizeIncompleteReason 归一原因：认识的原样返回；没见过的归为 unknown
// （仍按未完成处理，不猜具体含义）；空串表示不是未完成。
func normalizeIncompleteReason(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return ""
	}
	for _, known := range knownIncompleteReasons {
		if reason == known {
			return reason
		}
	}
	return "unknown"
}

// markMessageIncomplete 给消息打上未完成标记，界面据此在说明行之后提供「继续」。
func (s *BridgeService) markMessageIncomplete(session *sessionState, messageID, reason string) {
	msg := findSessionMessageByID(session, messageID)
	if msg == nil {
		return
	}
	msg.IncompleteReason = reason
}

// handleConversationInterruptedLocked 处理内核自己发出的取消/中断终态。
//
// 本地停止按钮走的是另一条路（见 handleConversationFailureLocked 的取消分支）。
// 这里只需要覆盖「内核先于本地状态收尾」的情况：此前这类事件落到空分支，随后
// 流关闭，被 finishConversation 判成「流式响应异常结束」——一次正常的停止被展示成故障。
func (s *BridgeService) handleConversationInterruptedLocked(frame conversationEventFrame) {
	if frame.assistantCompleted {
		return
	}
	stoppedText := s.t("conversation.stopped")
	if s.conversationCancelledLocked(frame.sessionID, frame.assistantMessageID) {
		// 本地停止：与失败路径的取消分支保持同一文案。
		stoppedText = s.t("conversation.stopped_manual")
	}
	frame.session.Running = false
	frame.session.NeedsAttention = false
	s.appendRuntimeEventLocked(frame.session, frame.assistantMessageID, "lifecycle", stoppedText, "", "failed")
	s.setMessageStatus(frame.session, frame.assistantMessageID, stoppedText, "warning", "failed")
}
