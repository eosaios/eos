//go:build !legacy

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// The tests in this file exercise the default (Rust stdio) bridge wiring
// and do NOT depend on the legacy in-process runtime adapter, which has
// been moved behind the `legacy` build tag. They guard the recent fixes
// for the "新增模型弹窗只显示标题/步骤/取消" regression where the bridge
// hard-coded `mode = "runtime"`, masked Rust stdio as if it were the
// in-process runtime, and hid Rust core catalog failures behind local
// preview data.

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/protocol"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// newPipeStdioClient creates a StdioClient whose stream is wired to a
// net.Pipe connection instead of a real process. Returns the client and
// the server-side of the pipe, which the test should hand to its mock
// server goroutine.
func newPipeStdioClient(t *testing.T) (client *adapter.StdioClient, serverConn net.Conn) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	client = adapter.NewStdioClientWithStream(adapter.StdioClientOptions{}, clientConn)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := client.StartWithStream(ctx, clientConn); err != nil {
		t.Fatalf("StartWithStream() error = %v", err)
	}
	return client, serverConn
}

// emptyCatalogMock returns a function that handles JSON-RPC requests the
// bridge service fires during LoadBootstrap(). It returns:
//   - the configured default workspace for workspace/default and
//     workspace/last
//   - an empty state snapshot
//   - empty payloads for model/catalog (the regression scenario)
//
// Anything not explicitly handled is answered with a method-not-found
// error so we notice if the bootstrap flow starts asking for new data
// without a corresponding mock handler. The handler is intentionally
// permissive: it returns "result: nil" for every mutating call so the
// bootstrap flow is never blocked by the mock.
type emptyCatalogMock struct {
	workspace                   string
	defaultWorkspaceUnavailable bool
	turnStarted                 chan struct{}
	blockTurnStart              chan struct{}
	emitTurnProgressBeforeResp  bool

	mu                       sync.Mutex
	createSessionCount       int
	saveMessagesCount        int
	setCurrentSessionCount   int
	turnStartCount           int
	workspaceUseCount        int
	workspaceRememberCount   int
	workspaceRememberFGCount int
	workspaceAddCount        int
	createdSessionWorkspace  string
	savedMessagesWorkspace   string
	currentSessionWorkspace  string
	usedWorkspace            string
	rememberedWorkspace      string
	rememberForeground       bool
	turnStartSessionID       string
	turnStartInput           string
	session                  *coreapi.Session
}

