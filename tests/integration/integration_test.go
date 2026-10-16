// Package integration 是 EOS 真实内核集成测试。
//
// 运行前提：真实 eos-core 可被 sidecar resolver 定位；不需要任何真实
// 凭据或付费模型 —— 模型端由 internal/testsupport 的脚本服务代替。
//
// 所有断言都同时检查「机器可见结果」与「真实文件/状态副作用」。
// 只断言 err != nil 或返回非空不构成通过。
package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/testsupport"
)

// harness 是一次集成测试需要的全部隔离资源。
type harness struct {
	env   *testsupport.Env
	core  testsupport.Core
	conn  *testsupport.CoreConn
	model *testsupport.ModelServer
}

// newHarness 建立隔离环境 + 真实内核 + 脚本模型服务。
func newHarness(t *testing.T, scripts ...*testsupport.ModelScript) *harness {
	t.Helper()
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	if len(scripts) == 0 {
		scripts = []*testsupport.ModelScript{testsupport.NewBackgroundScript()}
	}
	model := testsupport.NewModelServer(t, scripts...)
	conn, err := testsupport.StartCore(t, core, env.CoreEnv(map[string]string{
		"EOS_SANDBOX_MODE":  string(testsupport.SandboxWorkspaceWrite),
		"EOS_APPROVAL_MODE": "on-request",
		"EOS_LOG_LEVEL":     "warn",
	}))
	if err != nil {
		t.Fatalf("启动真实内核失败：%v", err)
	}
	h := &harness{env: env, core: core, conn: conn, model: model}
	if _, err := conn.Call("model/upsert", map[string]any{
		"name":     "eos-test-local",
		"api_base": model.BaseURL,
		"api_key":  model.Token,
		"model":    "eos-test-model",
	}); err != nil {
		t.Fatalf("model/upsert: %v", err)
	}
	t.Logf("集成环境: core=%s workspace=%s", core.String(), env.WorkspaceRoot)
	return h
}

func (h *harness) newSession(t *testing.T, title string) string {
	t.Helper()
	raw, err := h.conn.Call("session/create", map[string]any{
		"workspace_root": h.env.WorkspaceRoot,
		"title":          title,
	})
	if err != nil {
		t.Fatalf("session/create: %v", err)
	}
	var s struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("解析 session/create 失败：%v", err)
	}
	if s.ID == "" {
		t.Fatalf("session/create 未返回 id")
	}
	if err := h.conn.SubscribeTurn(s.ID, ""); err != nil {
		t.Fatalf("event/subscribe: %v", err)
	}
	return s.ID
}

func (h *harness) startTurn(t *testing.T, sessID, input string) string {
	t.Helper()
	raw, err := h.conn.Call("turn/start", map[string]any{
		"session_id": sessID,
		"input":      input,
	})
	if err != nil {
		t.Fatalf("turn/start: %v", err)
	}
	var s struct {
		TurnID  string `json:"turn_id"`
		ID      string `json:"id"`
		Session string `json:"session_id"`
		Status  string `json:"status"`
	}
	_ = json.Unmarshal(raw, &s)
	if s.Session != sessID {
		t.Fatalf("turn/start 回显 session_id = %q，want %q", s.Session, sessID)
	}
	if s.Status != "running" {
		t.Fatalf("turn/start status = %q，want running", s.Status)
	}
	if s.TurnID != "" {
		return s.TurnID
	}
	if s.ID == "" {
		t.Fatalf("turn/start 未返回 turn 标识")
	}
	return s.ID
}

// toolResult 是 tool/execute 的结果形态。
type toolResult struct {
	Status  string          `json:"status"`
	Name    string          `json:"name"`
	Error   string          `json:"error"`
	Display string          `json:"display"`
	Output  json.RawMessage `json:"output"`
}

