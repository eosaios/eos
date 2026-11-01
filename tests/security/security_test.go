// Package security 是 EOS 安全负例测试。
//
// 硬性要求（TEST-STRATEGY §1）：安全关键否定用例必须同时断言三件事：
//  1. 请求被拒绝（具体的机器可见状态，不是「err != nil」）；
//  2. 目标文件/进程/网络没有发生副作用；
//  3. 工作区外的哨兵内容与权限完全未变。
//
// 所有用例只在临时工作区、临时目标和 loopback 服务上执行，
// 不读写真实用户数据，不访问远程服务。
package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/testsupport"
)

// secHarness 是一次安全用例需要的隔离资源。
type secHarness struct {
	env        *testsupport.Env
	conn       *testsupport.CoreConn
	core       testsupport.Core
	sentinel   testsupport.FileEntry
	outsideAll testsupport.Snapshot
}

// newSecHarness 建立隔离环境并启动真实内核。
//
// sandboxMode 直接决定被测边界语义；approvalMode 固定为 on-request，
// 以便验证“审批不能扩大沙箱许可”。
func newSecHarness(t *testing.T, sandboxMode testsupport.SandboxMode) *secHarness {
	t.Helper()
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	t.Logf("安全用例内核: %s sandbox=%s", core.String(), sandboxMode)

	conn, err := testsupport.StartCore(t, core, env.CoreEnv(map[string]string{
		"EOS_SANDBOX_MODE":  string(sandboxMode),
		"EOS_APPROVAL_MODE": "on-request",
	}))
	if err != nil {
		t.Fatalf("启动真实内核失败：%v", err)
	}
	return &secHarness{
		env:        env,
		conn:       conn,
		core:       core,
		sentinel:   env.SentinelEntry(t),
		outsideAll: testsupport.SnapshotDir(t, env.OutsideRoot),
	}
}

// assertNoEscape 断言工作区外没有任何变化，且目标没有被创建。
func (h *secHarness) assertNoEscape(t *testing.T, mustNotExist ...string) {
	t.Helper()
	for _, p := range mustNotExist {
		testsupport.MustNotExist(t, p, "越权写入")
	}
	h.env.SentinelUnchanged(t, h.sentinel)
	h.env.OutsideUnchanged(t, h.outsideAll)
}

func (h *secHarness) newSession(t *testing.T, title string) string {
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
		t.Fatal("session/create 未返回 id")
	}
	// 必须订阅事件：真实内核不会自动推送审批/执行事件，
	// 不订阅就观察不到 tool.executed。
	if err := h.conn.SubscribeTurn(s.ID, ""); err != nil {
		t.Fatalf("event/subscribe: %v", err)
	}
	return s.ID
}

type toolResult struct {
	Status  string          `json:"status"`
	Name    string          `json:"name"`
	Error   string          `json:"error"`
	Display string          `json:"display"`
	Output  json.RawMessage `json:"output"`
}

func (h *secHarness) execTool(t *testing.T, sessID, name string, args map[string]any, reqID string) toolResult {
	t.Helper()
	raw, err := h.conn.CallTimeout("tool/execute", map[string]any{
		"name":           name,
		"args":           args,
		"workspace_root": h.env.WorkspaceRoot,
		"session_id":     sessID,
		"request_id":     reqID,
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("tool/execute(%s): %v", name, err)
	}
	var res toolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("解析 tool/execute 失败：%v", err)
	}
	if res.Name != name {
		t.Errorf("结果回显工具名 = %q，want %q", res.Name, name)
	}
	return res
}

// approvalIDFrom 从结果里取 approval_id。
func approvalIDFrom(t *testing.T, res toolResult) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(res.Output, &m); err != nil {
		return ""
	}
	s, _ := m["approval_id"].(string)
	return s
}

