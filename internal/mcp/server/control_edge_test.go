package server

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// control_edge_test.go — eos_* 控制工具的边角分支：
//   - sessions/list 的工作区过滤（EqualFold）与 state/snapshot 失败
//   - approvals/list 的列表映射与 Caller 失败
//   - session/create 的默认值回落与 Create 失败
//   - task_cancel / task_status / session_history 的缺参与错误通道
//   - approval/respond / inquiry/respond 的缺参、非法决策与 Respond 失败
//   - applyModelOverrideOnce 的空覆盖早退 + Models 失败传播 + 已应用去重
//   - truncate 边界 / clampTimeout 上下限 / jsonResult 序列化失败

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

// ── 可注入错误的局部 fakes（组合共享 fakes）──

type edgeState struct {
	controlState
	err error
}

func (s edgeState) Snapshot(context.Context, coreapi.StateSnapshotRequest) (coreapi.StateSnapshot, error) {
	if s.err != nil {
		return coreapi.StateSnapshot{}, s.err
	}
	return s.controlState.Snapshot(context.Background(), coreapi.StateSnapshotRequest{})
}

type edgeSessions struct {
	controlSessions
	createErr error
	loadErr   error
}

func (s edgeSessions) Create(ctx context.Context, req coreapi.CreateSessionRequest) (coreapi.Session, error) {
	if s.createErr != nil {
		return coreapi.Session{}, s.createErr
	}
	return s.controlSessions.Create(ctx, req)
}

func (s edgeSessions) LoadMessages(ctx context.Context, req coreapi.LoadSessionMessagesRequest) ([]coreapi.SessionMessage, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.controlSessions.LoadMessages(ctx, req)
}

type edgeTurns struct {
	*controlTurns
	interruptErr error
}

func (t edgeTurns) Interrupt(ctx context.Context, ref coreapi.TurnRef) error {
	if t.interruptErr != nil {
		return t.interruptErr
	}
	return t.controlTurns.Interrupt(ctx, ref)
}

type edgeCaller struct {
	controlCaller
	err error
}

func (c edgeCaller) Call(ctx context.Context, method string, _ any, out any) error {
	if c.err != nil {
		return c.err
	}
	return c.controlCaller.Call(ctx, method, nil, out)
}

type edgeModels struct {
	listErr  error
	setCalls []coreapi.SetSessionModelRequest
	setErr   error
}

func (m *edgeModels) List(context.Context) ([]coreapi.ModelConfig, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return []coreapi.ModelConfig{{Name: "gpt-test", Model: "gpt-test", ProviderID: "openai"}}, nil
}
func (m *edgeModels) Catalog(context.Context) (coreapi.ModelCatalogState, error) {
	return coreapi.ModelCatalogState{}, nil
}
func (m *edgeModels) SetSession(_ context.Context, req coreapi.SetSessionModelRequest) error {
	m.setCalls = append(m.setCalls, req)
	return m.setErr
}

// 其余 ModelService 方法：本测试不触达，返回零值。
func (m *edgeModels) Upsert(context.Context, coreapi.UpsertModelRequest) error { return nil }
func (m *edgeModels) Save(context.Context, coreapi.ModelSaveRequest) error    { return nil }
func (m *edgeModels) Delete(context.Context, coreapi.ModelNameRequest) error  { return nil }
func (m *edgeModels) Activate(context.Context, coreapi.ModelNameRequest) error {
	return nil
}
func (m *edgeModels) SyncEnv(context.Context) error { return nil }
func (m *edgeModels) Context(context.Context, coreapi.ModelContextRequest) (coreapi.ModelContextSnapshot, error) {
	return coreapi.ModelContextSnapshot{}, nil
}
func (m *edgeModels) SetWorkspace(context.Context, coreapi.SetWorkspaceModelRequest) error {
	return nil
}
func (m *edgeModels) ClearWorkspace(context.Context, coreapi.ClearWorkspaceModelRequest) error {
	return nil
}
func (m *edgeModels) ClearSession(context.Context, coreapi.ClearSessionModelRequest) error {
	return nil
}