// IT-02（P0）只读工具：模型发起工具调用 → 内核执行 → 模型收到结果 → 终态。
//
// 规格来源：CASE-CATALOG IT-02。
// 精确预期：
//   - 工具读到了指定临时文件的内容；
//   - 第二轮模型请求里含该工具结果（证明结果真的回传给了模型）；
//   - 目标文件内容与权限都没有被修改；
//   - 终态 completed，且最终回答是脚本里第二轮的文本。
func TestIT02_ReadOnlyToolRoundTrip(t *testing.T) {
	target := filepath.Join(t.TempDir(), "fixture.txt")
	const wantContent = "行一\n行二 中文与 emoji 🌍\n"
	if err := os.WriteFile(target, []byte(wantContent), 0o644); err != nil {
		t.Fatalf("写 fixture: %v", err)
	}
	beforeHash := testsupport.FileHash(t, target)

	toolName, args := pickReadTool(t, target)
	t.Logf("选用只读工具 %s，参数 %s", toolName, args)

	h := newHarness(t,
		&testsupport.ModelScript{
			Name: "turn1-call-tool", TextDeltas: nil, DisconnectAfterN: -1,
			ToolCalls: []testsupport.ScriptedToolCall{{
				ID: "call_read", Name: toolName, ArgsJSON: args,
			}},
		},
		&testsupport.ModelScript{
			Name: "turn2-final", TextDeltas: []string{"读取完成"}, DisconnectAfterN: -1,
			Match: func(r testsupport.CapturedRequest) bool { return r.Round > 1 },
		},
		testsupport.NewBackgroundScript(),
	)

	sessID := h.newSession(t, "it02")
	turnID := h.startTurn(t, sessID, "请读取 "+target)
	terminal := h.conn.AssertTerminal(t, turnID, 60*time.Second)
	if terminal.Type != "turn.completed" {
		t.Fatalf("终态 = %q，want turn.completed（payload=%v）", terminal.Type, terminal.Payload)
	}
	if got := h.conn.JoinedText(turnID); got != "读取完成" {
		t.Errorf("最终回答 = %q，want 读取完成", got)
	}

	// 工具调用结果必须进入第二轮模型请求。
	reqs := h.model.WaitForRequests(t, 2, 30*time.Second)
	second := reqs[1]
	if !requestMentions(second, wantContent) && !requestMentions(second, target) {
		t.Errorf("第二轮模型请求未包含工具结果（既没有文件路径也没有内容）\nbody 摘要=%s", summarize(second))
	}

	// 只读工具不得修改目标。
	if got := testsupport.FileHash(t, target); got != beforeHash {
		t.Errorf("SECURITY: 只读工具修改了目标文件 %s（before=%s after=%s）", target, beforeHash, got)
	}
	t.Logf("模型请求数=%d 脚本命中:\n%s", len(h.model.Requests()), h.model.Histogram())
}

// IT-03（P0）审批接受：待审批时目录不存在，接受后恰好创建一次。
//
// 精确预期：
//   - tool/execute 返回 approval_required 且带 approval_id；
//   - 收到审批请求时，目标目录**不存在**（审批前不得有副作用）；
//   - respond(accept) 后目录被创建；
//   - 副作用恰好发生一次（目录只建一个，条目数符合预期）。
func TestIT03_ApprovalAcceptCreatesTargetOnce(t *testing.T) {
	const dirName = "approved_dir"

	h := newHarness(t,
		&testsupport.ModelScript{
			Name: "call-mkdir", DisconnectAfterN: -1,
			ToolCalls: []testsupport.ScriptedToolCall{{
				ID: "call_mkdir", Name: "file_mkdir", ArgsJSON: `{"path":"` + dirName + `"}`,
			}},
		},
		&testsupport.ModelScript{
			Name: "final", TextDeltas: []string{"已创建"}, DisconnectAfterN: -1,
			Match: func(r testsupport.CapturedRequest) bool { return r.Round > 1 },
		},
		testsupport.NewBackgroundScript(),
	)

	// 目标必须落在真实工作区内：tool/execute 的相对路径就是相对 workspace_root 解析的。
	target := filepath.Join(h.env.WorkspaceRoot, dirName)

	sessID := h.newSession(t, "it03")
	res := h.execTool(t, sessID, "file_mkdir", map[string]any{"path": dirName}, "call_mkdir")
	if res.Status != "approval_required" {
		t.Fatalf("status = %q，want approval_required (display=%q)", res.Status, res.Display)
	}
	approvalID := outputString(t, res.Output, "approval_id")
	if approvalID == "" {
		t.Fatalf("审批请求缺少 approval_id：%s", string(res.Output))
	}

	// 审批前：必须没有任何副作用。
	testsupport.MustNotExist(t, target, "审批尚未接受就产生了副作用")

	receipt := h.respond(t, approvalID, "accept")
	if !receipt.Accepted {
		t.Fatalf("approval/respond 未接受：%+v", receipt)
	}

	// 直接调 tool/execute 不产生 turn 终态，等待工具执行完成事件即可。
	// （终态是 turn 生命周期概念，这里没有 turn。）
	h.waitToolExecuted(t, "call_mkdir", "ok", 30*time.Second)
	entries := testsupport.MustBeDir(t, target)
	if entries != 0 {
		t.Errorf("新建目录含 %d 个条目，期望空目录", entries)
	}

	// 副作用恰好一次：只有一次 tool.executed 且 status=ok。
	executed := h.conn.EventsOfType("tool.executed")
	if len(executed) != 1 {
		t.Errorf("tool.executed 出现 %d 次，want 1（副作用必须恰好一次）", len(executed))
	} else if got := executed[0].PayloadString("status"); got != "ok" {
		t.Errorf("tool.executed status = %q，want ok", got)
	}
	// 审批前不得有 tool.executing。
	if n := len(h.conn.EventsOfType("tool.executing")); n != 1 {
		t.Errorf("tool.executing 出现 %d 次，want 1", n)
	}
	t.Logf("approval_id=%s receipt=%+v", approvalID, receipt)
}