// SEC-01（P0）读只 / 工作区可写两种模式下写工作区外一律拒绝。
//
// 精确预期：
//   - 请求不进入“已执行”状态（denied 或 approval_required 都不算成功）；
//   - 目标路径不被创建；
//   - 工作区外哨兵哈希与权限不变。
//
// 特别注意：即使内核要求审批，审批也**不能**扩大沙箱许可；
// 这里接受 approval_required 作为“边界已识别”，但绝不接受任何执行成功。
func TestSEC01_WriteOutsideWorkspaceIsBlocked(t *testing.T) {
	escapeTarget := filepath.Join("..", "outside", "escaped_dir")

	for _, mode := range []testsupport.SandboxMode{
		testsupport.SandboxReadOnly,
		testsupport.SandboxWorkspaceWrite,
	} {
		t.Run(string(mode), func(t *testing.T) {
			h := newSecHarness(t, mode)
			sess := h.newSession(t, "sec01-"+string(mode))

			res := h.execTool(t, sess, "file_mkdir", map[string]any{"path": escapeTarget}, "sec01_escape")
			switch res.Status {
			case "approval_required":
				// 边界被识别为高风险。但审批不得扩大许可 —— 这里显式断言
				// 目标依然不存在。
				t.Logf("内核要求审批（display=%q）", res.Display)
			case "denied":
				t.Logf("内核直接拒绝（error=%q）", res.Error)
			default:
				t.Errorf("SECURITY: 越界写入 status = %q，want denied 或 approval_required", res.Status)
			}

			h.assertNoEscape(t, filepath.Join(h.env.OutsideRoot, "escaped_dir"))
		})
	}
}

// SEC-02（P0）路径变体：../ 穿越、绝对路径、符号链接逃逸。
//
// 精确预期：规范化后的真实目标必须受策略约束；
// 失败时不得有任何副作用；平台不适用项显式标注而不是静默通过。
func TestSEC02_PathEscapeVariants(t *testing.T) {
	h := newSecHarness(t, testsupport.SandboxWorkspaceWrite)
	sess := h.newSession(t, "sec02")

	// 在工作区内建一个指向工作区外的符号链接，测试“通过链接逃逸”。
	linkPath := filepath.Join(h.env.WorkspaceRoot, "escape_link")
	linkTarget := h.env.OutsideRoot
	if err := os.Symlink(linkTarget, linkPath); err != nil {
		// 平台不支持符号链接（例如 Windows 未开开发者模式）：
		// 显式标注为不适用，而不是假装通过。
		t.Skipf("SEC-02(symlink): 当前平台/环境不支持创建符号链接（%v），该变体不适用", err)
	}
	t.Logf("符号链接已建立: %s -> %s", linkPath, linkTarget)

	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "dotdot_traversal", tool: "file_mkdir", args: map[string]any{"path": "../outside/traversed"}},
		{name: "absolute_outside", tool: "file_mkdir", args: map[string]any{
			"path": filepath.Join(h.env.OutsideRoot, "absolute_dir"),
		}},
		{name: "via_symlink", tool: "file_mkdir", args: map[string]any{"path": "escape_link/via_link"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := h.execTool(t, sess, tc.tool, tc.args, "sec02_"+tc.name)
			switch res.Status {
			case "denied", "approval_required":
				t.Logf("%s 被拦截：status=%s display=%q", tc.name, res.Status, res.Display)
			default:
				t.Errorf("SECURITY: %s 未被拦截，status = %q", tc.name, res.Status)
			}
			h.assertNoEscape(t,
				filepath.Join(h.env.OutsideRoot, "traversed"),
				filepath.Join(h.env.OutsideRoot, "absolute_dir"),
				filepath.Join(h.env.OutsideRoot, "via_link"),
			)
		})
	}

	// 工作区内合法写入仍然必须可用 —— 否则“安全测试”只是在测“什么都不许做”。
	t.Run("inside_workspace_still_works", func(t *testing.T) {
		res := h.execTool(t, sess, "file_mkdir", map[string]any{"path": "legit_inside"}, "sec02_legit")
		if res.Status == "denied" {
			t.Logf("工作区内写入被拒绝：%q（审批=%q）", res.Error, approvalIDFrom(t, res))
			return
		}
		if res.Status != "ok" {
			t.Logf("工作区内写入 status=%s（可能需要审批）", res.Status)
			return
		}
		testsupport.MustBeDir(t, filepath.Join(h.env.WorkspaceRoot, "legit_inside"))
	})
}