type edgeEngine struct {
	controlEngine
	state     edgeState
	sessions  edgeSessions
	models    *edgeModels
	turnsEdge edgeTurns
	caller    edgeCaller
}

func (e *edgeEngine) Sessions() coreapi.SessionService { return e.sessions }
func (e *edgeEngine) State() coreapi.StateService      { return e.state }
func (e *edgeEngine) Models() coreapi.ModelService     { return e.models }
func (e *edgeEngine) Turns() coreapi.TurnService       { return e.turnsEdge }
func (e *edgeEngine) Caller() coreapi.Caller           { return e.caller }

func newEdgeEngine() *edgeEngine {
	base := &controlTurns{}
	approvals := &controlApprovals{}
	inquiries := &controlInquiries{}
	engine := &edgeEngine{
		state:     edgeState{},
		sessions:  edgeSessions{},
		models:    &edgeModels{},
		turnsEdge: edgeTurns{controlTurns: base, interruptErr: nil},
		caller:    edgeCaller{},
	}
	engine.controlEngine.turns = base
	engine.controlEngine.approvals = approvals
	engine.controlEngine.inquiries = inquiries
	return engine
}

// ── sessions/list ──

func TestHandleSessionsListFiltersByWorkspace(t *testing.T) {
	engine := newEdgeEngine()
	engine.state.sessions = []coreapi.SessionSnapshot{
		{ID: "s1", Title: "one", WorkspacePath: "C:/work/alpha", Running: true, MessageCount: 3},
		{ID: "s2", Title: "two", WorkspacePath: "C:/work/beta"},
	}
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	result, _ := host.handleSessionsList(context.Background(), chatRequest(map[string]any{"workspace": "c:/work/ALPHA"}))
	text := textContent(result)
	if !strings.Contains(text, `"count":1`) || !strings.Contains(text, "s1") || strings.Contains(text, "s2") {
		t.Fatalf("filtered list missing/extra sessions: %s", text)
	}
	// 空工作区：全部返回
	resultAll, _ := host.handleSessionsList(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(resultAll), `"count":2`) {
		t.Fatalf("unfiltered count wrong: %s", textContent(resultAll))
	}
}

func TestHandleSessionsListSnapshotError(t *testing.T) {
	engine := newEdgeEngine()
	engine.state.err = errors.New("state unreachable")
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	result, _ := host.handleSessionsList(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(result), "state unreachable") {
		t.Fatalf("snapshot error not surfaced: %s", textContent(result))
	}
}

// ── approvals/list ──

func TestHandleApprovalsListMapsEntries(t *testing.T) {
	engine := newEdgeEngine()
	engine.caller.pending = coreapi.PendingApprovalList{
		Approvals: []coreapi.PendingApprovalItem{
			{ApprovalID: "appr-1", SessionID: "sess-1", ToolName: "run_command", Reason: "rm -rf"},
		},
	}
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	result, _ := host.handleApprovalsList(context.Background(), chatRequest(map[string]any{"session_id": "sess-1"}))
	text := textContent(result)
	for _, want := range []string{"appr-1", "run_command", "rm -rf"} {
		if !strings.Contains(text, want) {
			t.Fatalf("approvals list missing %s: %s", want, text)
		}
	}
}

func TestHandleApprovalsListCallerError(t *testing.T) {
	engine := newEdgeEngine()
	engine.caller.err = errors.New("approval rpc down")
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	result, _ := host.handleApprovalsList(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(result), "approval rpc down") {
		t.Fatalf("caller error not surfaced: %s", textContent(result))
	}
}

// ── session/create ──

