package adapter

// 错误臂扫荡：scripted 服务器对所有请求回 JSON-RPC error，反射遍历
// StdioGateway 的全部错误返回方法，批量点亮 `if err != nil` 返回臂。
// 参数一律零值——错误臂在 marshal 之后、响应解析处触发，与参数内容无关。

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/protocol"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// newErrorReplayGateway 起一个 net.Pipe 服务器：所有请求回注入错误。
func newErrorReplayGateway(t *testing.T) *StdioGateway {
	t.Helper()
	client, serverConn := newPipeStdioClient(t)
	go func() {
		stream := protocoljsonrpc.NewStream(serverConn, serverConn)
		for {
			msg, err := stream.ReadMessage()
			if err != nil {
				return
			}
			if msg.Request == nil {
				continue
			}
			if err := stream.WriteMessage(protocoljsonrpc.Response{
				ID:    msg.Request.ID,
				Error: &protocoljsonrpc.Error{Code: -32000, Message: "injected failure"},
			}); err != nil {
				return
			}
		}
	}()
	return NewStdioGateway(client)
}

var ctxType = reflect.TypeOf((*context.Context)(nil)).Elem()
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// newMixedReplayGateway 成功回放表 + 指定 method 回错误。
func newMixedReplayGateway(t *testing.T, replies map[string]any, errorMethods map[string]bool) *StdioGateway {
	t.Helper()
	client, serverConn := newPipeStdioClient(t)
	go func() {
		stream := protocoljsonrpc.NewStream(serverConn, serverConn)
		for {
			msg, err := stream.ReadMessage()
			if err != nil {
				return
			}
			if msg.Request == nil {
				continue
			}
			if errorMethods[msg.Request.Method] {
				if err := stream.WriteMessage(protocoljsonrpc.Response{
					ID:    msg.Request.ID,
					Error: &protocoljsonrpc.Error{Code: -32000, Message: "injected failure"},
				}); err != nil {
					return
				}
				continue
			}
			result, ok := replies[msg.Request.Method]
			if !ok {
				result = map[string]any{}
			}
			raw, _ := json.Marshal(result)
			if err := stream.WriteMessage(protocoljsonrpc.Response{
				ID:     msg.Request.ID,
				Result: raw,
			}); err != nil {
				return
			}
		}
	}()
	return NewStdioGateway(client)
}

func TestStdioGatewayErrorArmsSweep(t *testing.T) {
	g := newErrorReplayGateway(t)
	gw := reflect.ValueOf(g)
	typ := gw.Type()

	called, errorArms := 0, 0
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		mt := m.Type
		if mt.IsVariadic() || strings.Contains(m.Name, "Close") {
			continue
		}
		hasError := mt.NumOut() > 0 && mt.Out(mt.NumOut()-1) == errorType
		args := make([]reflect.Value, 0, mt.NumIn()-1)
		for j := 1; j < mt.NumIn(); j++ {
			pt := mt.In(j)
			if pt == ctxType {
				args = append(args, reflect.ValueOf(context.Background()))
				continue
			}
			args = append(args, reflect.Zero(pt))
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panic: %v", m.Name, r)
				}
			}()
			results := gw.Method(i).Call(args)
			if hasError {
				if err, _ := results[mt.NumOut()-1].Interface().(error); err == nil {
					t.Errorf("%s: 注入错误后应返回 error", m.Name)
				}
				errorArms++
			}
			called++
		}()
	}
	if called < 150 || errorArms < 100 {
		t.Fatalf("扫荡覆盖异常偏少: called=%d errorArms=%d", called, errorArms)
	}
}

// TestStdioGatewaySnapshotFallbacksOnError 模式/快照 getter 在 RPC 失败时
// 回落零值（ExecutionMode/SandboxMode/ReasoningLevel/RuntimeSnapshot）。
func TestStdioGatewaySnapshotFallbacksOnError(t *testing.T) {
	g := newErrorReplayGateway(t)
	if got := g.ExecutionMode(); got != "" {
		t.Fatalf("ExecutionMode 错误回落应空: %q", got)
	}
	if got := g.SandboxMode(); got != "" {
		t.Fatalf("SandboxMode 错误回落应空: %q", got)
	}
	if got := g.ReasoningLevel(); got != "" {
		t.Fatalf("ReasoningLevel 错误回落应空: %q", got)
	}
	if snap := g.RuntimeSnapshot(); snap.ForegroundWorkspace != "" || snap.CurrentSession != nil {
		t.Fatalf("RuntimeSnapshot 错误回落应零值: %+v", snap)
	}
}