func (h *emptyCatalogMock) serve(t *testing.T, conn net.Conn, done <-chan struct{}) {
	t.Helper()
	stream := protocoljsonrpc.NewStream(conn, conn)
	for {
		select {
		case <-done:
			return
		default:
		}
		msg, err := stream.ReadMessage()
		if err != nil {
			return
		}
		if msg.Request == nil {
			continue
		}
		req := msg.Request
		var resp protocoljsonrpc.Response
		var notify *protocoljsonrpc.Notification
		deferResponse := false

		switch req.Method {
		case protocoljsonrpc.MethodInitialize:
			result := coreapijsonrpc.InitializeResult{
				ServerName:      "eos-core-mock",
				ProtocolVersion: "v1",
				Methods:         protocoljsonrpc.AllCoreMethods(),
			}
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, result)

		case protocoljsonrpc.MethodWorkspaceDefault:
			if h.defaultWorkspaceUnavailable {
				resp, _ = protocoljsonrpc.NewErrorResponse(req.ID, protocoljsonrpc.CodeMethodNotFound, "method not found", nil)
				break
			}
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, h.workspace)

		case protocoljsonrpc.MethodWorkspaceLast,
			protocoljsonrpc.MethodWorkspaceResolve:
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, h.workspace)

		case protocoljsonrpc.MethodWorkspaceUse:
			var params coreapi.WorkspacePathRequest
			_ = json.Unmarshal(req.Params, &params)
			h.recordWorkspaceUse(params.Path)
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]bool{"ok": true})

		case protocoljsonrpc.MethodWorkspaceAdd:
			var params coreapi.WorkspacePathRequest
			_ = json.Unmarshal(req.Params, &params)
			h.recordWorkspaceAdd()
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]bool{"ok": true})

		case protocoljsonrpc.MethodWorkspaceRemember:
			var params coreapi.RememberWorkspaceRequest
			_ = json.Unmarshal(req.Params, &params)
			h.recordWorkspaceRemember(params)
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]bool{"ok": true})

		case protocoljsonrpc.MethodStateSnapshot:
			// Empty runtime snapshot — keeps the bootstrap flow
			// deterministic and forces the bridge to fall back to its
			// own defaults for workspaces, sessions, etc.
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, h.stateSnapshot())

		case protocoljsonrpc.MethodSessionList:
			var params coreapi.ListSessionsRequest
			_ = json.Unmarshal(req.Params, &params)
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, h.sessions(params.WorkspaceRoot))

		case protocoljsonrpc.MethodSessionCreate:
			var params coreapi.CreateSessionRequest
			_ = json.Unmarshal(req.Params, &params)
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, h.createSession(params))

		case protocoljsonrpc.MethodSessionMessagesLoad:
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []coreapi.SessionMessage{})

		case protocoljsonrpc.MethodSessionMessagesSave:
			var params coreapi.SaveSessionMessagesRequest
			_ = json.Unmarshal(req.Params, &params)
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, h.saveMessages(params))

		case protocoljsonrpc.MethodSessionSetCurrent:
			var params coreapi.SetCurrentSessionRequest
			_ = json.Unmarshal(req.Params, &params)
			h.recordSetCurrent(params)
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]bool{"ok": true})

		case protocoljsonrpc.MethodEventSubscribe:
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, coreapi.EventSubscription{ID: "sub-default"})

		case protocoljsonrpc.MethodTurnStart:
			var params coreapi.StartTurnRequest
			_ = json.Unmarshal(req.Params, &params)
			turn := h.recordTurnStart(params)
			if h.emitTurnProgressBeforeResp {
				itemID := turn.ID + "_msg_1"
				for _, event := range []protocol.Envelope{
					protocol.NewEvent(protocol.EventTypeTurnStarted, protocol.EventOptions{
						SessionID: params.SessionID,
						TurnID:    turn.ID,
						Payload:   map[string]any{"message_count": 1},
					}),
					protocol.NewEvent(protocol.EventTypeTurnItemStarted, protocol.EventOptions{
						SessionID: params.SessionID,
						TurnID:    turn.ID,
						Payload: map[string]any{
							"item": map[string]any{"kind": "agent_message", "id": itemID, "text": ""},
						},
					}),
					protocol.NewEvent(protocol.EventTypeTurnItemDelta, protocol.EventOptions{
						SessionID: params.SessionID,
						TurnID:    turn.ID,
						Payload: map[string]any{
							"item_id":    itemID,
							"delta_type": "text",
							"delta":      "hello from core",
						},
					}),
				} {
					notification, _ := protocoljsonrpc.NewNotification(protocoljsonrpc.NotificationEvent, event)
					_ = stream.WriteMessage(notification)
				}
			}
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, turn)
			event := protocol.NewEvent(protocol.EventTypeRequestDone, protocol.EventOptions{
				SessionID:     params.SessionID,
				RequestID:     turn.ID,
				CorrelationID: turn.ID,
				Payload:       map[string]any{"message": "ok"},
			})
			notification, _ := protocoljsonrpc.NewNotification(protocoljsonrpc.NotificationEvent, event)
			notify = &notification
			if h.blockTurnStart != nil {
				respCopy := resp
				notifyCopy := notify
				deferResponse = true
				go func() {
					select {
					case <-h.blockTurnStart:
					case <-done:
						return
					}
					_ = stream.WriteMessage(respCopy)
					if notifyCopy != nil {
						_ = stream.WriteMessage(*notifyCopy)
					}
				}()
			}

		case protocoljsonrpc.MethodModelList,
			protocoljsonrpc.MethodModelCatalog:
			// The regression case: the rust core returns an empty
			// model catalog. The bridge must report the degraded
			// core catalog and must not synthesize a local catalog.
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, []map[string]string{})

		default:
			// Mutating or unimplemented methods succeed with a nil
			// result so the bootstrap flow is not blocked. We still
			// log them so the test output reveals the actual method
			// surface the bridge relies on.
			t.Logf("mock core: %s (default ok)", req.Method)
			resp, _ = protocoljsonrpc.NewResultResponse(req.ID, map[string]any{})
		}

		if deferResponse {
			continue
		}
		_ = stream.WriteMessage(resp)
		if notify != nil {
			_ = stream.WriteMessage(*notify)
		}
	}
}

func (h *emptyCatalogMock) stateSnapshot() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session == nil {
		return map[string]any{
			"workspaces":      []coreapi.WorkspaceSnapshot{},
			"sessions":        []coreapi.SessionSnapshot{},
			"messages":        []coreapi.SessionMessage{},
			"current_session": nil,
		}
	}
	sessions := []coreapi.SessionSnapshot{}
	var current *coreapi.SessionSnapshot
	snapshot := coreapi.SessionSnapshot{
		ID:            h.session.ID,
		WorkspacePath: h.session.WorkspaceRoot,
		Title:         "新对话",
		UpdatedAt:     h.session.UpdatedAt,
		Active:        true,
	}
	sessions = append(sessions, snapshot)
	if h.currentSessionWorkspace != "" {
		current = &snapshot
	}
	currentSessionID := ""
	if h.session != nil && h.currentSessionWorkspace != "" {
		currentSessionID = h.session.ID
	}
	foregroundWorkspace := h.workspace
	if h.currentSessionWorkspace != "" {
		foregroundWorkspace = h.currentSessionWorkspace
	}
	return map[string]any{
		"foreground_workspace": foregroundWorkspace,
		"workspaces": []coreapi.WorkspaceSnapshot{{
			Path:             foregroundWorkspace,
			Name:             "workspace",
			Active:           true,
			CurrentSessionID: currentSessionID,
		}},
		"sessions":        sessions,
		"messages":        []coreapi.SessionMessage{},
		"current_session": current,
	}
}

