package testsupport

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ModelServer 是确定性的 OpenAI 兼容 /chat/completions 服务。
//
// 为什么需要它：真实 Core 的 P0 集成/E2E 必须在不接触付费模型、不使用真实
// 凭据的前提下跑完整条链路。Core 的 openai_compatible provider 走
// `POST {api_base}/chat/completions` + SSE，本服务按已核实的 wire 形态回放脚本。
//
// 关键设计 —— 按谓词路由而不是按到达顺序：
// 真实 Core 除主 turn 外还会发后台请求（例如 Memory Consolidation）。
// 若按「第 N 个请求用第 N 个脚本」路由，后台流量会打乱脚本顺序，
// 让用例变得依赖内核内部实现。改为“某类请求命中某段脚本”可稳定复现。
type ModelServer struct {
	// BaseURL 是要写进 model/upsert 的 api_base。
	BaseURL string
	// Token 是测试专用假 token。绝不使用真实凭据。
	Token string

	srv *httptest.Server

	mu         sync.Mutex
	scripts    []*ModelScript
	barriers   []*Barrier
	nextScript int
	requests   []CapturedRequest
	unmatched  int
}

// ModelScript 是一段“按谓词命中”的模型脚本。
type ModelScript struct {
	// Name 用于失败信息定位。
	Name string
	// Match 判定该请求是否由这段脚本处理。
	// nil 表示“兜底脚本”，只有在其它脚本都不匹配时才使用。
	Match func(CapturedRequest) bool

	// TextDeltas 按顺序逐条推送的 assistant 内容增量。
	TextDeltas []string
	// ToolCalls 是一次性返回的工具调用。
	ToolCalls []ScriptedToolCall
	// HTTPStatus 非 2xx 时返回该状态码。
	HTTPStatus int
	// HTTPBody 是非 2xx 时的响应体。
	HTTPBody string
	// DisconnectAfterN 在推送 N 条数据后直接断开连接（制造“部分文本后断流”）。
	// 负值表示发完所有内容后再正常收尾。
	DisconnectAfterN int
	// Barrier 在推送任何内容前等待该屏障。
	Barrier *Barrier
	// BarrierAfterDeltas 在推送 N 条数据之后才等待屏障。
	//
	// 用于“先给一点增量、再卡住”的时序：证明消费侧在任务完成之前
	// 就已经拿到了增量，而不是一次性收到完整结果。
	// 与 Barrier 互斥；两者都设置时 Barrier 优先。
	BarrierAfterDeltas *Barrier
	// BarrierAfterN 是 BarrierAfterDeltas 的触发位置（1 = 第一条之后）。
	BarrierAfterN int
	// DelayBefore 为推送前的人为延迟。并发/时序用例优先用 Barrier。
	DelayBefore time.Duration
	// NoStream 让这段脚本返回一次性 JSON 响应而不是 SSE。
	//
	// 真相源：真实 Core 的记忆整合（Memory Consolidation）等后台子请求走
	// 非流式路径，发 SSE 会让它报 "invalid provider payload"。
	// 把它显式建模出来，测试噪音才不会伪装成产品故障。
	NoStream bool

	// used 记录命中次数，用于断言“脚本被调用了几次”。
	used int
}

// ScriptedToolCall 描述一个要被 Core 执行的工具调用。
type ScriptedToolCall struct {
	ID   string
	Name string
	// ArgsJSON 是 tool_calls[].function.arguments 的字符串内容。
	ArgsJSON string
}

// CapturedRequest 记录 Core 发来的一次真实 HTTP 请求。
type CapturedRequest struct {
	// Round 是全局到达序号（从 1 开始），便于报告。
	Round int
	// Script 是命中的脚本名。
	Script string
	// Model / Path / AuthHeader 原样保存，测试可据此断言配置真的生效。
	Model      string
	Path       string
	AuthHeader string
	// Body 是解析后的请求体。
	Body map[string]any
	// RawBody 是未经解析的原始字节。
	RawBody []byte
	// At 是收到请求的时间。
	At time.Time
}

