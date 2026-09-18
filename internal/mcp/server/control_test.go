package server

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// control_test.go — eos_* 控制/委托工具的单测：事件剧本驱动 eos_chat、
// 审批/问询快速返回、task_wait/cancel、决策映射与注册完整性。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/coreapi/generated"
	"github.com/eosaios/eos/pkg/protocol"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ── fakes ──

type controlSessions struct {
	currentErr error
	messages   []coreapi.SessionMessage
}

func (s controlSessions) Current(context.Context, coreapi.CurrentSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, s.currentErr
}
func (s controlSessions) Create(_ context.Context, req coreapi.CreateSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{ID: "sess-mcp", WorkspaceRoot: req.WorkspaceRoot}, nil
}
func (s controlSessions) Resume(context.Context, coreapi.ResumeSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s controlSessions) List(context.Context, coreapi.ListSessionsRequest) ([]coreapi.Session, error) {
	return nil, nil
}
func (s controlSessions) SetCurrent(context.Context, coreapi.SetCurrentSessionRequest) error {
	return nil
}
func (s controlSessions) Delete(context.Context, coreapi.DeleteSessionRequest) error { return nil }
func (s controlSessions) Rename(context.Context, coreapi.RenameSessionRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s controlSessions) SetMeta(context.Context, coreapi.SetSessionMetaRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}
func (s controlSessions) LoadMessages(context.Context, coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	return s.messages, nil
}
func (s controlSessions) SaveMessages(context.Context, coreapi.SaveSessionMessagesRequest) (coreapi.Session, error) {
	return coreapi.Session{}, nil
}

type controlTurns struct {
	mu        sync.Mutex
	started   []coreapi.StartTurnRequest
	interrupt []coreapi.TurnRef
	startErr  error
}

func (t *controlTurns) Start(_ context.Context, req coreapi.StartTurnRequest) (coreapi.Turn, error) {
	t.mu.Lock()
	t.started = append(t.started, req)
	t.mu.Unlock()
	if t.startErr != nil {
		return coreapi.Turn{}, t.startErr
	}
	return coreapi.Turn{ID: req.TurnID, SessionID: req.SessionID, Status: "running"}, nil
}
func (t *controlTurns) Interrupt(_ context.Context, ref coreapi.TurnRef) error {
	t.mu.Lock()
	t.interrupt = append(t.interrupt, ref)
	t.mu.Unlock()
	return nil
}

// startedCount / interrupts 线程安全读取（Start 在 StartTurnAsync 的
// goroutine 里记录，与断言存在调度竞态）。
func (t *controlTurns) startedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.started)
}
func (t *controlTurns) firstStart() coreapi.StartTurnRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.started) == 0 {
		return coreapi.StartTurnRequest{}
	}
	return t.started[0]
}
func (t *controlTurns) Resume(context.Context, coreapi.TurnRef) (coreapi.Turn, error) {
	return coreapi.Turn{}, nil
}

type controlEvents struct {
	events []protocol.Envelope
}