func (h *emptyCatalogMock) sessions(workspace string) []coreapi.Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session == nil {
		return []coreapi.Session{}
	}
	if strings.TrimSpace(workspace) != "" && !sameWorkspacePath(h.session.WorkspaceRoot, workspace) {
		return []coreapi.Session{}
	}
	return []coreapi.Session{*h.session}
}

func (h *emptyCatalogMock) createSession(params coreapi.CreateSessionRequest) coreapi.Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	workspace := strings.TrimSpace(params.WorkspaceRoot)
	if workspace == "" {
		workspace = h.workspace
	}
	h.createSessionCount++
	h.createdSessionWorkspace = workspace
	now := time.Now()
	session := coreapi.Session{
		ID:            "session-default-1",
		WorkspaceRoot: workspace,
		Metadata: map[string]any{
			"title": strings.TrimSpace(params.Title),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	h.session = &session
	return session
}

func (h *emptyCatalogMock) saveMessages(params coreapi.SaveSessionMessagesRequest) coreapi.Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	workspace := strings.TrimSpace(params.WorkspaceRoot)
	if workspace == "" {
		workspace = h.workspace
	}
	h.saveMessagesCount++
	h.savedMessagesWorkspace = workspace
	now := time.Now()
	if h.session == nil {
		h.session = &coreapi.Session{
			ID:            strings.TrimSpace(params.SessionID),
			WorkspaceRoot: workspace,
			CreatedAt:     now,
		}
	}
	h.session.WorkspaceRoot = workspace
	h.session.UpdatedAt = now
	if h.session.Metadata == nil {
		h.session.Metadata = map[string]any{}
	}
	h.session.Metadata["rounds"] = len(params.Messages)
	return *h.session
}

func (h *emptyCatalogMock) recordSetCurrent(params coreapi.SetCurrentSessionRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.setCurrentSessionCount++
	h.currentSessionWorkspace = strings.TrimSpace(params.WorkspaceRoot)
}

func (h *emptyCatalogMock) recordWorkspaceUse(path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workspaceUseCount++
	h.usedWorkspace = strings.TrimSpace(path)
}

func (h *emptyCatalogMock) recordWorkspaceAdd() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workspaceAddCount++
}

func (h *emptyCatalogMock) recordWorkspaceRemember(params coreapi.RememberWorkspaceRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workspaceRememberCount++
	h.rememberedWorkspace = strings.TrimSpace(params.Path)
	h.rememberForeground = params.Foreground
	if params.Foreground {
		h.workspaceRememberFGCount++
	}
}

func (h *emptyCatalogMock) recordTurnStart(params coreapi.StartTurnRequest) coreapi.Turn {
	h.mu.Lock()
	h.turnStartCount++
	h.turnStartSessionID = strings.TrimSpace(params.SessionID)
	h.turnStartInput = params.Input
	turnID := strings.TrimSpace(params.TurnID)
	if turnID == "" {
		turnID = "turn-default-1"
	}
	now := time.Now()
	h.mu.Unlock()
	if h.turnStarted != nil {
		select {
		case h.turnStarted <- struct{}{}:
		default:
		}
	}
	return coreapi.Turn{
		ID:        turnID,
		SessionID: strings.TrimSpace(params.SessionID),
		Status:    "running",
		StartedAt: now,
		UpdatedAt: now,
	}
}

func (h *emptyCatalogMock) snapshotCounts() (createSession, saveMessages, setCurrent, turnStart int, createdWorkspace, savedWorkspace, currentWorkspace, turnSessionID, turnInput string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.createSessionCount,
		h.saveMessagesCount,
		h.setCurrentSessionCount,
		h.turnStartCount,
		h.createdSessionWorkspace,
		h.savedMessagesWorkspace,
		h.currentSessionWorkspace,
		h.turnStartSessionID,
		h.turnStartInput
}

func (h *emptyCatalogMock) workspaceMutationCounts() (useCount, rememberCount, rememberForegroundCount, addCount int, usedWorkspace, rememberedWorkspace string, rememberForeground bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.workspaceUseCount,
		h.workspaceRememberCount,
		h.workspaceRememberFGCount,
		h.workspaceAddCount,
		h.usedWorkspace,
		h.rememberedWorkspace,
		h.rememberForeground
}