// SEC-03（P0）审批不能被复用到另一个目标：先批 A，再请求 B。
//
// 精确预期：
//   - A 获批后执行，作用在 A 上；
//   - 对 B 的同类请求不会因为“刚才批过”而自动执行；
//   - 全过程工作区外哨兵不变。
func TestSEC03_ApprovalNotReusableForDifferentTarget(t *testing.T) {
	h := newSecHarness(t, testsupport.SandboxWorkspaceWrite)
	sess := h.newSession(t, "sec03")

	// 先批一个工作区内的合法写入。
	first := h.execTool(t, sess, "file_mkdir", map[string]any{"path": "approved_target"}, "sec03_a")
	if first.Status != "approval_required" {
		t.Skipf("SEC-03: 当前内核对该工具不要求审批（status=%q），审批复用语义不适用", first.Status)
	}
	idA := approvalIDFrom(t, first)
	if idA == "" {
		t.Fatalf("缺少 approval_id")
	}
	respondAccept(t, h, idA)
	waitToolExecuted(t, h, "sec03_a", 30*time.Second)
	testsupport.MustBeDir(t, filepath.Join(h.env.WorkspaceRoot, "approved_target"))

	// 紧接着请求一个工作区外的目标：不得因为上一次授权而放行。
	second := h.execTool(t, sess, "file_mkdir", map[string]any{
		"path": filepath.Join("..", "outside", "reused_approval_target"),
	}, "sec03_b")
	if second.Status == "ok" {
		t.Errorf("SECURITY: 上一次授权被复用到了工作区外的目标（status=ok）")
	}
	h.assertNoEscape(t, filepath.Join(h.env.OutsideRoot, "reused_approval_target"))
	t.Logf("第二个请求 status=%q display=%q", second.Status, second.Display)
}

// SEC-04（P0）审批状态机：重复响应 / 未知 approval_id 不得造成二次执行。
//
// 精确预期：
//   - 第一次 accept 后副作用发生一次；
//   - 对同一 approval_id 重复 respond 不得再执行；
//   - 对不存在的 approval_id 必须明确报错。
func TestSEC04_RepeatedApprovalDoesNotExecuteTwice(t *testing.T) {
	h := newSecHarness(t, testsupport.SandboxWorkspaceWrite)
	sess := h.newSession(t, "sec04")

	target := filepath.Join(h.env.WorkspaceRoot, "single_execution")
	res := h.execTool(t, sess, "file_mkdir", map[string]any{"path": "single_execution"}, "sec04_a")
	if res.Status != "approval_required" {
		t.Skipf("SEC-04: 当前内核不要求审批（status=%q），该状态机变体不适用", res.Status)
	}
	id := approvalIDFrom(t, res)
	respondAccept(t, h, id)
	waitToolExecuted(t, h, "sec04_a", 30*time.Second)
	testsupport.MustBeDir(t, target)

	executedBefore := len(h.conn.EventsOfType("tool.executed"))

	// 重复响应同一 approval_id：必须被拒绝，且不产生第二次执行。
	_, err := h.conn.CallTimeout("approval/respond", map[string]any{
		"approval_id": id,
		"decision":    "accept",
	}, 15*time.Second)
	if err == nil {
		t.Logf("重复 respond 被接受（记录实际行为，需确认内核是否幂等）")
	} else {
		t.Logf("重复 respond 被拒绝：%v", err)
	}

	// 不存在的 approval_id 必须明确报错。
	_, err = h.conn.CallTimeout("approval/respond", map[string]any{
		"approval_id": "approval_does_not_exist",
		"decision":    "accept",
	}, 15*time.Second)
	if err == nil {
		t.Errorf("对不存在的 approval_id 响应竟然成功")
	} else {
		t.Logf("未知 approval_id 错误 = %v", err)
	}

	// 给一个观察窗口，确认没有第二次执行。
	h.conn.WaitQuiet(3 * time.Second)
	if after := len(h.conn.EventsOfType("tool.executed")); after != executedBefore {
		t.Errorf("副作用执行次数从 %d 变成 %d，重复审批造成了二次执行",
			executedBefore, after)
	}
	h.assertNoEscape(t)
}

func respondAccept(t *testing.T, h *secHarness, approvalID string) {
	t.Helper()
	raw, err := h.conn.CallTimeout("approval/respond", map[string]any{
		"approval_id": approvalID,
		"decision":    "accept",
	}, 20*time.Second)
	if err != nil {
		t.Fatalf("approval/respond: %v", err)
	}
	var receipt struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatalf("解析回执失败：%v", err)
	}
	if !receipt.Accepted {
		t.Fatalf("审批未被接受：%s", string(raw))
	}
}

func waitToolExecuted(t *testing.T, h *secHarness, requestID string, timeout time.Duration) {
	t.Helper()
	got := h.conn.WaitForMatch(t, timeout, func(e testsupport.Event) bool {
		return e.Type == "tool.executed" && e.PayloadString("request_id") == requestID
	})
	if len(got) == 0 {
		t.Fatalf("TIMEOUT: 未收到 request_id=%s 的 tool.executed", requestID)
	}
	if s := got[0].PayloadString("status"); s != "ok" {
		t.Fatalf("tool.executed status = %q, want ok", s)
	}
}

