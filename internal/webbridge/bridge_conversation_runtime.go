package webbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func (s *BridgeService) resolveSendSessionLocked(sessionID, workspace string) (*sessionState, error) {
	workspace = strings.TrimSpace(workspace)
	trimmed := strings.TrimSpace(sessionID)
	if trimmed != "" {
		if session := s.ensureSessionByIDLocked(trimmed); session != nil {
			// 一致性校验：传入 workspace 非空时，必须与该 session 的 workspace 一致。
			// 不一致说明前端传了陈旧 sessionID（属旧工作区）——不返回它，落入下方按
			// workspace 解析/新建，避免 turn 跑在错误工作区的 session 上（cwd 注入旧路径）。
			if workspace == "" || sameWorkspacePath(session.WorkspacePath, workspace) {
				if strings.TrimSpace(session.WorkspacePath) == "" {
					session.WorkspacePath = workspace
				}
				return session, nil
			}
		}
	}
	// 按 workspace 解析/新建会话（workspace 为空时回退 s.activeWorkspace，兼容自动化等无显式工作区的入口）
	if workspace == "" {
		workspace = strings.TrimSpace(s.activeWorkspace)
	}
	if workspace == "" {
		var err error
		workspace, err = s.ensureDefaultWorkspaceReady(WorkspaceActivationForeground)
		if err != nil {
			if strings.TrimSpace(workspace) != "" {
				s.activeWorkspace = strings.TrimSpace(workspace)
			}
			return nil, err
		}
		if workspace != "" {
			s.activeWorkspace = workspace
		}
	}
	if workspace == "" {
		return nil, errors.New("默认工作区初始化失败：工作区路径为空")
	}
	session, err := s.ensureActiveSessionLockedWithError(workspace)
	if err != nil {
		return nil, fmt.Errorf("创建默认会话失败: %w", err)
	}
	return session, nil
}

func (s *BridgeService) ensureWorkspaceSessionLocked(workspace, preferredSessionID string) *sessionState {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil
	}
	preferredSessionID = strings.TrimSpace(preferredSessionID)
	if preferredSessionID != "" {
		if session := s.ensureSessionByIDLocked(preferredSessionID); session != nil {
			if strings.TrimSpace(session.WorkspacePath) == "" {
				session.WorkspacePath = workspace
			}
			s.currentSessionID = session.ID
			return session
		}
		if restored := s.restoreRuntimeSessionLocked(preferredSessionID, workspace); restored != nil {
			s.currentSessionID = restored.ID
			return restored
		}
	}
	if current := s.ensureActiveSessionLocked(workspace); current != nil {
		s.currentSessionID = current.ID
		return current
	}
	return nil
}

func (s *BridgeService) conversationCancelledLocked(sessionID, assistantMessageID string) bool {
	running := s.runningConversations[strings.TrimSpace(sessionID)]
	if running == nil {
		return false
	}
	return running.AssistantMessageID == assistantMessageID && running.Cancelled
}