// TestStdioRpcSuccessBranches 成功回复下的体内分支：getter 成功臂、
// 归档过滤链、会话工作区解析、订阅 ID 回退、turn 终态回落事件。
func TestStdioRpcSuccessBranches(t *testing.T) {
	sessions := []any{
		map[string]any{"id": "s-arch", "workspace_root": "/ws", "metadata": map[string]any{"archived": true}},
		map[string]any{"id": "s-live", "workspace_root": "/ws", "metadata": map[string]any{"archived": false}},
		map[string]any{"id": "s-nometa", "workspace_root": "/ws"},
		map[string]any{"id": "s-badmeta", "workspace_root": "/ws", "metadata": map[string]any{"archived": "yes"}},
	}
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodWorkspaceDefault: "ws-default",
		protocoljsonrpc.MethodWorkspaceLast:    "ws-last",
		protocoljsonrpc.MethodTaskCleanup:      map[string]any{"count": 2},
		protocoljsonrpc.MethodRemoteRepoCurrent: map[string]any{
			"ok":    true,
			"state": map[string]any{"url": "https://git.example/x.git"},
		},
		protocoljsonrpc.MethodSessionList:      sessions,
		protocoljsonrpc.MethodSessionSetMeta:   map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodEventSubscribe:   map[string]any{"id": "sub-fallback"},
		protocoljsonrpc.MethodEventUnsubscribe: map[string]any{},
		protocoljsonrpc.MethodTurnResume:       map[string]any{"id": "turn-r", "session_id": "s-1", "status": "completed"},
	})
	ctx := context.Background()

	if got := g.DefaultWorkspacePath(); got != "ws-default" {
		t.Fatalf("DefaultWorkspacePath = %q", got)
	}
	if got := g.LastWorkspace(); got != "ws-last" {
		t.Fatalf("LastWorkspace = %q", got)
	}
	if got := g.CleanupTasks(); got != 2 {
		t.Fatalf("CleanupTasks = %d", got)
	}
	if state, ok := g.CurrentRemoteRepo(); !ok {
		t.Fatalf("CurrentRemoteRepo ok=false: %+v", state)
	}

	// 归档过滤：只有 metadata.archived==true 的会话入选。
	archived, err := g.CoreListArchivedSessionsRPC(ctx)
	if err != nil || len(archived) != 1 || archived[0].ID != "s-arch" {
		t.Fatalf("CoreListArchivedSessionsRPC = %+v, %v", archived, err)
	}

	// 会话工作区解析：命中 + 未命中两臂。
	if ws, err := g.ResolveSessionWorkspace("s-live"); err != nil || ws != "/ws" {
		t.Fatalf("ResolveSessionWorkspace hit = %q, %v", ws, err)
	}
	if _, err := g.ResolveSessionWorkspace("missing"); err == nil {
		t.Fatal("ResolveSessionWorkspace 未命中应报错")
	}

	// set_meta 成功臂（归档开关 + 会话沙箱模式）。
	if err := g.CoreArchiveSessionRPC(ctx, "s-1", true); err != nil {
		t.Fatalf("CoreArchiveSessionRPC: %v", err)
	}
	if err := g.CoreSetSessionSandboxModeRPC(ctx, "s-1", "full_access"); err != nil {
		t.Fatalf("CoreSetSessionSandboxModeRPC: %v", err)
	}

	// 订阅：subscription_id 缺失回退 id 字段；nil ctx 走 rpcCtx()。
	events, unsubscribe, err := g.CoreSubscribeEventsRPC(nil, "s-1", "", "", 8)
	if err != nil {
		t.Fatalf("CoreSubscribeEventsRPC: %v", err)
	}
	if events == nil {
		t.Fatal("订阅应返回事件通道")
	}
	unsubscribe()
	unsubscribe() // once 语义：二次调用不重复请求

	// resume 流：nil ctx + 空 turnID 自动生成，终态 completed 触发回落事件。
	ch, _, err := g.CoreResumeTurnStreamRPC(nil, "s-1", "")
	if err != nil {
		t.Fatalf("CoreResumeTurnStreamRPC: %v", err)
	}
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("resume 事件通道提前关闭")
		}
		if ev.TurnID == "" {
			t.Fatalf("回落事件缺 turnID: %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resume 终态回落事件未在时限内到达")
	}
}