func newRustStdioBridgeForTest(t *testing.T) *BridgeService {
	service, _ := newRustStdioBridgeWithMockForTest(t)
	return service
}

func newRustStdioBridgeWithMockForTest(t *testing.T) (*BridgeService, *emptyCatalogMock) {
	t.Helper()
	return newRustStdioBridgeWithMockOptionsForTest(t, false)
}

func newRustStdioBridgeWithMockOptionsForTest(t *testing.T, defaultWorkspaceUnavailable bool) (*BridgeService, *emptyCatalogMock) {
	t.Helper()
	configureWorkspaceTestEnvForRust(t)

	client, serverConn := newPipeStdioClient(t)
	workspace := t.TempDir()
	mock := &emptyCatalogMock{
		workspace:                   workspace,
		defaultWorkspaceUnavailable: defaultWorkspaceUnavailable,
		turnStarted:                 make(chan struct{}, 1),
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mock.serve(t, serverConn, stop)
	}()

	oldStarter := startBridgeStdioGateway
	resolved := adapter.StdioResolvedBinary{
		Path:         `C:\resolved\eos-core.exe`,
		ManifestPath: `C:\resolved\manifest.json`,
		Source:       "test-search",
		Target:       "x86_64-pc-windows-gnu",
	}
	startBridgeStdioGateway = func(context.Context, adapter.StdioClientOptions) (bridgeRuntimeGateway, func() error, adapter.StdioResolvedBinary, error) {
		gateway := adapter.NewStdioGateway(client)
		return gateway, func() error {
			_ = client.Close()
			return nil
		}, resolved, nil
	}
	t.Cleanup(func() {
		startBridgeStdioGateway = oldStarter
		close(stop)
		_ = client.Close()
		_ = serverConn.Close()
		wg.Wait()
	})

	service := NewBridgeServiceWithOptions(BridgeServiceOptions{
		RuntimeGateway:    "rust-stdio",
		StdioStartTimeout: 2 * time.Second,
	})
	t.Cleanup(func() { service.Close() })
	if service.runtimeGatewayMode != bridgeRuntimeGatewayModeRust {
		t.Fatalf("runtimeGatewayMode=%q, want %q", service.runtimeGatewayMode, bridgeRuntimeGatewayModeRust)
	}
	if service.runtimeGatewayStartError != "" {
		t.Fatalf("runtimeGatewayStartError=%q, want empty for rust-stdio", service.runtimeGatewayStartError)
	}
	return service, mock
}

func configureWorkspaceTestEnvForRust(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv(bridgeRuntimeGatewayEnv, "")
	t.Setenv(bridgeRuntimeGatewayAppServerEnv, "")
	t.Setenv(bridgeRuntimeGatewayAppServerArgsEnv, "")
	t.Setenv(bridgeRuntimeGatewayStartTimeoutEnv, "")
}

func TestLoadBootstrapReportsRustStdioModeAndUsesCoreReady(t *testing.T) {
	service := newRustStdioBridgeForTest(t)

	state := service.LoadBootstrap()
	if state.BridgeMode != "rust-stdio" {
		t.Fatalf("BridgeMode=%q, want rust-stdio", state.BridgeMode)
	}
	if !service.coreReady() {
		t.Fatal("coreReady()=false, want true for rust-stdio gateway")
	}
	if got := service.bridgeCoreMode(); got != "rust-stdio" {
		t.Fatalf("bridgeCoreMode()=%q, want rust-stdio", got)
	}
}