func (s *BridgeService) attachRunningTurn(sessionID, assistantMessageID string, stream conversationStreamHandle) {
	sessionID = strings.TrimSpace(sessionID)
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if sessionID == "" || assistantMessageID == "" {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	running := s.runningConversations[sessionID]
	if running == nil || strings.TrimSpace(running.AssistantMessageID) != assistantMessageID {
		return
	}
	running.TurnID = strings.TrimSpace(stream.TurnID)
	running.Interrupt = stream.Interrupt
	// 把 turn_id 回填到该 turn 的 assistant ChatMessage，使持久化时
	// sessionMessagesFromChatMessage 能按 turn_id 展开_items。
	if turnID := running.TurnID; turnID != "" {
		if session := s.sessions[sessionID]; session != nil {
			if msg := findSessionMessageByID(session, assistantMessageID); msg != nil {
				msg.turnID = turnID
			}
		}
	}
}

func (s *BridgeService) invokeConversationTurnStream(ctx context.Context, sessionID, input string, attachments []adapter.Attachment, turnID string) (conversationStreamHandle, error) {
	if s.invokeSession != nil {
		core := s.threadCoreIfExists(sessionID)
		if core == nil {
			return conversationStreamHandle{}, errors.New("runtime unavailable: core not ready")
		}
		imagePaths := imagePathsFromRuntimeAttachments(attachments)
		stream, err := s.invokeSession(core, ctx, input, imagePaths)
		return conversationStreamHandle{Events: stream}, err
	}
	return s.startConversationTurnRPC(ctx, sessionID, input, attachments, turnID)
}

// invokeConversationResumeStream 续跑失败 turn：不发送用户输入，直接走
// turn/resume（内核按已提交历史续写）。仅支持 runtime gateway 路径。
func (s *BridgeService) invokeConversationResumeStream(ctx context.Context, sessionID, turnID string) (conversationStreamHandle, error) {
	if s.invokeSession != nil {
		return conversationStreamHandle{}, errors.New("turn resume requires the runtime gateway path")
	}
	return s.startResumeConversationTurnRPC(ctx, sessionID, turnID)
}

func messageInTerminalState(session *sessionState, messageID string) bool {
	item := findSessionMessageByID(session, messageID)
	if item == nil {
		return true
	}
	state := strings.ToLower(strings.TrimSpace(item.State))
	return state == "completed" || state == "failed"
}

func (s *BridgeService) finishConversation(sessionID, assistantMessageID string, ctx context.Context) {
	s.stateMu.Lock()
	sessionKey := strings.TrimSpace(sessionID)
	running := s.runningConversations[sessionKey]
	session := s.sessions[strings.TrimSpace(sessionID)]
	if session != nil && session.Running {
		if assistantMessageCompleted(session, assistantMessageID) {
			session.Running = false
			session.NeedsAttention = false
			session.UpdatedAt = time.Now()
			if running != nil && running.AssistantMessageID == assistantMessageID {
				delete(s.runningConversations, sessionKey)
			}
			if persistErr := s.persistSessionLocked(session); persistErr != nil {
				s.pushNotificationLocked(s.t("notification.request_failed.title"), persistErr.Error(), "danger")
			}
			s.stateMu.Unlock()
			s.emitShellUpdatedForSession(sessionID)
			return
		}
		switch {
		// 修复点 #5a：静默看门狗触发——stream 长时间无事件被判为「会话无响应」。
		// 必须先于 context.Canceled 判定（WithCancelCause 取消后 ctx.Err() 也是 Canceled）。
		case errors.Is(context.Cause(ctx), errTurnWatchdogTripped):
			session.Running = false
			session.NeedsAttention = true
			s.appendRuntimeEventLocked(session, assistantMessageID, "error", "会话无响应", lastRuntimeEventTitle(session, assistantMessageID), "failed")
			s.setMessageStatus(session, assistantMessageID, "会话无响应，请重试", "error", "failed")
			s.pushNotificationLocked(s.t("notification.request_failed.title"), "会话无响应，已强制结束", "danger")
		case errors.Is(ctx.Err(), context.Canceled):
			session.Running = false
			session.NeedsAttention = false
			s.appendRuntimeEventLocked(session, assistantMessageID, "lifecycle", "已停止生成", "", "failed")
			s.setMessageStatus(session, assistantMessageID, "已停止生成", "warning", "failed")
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			session.Running = false
			session.NeedsAttention = true
			s.appendRuntimeEventLocked(session, assistantMessageID, "error", "请求超时", lastRuntimeEventTitle(session, assistantMessageID), "failed")
			s.setMessageStatus(session, assistantMessageID, runtimeTimeoutMessage(session, assistantMessageID), "error", "failed")
			s.pushNotificationLocked(s.t("notification.request_failed.title"), "请求超时", "danger")
		default:
			session.Running = false
			session.NeedsAttention = true
			s.appendRuntimeEventLocked(session, assistantMessageID, "error", "流式响应异常结束", lastRuntimeEventTitle(session, assistantMessageID), "failed")
			s.setMessageStatus(session, assistantMessageID, runtimeClosedStreamMessage(session, assistantMessageID), "error", "failed")
			s.pushNotificationLocked(s.t("notification.request_failed.title"), "流式响应异常结束", "danger")
		}
		// turn 已终止：挂起的审批/问询随之失效，必须一并收起——否则出现
		// 「会话都报错中止了，下面还挂着审批卡」的矛盾 UI（用户点了也会扑空）。
		s.dismissPendingPromptsLocked(sessionID)
		session.UpdatedAt = time.Now()
		if running != nil && running.AssistantMessageID == assistantMessageID {
			delete(s.runningConversations, sessionKey)
		}
		if persistErr := s.persistSessionLocked(session); persistErr != nil {
			s.pushNotificationLocked(s.t("notification.request_failed.title"), persistErr.Error(), "danger")
		}
	} else {
		if session != nil && !messageInTerminalState(session, assistantMessageID) {
			session.Running = false
			session.NeedsAttention = true
			s.appendRuntimeEventLocked(session, assistantMessageID, "error", "流式响应异常结束", lastRuntimeEventTitle(session, assistantMessageID), "failed")
			s.setMessageStatus(session, assistantMessageID, runtimeClosedStreamMessage(session, assistantMessageID), "error", "failed")
			s.dismissPendingPromptsLocked(sessionID)
			session.UpdatedAt = time.Now()
			if persistErr := s.persistSessionLocked(session); persistErr != nil {
				s.pushNotificationLocked(s.t("notification.request_failed.title"), persistErr.Error(), "danger")
			}
			s.pushNotificationLocked(s.t("notification.request_failed.title"), "流式响应异常结束", "danger")
		}
		if running != nil && running.AssistantMessageID == assistantMessageID {
			delete(s.runningConversations, sessionKey)
			if session != nil {
				if persistErr := s.persistSessionLocked(session); persistErr != nil {
					s.pushNotificationLocked(s.t("notification.request_failed.title"), persistErr.Error(), "danger")
				}
			}
		}
	}
	s.stateMu.Unlock()
	s.emitShellUpdatedForSession(sessionID)
}

// dismissPendingPromptsLocked 把本会话挂起的审批/问询 prompt 删除、对应状态行与
// 工具卡片审批态翻成 cancelled。turn 已终止（看门狗强杀/超时/流异常/用户停止），
// 挂起的审批已无意义——留着会出现「会话都报错中止了，下面还挂着审批卡」的矛盾
// UI，用户点了也会扑空。锁内调用；内核侧的挂起审批由 interrupt/turn 终止路径
// 回收，这里不发 RPC（禁止锁内 RPC）。
func (s *BridgeService) dismissPendingPromptsLocked(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if s == nil || sessionID == "" {
		return
	}
	session := s.sessions[sessionID]
	for promptID, prompt := range s.prompts {
		if strings.TrimSpace(prompt.SessionID) != sessionID {
			continue
		}
		delete(s.prompts, promptID)
		if session == nil {
			continue
		}
		statusKey := promptStatusKey(promptID)
		s.setMessageStatusWithItemStateKey(
			session,
			prompt.AssistantMessageID,
			statusKey,
			"会话已中止，审批失效",
			"warning",
			"failed",
			"completed",
		)
		s.settleItemApprovalLocked(session, prompt, "cancelled")
	}
}