// TestStdioRpcTurnStreamPushedEvents 服务器推送 notification 事件：
// 订阅通道 → out 转发 → 终态事件收口（channel 关闭）。
func TestStdioRpcTurnStreamPushedEvents(t *testing.T) {
	done := make(chan struct{})
	client, serverConn := newPipeStdioClient(t)
	go func() {
		defer close(done)
		stream := protocoljsonrpc.NewStream(serverConn, serverConn)
		for {
			msg, err := stream.ReadMessage()
			if err != nil {
				return
			}
			if msg.Request == nil {
				continue
			}
			switch msg.Request.Method {
			case protocoljsonrpc.MethodEventSubscribe, protocoljsonrpc.MethodEventUnsubscribe:
				_ = stream.WriteMessage(protocoljsonrpc.Response{
					ID: msg.Request.ID, Result: json.RawMessage(`{"subscription_id":"sub-1"}`),
				})
			case protocoljsonrpc.MethodTurnStart:
				_ = stream.WriteMessage(protocoljsonrpc.Response{
					ID: msg.Request.ID, Result: json.RawMessage(`{"id":"t-fixed","session_id":"s-1","status":"running"}`),
				})
				// 转一条普通事件 + 一条终态事件（request.completed）。
				for _, typ := range []string{"turn.delta", "request.completed"} {
					payload, _ := json.Marshal(map[string]any{
						"version":    "v1",
						"event_id":   "e-" + typ,
						"event_type": typ,
						"session_id": "s-1",
						"turn_id":    "t-fixed",
						"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
						"source":     "runtime",
						"payload":    map[string]any{"text": "hi"},
					})
					_ = stream.WriteMessage(protocoljsonrpc.Notification{
						Method: protocoljsonrpc.NotificationEvent,
						Params: payload,
					})
				}
			default:
				_ = stream.WriteMessage(protocoljsonrpc.Response{
					ID: msg.Request.ID, Result: json.RawMessage(`{}`),
				})
			}
		}
	}()
	g := NewStdioGateway(client)

	ch, _, err := g.CoreStartTurnStreamWithRequestRPC(context.Background(), coreapi.StartTurnRequest{
		SessionID: "s-1", TurnID: "t-fixed", Input: "hi",
	})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	sawDelta, sawClose := false, false
	deadline := time.After(5 * time.Second)
	for !(sawDelta && sawClose) {
		select {
		case ev, ok := <-ch:
			if !ok {
				sawClose = true
				continue
			}
			if ev.EventType == "turn.delta" {
				sawDelta = true
			}
			if ev.EventType == "request.completed" {
				// 终态事件转发后流应收口。
			}
		case <-deadline:
			t.Fatalf("推送事件未按期到达: delta=%v close=%v", sawDelta, sawClose)
		}
	}
	if !sawDelta {
		t.Fatal("未收到 turn.delta 转发事件")
	}
}

