package server

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// control.go — eos_* 控制/委托工具集：让外部 MCP 宿主把完整编码任务
// 委托给 EOS agent（eos_chat），并管理会话、审批与问询闭环。
//
// 与 tools.go 的原子工具直通互补；语义契约见 internal/docs/mcp/SERVER.md。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/headless"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/coreapi/generated"
	"github.com/eosaios/eos/pkg/protocol"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// 聊天/等待的默认与上限超时（秒）。默认对齐一次中等编码任务的时长；
// 上限防止宿主误传超大值把 worker 长期占死。
const (
	defaultChatTimeoutSecs = 600
	maxChatTimeoutSecs     = 3600
	pollInterval           = time.Second
)

// requestUserInputEvent 是 turn.request_user_input 的原始事件名（Go protocol
// 包暂无该常量；按原始字符串匹配，payload 透传）。
const requestUserInputEvent = "turn.request_user_input"

// chatStatusCompleted 等状态值出现在 eos_chat / eos_task_wait 的 JSON 结果里。
const (
	chatStatusCompleted       = "completed"
	chatStatusWaitingApproval = "waiting_approval"
	chatStatusUserInput       = "request_user_input"
	chatStatusTimeout         = "timeout"
	chatStatusError           = "error"
)

// registerControlTools 注册 eos_* 控制/委托工具。
func (h *MCPHost) registerControlTools(s *server.MCPServer) {
	// ── 任务委托 ──
	eosChat := mcp.NewTool("eos_chat",
		mcp.WithDescription("委托一个编码/分析任务给 EOS agent：发送消息并驱动完整一轮 "+
			"（读文件、编辑、执行命令、子 agent 等全部能力），阻塞至收尾。返回 JSON："+
			"status（completed/waiting_approval/request_user_input/timeout/error）、reply（最终回复）、"+
			"files_changed（变更文件摘要）、session_id。遇到审批或问询会立即返回对应状态与 "+
			"approval_id，宿主决策后用 eos_approval_respond / eos_inquiry_respond 回应，"+
			"再调 eos_task_wait 获取最终结果。超时≠失败：turn 继续在后台运行，用 eos_task_status 续查。"),
		mcp.WithString("message", mcp.Required(), mcp.Description("发给 EOS agent 的任务描述/用户消息")),
		mcp.WithString("session_id", mcp.Description("目标会话；缺省用连接默认会话。返回值带 session_id 供续聊")),
		mcp.WithNumber("timeout_secs", mcp.Description("阻塞上限秒数（默认 600，上限 3600）")),
	)
	s.AddTool(eosChat, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleChat(ctx, req)
	})

	eosTaskWait := mcp.NewTool("eos_task_wait",
		mcp.WithDescription("等待会话当前 turn 收尾（审批/问询回应后的续跑、或 eos_chat 超时后的任务），"+
			"返回与 eos_chat 同构的 JSON 结果。会话空闲时立即返回最近一轮结果。"),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("目标会话")),
		mcp.WithNumber("timeout_secs", mcp.Description("等待上限秒数（默认 600，上限 3600）")),
	)
	s.AddTool(eosTaskWait, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleTaskWait(ctx, req)
	})

	eosTaskStatus := mcp.NewTool("eos_task_status",
		mcp.WithDescription("查询会话当前状态：是否运行中、是否待审批/待输入、最近回复摘要。"),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("目标会话")),
	)
	s.AddTool(eosTaskStatus, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleTaskStatus(ctx, req)
	})

	eosTaskCancel := mcp.NewTool("eos_task_cancel",
		mcp.WithDescription("打断会话当前正在运行的 turn（异步生效；已落盘的文件变更不会回滚）。"),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("目标会话")),
	)
	s.AddTool(eosTaskCancel, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleTaskCancel(ctx, req)
	})

	// ── 会话管理 ──
	eosSessionCreate := mcp.NewTool("eos_session_create",
		mcp.WithDescription("显式创建一个 EOS 会话（默认会话在首次调用时懒创建；多任务隔离可用本工具分会话）。返回 session_id。"),
		mcp.WithString("workspace", mcp.Description("工作区根目录（缺省用服务启动时的 --workspace）")),
		mcp.WithString("title", mcp.Description("会话标题（缺省 MCP session）")),
	)
	s.AddTool(eosSessionCreate, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleSessionCreate(ctx, req)
	})

	eosSessionsList := mcp.NewTool("eos_sessions_list",
		mcp.WithDescription("列出会话（含运行状态与待审批计数）。"),
		mcp.WithString("workspace", mcp.Description("按工作区过滤")),
	)
	s.AddTool(eosSessionsList, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleSessionsList(ctx, req)
	})

	eosSessionHistory := mcp.NewTool("eos_session_history",
		mcp.WithDescription("读取会话消息历史（尾部 N 条，含变更文件标记）。"),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("目标会话")),
		mcp.WithNumber("limit", mcp.Description("返回最近 N 条（默认 20）")),
	)
	s.AddTool(eosSessionHistory, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleSessionHistory(ctx, req)
	})

	// ── 审批/问询闭环 ──
	eosApprovalsList := mcp.NewTool("eos_approvals_list",
		mcp.WithDescription("列出待处理审批（工具调用被审批策略拦下时，宿主据此决策）。"),
		mcp.WithString("session_id", mcp.Description("按会话过滤；缺省列出全部")),
	)
	s.AddTool(eosApprovalsList, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleApprovalsList(ctx, req)
	})

	eosApprovalRespond := mcp.NewTool("eos_approval_respond",
		mcp.WithDescription("回应一条待审批（decision：accept 执行一次；accept_for_session 本会话后续同类直接放行；"+
			"decline 拒绝；cancel 拒绝并打断 turn）。回应后用 eos_task_wait 拿最终结果。"),
		mcp.WithString("approval_id", mcp.Required(), mcp.Description("审批 ID（来自 eos_chat 结果或 eos_approvals_list）")),
		mcp.WithString("decision", mcp.Required(), mcp.Description("accept / accept_for_session / decline / cancel")),
		mcp.WithString("reason", mcp.Description("决策说明（可选）")),
	)
	s.AddTool(eosApprovalRespond, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleApprovalRespond(ctx, req)
	})

	eosInquiryRespond := mcp.NewTool("eos_inquiry_respond",
		mcp.WithDescription("回应 Plan 模式的问询（turn.request_user_input 挂起时，按问题选项回填）。"),
		mcp.WithString("inquiry_id", mcp.Required(), mcp.Description("问询 ID")),
		mcp.WithString("option", mcp.Description("所选选项 token")),
		mcp.WithString("text", mcp.Description("自由文本补充（可选）")),
	)
	s.AddTool(eosInquiryRespond, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleInquiryRespond(ctx, req)
	})
}