func (e controlEvents) Subscribe(context.Context, coreapi.EventFilter) (<-chan protocol.Envelope, error) {
	ch := make(chan protocol.Envelope, len(e.events)+1)
	for _, ev := range e.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

type controlState struct {
	sessions []coreapi.SessionSnapshot
}

func (s controlState) Snapshot(context.Context, coreapi.StateSnapshotRequest) (coreapi.StateSnapshot, error) {
	return coreapi.StateSnapshot{Sessions: s.sessions}, nil
}

type controlApprovals struct {
	responded []coreapi.ApprovalResponse
	err       error
}

func (a *controlApprovals) Respond(_ context.Context, r coreapi.ApprovalResponse) error {
	a.responded = append(a.responded, r)
	return a.err
}

type controlInquiries struct {
	responded []coreapi.InquiryResponse
}

func (q *controlInquiries) Respond(_ context.Context, r coreapi.InquiryResponse) error {
	q.responded = append(q.responded, r)
	return nil
}

type controlCaller struct {
	pending coreapi.PendingApprovalList
}

func (c controlCaller) Call(_ context.Context, method string, _ any, out any) error {
	if method != generated.MethodApprovalList {
		return errors.New("unexpected method " + method)
	}
	data, _ := json.Marshal(c.pending)
	return json.Unmarshal(data, out)
}

type controlEngine struct {
	coreapi.Engine
	sessions  controlSessions
	turns     *controlTurns
	events    coreapi.EventSubscriber
	state     controlState
	approvals *controlApprovals
	inquiries *controlInquiries
	caller    controlCaller
}

func (e *controlEngine) Sessions() coreapi.SessionService { return e.sessions }
func (e *controlEngine) Turns() coreapi.TurnService       { return e.turns }
func (e *controlEngine) Events() coreapi.EventSubscriber  { return e.events }
func (e *controlEngine) State() coreapi.StateService      { return e.state }
func (e *controlEngine) Approvals() coreapi.ApprovalService {
	return e.approvals
}
func (e *controlEngine) Inquiries() coreapi.InquiryService { return e.inquiries }
func (e *controlEngine) Caller() coreapi.Caller            { return e.caller }

func newControlHost(engine *controlEngine) *MCPHost {
	return &MCPHost{engine: engine, chatTimeout: 30}
}

func chatRequest(params map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = params
	req.Params.RawArguments = mustJSON(params)
	return req
}

func mustJSON(v any) json.RawMessage {
	bs, _ := json.Marshal(v)
	return bs
}

// decodeChatOutcome 反序列化工具结果 JSON。
func decodeChatOutcome(t *testing.T, result *mcp.CallToolResult) chatOutcome {
	t.Helper()
	var out chatOutcome
	if err := json.Unmarshal([]byte(textContent(result)), &out); err != nil {
		t.Fatalf("decode result: %v\nraw: %s", err, textContent(result))
	}
	return out
}

// ── eos_chat ──

func assistantMessages() []coreapi.SessionMessage {
	return []coreapi.SessionMessage{
		{Role: "user", Content: "修一下 bug"},
		{Role: "assistant", Content: "已修复 main.go", ChangeSet: &coreapi.MessageChangeSet{
			Summary: "1 个文件已更改",
			Files: []coreapi.ChangedFile{
				{Path: "main.go", Status: "M", Additions: 3, Deletions: 1},
			},
		}},
	}
}

func TestHandleChatHappyPath(t *testing.T) {
	engine := &controlEngine{
		sessions: controlSessions{messages: assistantMessages()},
		turns:    &controlTurns{},
		events: controlEvents{events: []protocol.Envelope{
			{EventType: protocol.EventTypeItemDelta, Payload: map[string]any{"delta_type": "text", "delta": "正在处理"}},
			{EventType: protocol.EventTypeItemCompleted, Payload: map[string]any{"item": map[string]any{"kind": "agent_message", "text": "已修复 main.go"}}},
			{EventType: protocol.EventTypeRequestDone, Payload: map[string]any{}},
		}},
		caller: controlCaller{},
	}
	host := newControlHost(engine)

	result, err := host.handleChat(context.Background(), chatRequest(map[string]any{
		"message":    "修一下 bug",
		"session_id": "sess-1",
	}))
	if err != nil {
		t.Fatalf("handleChat: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", textContent(result))
	}
	out := decodeChatOutcome(t, result)
	if out.Status != chatStatusCompleted {
		t.Fatalf("status=%q, want completed", out.Status)
	}
	if out.Reply != "已修复 main.go" {
		t.Fatalf("reply=%q", out.Reply)
	}
	if out.SessionID != "sess-1" {
		t.Fatalf("session_id=%q, want sess-1", out.SessionID)
	}
	if len(out.FilesChanged) != 1 || out.FilesChanged[0].Path != "main.go" {
		t.Fatalf("files_changed=%+v", out.FilesChanged)
	}
	waitFor(t, "turn/start issued", func() bool { return engine.turns.startedCount() == 1 })
	if first := engine.turns.firstStart(); first.Input != "修一下 bug" || first.SessionID != "sess-1" {
		t.Fatalf("turn/start = %+v", first)
	}
}

func TestHandleChatWaitingApprovalFastReturn(t *testing.T) {
	engine := &controlEngine{
		sessions: controlSessions{},
		turns:    &controlTurns{},
		events: controlEvents{events: []protocol.Envelope{
			{EventType: protocol.EventTypeTurnWaitingApproval, Payload: map[string]any{"session_id": "sess-1"}},
		}},
		caller: controlCaller{pending: coreapi.PendingApprovalList{Approvals: []coreapi.PendingApprovalItem{
			{ApprovalID: "appr-1", SessionID: "sess-1", ToolName: "edit", Reason: "high risk"},
		}}},
	}
	host := newControlHost(engine)

	result, _ := host.handleChat(context.Background(), chatRequest(map[string]any{
		"message": "删库", "session_id": "sess-1", "timeout_secs": 5,
	}))
	out := decodeChatOutcome(t, result)
	if out.Status != chatStatusWaitingApproval {
		t.Fatalf("status=%q, want waiting_approval", out.Status)
	}
	if out.Approval == nil || out.Approval.ApprovalID != "appr-1" || out.Approval.ToolName != "edit" {
		t.Fatalf("approval=%+v, want appr-1/edit", out.Approval)
	}
	if !strings.Contains(out.Note, "eos_approval_respond") {
		t.Fatalf("note should guide approval loop: %q", out.Note)
	}
}

func TestHandleChatRequestUserInputPassthrough(t *testing.T) {
	engine := &controlEngine{
		sessions: controlSessions{},
		turns:    &controlTurns{},
		events: controlEvents{events: []protocol.Envelope{
			{EventType: protocol.EventType("turn.request_user_input"), Payload: map[string]any{
				"call_id": "ciu-1", "questions": []any{map[string]any{"id": "q1"}},
			}},
		}},
		caller: controlCaller{},
	}
	host := newControlHost(engine)

	result, _ := host.handleChat(context.Background(), chatRequest(map[string]any{
		"message": "做计划", "session_id": "sess-1",
	}))
	out := decodeChatOutcome(t, result)
	if out.Status != chatStatusUserInput {
		t.Fatalf("status=%q, want request_user_input", out.Status)
	}
	if out.Questions["call_id"] != "ciu-1" {
		t.Fatalf("questions payload lost: %+v", out.Questions)
	}
}

func TestHandleChatFailure(t *testing.T) {
	engine := &controlEngine{
		sessions: controlSessions{},
		turns:    &controlTurns{},
		events: controlEvents{events: []protocol.Envelope{
			{EventType: protocol.EventTypeRequestFailed, Payload: map[string]any{"error": "model 429"}},
		}},
		caller: controlCaller{},
	}
	host := newControlHost(engine)

	result, _ := host.handleChat(context.Background(), chatRequest(map[string]any{
		"message": "hi", "session_id": "sess-1",
	}))
	out := decodeChatOutcome(t, result)
	if out.Status != chatStatusError || out.Error != "model 429" {
		t.Fatalf("status=%q error=%q, want error/model 429", out.Status, out.Error)
	}
}

func TestHandleChatTimeoutReportsRunning(t *testing.T) {
	// 事件通道立即关闭且无终止事件：AwaitTurn 只等 start 完成——用永不返回
	// start 的 fake 无法干净构造，改用阻塞 subscribe：通道保持打开无事件，
	// timeout_secs=1 触发 DeadlineExceeded 分支。
	engine := &controlEngine{
		sessions: controlSessions{},
		turns:    &controlTurns{},
		events:   blockingEvents{},
		caller:   controlCaller{},
	}
	host := newControlHost(engine)

	start := time.Now()
	result, _ := host.handleChat(context.Background(), chatRequest(map[string]any{
		"message": "长任务", "session_id": "sess-1", "timeout_secs": 1,
	}))
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout not honored, elapsed=%v", elapsed)
	}
	out := decodeChatOutcome(t, result)
	if out.Status != chatStatusTimeout {
		t.Fatalf("status=%q, want timeout", out.Status)
	}
	if !strings.Contains(out.Note, "eos_task_status") {
		t.Fatalf("note should point to task_status: %q", out.Note)
	}
}