// Messages 从 Body 里抽出 messages 数组，便于断言工具结果是否回传。
func (r CapturedRequest) Messages() []map[string]any {
	raw, ok := r.Body["messages"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// HasSystemMessageContaining 报告系统提示里是否包含某段文本。
// 用于识别后台子请求（例如 Memory Consolidation）。
func (r CapturedRequest) HasSystemMessageContaining(needle string) bool {
	for _, m := range r.Messages() {
		if stringAt(m, "role") != "system" {
			continue
		}
		if content, ok := m["content"].(string); ok && strings.Contains(content, needle) {
			return true
		}
	}
	return false
}

// HasUserTextContaining 报告用户消息里是否包含某段文本。
func (r CapturedRequest) HasUserTextContaining(needle string) bool {
	for _, m := range r.Messages() {
		if stringAt(m, "role") != "user" {
			continue
		}
		if content, ok := m["content"].(string); ok && strings.Contains(content, needle) {
			return true
		}
	}
	return false
}

// Barrier 是一个两阶段放行点：服务端先宣告“到达”，再由测试放行。
//
// reachedOnce 与 releaseOnce 必须分开：到达可能被多个判定分支触发，
// 而放行只能生效一次。共用一个 Once 会让 Release 变成空操作、
// 服务端永久阻塞（表现为下游拿不到终态）。
type Barrier struct {
	reached     chan struct{}
	release     chan struct{}
	reachedOnce sync.Once
	releaseOnce sync.Once
}

// Reached 在服务端到达该屏障时关闭，供测试确认“服务端确实卡住了”。
func (b *Barrier) Reached() <-chan struct{} { return b.reached }

// Release 放行服务端（幂等）。
func (b *Barrier) Release() { b.releaseOnce.Do(func() { close(b.release) }) }

// signalReached 宣告服务端已到达屏障（幂等）。
func (b *Barrier) signalReached() { b.reachedOnce.Do(func() { close(b.reached) }) }

// Await 等待服务端到达屏障，带 deadline；超时直接失败而不是无限等。
func (b *Barrier) Await(t testing.TB, timeout time.Duration) {
	t.Helper()
	select {
	case <-b.reached:
	case <-time.After(timeout):
		t.Fatalf("BARRIER TIMEOUT: 服务端未在 %s 内到达屏障点", timeout)
	}
}

// NewBarrier 构造一个屏障。
func NewBarrier() *Barrier {
	return &Barrier{reached: make(chan struct{}), release: make(chan struct{})}
}

// NewModelServer 启动脚本模型服务并在测试结束时关闭。
func NewModelServer(t testing.TB, scripts ...*ModelScript) *ModelServer {
	t.Helper()
	if len(scripts) == 0 {
		t.Fatalf("NewModelServer 需要至少一段脚本；空脚本等于没有确定性模型")
	}
	s := &ModelServer{scripts: scripts}
	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.srv.Close)
	s.BaseURL = s.srv.URL
	s.Token = "eos-test-fake-token-not-a-credential"
	return s
}

// Requests 返回已捕获的请求副本。
func (s *ModelServer) Requests() []CapturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CapturedRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

// MainRequests 返回命中非兜底脚本的请求，即“真正的 turn 请求”。
func (s *ModelServer) MainRequests() []CapturedRequest {
	var out []CapturedRequest
	for _, r := range s.Requests() {
		if strings.HasPrefix(r.Script, "background") {
			continue
		}
		out = append(out, r)
	}
	return out
}

// WaitForRequests 等待至少 n 次主请求到达，带 deadline。
func (s *ModelServer) WaitForRequests(t testing.TB, n int, timeout time.Duration) []CapturedRequest {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got := s.MainRequests()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("模型服务未在 %s 内收到 %d 次主请求（实际 %d 次，全部 %d 次）\nhistogram:\n%s",
				timeout, n, len(got), len(s.Requests()), s.Histogram())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Histogram 返回脚本命中计数。
func (s *ModelServer) Histogram() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for _, sc := range s.scripts {
		fmt.Fprintf(&b, "  script %-24s x%d\n", sc.Name, sc.used)
	}
	return b.String()
}

// ScriptUseCount 返回某段脚本的命中次数。
func (s *ModelServer) ScriptUseCount(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sc := range s.scripts {
		if sc.Name == name {
			return sc.used
		}
	}
	return -1
}

func (s *ModelServer) handle(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	body := map[string]any{}
	_ = json.Unmarshal(raw, &body)

	s.mu.Lock()
	captured := CapturedRequest{
		Round:      len(s.requests) + 1,
		Model:      stringAt(body, "model"),
		Path:       r.URL.Path,
		AuthHeader: r.Header.Get("Authorization"),
		Body:       body,
		RawBody:    raw,
		At:         time.Now(),
	}
	var script *ModelScript
	for _, sc := range s.scripts {
		if sc.Match != nil && sc.Match(captured) {
			script = sc
			break
		}
	}
	if script == nil {
		for _, sc := range s.scripts {
			if sc.Match == nil {
				script = sc
				break
			}
		}
	}
	if script == nil {
		s.unmatched++
		s.requests = append(s.requests, captured)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"eos test model: no script matched this request","type":"no_script"}}`))
		return
	}
	captured.Script = script.Name
	script.used++
	s.requests = append(s.requests, captured)
	s.mu.Unlock()

	if script.HTTPStatus != 0 && (script.HTTPStatus < 200 || script.HTTPStatus >= 300) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(script.HTTPStatus)
		errBody := script.HTTPBody
		if errBody == "" {
			errBody = fmt.Sprintf(`{"error":{"message":"eos test injected status %d","type":"injected"}}`, script.HTTPStatus)
		}
		_, _ = w.Write([]byte(errBody))
		return
	}

	if script.Barrier != nil {
		script.Barrier.signalReached()
		select {
		case <-script.Barrier.release:
		case <-r.Context().Done():
			return
		case <-time.After(60 * time.Second):
			return
		}
	}
	if script.DelayBefore > 0 {
		select {
		case <-time.After(script.DelayBefore):
		case <-r.Context().Done():
			return
		}
	}

	if script.NoStream {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		text := strings.Join(script.TextDeltas, "")
		payload, _ := json.Marshal(map[string]any{
			"id":      "eos-nonstream",
			"object":  "chat.completion",
			"created": 1,
			"model":   stringAt(body, "model"),
			"choices": []any{map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": text},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18,
			},
		})
		_, _ = w.Write(payload)
		return
	}

	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	sent := 0
	model := stringAt(body, "model")
	emit := func(payload string) bool {
		if _, err := io.WriteString(w, "data: "+payload+"\n\n"); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		sent++
		// 先给 N 条增量再卡住：让消费侧有机会证明“增量早于完成”。
		if script.BarrierAfterDeltas != nil && script.BarrierAfterN > 0 && sent == script.BarrierAfterN {
			b := script.BarrierAfterDeltas
			b.signalReached()
			select {
			case <-b.release:
			case <-r.Context().Done():
				return false
			case <-time.After(60 * time.Second):
				return false
			}
		}
		return script.DisconnectAfterN < 0 || sent < script.DisconnectAfterN
	}

	for i, d := range script.TextDeltas {
		if !emit(sseTextChunk(fmt.Sprintf("eos-chunk-%d", i), model, d)) {
			return
		}
	}
	if len(script.ToolCalls) > 0 && !emit(sseToolChunk("eos-toolcall", model, script.ToolCalls)) {
		return
	}
	if !emit(fmt.Sprintf(
		`{"id":"eos-final","object":"chat.completion.chunk","created":1,"model":%q,`+
			`"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`, model)) {
		return
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func sseTextChunk(id, model, delta string) string {
	payload, _ := json.Marshal(map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": 1,
		"model":   model,
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"content": delta},
		}},
	})
	return string(payload)
}