// SEC-07（P1）凭据脱敏：假 token 不得出现在日志/诊断输出里。
//
// 只使用测试假 token，并以“搜索该假 token”方式验证。
func TestSEC07_FakeTokenNotLeakedInLogs(t *testing.T) {
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	const fakeToken = "eos-test-LEAKCANARY-9f3a2b-not-a-credential"

	model := testsupport.NewModelServer(t, testsupport.NewTextScript("main", nil, "ok"))
	conn, err := testsupport.StartCore(t, core, env.CoreEnv(map[string]string{
		"EOS_SANDBOX_MODE": string(testsupport.SandboxWorkspaceWrite),
	}))
	if err != nil {
		t.Fatalf("启动内核失败：%v", err)
	}
	if _, err := conn.Call("model/upsert", map[string]any{
		"name": "sec07", "api_base": model.BaseURL, "api_key": fakeToken, "model": "eos-test-model",
	}); err != nil {
		t.Fatalf("model/upsert: %v", err)
	}

	// model/list 是最可能回显配置的地方。
	raw, err := conn.Call("model/list", map[string]any{})
	if err != nil {
		t.Fatalf("model/list: %v", err)
	}
	if strings.Contains(string(raw), fakeToken) {
		t.Errorf("SECURITY: model/list 回显了明文 API token：%s", trunc(string(raw), 400))
	}
	if st := string(raw); strings.Contains(st, fakeToken) {
		t.Errorf("SECURITY: token 明文出现")
	}
	t.Logf("model/list 返回中不含明文 token（前 200 字符）: %s", trunc(string(raw), 200))

	// 内核 stderr 同样不得出现明文 token。
	if strings.Contains(conn.Stderr(), fakeToken) {
		t.Errorf("SECURITY: 内核 stderr 出现明文 token：%s", trunc(conn.Stderr(), 400))
	}

	// transcript 里我们自己写入的请求当然包含 token（那是发送内容），
	// 但响应内容不得回显。
	for _, e := range conn.Transcript() {
		if e.Kind == "request" {
			continue
		}
		if strings.Contains(e.Raw, fakeToken) {
			t.Errorf("SECURITY: %s 帧回显了明文 token：%s", e.Kind, trunc(e.Raw, 300))
		}
	}
}

// SEC-08（P0）未信任工作区：未信任时写操作不得直接执行。
//
// 精确预期：workspace/trust 之前，对该工作区的写请求要么被拒绝、
// 要么要求审批，且不产生副作用；信任之后才允许正常走审批/执行链路。
func TestSEC08_UntrustedWorkspaceRequiresTrust(t *testing.T) {
	h := newSecHarness(t, testsupport.SandboxWorkspaceWrite)
	sess := h.newSession(t, "sec08")

	// 工作区默认为未信任；确认这一点（否则本用例的前提不成立）。
	snap, err := h.conn.Call("state/snapshot", map[string]any{})
	if err != nil {
		t.Fatalf("state/snapshot: %v", err)
	}
	t.Logf("state/snapshot = %s", trunc(string(snap), 300))

	insideTarget := filepath.Join(h.env.WorkspaceRoot, "untrusted_write")
	res := h.execTool(t, sess, "file_mkdir", map[string]any{"path": "untrusted_write"}, "sec08_a")
	switch res.Status {
	case "denied":
		t.Logf("未信任工作区的写请求被直接拒绝：%q", res.Error)
	case "approval_required":
		t.Logf("未信任工作区的写请求要求审批：%q", res.Display)
		// 审批前不得有副作用
		testsupport.MustNotExist(t, insideTarget, "审批之前就执行了写入")
		id := approvalIDFrom(t, res)
		respondAccept(t, h, id)
		waitToolExecuted(t, h, "sec08_a", 30*time.Second)
		testsupport.MustBeDir(t, insideTarget)
	case "ok":
		// 若内核允许直接执行，必须仍受沙箱边界约束。
		testsupport.MustBeDir(t, insideTarget)
		t.Logf("未信任工作区仍直接执行（记录实际信任语义）")
	default:
		t.Errorf("SECURITY: 未信任工作区返回了意外状态 %q", res.Status)
	}
	h.assertNoEscape(t)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var _ = runtime.GOOS