// chatOutcome 是一次 eos_chat / eos_task_wait 的结构化结果。
type chatOutcome struct {
	Status       string         `json:"status"`
	SessionID    string         `json:"session_id"`
	TurnID       string         `json:"turn_id,omitempty"`
	Reply        string         `json:"reply,omitempty"`
	FilesChanged []changedFile  `json:"files_changed,omitempty"`
	Approval     *pendingView   `json:"approval,omitempty"`
	Questions    map[string]any `json:"questions,omitempty"`
	Error        string         `json:"error,omitempty"`
	Note         string         `json:"note,omitempty"`
}

type changedFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int64  `json:"additions"`
	Deletions int64  `json:"deletions"`
}

type pendingView struct {
	ApprovalID string `json:"approval_id"`
	ToolName   string `json:"tool_name,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// handleChat — eos_chat：解析会话 → 应用模型覆盖（每会话一次）→ 驱动 turn，
// 审批/问询快速返回，超时/失败结构化报告。
func (h *MCPHost) handleChat(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	message, err := req.RequireString("message")
	if err != nil {
		return errorResult("message is required"), nil
	}
	if strings.TrimSpace(message) == "" {
		return errorResult("message must not be empty"), nil
	}
	sessionID, err := h.resolveExplicitSession(ctx, req)
	if err != nil {
		return errorResult(fmt.Sprintf("resolve session: %v", err)), nil
	}
	if err := h.applyModelOverrideOnce(ctx, sessionID); err != nil {
		return errorResult(fmt.Sprintf("apply model override: %v", err)), nil
	}

	timeout := clampTimeout(req.GetInt("timeout_secs", h.chatTimeoutSecs()))
	turnID := headless.NewTurnID("mcp_turn")
	out := chatOutcome{SessionID: sessionID, TurnID: turnID}

	events := headless.SubscribeTurnEvents(ctx, h.engine, sessionID, turnID)
	startDone := headless.StartTurnAsync(ctx, h.engine, coreapi.StartTurnRequest{
		SessionID: sessionID,
		TurnID:    turnID,
		Input:     message,
	})

	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var sinkErr error
	awaitErr := headless.AwaitTurn(tctx, startDone, events, func(eventType protocol.EventType, payload map[string]any, raw protocol.EventType) (bool, error) {
		switch {
		case eventType == protocol.EventTypeItemDelta:
			if dt := headless.PayloadText("", payload, "delta_type"); dt != "" && dt != "text" {
				return false, nil
			}
			out.Reply += headless.PayloadText("", payload, "delta", "text", "message")
			return false, nil
		case eventType == protocol.EventTypeItemCompleted:
			if item, ok := payload["item"].(map[string]any); ok {
				if k, _ := item["kind"].(string); k == "agent_message" {
					if text, _ := item["text"].(string); text != "" {
						out.Reply = text
					}
				}
			}
			return false, nil
		case eventType == protocol.EventTypeTextFinal:
			if text := headless.PayloadText("", payload, "text", "message"); text != "" {
				out.Reply = text
			}
			return false, nil
		case raw == protocol.EventTypeTurnWaitingApproval:
			out.Status = chatStatusWaitingApproval
			return true, nil
		case string(raw) == requestUserInputEvent:
			out.Status = chatStatusUserInput
			out.Questions = payload
			return true, nil
		case eventType == protocol.EventTypeRequestDone:
			out.Status = chatStatusCompleted
			return true, nil
		case eventType == protocol.EventTypeRequestFailed:
			out.Status = chatStatusError
			sinkErr = fmt.Errorf("%s", headless.FailureMessage(raw, payload))
			return true, nil
		}
		return false, nil
	})

	switch {
	case out.Status == chatStatusWaitingApproval:
		out.Note = "任务挂起等待审批：用 eos_approval_respond 回应后调 eos_task_wait 获取结果"
		if pending := h.firstPendingApproval(ctx, sessionID); pending != nil {
			out.Approval = pending
		}
	case out.Status == chatStatusUserInput:
		out.Note = "任务挂起等待问询：用 eos_inquiry_respond 回应后调 eos_task_wait 获取结果"
		if pending := h.firstPendingApproval(ctx, sessionID); pending != nil {
			out.Approval = pending
		}
	case awaitErr != nil && errors.Is(awaitErr, context.DeadlineExceeded):
		out.Status = chatStatusTimeout
		out.Note = "阻塞超时，turn 仍在后台运行：用 eos_task_status / eos_task_wait 续查"
	case awaitErr != nil && out.Status == "":
		// turn/start 失败或事件流异常关闭。
		out.Status = chatStatusError
		out.Error = awaitErr.Error()
	case sinkErr != nil:
		out.Error = sinkErr.Error()
	case out.Status == "":
		out.Status = chatStatusCompleted
	}
	if out.Status == chatStatusCompleted {
		h.attachTurnSummary(ctx, sessionID, &out)
	}
	return jsonResult(out), nil
}

// handleTaskWait — eos_task_wait：轮询会话 Running 标志至收尾或超时，返回
// 最近一轮 assistant 结果（与 eos_chat 同构）。
func (h *MCPHost) handleTaskWait(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := req.RequireString("session_id")
	if err != nil {
		return errorResult("session_id is required"), nil
	}
	sessionID = strings.TrimSpace(sessionID)
	timeout := clampTimeout(req.GetInt("timeout_secs", h.chatTimeoutSecs()))
	deadline := time.Now().Add(timeout)

	for {
		running, sErr := h.sessionRunning(ctx, sessionID)
		if sErr != nil {
			return errorResult(sErr.Error()), nil
		}
		if !running {
			out := chatOutcome{SessionID: sessionID, Status: chatStatusCompleted}
			h.attachTurnSummary(ctx, sessionID, &out)
			if out.Error != "" {
				out.Status = chatStatusError
			}
			return jsonResult(out), nil
		}
		if time.Now().After(deadline) {
			return jsonResult(chatOutcome{
				Status:    chatStatusTimeout,
				SessionID: sessionID,
				Note:      "等待超时，turn 仍在后台运行：用 eos_task_status 续查或再次 eos_task_wait",
			}), nil
		}
		select {
		case <-ctx.Done():
			return errorResult(ctx.Err().Error()), nil
		case <-time.After(pollInterval):
		}
	}
}

// handleTaskStatus — eos_task_status：会话运行状态 + 最近回复摘要。
func (h *MCPHost) handleTaskStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := req.RequireString("session_id")
	if err != nil {
		return errorResult("session_id is required"), nil
	}
	sessionID = strings.TrimSpace(sessionID)
	snapshot, err := h.sessionSnapshot(ctx, sessionID)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	status := map[string]any{
		"session_id":      sessionID,
		"running":         snapshot.Running,
		"needs_attention": snapshot.NeedsAttention,
		"pending_prompts": snapshot.PendingPrompts,
		"message_count":   snapshot.MessageCount,
		"title":           snapshot.Title,
	}
	if last := h.lastAssistantMessage(ctx, sessionID); last != nil {
		status["last_reply"] = truncate(last.Content, 500)
	}
	if snapshot.NeedsAttention {
		if pending := h.firstPendingApproval(ctx, sessionID); pending != nil {
			status["pending_approval"] = pending
		}
	}
	return jsonResult(status), nil
}

// handleTaskCancel — eos_task_cancel：空 TurnID 打断会话活动 turn（内核语义）。
func (h *MCPHost) handleTaskCancel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := req.RequireString("session_id")
	if err != nil {
		return errorResult("session_id is required"), nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if err := h.engine.Turns().Interrupt(ctx, coreapi.TurnRef{SessionID: sessionID}); err != nil {
		return errorResult(fmt.Sprintf("interrupt: %v", err)), nil
	}
	return jsonResult(map[string]any{
		"status":     "interrupt_requested",
		"session_id": sessionID,
		"note":       "打断异步生效；已完成的文件变更不会被回滚",
	}), nil
}

// handleSessionCreate — 显式建会话（source=mcp）。
func (h *MCPHost) handleSessionCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workspace := strings.TrimSpace(req.GetString("workspace", ""))
	if workspace == "" {
		workspace = h.workspaceRoot
	}
	title := strings.TrimSpace(req.GetString("title", ""))
	if title == "" {
		title = "MCP session"
	}
	session, err := h.engine.Sessions().Create(ctx, coreapi.CreateSessionRequest{
		WorkspaceRoot: workspace,
		Title:         title,
		Metadata:      map[string]any{"source": "mcp"},
	})
	if err != nil {
		return errorResult(fmt.Sprintf("create session: %v", err)), nil
	}
	return jsonResult(map[string]any{"session_id": session.ID, "workspace": session.WorkspaceRoot}), nil
}

// handleSessionsList — 会话列表摘要（state/snapshot 的 SessionSnapshot 带
// 运行态字段，比 session/list 的精简 Session 更适合宿主决策）。
func (h *MCPHost) handleSessionsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workspace := strings.TrimSpace(req.GetString("workspace", ""))
	state, err := h.engine.State().Snapshot(ctx, coreapi.StateSnapshotRequest{})
	if err != nil {
		return errorResult(fmt.Sprintf("state/snapshot: %v", err)), nil
	}
	items := make([]map[string]any, 0, len(state.Sessions))
	for _, s := range state.Sessions {
		if workspace != "" && !strings.EqualFold(strings.TrimSpace(s.WorkspacePath), workspace) {
			continue
		}
		items = append(items, map[string]any{
			"session_id":      s.ID,
			"title":           s.Title,
			"workspace":       s.WorkspacePath,
			"running":         s.Running,
			"needs_attention": s.NeedsAttention,
			"message_count":   s.MessageCount,
			"updated_at":      s.UpdatedAt,
		})
	}
	return jsonResult(map[string]any{"sessions": items, "count": len(items)}), nil
}

// handleSessionHistory — 会话消息历史（尾部 N 条）。
func (h *MCPHost) handleSessionHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := req.RequireString("session_id")
	if err != nil {
		return errorResult("session_id is required"), nil
	}
	sessionID = strings.TrimSpace(sessionID)
	limit := req.GetInt("limit", 20)
	if limit <= 0 {
		limit = 20
	}
	messages, err := h.loadMessages(ctx, sessionID)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	items := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		item := map[string]any{
			"role":    m.Role,
			"content": truncate(m.Content, 2000),
			"time":    m.Time,
		}
		if m.ChangeSet != nil {
			item["files_changed"] = len(m.ChangeSet.Files)
		}
		items = append(items, item)
	}
	return jsonResult(map[string]any{"session_id": sessionID, "messages": items}), nil
}

// handleApprovalsList — 待审批列表（经 Caller 逃生舱调 approval/list，
// Engine 接口未包装该方法）。
func (h *MCPHost) handleApprovalsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := strings.TrimSpace(req.GetString("session_id", ""))
	var pending coreapi.PendingApprovalList
	listReq := coreapi.PendingApprovalListRequest{SessionID: sessionID}
	if err := h.engine.Caller().Call(ctx, generated.MethodApprovalList, listReq, &pending); err != nil {
		return errorResult(fmt.Sprintf("approval/list: %v", err)), nil
	}
	items := make([]map[string]any, 0, len(pending.Approvals))
	for _, a := range pending.Approvals {
		items = append(items, map[string]any{
			"approval_id": a.ApprovalID,
			"session_id":  a.SessionID,
			"tool_name":   a.ToolName,
			"reason":      a.Reason,
		})
	}
	return jsonResult(map[string]any{"approvals": items}), nil
}

// handleApprovalRespond — 审批决策映射（accept_for_session → acceptForSession）。
func (h *MCPHost) handleApprovalRespond(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	approvalID, err := req.RequireString("approval_id")
	if err != nil {
		return errorResult("approval_id is required"), nil
	}
	decision, err := req.RequireString("decision")
	if err != nil {
		return errorResult("decision is required"), nil
	}
	mapped, ok := mapApprovalDecision(strings.TrimSpace(decision))
	if !ok {
		return errorResult(fmt.Sprintf("invalid decision %q: expect accept / accept_for_session / decline / cancel", decision)), nil
	}
	if err := h.engine.Approvals().Respond(ctx, coreapi.ApprovalResponse{
		ApprovalID: strings.TrimSpace(approvalID),
		Decision:   mapped,
		Reason:     req.GetString("reason", ""),
	}); err != nil {
		return errorResult(fmt.Sprintf("approval/respond: %v", err)), nil
	}
	return jsonResult(map[string]any{
		"approval_id": approvalID,
		"decision":    mapped,
		"note":        "已提交决策；用 eos_task_wait 获取任务最终结果",
	}), nil
}

// handleInquiryRespond — 问询回应。
func (h *MCPHost) handleInquiryRespond(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	inquiryID, err := req.RequireString("inquiry_id")
	if err != nil {
		return errorResult("inquiry_id is required"), nil
	}
	if err := h.engine.Inquiries().Respond(ctx, coreapi.InquiryResponse{
		InquiryID: strings.TrimSpace(inquiryID),
		Option:    strings.TrimSpace(req.GetString("option", "")),
		Text:      req.GetString("text", ""),
	}); err != nil {
		return errorResult(fmt.Sprintf("inquiry/respond: %v", err)), nil
	}
	return jsonResult(map[string]any{
		"inquiry_id": inquiryID,
		"note":       "已提交回应；用 eos_task_wait 获取任务最终结果",
	}), nil
}

// ── 内部辅助 ──

// resolveExplicitSession 解析 eos_chat 的目标会话：参数 > _meta > 默认会话。
func (h *MCPHost) resolveExplicitSession(ctx context.Context, req mcp.CallToolRequest) (string, error) {
	if sid := strings.TrimSpace(req.GetString("session_id", "")); sid != "" {
		return sid, nil
	}
	if sid := sessionIDFromMeta(req); strings.TrimSpace(sid) != "" {
		return strings.TrimSpace(sid), nil
	}
	return h.ensureSession(ctx)
}

// applyModelOverrideOnce 对目标会话应用 --model 覆盖（每会话仅一次）。
func (h *MCPHost) applyModelOverrideOnce(ctx context.Context, sessionID string) error {
	if h.modelOverride == "" {
		return nil
	}
	h.mu.Lock()
	if h.modelApplied == nil {
		h.modelApplied = map[string]bool{}
	}
	applied := h.modelApplied[sessionID]
	h.mu.Unlock()
	if applied {
		return nil
	}
	session := coreapi.Session{ID: sessionID, WorkspaceRoot: h.workspaceRoot}
	if err := headless.ApplyModelOverride(ctx, h.engine, session, h.modelOverride); err != nil {
		return err
	}
	h.mu.Lock()
	h.modelApplied[sessionID] = true
	h.mu.Unlock()
	return nil
}

// sessionRunning 报告会话是否仍有活动 turn；会话不存在返回错误。
func (h *MCPHost) sessionRunning(ctx context.Context, sessionID string) (bool, error) {
	snapshot, err := h.sessionSnapshot(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return snapshot.Running, nil
}

func (h *MCPHost) sessionSnapshot(ctx context.Context, sessionID string) (coreapi.SessionSnapshot, error) {
	state, err := h.engine.State().Snapshot(ctx, coreapi.StateSnapshotRequest{})
	if err != nil {
		return coreapi.SessionSnapshot{}, fmt.Errorf("state/snapshot: %w", err)
	}
	for _, s := range state.Sessions {
		if s.ID == sessionID {
			return s, nil
		}
	}
	return coreapi.SessionSnapshot{}, fmt.Errorf("session %s not found", sessionID)
}

// loadMessages 读取会话消息历史。
func (h *MCPHost) loadMessages(ctx context.Context, sessionID string) ([]coreapi.SessionMessage, error) {
	messages, err := h.engine.Sessions().LoadMessages(ctx, coreapi.LoadSessionMessagesRequest{SessionID: sessionID})
	if err != nil {
		return nil, fmt.Errorf("session/messages/load: %w", err)
	}
	return messages, nil
}

// lastAssistantMessage 返回最后一条 assistant 文本消息（无则 nil）。
func (h *MCPHost) lastAssistantMessage(ctx context.Context, sessionID string) *coreapi.SessionMessage {
	messages, err := h.loadMessages(ctx, sessionID)
	if err != nil {
		return nil
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Type != "tool" {
			return &messages[i]
		}
	}
	return nil
}

// attachTurnSummary 用最近 assistant 消息补全 reply 与 files_changed；失败静默
// （结果主体已可用）。
func (h *MCPHost) attachTurnSummary(ctx context.Context, sessionID string, out *chatOutcome) {
	last := h.lastAssistantMessage(ctx, sessionID)
	if last == nil {
		return
	}
	if strings.TrimSpace(out.Reply) == "" {
		out.Reply = last.Content
	}
	if last.ChangeSet != nil {
		out.FilesChanged = make([]changedFile, 0, len(last.ChangeSet.Files))
		for _, f := range last.ChangeSet.Files {
			out.FilesChanged = append(out.FilesChanged, changedFile{
				Path:      f.Path,
				Status:    f.Status,
				Additions: f.Additions,
				Deletions: f.Deletions,
			})
		}
	}
	if errMsg, ok := last.Metadata["error"].(string); ok && errMsg != "" && out.Error == "" {
		out.Error = errMsg
	}
}

// firstPendingApproval 取会话第一条待审批（无则 nil）。
func (h *MCPHost) firstPendingApproval(ctx context.Context, sessionID string) *pendingView {
	var pending coreapi.PendingApprovalList
	req := coreapi.PendingApprovalListRequest{SessionID: sessionID}
	if err := h.engine.Caller().Call(ctx, generated.MethodApprovalList, req, &pending); err != nil {
		return nil
	}
	for _, a := range pending.Approvals {
		if sessionID == "" || a.SessionID == sessionID {
			return &pendingView{
				ApprovalID: a.ApprovalID,
				ToolName:   a.ToolName,
				Reason:     a.Reason,
			}
		}
	}
	return nil
}

// mapApprovalDecision 把宿主友好的决策名映射为内核 wire 枚举。
func mapApprovalDecision(decision string) (coreapi.ApprovalDecision, bool) {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "accept":
		return coreapi.ApprovalAccept, true
	case "accept_for_session", "acceptforsession", "acceptForSession":
		return coreapi.ApprovalAcceptForSession, true
	case "decline", "deny":
		return coreapi.ApprovalDecline, true
	case "cancel", "abort":
		return coreapi.ApprovalCancel, true
	}
	return "", false
}

func clampTimeout(secs int) time.Duration {
	if secs <= 0 {
		secs = defaultChatTimeoutSecs
	}
	if secs > maxChatTimeoutSecs {
		secs = maxChatTimeoutSecs
	}
	return time.Duration(secs) * time.Second
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

// jsonResult 把结构化结果包装为 MCP TextContent（JSON 文本）。
func jsonResult(payload any) *mcp.CallToolResult {
	bs, err := json.Marshal(payload)
	if err != nil {
		return errorResult(fmt.Sprintf("marshal result: %v", err))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(string(bs))},
	}
}
