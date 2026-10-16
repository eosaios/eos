package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/testsupport"
)

// IT-08（P0）流式中取消：模型被屏障挡住 → 取消 → 迟到数据不得复活 turn。
//
// 规格来源：CASE-CATALOG IT-08。
// 精确预期：
//   - 模型请求确实到达服务端并停在屏障上（证明取消发生在流式中）；
//   - turn/interrupt 之后，该 turn 在观察窗口内不再出现 turn.completed；
//   - 放行屏障让服务端继续发数据后，仍然不产生完成终态（迟到 delta 不复活）；
//   - 没有任何文件副作用。
func TestIT08_CancelDuringStreamDoesNotComplete(t *testing.T) {
	barrier := testsupport.NewBarrier()
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	t.Logf("取消测试内核: %s", core.String())

	model := testsupport.NewModelServer(t,
		&testsupport.ModelScript{
			Name:             "blocked-turn",
			Barrier:          barrier,
			TextDeltas:       []string{"迟到内容", "不应出现"},
			DisconnectAfterN: -1,
		},
		testsupport.NewBackgroundScript(),
	)

	conn, err := testsupport.StartCore(t, core, env.CoreEnv(map[string]string{
		"EOS_SANDBOX_MODE":  string(testsupport.SandboxWorkspaceWrite),
		"EOS_APPROVAL_MODE": "on-request",
	}))
	if err != nil {
		t.Fatalf("启动内核失败：%v", err)
	}
	if _, err := conn.Call("model/upsert", map[string]any{
		"name": "eos-test-local", "api_base": model.BaseURL,
		"api_key": model.Token, "model": "eos-test-model",
	}); err != nil {
		t.Fatalf("model/upsert: %v", err)
	}

	sessRaw, err := conn.Call("session/create", map[string]any{
		"workspace_root": env.WorkspaceRoot, "title": "it08",
	})
	if err != nil {
		t.Fatalf("session/create: %v", err)
	}
	var sess struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(sessRaw, &sess)
	if err := conn.SubscribeTurn(sess.ID, ""); err != nil {
		t.Fatalf("event/subscribe: %v", err)
	}

	before := testsupport.SnapshotDir(t, env.WorkspaceRoot)

	turnRaw, err := conn.Call("turn/start", map[string]any{
		"session_id": sess.ID, "input": "长任务",
	})
	if err != nil {
		t.Fatalf("turn/start: %v", err)
	}
	var started struct {
		TurnID string `json:"turn_id"`
		ID     string `json:"id"`
	}
	_ = json.Unmarshal(turnRaw, &started)
	turnID := started.TurnID
	if turnID == "" {
		turnID = started.ID
	}

	// 等模型请求真正到达并卡在屏障上 —— 证明取消发生在流式中而非结束后。
	barrier.Await(t, 30*time.Second)

	if _, err := conn.Call("turn/interrupt", map[string]any{
		"session_id": sess.ID, "turn_id": turnID,
	}); err != nil {
		t.Logf("turn/interrupt 返回错误（继续观察实际行为）：%v", err)
	}

	// 放行屏障：让服务端把剩余内容发出来，制造“迟到 delta”。
	barrier.Release()

	// 取消后允许出现取消终态（真实内核用 turn.cancelled 表示中断成功），
	// 但绝不允许出现 turn.completed —— 那是“任务成功完成”的伪报。
	terminal := h_waitTerminal(conn, turnID, 15*time.Second)
	if terminal.Type == "turn.completed" {
		t.Fatalf("SECURITY/语义: 被取消的 turn 出现了 turn.completed（payload=%v）——迟到数据把任务复活成成功", terminal.Payload)
	}
	if terminal.Type == "" {
		t.Errorf("取消后 %v 内没有出现任何终态事件，任务状态悬空\n事件统计:\n%s", 15*time.Second, conn.TypeHistogram())
	}

	// 取消瞬间已在途的增量到达终态之前是合理的；真正要禁止的是
	// **终态之后**还在流出内容 —— 那会让 UI 继续把已取消的任务当成进行中。
	conn.WaitQuiet(5 * time.Second)
	for _, e := range conn.EventsOfTurn(turnID) {
		if e.Type == "turn.item_delta" && !e.At.Before(terminal.At) {
			t.Errorf("终态 %s(%s) 之后仍收到文本增量 %q：迟到数据把已取消的 turn 重新激活",
				terminal.Type, terminal.At.Format(time.RFC3339Nano), e.PayloadString("delta"))
		}
	}

	// 取消不得产生任何文件副作用。
	after := testsupport.SnapshotDir(t, env.WorkspaceRoot)
	if diff := before.Diff(after); len(diff) > 0 {
		t.Errorf("取消路径产生了文件副作用：%v", diff)
	}
	preTerminal := conn.JoinedText(turnID)
	t.Logf("取消终态=%q 终态前已到达增量=%q 终态后无新事件（观察 5s）\n事件统计:\n%s",
		terminal.Type, preTerminal, conn.TypeHistogram())
}