func TestHandleSessionCreateDefaultsAndError(t *testing.T) {
	engine := newEdgeEngine()
	host := newControlHost(&engine.controlEngine)
	host.engine = engine
	host.workspaceRoot = "C:/work/default"

	// 显式工作区与标题
	result, _ := host.handleSessionCreate(context.Background(), chatRequest(map[string]any{
		"workspace": "C:/explicit", "title": "  定制标题  ",
	}))
	text := textContent(result)
	if !strings.Contains(text, "sess-mcp") || !strings.Contains(text, "C:/explicit") {
		t.Fatalf("create result wrong: %s", text)
	}
	// 默认回落：workspace→host.workspaceRoot，title→"MCP session"
	resultDefault, _ := host.handleSessionCreate(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(resultDefault), "C:/work/default") {
		t.Fatalf("default workspace missing: %s", textContent(resultDefault))
	}
	// Create 失败
	engine.sessions.createErr = errors.New("disk full")
	resultErr, _ := host.handleSessionCreate(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(resultErr), "disk full") {
		t.Fatalf("create error not surfaced: %s", textContent(resultErr))
	}
}

// ── task_cancel / task_status / session_history 边角 ──

func TestHandleTaskCancelEdgeBranches(t *testing.T) {
	engine := newEdgeEngine()
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	missing, _ := host.handleTaskCancel(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(missing), "session_id is required") {
		t.Fatalf("missing session_id not rejected: %s", textContent(missing))
	}
	engine.turnsEdge.interruptErr = errors.New("interrupt rejected")
	failed, _ := host.handleTaskCancel(context.Background(), chatRequest(map[string]any{"session_id": "sess-1"}))
	if !strings.Contains(textContent(failed), "interrupt rejected") {
		t.Fatalf("interrupt error not surfaced: %s", textContent(failed))
	}
}

func TestHandleTaskStatusMissingIDAndNotFound(t *testing.T) {
	engine := newEdgeEngine()
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	missing, _ := host.handleTaskStatus(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(missing), "session_id is required") {
		t.Fatalf("missing session_id not rejected: %s", textContent(missing))
	}
	notFound, _ := host.handleTaskStatus(context.Background(), chatRequest(map[string]any{"session_id": "ghost"}))
	if !strings.Contains(textContent(notFound), "not found") {
		t.Fatalf("unknown session not reported: %s", textContent(notFound))
	}
}

func TestHandleSessionHistoryLimitAndErrors(t *testing.T) {
	engine := newEdgeEngine()
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	missing, _ := host.handleSessionHistory(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(missing), "session_id is required") {
		t.Fatalf("missing session_id not rejected: %s", textContent(missing))
	}
	// limit<=0 回落 20：25 条消息取尾 20
	long := make([]coreapi.SessionMessage, 0, 25)
	for i := 0; i < 25; i++ {
		long = append(long, coreapi.SessionMessage{Role: "user", Content: "m"})
	}
	engine.sessions.messages = long
	result, _ := host.handleSessionHistory(context.Background(), chatRequest(map[string]any{"session_id": "sess-1", "limit": 0}))
	if !strings.Contains(textContent(result), `"messages"`) {
		t.Fatalf("history result malformed: %s", textContent(result))
	}
	var decoded struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Messages) != 20 {
		t.Fatalf("limit fallback got %d messages, want 20", len(decoded.Messages))
	}
	// LoadMessages 失败
	engine.sessions.loadErr = errors.New("corrupt store")
	failed, _ := host.handleSessionHistory(context.Background(), chatRequest(map[string]any{"session_id": "sess-1"}))
	if !strings.Contains(textContent(failed), "corrupt store") {
		t.Fatalf("load error not surfaced: %s", textContent(failed))
	}
}

// ── approval/respond / inquiry/respond 边角 ──

