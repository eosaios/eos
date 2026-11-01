package testsupport_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/testsupport"
)

// TestFoundationRealCoreTurnEndToEnd 是测试底座自身的可信度自检：
// 真实内核 + 脚本模型服务必须能跑通「建会话 → 声明模型 → 起 turn → 收到终态」。
//
// 这个用例存在的意义：后续所有 P0 集成/E2E 都建立在底座之上。
// 如果底座本身不可信，上层“通过”没有意义。
// 它同时证明脚本模型服务确实被真实内核调用（断言收到的真实请求）。
func TestFoundationRealCoreTurnEndToEnd(t *testing.T) {
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	t.Logf("实际内核: %s", core.String())

	model := testsupport.NewModelServer(t,
		testsupport.NewTextScript("main", nil, "你好，", "世界🌍"),
		testsupport.NewBackgroundScript(),
	)

	conn, err := testsupport.StartCore(t, core, env.CoreEnv(nil))
	if err != nil {
		t.Fatalf("start core: %v", err)
	}

	// model/upsert：真相源是 Core 的 model_upsert 实现 —— 新条目自动激活。
	if _, err := conn.Call("model/upsert", map[string]any{
		"name":     "eos-test-local",
		"api_base": model.BaseURL,
		"api_key":  model.Token,
		"model":    "eos-test-model",
	}); err != nil {
		t.Fatalf("model/upsert: %v", err)
	}

	sessionID := createSession(t, conn, env.WorkspaceRoot)
	if err := conn.SubscribeTurn(sessionID, ""); err != nil {
		t.Fatalf("event/subscribe: %v", err)
	}

	turnID := startTurn(t, conn, sessionID, "打个招呼")
	t.Logf("turn_id=%s", turnID)

	terminal := conn.AssertTerminal(t, turnID, 60*time.Second)
	if terminal.Type != "turn.completed" {
		t.Fatalf("终态类型 = %q，want turn.completed (payload=%v)", terminal.Type, terminal.Payload)
	}

	// 流式语义：增量必须先于终态到达，且拼接后等于最终 item 文本。
	deltas := conn.JoinedText(turnID)
	if deltas != "你好，世界🌍" {
		t.Fatalf("增量拼接 = %q，want %q", deltas, "你好，世界🌍")
	}
	completedItems, _ := terminal.Payload["items"].([]any)
	if len(completedItems) != 1 {
		t.Fatalf("turn.completed items = %d，want 1 (payload=%v)", len(completedItems), terminal.Payload)
	}
	item, _ := completedItems[0].(map[string]any)
	if got := stringAt(item, "text"); got != "你好，世界🌍" {
		t.Fatalf("终态 item 文本 = %q，want %q", got, "你好，世界🌍")
	}
	// 流式语义：首条增量必须早于终态到达，证明不是一次性拼出来的。
	if len(conn.EventsOfType("turn.item_delta")) == 0 {
		t.Errorf("没有收到任何 turn.item_delta，无法证明流式增量先于终态到达")
	}
	if first := conn.EventsOfType("turn.item_delta")[0]; !first.At.Before(terminal.At) {
		t.Errorf("首条增量时间 %s 不早于终态时间 %s", first.At, terminal.At)
	}

	// 底座可信度：必须证明脚本模型服务真的收到了真实内核的请求。
	reqs := model.WaitForRequests(t, 1, 30*time.Second)
	req := reqs[0]
	if req.Model != "eos-test-model" {
		t.Errorf("内核发给模型的 model = %q，want eos-test-model", req.Model)
	}
	if req.AuthHeader == "" {
		t.Errorf("内核未发送 Authorization 头；api_key 配置可能未生效")
	}
	t.Logf("模型服务收到路径=%s auth 前缀=%s", req.Path, maskAuth(req.AuthHeader))
	t.Logf("事件类型统计:\n%s", conn.TypeHistogram())
	t.Logf("脚本命中统计:\n%s", model.Histogram())
}

func startTurn(t *testing.T, conn *testsupport.CoreConn, sessionID, input string) string {
	t.Helper()
	raw, err := conn.Call("turn/start", map[string]any{
		"session_id": sessionID,
		"input":      input,
	})
	if err != nil {
		t.Fatalf("turn/start: %v", err)
	}
	var started struct {
		TurnID  string `json:"turn_id"`
		ID      string `json:"id"`
		Status  string `json:"status"`
		Session string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &started); err != nil {
		t.Fatalf("decode turn/start: %v (raw=%s)", err, string(raw))
	}
	turnID := started.TurnID
	if turnID == "" {
		turnID = started.ID
	}
	if turnID == "" {
		t.Fatalf("turn/start 未返回 turn 标识 (raw=%s)", string(raw))
	}
	if started.Session != sessionID {
		t.Fatalf("turn/start session_id = %q，want %q", started.Session, sessionID)
	}
	if started.Status != "running" {
		t.Fatalf("turn/start status = %q，want running", started.Status)
	}
	return turnID
}

func createSession(t *testing.T, conn *testsupport.CoreConn, workspaceRoot string) string {
	t.Helper()
	raw, err := conn.Call("session/create", map[string]any{
		"workspace_root": workspaceRoot,
		"title":          "testsupport smoke",
	})
	if err != nil {
		t.Fatalf("session/create: %v\ntranscript:\n%s", err, conn.TranscriptString())
	}
	var sess struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &sess); err != nil {
		t.Fatalf("decode session/create: %v (raw=%s)", err, string(raw))
	}
	if sess.ID == "" {
		t.Fatalf("session/create 未返回 id (raw=%s)", string(raw))
	}
	return sess.ID
}

func stringAt(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func maskAuth(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12] + "***"
}