// TestStdioRpcTurnStreamCancelArms 流侧取消竞态臂：goroutine 停在外层
// select 时 cancel（Done/通道关闭两臂随机命中），以及 out 缓冲填满停在
// 内层发送时 cancel（Done 分支确定性命中）。迭代多轮让随机臂都命中。
func TestStdioRpcTurnStreamCancelArms(t *testing.T) {
	newServer := func(t *testing.T, pushCount int) *StdioGateway {
		t.Helper()
		client, serverConn := newPipeStdioClient(t)
		go func() {
			stream := protocoljsonrpc.NewStream(serverConn, serverConn)
			for {
				msg, err := stream.ReadMessage()
				if err != nil {
					return
				}
				if msg.Request == nil {
					continue
				}
				switch msg.Request.Method {
				case protocoljsonrpc.MethodEventSubscribe, protocoljsonrpc.MethodEventUnsubscribe:
					_ = stream.WriteMessage(protocoljsonrpc.Response{
						ID: msg.Request.ID, Result: json.RawMessage(`{"subscription_id":"sub-1"}`),
					})
				case protocoljsonrpc.MethodTurnStart:
					_ = stream.WriteMessage(protocoljsonrpc.Response{
						ID: msg.Request.ID, Result: json.RawMessage(`{"id":"t-c","session_id":"s-1","status":"running"}`),
					})
					for i := 0; i < pushCount; i++ {
						payload, _ := json.Marshal(map[string]any{
							"version": "v1", "event_id": "e", "event_type": "turn.delta",
							"session_id": "s-1", "turn_id": "t-c",
							"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
							"source":    "runtime", "payload": map[string]any{"seq": i},
						})
						_ = stream.WriteMessage(protocoljsonrpc.Notification{
							Method: protocoljsonrpc.NotificationEvent, Params: payload,
						})
					}
				default:
					_ = stream.WriteMessage(protocoljsonrpc.Response{
						ID: msg.Request.ID, Result: json.RawMessage(`{}`),
					})
				}
			}
		}()
		return NewStdioGateway(client)
	}
	waitClosed := func(ch <-chan Event) bool {
		deadline := time.After(5 * time.Second)
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return true
				}
			case <-deadline:
				return false
			}
		}
	}

	// 外层 parked 时 cancel：多轮命中 Done 与订阅通道关闭两臂。
	for i := 0; i < 6; i++ {
		g := newServer(t, 0)
		ctx, cancel := context.WithCancel(context.Background())
		ch, _, err := g.CoreStartTurnStreamWithRequestRPC(ctx, coreapi.StartTurnRequest{
			SessionID: "s-1", TurnID: "t-c", Input: "hi",
		})
		if err != nil {
			cancel()
			t.Fatalf("iter %d start: %v", i, err)
		}
		time.Sleep(30 * time.Millisecond) // 等流 goroutine 停到外层 select
		cancel()
		if !waitClosed(ch) {
			cancel()
			t.Fatalf("iter %d 取消后通道未收口", i)
		}
	}

	// 缓冲填满停在 inner send 时 cancel：Done 分支确定性命中。
	g := newServer(t, 70)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _, err := g.CoreStartTurnStreamWithRequestRPC(ctx, coreapi.StartTurnRequest{
		SessionID: "s-1", TurnID: "t-c", Input: "hi",
	})
	if err != nil {
		t.Fatalf("full-buffer start: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // 等 64 缓冲填满、goroutine 停在内层发送
	cancel()
	if !waitClosed(ch) {
		t.Fatal("满缓冲取消后通道未收口")
	}

	// 订阅通道关闭臂（!ok）：start 成功返回后立即 cancel，streamCtx.Done
	// 与订阅通道关闭近乎同时 ready，外层 select 随机命中两臂——多轮迭代
	// 保证覆盖（sink 的 close() 生产上无人调用，通道关闭只随 ctx 取消发生）。
	for i := 0; i < 20; i++ {
		g2 := newServer(t, 0)
		ctx2, cancel2 := context.WithCancel(context.Background())
		ch2, _, err := g2.CoreStartTurnStreamWithRequestRPC(ctx2, coreapi.StartTurnRequest{
			SessionID: "s-1", TurnID: "t-c", Input: "hi",
		})
		if err != nil {
			cancel2()
			t.Fatalf("post-cancel iter %d start: %v", i, err)
		}
		cancel2()
		if !waitClosed(ch2) {
			t.Fatalf("post-cancel iter %d 通道未收口", i)
		}
	}
}

// TestStdioRpcPureHelpers 附件转换 / 终态判定纯函数表测。
func TestStdioRpcPureHelpers(t *testing.T) {
	items := []Attachment{
		{Name: " a ", Path: " /a.png ", MIME: " image/png ", Kind: " image "},
		{Name: "empty", Path: "   "},
	}
	out := CoreAPIAttachments(items)
	if len(out) != 1 || out[0].Name != "a" || out[0].Path != "/a.png" || out[0].MIME != "image/png" || out[0].Kind != "image" {
		t.Fatalf("CoreAPIAttachments = %+v", out)
	}
	if got := CoreAPIAttachments(nil); len(got) != 0 {
		t.Fatalf("空附件应返回空: %+v", got)
	}
	if !stdioIsTerminalCoreTurnEvent(Event{EventType: "request.completed"}) ||
		!stdioIsTerminalCoreTurnEvent(Event{EventType: "request.failed"}) {
		t.Fatal("completed/failed 应判终态")
	}
	if stdioIsTerminalCoreTurnEvent(Event{EventType: "turn.delta"}) {
		t.Fatal("delta 非终态")
	}
}

// TestStdioRpcTurnStartTerminalFallback turn/start 直接回终态 error：
// 流侧应合成 request.failed 回落事件。
func TestStdioRpcTurnStartTerminalFallback(t *testing.T) {
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodEventSubscribe:   map[string]any{"subscription_id": "sub-1"},
		protocoljsonrpc.MethodEventUnsubscribe: map[string]any{},
		protocoljsonrpc.MethodTurnStart:        map[string]any{"id": "t-1", "session_id": "s-1", "status": "error"},
	})
	ch, _, err := g.CoreStartTurnStreamWithRequestRPC(context.Background(), coreapi.StartTurnRequest{
		SessionID: "s-1", Input: "hi",
	})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	select {
	case ev := <-ch:
		if ev.EventType != string(protocol.EventTypeRequestFailed) {
			t.Fatalf("终态 error 应回落 request.failed 事件: %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("failed 回落事件未在时限内到达")
	}
}

// TestStdioRpcBareGatewayGuards 未启动网关（client/events 为 nil）的
// 守卫臂 + 已启动网关的 nil-ctx 臂 + turn/start 错误回复的 result.err
// 合成路径 + set_meta 错误臂（非空 sessionID 才能越过参数校验）。
func TestStdioRpcBareGatewayGuardsAndErrorPaths(t *testing.T) {
	bare := &StdioGateway{}
	ctx := context.Background()
	if _, _, err := bare.CoreSubscribeEventsRPC(ctx, "s", "t", "a", 8); err == nil {
		t.Fatal("未启动网关订阅应报错")
	}
	if _, _, err := bare.CoreStartTurnStreamWithRequestRPC(ctx, coreapi.StartTurnRequest{}); err == nil {
		t.Fatal("未启动网关 start turn 应报错")
	}
	if _, _, err := bare.CoreResumeTurnStreamRPC(ctx, "s", "t"); err == nil {
		t.Fatal("未启动网关 resume 应报错")
	}

	// 已启动网关：set_meta 错误臂（错误网关 + 非空 sessionID）。
	eg := newErrorReplayGateway(t)
	if err := eg.CoreArchiveSessionRPC(ctx, "s-1", false); err == nil {
		t.Fatal("归档开关错误臂应报错")
	}
	if err := eg.CoreSetSessionSandboxModeRPC(ctx, "s-1", "full_access"); err == nil {
		t.Fatal("会话沙箱模式错误臂应报错")
	}

	// turn/start 回 JSON-RPC 错误：流侧走 result.err → request.failed。
	// newReplayGateway 对未登记 method 回空对象——这里需要真错误响应，
	// 用混合服务器：subscribe 成功、turn/start 报错。
	g2 := newMixedReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodEventSubscribe: map[string]any{"subscription_id": "sub-1"},
	}, map[string]bool{
		protocoljsonrpc.MethodTurnStart: true,
	})
	ch, _, err := g2.CoreStartTurnStreamWithRequestRPC(nil, coreapi.StartTurnRequest{ // nil ctx 走 rpcCtx
		SessionID: "s-1", Input: "hi",
	})
	if err != nil {
		t.Fatalf("start turn (nil ctx): %v", err)
	}
	select {
	case ev := <-ch:
		if ev.EventType != string(protocol.EventTypeRequestFailed) {
			t.Fatalf("start 错误应合成 request.failed: %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request.failed 合成事件未到达")
	}
}