func TestHandleApprovalRespondEdgeBranches(t *testing.T) {
	engine := newEdgeEngine()
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	missing, _ := host.handleApprovalRespond(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(missing), "approval_id is required") {
		t.Fatalf("missing approval_id not rejected: %s", textContent(missing))
	}
	missingDecision, _ := host.handleApprovalRespond(context.Background(), chatRequest(map[string]any{"approval_id": "a1"}))
	if !strings.Contains(textContent(missingDecision), "decision is required") {
		t.Fatalf("missing decision not rejected: %s", textContent(missingDecision))
	}
	invalid, _ := host.handleApprovalRespond(context.Background(), chatRequest(map[string]any{"approval_id": "a1", "decision": "maybe"}))
	if !strings.Contains(textContent(invalid), "invalid decision") {
		t.Fatalf("invalid decision not rejected: %s", textContent(invalid))
	}
	engine.controlEngine.approvals.err = errors.New("respond conflict")
	failed, _ := host.handleApprovalRespond(context.Background(), chatRequest(map[string]any{"approval_id": "a1", "decision": "accept"}))
	if !strings.Contains(textContent(failed), "respond conflict") {
		t.Fatalf("respond error not surfaced: %s", textContent(failed))
	}
}

func TestHandleInquiryRespondMissingID(t *testing.T) {
	engine := newEdgeEngine()
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	missing, _ := host.handleInquiryRespond(context.Background(), chatRequest(map[string]any{}))
	if !strings.Contains(textContent(missing), "inquiry_id is required") {
		t.Fatalf("missing inquiry_id not rejected: %s", textContent(missing))
	}
}

// ── applyModelOverrideOnce ──

func TestApplyModelOverrideOnceBranches(t *testing.T) {
	engine := newEdgeEngine()
	host := newControlHost(&engine.controlEngine)
	host.engine = engine

	// 空覆盖：立即成功且不触碰 Models
	if err := host.applyModelOverrideOnce(context.Background(), "sess-1"); err != nil {
		t.Fatalf("empty override should be no-op, got %v", err)
	}
	if len(engine.models.setCalls) != 0 {
		t.Fatalf("empty override touched models: %+v", engine.models.setCalls)
	}
	// Models.List 失败：错误传播
	host.modelOverride = "gpt-test"
	engine.models.listErr = errors.New("catalog down")
	if err := host.applyModelOverrideOnce(context.Background(), "sess-1"); err == nil || !strings.Contains(err.Error(), "catalog down") {
		t.Fatalf("list error not propagated: %v", err)
	}
	// 成功路径：SetSession 命中会话 + 二次调用去重（不再打 Models）
	engine.models.listErr = nil
	if err := host.applyModelOverrideOnce(context.Background(), "sess-1"); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(engine.models.setCalls) != 1 || engine.models.setCalls[0].SessionID != "sess-1" {
		t.Fatalf("SetSession calls wrong: %+v", engine.models.setCalls)
	}
	if err := host.applyModelOverrideOnce(context.Background(), "sess-1"); err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	if len(engine.models.setCalls) != 1 {
		t.Fatalf("override not deduped per session: %+v", engine.models.setCalls)
	}
	// SetSession 失败：错误传播且不标记已应用
	engine.models.setErr = errors.New("session model locked")
	if err := host.applyModelOverrideOnce(context.Background(), "sess-2"); err == nil || !strings.Contains(err.Error(), "session model locked") {
		t.Fatalf("set error not propagated: %v", err)
	}
	if host.modelApplied["sess-2"] {
		t.Fatal("failed apply must not be marked applied")
	}
}

// ── 纯函数边界 ──

func TestTruncateBoundary(t *testing.T) {
	if got := truncate("abc", 3); got != "abc" {
		t.Fatalf("at-limit should be identity, got %q", got)
	}
	if got := truncate("abcd", 3); got != "abc…" {
		t.Fatalf("over-limit got %q", got)
	}
}

func TestJSONResultMarshalFailure(t *testing.T) {
	result := jsonResult(map[string]any{"ch": make(chan int)})
	if !strings.Contains(textContent(result), "marshal result") {
		t.Fatalf("marshal failure not surfaced: %s", textContent(result))
	}
}