type blockingEvents struct{}

func (blockingEvents) Subscribe(context.Context, coreapi.EventFilter) (<-chan protocol.Envelope, error) {
	return make(chan protocol.Envelope), nil // 永不推送也永不关闭
}

func TestHandleChatRequiresMessage(t *testing.T) {
	host := newControlHost(&controlEngine{turns: &controlTurns{}})
	result, _ := host.handleChat(context.Background(), chatRequest(map[string]any{"session_id": "s"}))
	if !result.IsError {
		t.Fatal("missing message must be an error result")
	}
}

// ── task_wait / status / cancel ──

func TestHandleTaskWaitIdleReturnsLastOutcome(t *testing.T) {
	engine := &controlEngine{
		sessions: controlSessions{messages: assistantMessages()},
		turns:    &controlTurns{},
		state: controlState{sessions: []coreapi.SessionSnapshot{
			{ID: "sess-1", Running: false},
		}},
		caller: controlCaller{},
	}
	host := newControlHost(engine)

	result, _ := host.handleTaskWait(context.Background(), chatRequest(map[string]any{"session_id": "sess-1"}))
	out := decodeChatOutcome(t, result)
	if out.Status != chatStatusCompleted || out.Reply != "已修复 main.go" {
		t.Fatalf("status=%q reply=%q", out.Status, out.Reply)
	}
	if len(out.FilesChanged) != 1 {
		t.Fatalf("files_changed=%+v", out.FilesChanged)
	}
}