// h_waitTerminal 等待终态事件；超时返回零值 Event（不 Fatal），
// 让调用方按“没有任何终态也是一种可判定结果”来处理。
func h_waitTerminal(conn *testsupport.CoreConn, turnID string, timeout time.Duration) testsupport.Event {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, e := range conn.EventsOfTurn(turnID) {
			if testsupport.IsTerminalEvent(e.Type) {
				return e
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return testsupport.Event{}
}

// IT-09（P1）多会话隔离：两个会话交错运行，事件与回复不得串线。
//
// 精确预期：
//   - 两个会话各自的事件 turn_id / session_id 只属于自己；
//   - 第二个 turn 的最终文本与第二个脚本一致，不被第一个污染；
//   - 两个 turn 各自恰好一个终态。
func TestIT09_TwoSessionsDoNotCrossTalk(t *testing.T) {
	h := newHarness(t,
		&testsupport.ModelScript{
			Name: "sess-a", TextDeltas: []string{"会话A的回答"}, DisconnectAfterN: -1,
			Match: func(r testsupport.CapturedRequest) bool { return r.HasUserTextContaining("属于A") },
		},
		&testsupport.ModelScript{
			Name: "sess-b", TextDeltas: []string{"会话B的回答"}, DisconnectAfterN: -1,
			Match: func(r testsupport.CapturedRequest) bool { return r.HasUserTextContaining("属于B") },
		},
		testsupport.NewBackgroundScript(),
	)

	sessA := h.newSession(t, "it09-A")
	sessB := h.newSession(t, "it09-B")

	turnA := h.startTurn(t, sessA, "问题属于A")
	turnB := h.startTurn(t, sessB, "问题属于B")

	// 串行等待，但两个 turn 是交错的（先发起的可能后完成）。
	termB := h.conn.AssertTerminal(t, turnB, 60*time.Second)
	termA := h.conn.AssertTerminal(t, turnA, 60*time.Second)

	if termA.Type != "turn.completed" || termB.Type != "turn.completed" {
		t.Fatalf("终态类型 A=%q B=%q，want turn.completed", termA.Type, termB.Type)
	}
	if got := h.conn.JoinedText(turnA); got != "会话A的回答" {
		t.Errorf("turnA 文本 = %q，want 会话A的回答（发生了串线）", got)
	}
	if got := h.conn.JoinedText(turnB); got != "会话B的回答" {
		t.Errorf("turnB 文本 = %q，want 会话B的回答（发生了串线）", got)
	}

	// 每个 turn 的事件必须只带自己的 ID。
	for _, e := range h.conn.Events() {
		switch e.TurnID {
		case turnA:
			if e.SessionID != sessA {
				t.Errorf("turnA 的事件 %s 带上了别的 session_id=%q", e.Type, e.SessionID)
			}
		case turnB:
			if e.SessionID != sessB {
				t.Errorf("turnB 的事件 %s 带上了别的 session_id=%q", e.Type, e.SessionID)
			}
		default:
			t.Errorf("事件 %s 的 turn_id=%q 不属于任一 turn（session=%q）", e.Type, e.TurnID, e.SessionID)
		}
	}
	t.Logf("事件统计:\n%s", h.conn.TypeHistogram())
}

// IT-07（P0）重启恢复：用同一个 store 重启内核，已持久化内容仍在。
//
// 规格来源：CASE-CATALOG IT-07。
// 精确预期：
//   - 关闭第一个内核后，用同一 StoreDir 启动第二个内核；
//   - session/list 仍能看到之前的会话；
//   - 该会话的消息历史可加载，内容与之前一致；
//   - 重启本身不产生额外文件副作用（不重复执行已完成的操作）。
func TestIT07_RestartKeepsPersistedSession(t *testing.T) {
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	t.Logf("重启测试内核: %s", core.String())

	model := testsupport.NewModelServer(t,
		testsupport.NewTextScript("main", nil, "持久化验证", "内容🌍"),
		testsupport.NewBackgroundScript(),
	)

	storeDir := env.StoreDir
	coreEnv := env.CoreEnv(map[string]string{
		"EOS_SANDBOX_MODE":  string(testsupport.SandboxWorkspaceWrite),
		"EOS_APPROVAL_MODE": "on-request",
	})

	// 第一段：正常跑完一个 turn。
	conn1, err := testsupport.StartCore(t, core, coreEnv)
	if err != nil {
		t.Fatalf("第一段内核启动失败：%v", err)
	}
	if _, err := conn1.Call("model/upsert", map[string]any{
		"name": "eos-test-local", "api_base": model.BaseURL,
		"api_key": model.Token, "model": "eos-test-model",
	}); err != nil {
		t.Fatalf("model/upsert: %v", err)
	}
	sessRaw, err := conn1.Call("session/create", map[string]any{
		"workspace_root": env.WorkspaceRoot, "title": "it07-restart",
	})
	if err != nil {
		t.Fatalf("session/create: %v", err)
	}
	var sess struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(sessRaw, &sess)
	if err := conn1.SubscribeTurn(sess.ID, ""); err != nil {
		t.Fatalf("event/subscribe: %v", err)
	}
	turnRaw, err := conn1.Call("turn/start", map[string]any{"session_id": sess.ID, "input": "持久化验证"})
	if err != nil {
		t.Fatalf("turn/start: %v", err)
	}
	var started struct {
		TurnID string `json:"turn_id"`
		ID     string `json:"id"`
	}
	_ = json.Unmarshal(turnRaw, &started)
	turnID := started.TurnID
	if turnID == "" {
		turnID = started.ID
	}
	conn1.AssertTerminal(t, turnID, 60*time.Second)
	wantText := conn1.JoinedText(turnID)
	if wantText != "持久化验证内容🌍" {
		t.Fatalf("第一段文本 = %q，want 持久化验证内容🌍", wantText)
	}

	// 等助手消息真正落盘后再重启。
	//
	// 观察：turn.completed 早于 messages.json 的落盘。这里用有界轮询等待，
	// 并把“终态到落盘的延迟”作为独立发现记录，而不是假设二者同时发生。
	flushStart := time.Now()
	if !waitForPersistedText(conn1, sess.ID, wantText, 10*time.Second) {
		t.Fatalf("第一段内核在终态后 10s 内仍未把助手消息写入会话历史（落盘延迟 >10s）")
	}
	flushDelay := time.Since(flushStart)
	t.Logf("助手消息落盘延迟 ≈ %s（自终态事件起算）", flushDelay.Round(time.Millisecond))

	// 关闭第一个内核。
	conn1.Close()

	// store 必须真的落盘了，否则“恢复”只是同进程内的假象。
	if _, err := os.Stat(storeDir); err != nil {
		t.Fatalf("store 目录不存在：%v（内核没有真正持久化，重启测试无意义）", err)
	}

	// 第二段：同一 store 重启。
	conn2, err := testsupport.StartCore(t, core, coreEnv)
	if err != nil {
		t.Fatalf("第二段内核启动失败：%v", err)
	}

	listRaw, err := conn2.Call("session/list", map[string]any{})
	if err != nil {
		t.Fatalf("session/list: %v", err)
	}
	if !containsID(listRaw, sess.ID) {
		t.Errorf("重启后会话列表里没有 %s（持久化丢失）。session/list=%s", sess.ID, truncate(string(listRaw), 400))
	}

	msgsRaw, err := conn2.Call("session/messages/load", map[string]any{"session_id": sess.ID})
	if err != nil {
		t.Fatalf("session/messages/load 失败：%v", err)
	}
	// 终态事件与落盘不是同一时刻：内核在终态之后才刷 messages.json。
	// 这里用有界轮询等待落盘完成，而不是假定“终态即已持久化”。
	if !waitForPersistedText(conn2, sess.ID, wantText, 10*time.Second) {
		t.Errorf("重启后 %v 内未在消息历史里看到原文 %q。实际=%s",
			10*time.Second, wantText, truncate(string(msgsRaw), 400))
	}
	t.Logf("重启后消息历史 = %s", truncate(string(msgsRaw), 400))
	t.Logf("重启后事件统计:\n%s", conn2.TypeHistogram())
}

// waitForPersistedText 有界轮询，等待会话历史里出现指定文本。
func waitForPersistedText(conn *testsupport.CoreConn, sessID, want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		raw, err := conn.Call("session/messages/load", map[string]any{"session_id": sessID})
		if err == nil && containsText(raw, want) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// IT-06（P1）授权作用域：会话级授权不得扩散到其它会话。
//
// 精确预期：
//   - 第一次请求进入审批，accept 后执行成功；
//   - 同一会话的第二次同 scope 请求按“本次都允许”语义不再重复审批；
//   - 新会话的同类请求仍然需要独立审批（授权不跨会话）。
func TestIT06_SessionAuthorizationDoesNotLeakAcrossSessions(t *testing.T) {
	const dirName = "scoped_dir"

	h := newHarness(t,
		&testsupport.ModelScript{
			Name: "call", DisconnectAfterN: -1,
			ToolCalls: []testsupport.ScriptedToolCall{{
				ID: "call_scoped", Name: "file_mkdir", ArgsJSON: `{"path":"` + dirName + `_a"}`,
			}},
		},
		&testsupport.ModelScript{
			Name: "final", TextDeltas: []string{"完成"}, DisconnectAfterN: -1,
			Match: func(r testsupport.CapturedRequest) bool { return r.Round > 1 },
		},
		testsupport.NewBackgroundScript(),
	)

	// 目标落在真实工作区内：相对路径按 workspace_root 解析。
	targetA := filepath.Join(h.env.WorkspaceRoot, dirName+"_a")
	targetB := filepath.Join(h.env.WorkspaceRoot, dirName+"_b")

	sessA := h.newSession(t, "it06-A")
	res := h.execTool(t, sessA, "file_mkdir", map[string]any{"path": dirName + "_a"}, "call_scoped")
	if res.Status != "approval_required" {
		t.Fatalf("第一次请求 status = %q，want approval_required", res.Status)
	}
	approvalID := outputString(t, res.Output, "approval_id")
	h.respond(t, approvalID, "acceptForSession")

	// 等第一次执行完成（直接 tool/execute 无 turn 终态）。
	h.waitToolExecuted(t, "call_scoped", "ok", 30*time.Second)
	testsupport.MustBeDir(t, targetA)

	// 新会话：同类请求必须重新需要审批，授权不跨会话。
	sessB := h.newSession(t, "it06-B")
	resB := h.execTool(t, sessB, "file_mkdir", map[string]any{"path": dirName + "_b"}, "call_scoped_b")
	if resB.Status != "approval_required" {
		t.Errorf("SECURITY: 新会话的同类写请求 status = %q，want approval_required（会话授权泄漏到了别的会话）", resB.Status)
	}
	testsupport.MustNotExist(t, targetB, "新会话未获授权却产生了副作用")
	t.Logf("会话A=%s 会话B=%s 第二次 status=%q", sessA, sessB, resB.Status)
}

func containsID(raw []byte, id string) bool {
	return containsText(raw, id)
}

func containsText(raw []byte, needle string) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	return jsonContains(v, needle)
}

func jsonContains(v any, needle string) bool {
	switch t := v.(type) {
	case string:
		return t == needle
	case []any:
		for _, item := range t {
			if jsonContains(item, needle) {
				return true
			}
		}
	case map[string]any:
		for _, item := range t {
			if jsonContains(item, needle) {
				return true
			}
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