func sseToolChunk(id, model string, calls []ScriptedToolCall) string {
	raw := make([]any, 0, len(calls))
	for i, c := range calls {
		raw = append(raw, map[string]any{
			"index": i,
			"id":    c.ID,
			"type":  "function",
			"function": map[string]any{
				"name":      c.Name,
				"arguments": c.ArgsJSON,
			},
		})
	}
	payload, _ := json.Marshal(map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": 1,
		"model":   model,
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"tool_calls": raw},
		}},
	})
	return string(payload)
}

// NewTextScript 构造一段“纯文本回答”的脚本。
func NewTextScript(name string, match func(CapturedRequest) bool, deltas ...string) *ModelScript {
	return &ModelScript{Name: name, Match: match, TextDeltas: deltas, DisconnectAfterN: -1}
}

// NewToolScript 构造一段“发起工具调用”的脚本。
func NewToolScript(name string, match func(CapturedRequest) bool, call ScriptedToolCall) *ModelScript {
	return &ModelScript{Name: name, Match: match, ToolCalls: []ScriptedToolCall{call}, DisconnectAfterN: -1}
}

// NewErrorScript 构造一段 HTTP 故障脚本。
func NewErrorScript(name string, match func(CapturedRequest) bool, status int, body string) *ModelScript {
	return &ModelScript{Name: name, Match: match, HTTPStatus: status, HTTPBody: body}
}

// NewBackgroundScript 构造兜底脚本：接住内核的后台请求（如记忆整合），
// 返回一个非流式空回答，避免它们打乱主 turn 的脚本顺序并产生解析噪音。
//
// 名字以 "background" 开头时不会被计入 MainRequests。
func NewBackgroundScript() *ModelScript {
	return &ModelScript{
		Name:             "background-default",
		TextDeltas:       []string{""},
		DisconnectAfterN: -1,
		NoStream:         true,
	}
}

// NewBackgroundNamed 构造带指定名字的后台兜底脚本，便于报告里区分来源。
func NewBackgroundNamed(name string) *ModelScript {
	s := NewBackgroundScript()
	s.Name = "background-" + name
	return s
}

// MatchUserText 返回一个谓词：命中系统提示里含 marker 的后台请求。
func MatchBackground(marker string) func(CapturedRequest) bool {
	return func(r CapturedRequest) bool { return r.HasSystemMessageContaining(marker) }
}

// MatchUserTextContaining 返回一个谓词：命中用户输入含 needle 的请求。
func MatchUserTextContaining(needle string) func(CapturedRequest) bool {
	return func(r CapturedRequest) bool { return r.HasUserTextContaining(needle) }
}

// TrimANSI 去掉终端控制序列，便于对 stdout 做内容断言。
func TrimANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !isANSITerminator(s[j]) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func isANSITerminator(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