func TestSendChatFirstLaunchCreatesDefaultWorkspaceSessionOverRustStdio(t *testing.T) {
	service, mock := newRustStdioBridgeWithMockForTest(t)
	defaultWorkspace := filepath.Clean(mock.workspace)

	initial := service.LoadBootstrap()
	if _, err := os.Stat(defaultWorkspace); err != nil {
		t.Fatalf("expected default workspace dir %q: %v", defaultWorkspace, err)
	}
	if got := filepath.Clean(initial.ActiveWorkspace); got != defaultWorkspace {
		t.Fatalf("initial ActiveWorkspace=%q, want %q", got, defaultWorkspace)
	}
	foundDefault := false
	for _, item := range initial.Workspaces {
		if !sameWorkspacePath(item.Path, defaultWorkspace) {
			continue
		}
		foundDefault = true
		if item.Removable {
			t.Fatalf("default workspace card is removable: %+v", item)
		}
	}
	if !foundDefault {
		t.Fatalf("default workspace %q not found in initial bootstrap: %+v", defaultWorkspace, initial.Workspaces)
	}

	state, err := service.SendChatWithReasoning("", "", "你好", nil, "")
	if err != nil {
		t.Fatalf("SendChatWithReasoning() error = %v", err)
	}
	if got := filepath.Clean(state.ActiveWorkspace); got != defaultWorkspace {
		t.Fatalf("ActiveWorkspace=%q, want %q", got, defaultWorkspace)
	}
	if strings.TrimSpace(state.CurrentSessionID) == "" {
		t.Fatal("expected current session to be created")
	}
	if len(state.Messages) < 2 {
		t.Fatalf("Messages=%d, want user + assistant placeholder", len(state.Messages))
	}
	if state.Messages[0].Role != "user" || state.Messages[0].Content != "你好" {
		t.Fatalf("first message=%+v, want sent user message", state.Messages[0])
	}
	assistant := state.Messages[len(state.Messages)-1]
	if assistant.Role != "assistant" || assistant.State != "streaming" || assistant.Content != "思考中" {
		t.Fatalf("assistant placeholder=%+v, want streaming placeholder", assistant)
	}
	if !assistant.IsPlaceholder {
		t.Fatalf("assistant placeholder IsPlaceholder=%v, want true", assistant.IsPlaceholder)
	}

	select {
	case <-mock.turnStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for turn/start")
	}

	createCount, saveCount, setCurrentCount, turnStartCount, createdWorkspace, savedWorkspace, currentWorkspace, turnSessionID, turnInput := mock.snapshotCounts()
	if createCount == 0 {
		t.Fatal("expected session/create to be called")
	}
	if saveCount == 0 {
		t.Fatal("expected session/messages/save to be called")
	}
	if setCurrentCount == 0 {
		t.Fatal("expected session/set_current to be called")
	}
	if turnStartCount == 0 {
		t.Fatal("expected turn/start to be called")
	}
	if !sameWorkspacePath(createdWorkspace, defaultWorkspace) {
		t.Fatalf("session/create workspace=%q, want %q", createdWorkspace, defaultWorkspace)
	}
	if !sameWorkspacePath(savedWorkspace, defaultWorkspace) {
		t.Fatalf("session/messages/save workspace=%q, want %q", savedWorkspace, defaultWorkspace)
	}
	if !sameWorkspacePath(currentWorkspace, defaultWorkspace) {
		t.Fatalf("session/set_current workspace=%q, want %q", currentWorkspace, defaultWorkspace)
	}
	useCount, rememberCount, rememberForegroundCount, _, usedWorkspace, rememberedWorkspace, _ := mock.workspaceMutationCounts()
	if useCount == 0 {
		t.Fatal("expected workspace/use to be called")
	}
	if rememberCount == 0 {
		t.Fatal("expected workspace/remember to be called")
	}
	if !sameWorkspacePath(usedWorkspace, defaultWorkspace) {
		t.Fatalf("workspace/use path=%q, want %q", usedWorkspace, defaultWorkspace)
	}
	if !sameWorkspacePath(rememberedWorkspace, defaultWorkspace) {
		t.Fatalf("workspace/remember path=%q, want %q", rememberedWorkspace, defaultWorkspace)
	}
	if rememberForegroundCount == 0 {
		t.Fatal("expected workspace/remember to mark the default workspace foreground at least once")
	}
	if turnSessionID != state.CurrentSessionID {
		t.Fatalf("turn/start session=%q, want %q", turnSessionID, state.CurrentSessionID)
	}
	if turnInput != "你好" {
		t.Fatalf("turn/start input=%q, want 你好", turnInput)
	}

	finalState := service.LoadBootstrap()
	foundDefault = false
	for _, item := range finalState.Workspaces {
		if !sameWorkspacePath(item.Path, defaultWorkspace) {
			continue
		}
		foundDefault = true
		if item.Removable {
			t.Fatalf("default workspace card became removable: %+v", item)
		}
	}
	if !foundDefault {
		t.Fatalf("default workspace %q not found in final bootstrap: %+v", defaultWorkspace, finalState.Workspaces)
	}
}