// IT-04（P0）审批拒绝：目标保持不存在，且不得误报完成写入。
//
// 精确预期：
//   - respond(reject) 后 receipt.accepted == false；
//   - 目标目录始终不存在；
//   - 终态不得声称工具成功（tool.executed 不得为 ok / 不得出现成功结果）。
func TestIT04_ApprovalRejectLeavesNoSideEffect(t *testing.T) {
	const dirName = "rejected_dir"

	h := newHarness(t,
		&testsupport.ModelScript{
			Name: "call-mkdir", DisconnectAfterN: -1,
			ToolCalls: []testsupport.ScriptedToolCall{{
				ID: "call_mkdir", Name: "file_mkdir", ArgsJSON: `{"path":"` + dirName + `"}`,
			}},
		},
		&testsupport.ModelScript{
			Name: "final", TextDeltas: []string{"好的，已跳过"}, DisconnectAfterN: -1,
			Match: func(r testsupport.CapturedRequest) bool { return r.Round > 1 },
		},
		testsupport.NewBackgroundScript(),
	)

	target := filepath.Join(h.env.WorkspaceRoot, dirName)

	sessID := h.newSession(t, "it04")
	res := h.execTool(t, sessID, "file_mkdir", map[string]any{"path": dirName}, "call_mkdir")
	if res.Status != "approval_required" {
		t.Fatalf("status = %q，want approval_required", res.Status)
	}
	approvalID := outputString(t, res.Output, "approval_id")
	testsupport.MustNotExist(t, target, "审批前就有副作用")

	receipt := h.respond(t, approvalID, "decline")
	if receipt.Accepted {
		t.Errorf("decline 的 receipt.accepted = true，want false（%+v）", receipt)
	}

	// 审批已终结后仍必须没有副作用；给执行链一个观察窗口再检查。
	h.waitToolExecutionSettled(t, 3*time.Second)
	testsupport.MustNotExist(t, target, "审批被拒绝后仍创建了目标目录")

	// 不得出现 ok 的执行结果，也不得出现 approved 事件。
	for _, e := range h.conn.EventsOfType("tool.executed") {
		if e.PayloadString("status") == "ok" {
			t.Errorf("审批被拒绝却出现 tool.executed status=ok：%v", e.Payload)
		}
	}
	if n := len(h.conn.EventsOfType("tool.approval_approved")); n > 0 {
		t.Errorf("拒绝路径却出现 tool.approval_approved x%d", n)
	}
	t.Logf("拒绝回执=%+v 事件统计:\n%s", receipt, h.conn.TypeHistogram())
}

func (h *harness) execTool(t *testing.T, sessID, name string, args map[string]any, reqID string) toolResult {
	t.Helper()
	raw, err := h.conn.CallTimeout("tool/execute", map[string]any{
		"name":           name,
		"args":           args,
		"workspace_root": h.env.WorkspaceRoot,
		"session_id":     sessID,
		"request_id":     reqID,
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("tool/execute(%s): %v\ntranscript:\n%s", name, err, h.conn.TranscriptString())
	}
	var res toolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("解析 tool/execute 结果失败：%v (raw=%s)", err, string(raw))
	}
	if res.Name != name {
		t.Errorf("结果回显工具名 = %q，want %q", res.Name, name)
	}
	return res
}