func TestHandleTaskWaitUnknownSession(t *testing.T) {
	host := newControlHost(&controlEngine{turns: &controlTurns{}})
	result, _ := host.handleTaskWait(context.Background(), chatRequest(map[string]any{"session_id": "nope"}))
	if !result.IsError {
		t.Fatal("unknown session must error")
	}
}

func TestHandleTaskCancelInterruptsActiveTurn(t *testing.T) {
	turns := &controlTurns{}
	host := newControlHost(&controlEngine{turns: turns})

	result, _ := host.handleTaskCancel(context.Background(), chatRequest(map[string]any{"session_id": "sess-1"}))
	if result.IsError {
		t.Fatalf("cancel failed: %s", textContent(result))
	}
	if got := interruptsOf(turns); len(got) != 1 || got[0].SessionID != "sess-1" || got[0].TurnID != "" {
		t.Fatalf("interrupt calls=%+v, want empty-turn interrupt for sess-1", got)
	}
}

func TestHandleTaskStatusIncludesApproval(t *testing.T) {
	engine := &controlEngine{
		sessions: controlSessions{messages: assistantMessages()},
		turns:    &controlTurns{},
		state: controlState{sessions: []coreapi.SessionSnapshot{
			{ID: "sess-1", Running: false, NeedsAttention: true, PendingPrompts: 1},
		}},
		caller: controlCaller{pending: coreapi.PendingApprovalList{Approvals: []coreapi.PendingApprovalItem{
			{ApprovalID: "appr-9", SessionID: "sess-1", ToolName: "bash"},
		}}},
	}
	host := newControlHost(engine)

	result, _ := host.handleTaskStatus(context.Background(), chatRequest(map[string]any{"session_id": "sess-1"}))
	var status map[string]any
	if err := json.Unmarshal([]byte(textContent(result)), &status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status["needs_attention"] != true {
		t.Fatalf("needs_attention=%v", status["needs_attention"])
	}
	pending, ok := status["pending_approval"].(map[string]any)
	if !ok || pending["approval_id"] != "appr-9" {
		t.Fatalf("pending_approval=%+v", status["pending_approval"])
	}
}

// ── 审批/问询 ──

func TestMapApprovalDecision(t *testing.T) {
	cases := map[string]coreapi.ApprovalDecision{
		"accept":             coreapi.ApprovalAccept,
		"ACCEPT":             coreapi.ApprovalAccept,
		"accept_for_session": coreapi.ApprovalAcceptForSession,
		"decline":            coreapi.ApprovalDecline,
		"deny":               coreapi.ApprovalDecline,
		"cancel":             coreapi.ApprovalCancel,
	}
	for in, want := range cases {
		got, ok := mapApprovalDecision(in)
		if !ok || got != want {
			t.Fatalf("mapApprovalDecision(%q)=%q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := mapApprovalDecision("explode"); ok {
		t.Fatal("unknown decision must be rejected")
	}
}

func TestHandleApprovalRespondRejectsBadDecision(t *testing.T) {
	host := newControlHost(&controlEngine{approvals: &controlApprovals{}})
	result, _ := host.handleApprovalRespond(context.Background(), chatRequest(map[string]any{
		"approval_id": "a1", "decision": "yes",
	}))
	if !result.IsError {
		t.Fatal("bad decision must error")
	}
}

func TestHandleApprovalRespondForwards(t *testing.T) {
	approvals := &controlApprovals{}
	host := newControlHost(&controlEngine{approvals: approvals})

	result, _ := host.handleApprovalRespond(context.Background(), chatRequest(map[string]any{
		"approval_id": "a1", "decision": "accept_for_session", "reason": "ok",
	}))
	if result.IsError {
		t.Fatalf("respond failed: %s", textContent(result))
	}
	if len(approvals.responded) != 1 ||
		approvals.responded[0].ApprovalID != "a1" ||
		approvals.responded[0].Decision != coreapi.ApprovalAcceptForSession {
		t.Fatalf("responded=%+v", approvals.responded)
	}
}

func TestHandleInquiryRespondForwards(t *testing.T) {
	inquiries := &controlInquiries{}
	host := newControlHost(&controlEngine{inquiries: inquiries})

	result, _ := host.handleInquiryRespond(context.Background(), chatRequest(map[string]any{
		"inquiry_id": "q1", "option": "proceed", "text": "go",
	}))
	if result.IsError {
		t.Fatalf("respond failed: %s", textContent(result))
	}
	if len(inquiries.responded) != 1 || inquiries.responded[0].Option != "proceed" {
		t.Fatalf("responded=%+v", inquiries.responded)
	}
}

// ── 会话工具与注册完整性 ──

func TestHandleSessionCreate(t *testing.T) {
	host := newControlHost(&controlEngine{turns: &controlTurns{}})
	result, _ := host.handleSessionCreate(context.Background(), chatRequest(map[string]any{
		"workspace": "C:/ws", "title": "任务A",
	}))
	if result.IsError {
		t.Fatalf("create failed: %s", textContent(result))
	}
	var created map[string]any
	_ = json.Unmarshal([]byte(textContent(result)), &created)
	if created["session_id"] != "sess-mcp" {
		t.Fatalf("session_id=%v", created["session_id"])
	}
}

func TestHandleSessionHistory(t *testing.T) {
	host := newControlHost(&controlEngine{
		sessions: controlSessions{messages: assistantMessages()},
		turns:    &controlTurns{},
	})
	result, _ := host.handleSessionHistory(context.Background(), chatRequest(map[string]any{
		"session_id": "sess-1", "limit": 1,
	}))
	if result.IsError {
		t.Fatalf("history failed: %s", textContent(result))
	}
	var payload struct {
		Messages []map[string]any `json:"messages"`
	}
	_ = json.Unmarshal([]byte(textContent(result)), &payload)
	if len(payload.Messages) != 1 || payload.Messages[0]["role"] != "assistant" {
		t.Fatalf("messages=%+v (want tail 1 assistant)", payload.Messages)
	}
	if payload.Messages[0]["files_changed"] != float64(1) {
		t.Fatalf("files_changed flag lost: %+v", payload.Messages[0])
	}
}

func TestRegisterControlToolsRegistersAllTen(t *testing.T) {
	host := newControlHost(&controlEngine{turns: &controlTurns{}})
	s := server.NewMCPServer("eos-test", "0")
	host.registerControlTools(s)
	for _, name := range []string{
		"eos_chat", "eos_task_wait", "eos_task_status", "eos_task_cancel",
		"eos_session_create", "eos_sessions_list", "eos_session_history",
		"eos_approvals_list", "eos_approval_respond", "eos_inquiry_respond",
	} {
		if s.GetTool(name) == nil {
			t.Fatalf("control tool %s not registered", name)
		}
	}
}

func TestClampTimeoutBounds(t *testing.T) {
	if got := clampTimeout(0); got != 600*time.Second {
		t.Fatalf("clampTimeout(0)=%v, want 600s", got)
	}
	if got := clampTimeout(99999); got != 3600*time.Second {
		t.Fatalf("clampTimeout(99999)=%v, want 3600s", got)
	}
	if got := clampTimeout(30); got != 30*time.Second {
		t.Fatalf("clampTimeout(30)=%v", got)
	}
}

func interruptsOf(t *controlTurns) []coreapi.TurnRef {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]coreapi.TurnRef(nil), t.interrupt...)
}

// waitFor 轮询断言条件成立（200ms 间隔，3s 上限）。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