func TestCreateAndEnsureSessionUseDefaultWorkspaceOverRustStdio(t *testing.T) {
	t.Run("create session", func(t *testing.T) {
		service, mock := newRustStdioBridgeWithMockForTest(t)
		defaultWorkspace := filepath.Clean(mock.workspace)

		state, err := service.CreateSession("")
		if err != nil {
			t.Fatalf("CreateSession(\"\") error = %v", err)
		}
		if got := filepath.Clean(state.ActiveWorkspace); got != defaultWorkspace {
			t.Fatalf("ActiveWorkspace=%q, want %q", got, defaultWorkspace)
		}
		if strings.TrimSpace(state.CurrentSessionID) == "" {
			t.Fatal("expected CreateSession(\"\") to create a current session")
		}

		createCount, _, setCurrentCount, _, createdWorkspace, _, currentWorkspace, _, _ := mock.snapshotCounts()
		if createCount == 0 {
			t.Fatal("expected session/create to be called")
		}
		if setCurrentCount == 0 {
			t.Fatal("expected session/set_current to be called")
		}
		if !sameWorkspacePath(createdWorkspace, defaultWorkspace) {
			t.Fatalf("session/create workspace=%q, want %q", createdWorkspace, defaultWorkspace)
		}
		if !sameWorkspacePath(currentWorkspace, defaultWorkspace) {
			t.Fatalf("session/set_current workspace=%q, want %q", currentWorkspace, defaultWorkspace)
		}
	})

	t.Run("ensure session", func(t *testing.T) {
		service, mock := newRustStdioBridgeWithMockForTest(t)
		defaultWorkspace := filepath.Clean(mock.workspace)

		state, err := service.EnsureWorkspaceSession("")
		if err != nil {
			t.Fatalf("EnsureWorkspaceSession(\"\") error = %v", err)
		}
		if got := filepath.Clean(state.ActiveWorkspace); got != defaultWorkspace {
			t.Fatalf("ActiveWorkspace=%q, want %q", got, defaultWorkspace)
		}
		if strings.TrimSpace(state.CurrentSessionID) == "" {
			t.Fatal("expected EnsureWorkspaceSession(\"\") to create a current session")
		}

		createCount, _, setCurrentCount, _, createdWorkspace, _, currentWorkspace, _, _ := mock.snapshotCounts()
		if createCount == 0 {
			t.Fatal("expected session/create to be called")
		}
		if setCurrentCount == 0 {
			t.Fatal("expected session/set_current to be called")
		}
		if !sameWorkspacePath(createdWorkspace, defaultWorkspace) {
			t.Fatalf("session/create workspace=%q, want %q", createdWorkspace, defaultWorkspace)
		}
		if !sameWorkspacePath(currentWorkspace, defaultWorkspace) {
			t.Fatalf("session/set_current workspace=%q, want %q", currentWorkspace, defaultWorkspace)
		}
	})
}

func TestDefaultWorkspaceFallsBackToUserHomeWhenRustDefaultUnavailable(t *testing.T) {
	service, mock := newRustStdioBridgeWithMockOptionsForTest(t, true)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	defaultWorkspace := filepath.Clean(filepath.Join(home, ".eos", "workspace"))

	state := service.LoadBootstrap()
	if got := filepath.Clean(state.ActiveWorkspace); got != defaultWorkspace {
		t.Fatalf("ActiveWorkspace=%q, want fallback %q", got, defaultWorkspace)
	}
	if _, err := os.Stat(defaultWorkspace); err != nil {
		t.Fatalf("expected fallback default workspace dir %q: %v", defaultWorkspace, err)
	}

	state, err = service.SendChatWithReasoning("", "", "你好", nil, "")
	if err != nil {
		t.Fatalf("SendChatWithReasoning() error = %v", err)
	}
	if got := filepath.Clean(state.ActiveWorkspace); got != defaultWorkspace {
		t.Fatalf("ActiveWorkspace=%q, want fallback %q", got, defaultWorkspace)
	}

	createCount, _, _, _, createdWorkspace, _, _, _, _ := mock.snapshotCounts()
	if createCount == 0 {
		t.Fatal("expected session/create to be called")
	}
	if !sameWorkspacePath(createdWorkspace, defaultWorkspace) {
		t.Fatalf("session/create workspace=%q, want fallback %q", createdWorkspace, defaultWorkspace)
	}
}

func TestCancelFirstLaunchSendWithOptimisticSessionIDDoesNotBlock(t *testing.T) {
	service, mock := newRustStdioBridgeWithMockForTest(t)
	mock.blockTurnStart = make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-mock.blockTurnStart:
		default:
			close(mock.blockTurnStart)
		}
	})

	state, err := service.SendChatWithReasoning("", "", "你好", nil, "")
	if err != nil {
		t.Fatalf("SendChatWithReasoning() error = %v", err)
	}
	realSessionID := strings.TrimSpace(state.CurrentSessionID)
	if realSessionID == "" {
		t.Fatal("expected real session id")
	}

	select {
	case <-mock.turnStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked turn/start")
	}

	done := make(chan struct{})
	var cancelState BootstrapState
	var cancelErr error
	go func() {
		cancelState, cancelErr = service.CancelSession("session-local-front")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("CancelSession() blocked while turn/start was still pending")
	}
	if cancelErr != nil {
		t.Fatalf("CancelSession() error = %v", cancelErr)
	}
	if cancelState.CurrentSessionID != realSessionID {
		t.Fatalf("cancel CurrentSessionID=%q, want %q", cancelState.CurrentSessionID, realSessionID)
	}
	session := sessionCardByID(cancelState.Sessions, realSessionID)
	if session == nil {
		t.Fatalf("cancel state missing session %q: %+v", realSessionID, cancelState.Sessions)
	}
	if session.Running {
		t.Fatalf("cancel state session still running: %+v", session)
	}
	if len(cancelState.Messages) == 0 {
		t.Fatal("cancel state returned no messages")
	}
	assistant := cancelState.Messages[len(cancelState.Messages)-1]
	if assistant.Role != "assistant" || assistant.State != "failed" {
		t.Fatalf("cancel assistant message=%+v, want stopped assistant", assistant)
	}
	// "已停止生成" 现在走 status item（不再写 content）。
	found := false
	for _, item := range assistant.Items {
		if item.Kind == "status" && item.Text == "已停止生成" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cancel assistant missing status item '已停止生成': items=%+v", assistant.Items)
	}
	if notificationContainsForRust(cancelState.Notifications, "已手动停止") {
		t.Fatalf("did not expect stop notification, got %+v", cancelState.Notifications)
	}
}