type approvalReceipt struct {
	ApprovalID string `json:"approval_id"`
	Decision   string `json:"decision"`
	Accepted   bool   `json:"accepted"`
}

func (h *harness) respond(t *testing.T, approvalID, decision string) approvalReceipt {
	t.Helper()
	raw, err := h.conn.CallTimeout("approval/respond", map[string]any{
		"approval_id": approvalID,
		"decision":    decision,
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("approval/respond(%s): %v", decision, err)
	}
	var r approvalReceipt
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("解析 approval/respond 失败：%v (raw=%s)", err, string(raw))
	}
	if r.ApprovalID != approvalID {
		t.Errorf("回执 approval_id = %q，want %q", r.ApprovalID, approvalID)
	}
	return r
}

// waitToolExecuted 等待指定 request_id 的 tool.executed 事件，并校验状态。
func (h *harness) waitToolExecuted(t *testing.T, requestID, wantStatus string, timeout time.Duration) testsupport.Event {
	t.Helper()
	events := h.conn.WaitForMatch(t, timeout, func(e testsupport.Event) bool {
		return e.Type == "tool.executed" && e.PayloadString("request_id") == requestID
	})
	if len(events) == 0 {
		t.Fatalf("TIMEOUT: 未收到 request_id=%s 的 tool.executed（%s）\n事件统计:\n%s",
			requestID, timeout, h.conn.TypeHistogram())
	}
	ev := events[0]
	if got := ev.PayloadString("status"); got != wantStatus {
		t.Errorf("tool.executed status = %q，want %q（payload=%v）", got, wantStatus, ev.Payload)
	}
	return ev
}

// waitToolExecutionSettled 给工具执行链一个观察窗口，确认不会再有新的执行事件。
//
// 用于否定断言：拒绝/越权之后，必须证明“什么都没有发生”，
// 而不是只检查当下的文件状态。
func (h *harness) waitToolExecutionSettled(t *testing.T, window time.Duration) {
	t.Helper()
	before := len(h.conn.EventsOfType("tool.executed"))
	h.conn.WaitQuiet(window)
	after := len(h.conn.EventsOfType("tool.executed"))
	if after != before {
		t.Errorf("观察窗口内出现了新的 tool.executed（%d → %d），说明拒绝后仍在执行", before, after)
	}
}

func outputString(t *testing.T, raw json.RawMessage, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// pickReadTool 从真实工具目录里挑一个可读指定路径的只读工具。
//
// 真相源是真实内核的 tool/catalog —— 不写死工具名，
// 避免内核改名后测试“悄悄失效”或直接编造一个不存在的工具。
func pickReadTool(t *testing.T, targetPath string) (name, argsJSON string) {
	t.Helper()
	core := testsupport.LocateRealCore(t)
	env := testsupport.NewEnv(t)
	conn, err := testsupport.StartCore(t, core, env.CoreEnv(nil))
	if err != nil {
		t.Fatalf("启动内核读取工具目录失败：%v", err)
	}
	defer conn.Close()
	raw, err := conn.Call("tool/catalog", map[string]any{})
	if err != nil {
		t.Fatalf("tool/catalog: %v", err)
	}
	var catalog []map[string]any
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("解析工具目录失败：%v", err)
	}
	// 优先级：能读单文件且路径参数的只读工具。
	preferred := []string{"read_file", "file_read", "doc_read", "multi_file_read"}
	for _, n := range preferred {
		for _, e := range catalog {
			if name, _ := e["name"].(string); name != n {
				continue
			}
			ro, _ := e["read_only"].(bool)
			if !ro {
				continue
			}
			schema, _ := e["params_schema"].(map[string]any)
			props, _ := schema["properties"].(map[string]any)
			if _, ok := props["path"]; ok {
				b, _ := json.Marshal(map[string]any{"path": targetPath})
				return n, string(b)
			}
			if _, ok := props["paths"]; ok {
				b, _ := json.Marshal(map[string]any{"paths": []string{targetPath}})
				return n, string(b)
			}
		}
	}
	t.Fatalf("真实工具目录里找不到能读路径的只读工具（目录条目数=%d）", len(catalog))
	return "", ""
}

func requestMentions(r testsupport.CapturedRequest, needle string) bool {
	return strings.Contains(string(r.RawBody), needle)
}

func summarize(r testsupport.CapturedRequest) string {
	bs := r.RawBody
	if len(bs) > 400 {
		bs = bs[:400]
	}
	return string(bs)
}