func TestSendChatConsumesRustTurnEventsBeforeTurnStartReturns(t *testing.T) {
	service, mock := newRustStdioBridgeWithMockForTest(t)
	mock.emitTurnProgressBeforeResp = true
	mock.blockTurnStart = make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-mock.blockTurnStart:
		default:
			close(mock.blockTurnStart)
		}
	})

	state, err := service.SendChatWithReasoning("", "", "你好", nil, "")
	if err != nil {
		t.Fatalf("SendChatWithReasoning() error = %v", err)
	}
	if strings.TrimSpace(state.CurrentSessionID) == "" {
		t.Fatal("expected current session id")
	}
	select {
	case <-mock.turnStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for turn/start")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		latest := service.LoadBootstrap()
		if len(latest.Messages) > 0 {
			assistant := latest.Messages[len(latest.Messages)-1]
			// delta 现在进 items 的 agent_message（不再写 content）。
			for _, item := range assistant.Items {
				if item.Kind == "agent_message" && strings.Contains(item.Text, "hello from core") {
					if assistant.State != "streaming" {
						t.Fatalf("assistant state=%q, want streaming while turn/start is blocked", assistant.State)
					}
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	latest := service.LoadBootstrap()
	t.Fatalf("assistant never received pre-response turn.item_delta; messages=%+v", latest.Messages)
}

func TestLoadBootstrapDoesNotFallbackToPreviewCatalogWhenRustCoreReturnsEmpty(t *testing.T) {
	service := newRustStdioBridgeForTest(t)

	state := service.LoadBootstrap()
	if len(state.ModelCatalog.Providers) != 0 {
		t.Fatalf("ModelCatalog.Providers=%+v, want empty when Rust core returns empty catalog", state.ModelCatalog.Providers)
	}
	if len(state.ModelCatalog.Presets) != 0 {
		t.Fatalf("ModelCatalog.Presets=%+v, want empty when Rust core returns empty catalog", state.ModelCatalog.Presets)
	}
	if !state.ModelCatalog.AllowCustomProvider || !state.ModelCatalog.AllowCustomModel {
		t.Fatalf("custom flags were not preserved on degraded catalog: %+v", state.ModelCatalog)
	}
	if !strings.Contains(service.modelCatalogFallback, "模型目录数据不可用") {
		t.Fatalf("modelCatalogFallback=%q, want user-facing 模型目录数据不可用 copy", service.modelCatalogFallback)
	}
	if hasInternalTerminology(service.modelCatalogFallback) {
		t.Fatalf("modelCatalogFallback=%q, must not expose internal terminology", service.modelCatalogFallback)
	}
}

func TestLoadBootstrapAddsResourceCheckAndNotificationWhenModelCatalogFallsBack(t *testing.T) {
	service := newRustStdioBridgeForTest(t)

	state := service.LoadBootstrap()
	check := resourceCheckByNameForRust(state.ResourceChecks, "核心初始化失败")
	if check == nil {
		t.Fatalf("expected '核心初始化失败' resource check, got %+v", state.ResourceChecks)
	}
	if check.Status != "warning" {
		t.Fatalf("fallback check status=%q, want warning", check.Status)
	}
	if !strings.Contains(check.Detail, "模型目录数据不可用") {
		t.Fatalf("fallback check detail=%q, want user-facing 模型目录数据不可用 copy", check.Detail)
	}
	if hasInternalTerminology(check.Detail) {
		t.Fatalf("fallback check detail=%q, must not expose internal terminology", check.Detail)
	}
	if !notificationContainsForRust(state.Notifications, "核心初始化失败") {
		t.Fatalf("expected '核心初始化失败' notification, got %+v", state.Notifications)
	}
}

func TestLoadBootstrapBridgeModeResourceCheckMatchesRustStdioSemantics(t *testing.T) {
	service := newRustStdioBridgeForTest(t)

	state := service.LoadBootstrap()
	transport := resourceCheckByNameForRust(state.ResourceChecks, "核心就绪状态")
	if transport == nil {
		t.Fatal("expected 核心就绪状态 resource check")
	}
	if transport.Status != "ready" {
		t.Fatalf("transport.Status=%q, want ready", transport.Status)
	}
	if hasInternalTerminology(transport.Detail) {
		t.Fatalf("transport.Detail=%q, must not expose internal terminology", transport.Detail)
	}
	sharedBridge := resourceCheckByNameForRust(state.ResourceChecks, "共享核心桥接")
	if sharedBridge == nil {
		t.Fatal("expected '共享核心桥接' resource check")
	}
	if sharedBridge.Status != "ready" {
		t.Fatalf("sharedBridge.Status=%q, want ready for rust-stdio", sharedBridge.Status)
	}
	if hasInternalTerminology(sharedBridge.Detail) {
		t.Fatalf("sharedBridge.Detail=%q, must not expose internal terminology", sharedBridge.Detail)
	}
}

func TestLoadBootstrapModelListDoesNotFallbackWhenRustCoreReturnsEmpty(t *testing.T) {
	service := newRustStdioBridgeForTest(t)

	state := service.LoadBootstrap()
	if len(state.Models) != 0 {
		t.Fatalf("Models=%+v, want empty when Rust core returns empty model/list", state.Models)
	}
}

func TestBridgeCoreModeReportsUnavailableWhenNoGatewayConfigured(t *testing.T) {
	configureWorkspaceTestEnvForRust(t)
	oldStarter := startBridgeStdioGateway
	startBridgeStdioGateway = func(context.Context, adapter.StdioClientOptions) (bridgeRuntimeGateway, func() error, adapter.StdioResolvedBinary, error) {
		return nil, nil, adapter.StdioResolvedBinary{
			Path:         `C:\broken\eos-core.exe`,
			ManifestPath: `C:\broken\manifest.json`,
			Source:       "test-search",
			Target:       "x86_64-pc-windows-gnu",
		}, errSimulatedRustUnavailable
	}
	t.Cleanup(func() { startBridgeStdioGateway = oldStarter })

	service := NewBridgeServiceWithOptions(BridgeServiceOptions{
		RuntimeGateway:    "rust-stdio",
		StdioStartTimeout: 100 * time.Millisecond,
	})
	t.Cleanup(func() { service.Close() })
	if got := service.bridgeCoreMode(); got != "unavailable" {
		t.Fatalf("bridgeCoreMode()=%q, want unavailable", got)
	}
	if service.coreReady() {
		t.Fatal("coreReady()=true, want false when gateway failed to start")
	}
	state := service.LoadBootstrap()
	if state.BridgeMode != "unavailable" {
		t.Fatalf("BridgeMode=%q, want unavailable", state.BridgeMode)
	}
	if len(state.ModelCatalog.Providers) != 0 {
		t.Fatalf("ModelCatalog.Providers=%+v, want empty when core unavailable", state.ModelCatalog.Providers)
	}
	if len(state.ModelCatalog.Presets) != 0 {
		t.Fatalf("ModelCatalog.Presets=%+v, want empty when core unavailable", state.ModelCatalog.Presets)
	}
	if !strings.Contains(service.modelCatalogFallback, "模型目录数据不可用") {
		t.Fatalf("modelCatalogFallback=%q, want user-facing 模型目录数据不可用 copy", service.modelCatalogFallback)
	}
	if hasInternalTerminology(service.modelCatalogFallback) {
		t.Fatalf("modelCatalogFallback=%q, must not expose internal terminology", service.modelCatalogFallback)
	}
}

var errSimulatedRustUnavailable = stubError("simulated rust core unavailable")

type stubError string

func (e stubError) Error() string { return string(e) }

func resourceCheckByNameForRust(items []ResourceCheck, name string) *ResourceCheck {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func notificationContainsForRust(items []NotificationItem, title string) bool {
	for _, item := range items {
		if strings.Contains(item.Title, title) || strings.Contains(item.Message, title) {
			return true
		}
	}
	return false
}

// hasInternalTerminology guards user-facing bridge copy against leaking
// engineering terms (binary paths, RPC, "Rust core", "fallback", transport
// implementation names, etc.) and the self-deceiving "please retry" phrasing.
// Such details belong in slog, not the UI.
func hasInternalTerminology(s string) bool {
	for _, needle := range []string{
		"兜底", "本地目录", "Rust core", "RPC", "JSON-RPC", "[解析结果",
		"model/catalog", "legacy", "eos-core", "--stdio", "Wails", "进程内",
		"请稍后重试", "重新打开向导",
	} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
