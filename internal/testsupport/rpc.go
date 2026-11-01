package testsupport

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// CoreConn 是到真实 eos-core 的原始 JSON-RPC 连接。
//
// 为什么不用 sidecarclient：契约与集成测试需要断言“线上真实帧”的形状
// （Content-Length 按字节、无 jsonrpc 版本字段、错误对象字段），
// 走公开 client 会把 wire 细节藏起来。
//
// 并发模型：stdout 只有唯一读取者（pumpLoop）。响应按 id 派发给等待方，
// 通知进入事件流。这样 Call 与事件订阅可以并发进行而不会互相抢读。
type CoreConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu         sync.Mutex
	nextID     int64
	pending    map[int64]chan rawFrame
	transcript []TranscriptEntry
	closed     bool
	readErr    error

	// events 是收到��全部通知。
	events []Event
	// eventChanged 用于唤醒等待事件的测试协程。
	eventChanged *sync.Cond

	stderrBuf *lockedBuffer

	// InitResult 是 initialize 握手的原始响应体，契约用例据此断言
	// 真实内核声明的 methods / capabilities。
	InitResult json.RawMessage
}

// rawFrame 是一帧的原始解码结果。
type rawFrame struct {
	Msg map[string]any
	Err error
	Raw string
}

// lockedBuffer 是并发安全的字节缓冲。
type lockedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// TranscriptEntry 记录一次往来的可复核证据。
type TranscriptEntry struct {
	Kind string `json:"kind"` // request | response | notification | stray-response
	ID   int64  `json:"id,omitempty"`
	Raw  string `json:"raw"`
	At   string `json:"at"`
}

// RPCError 是内核返回的结构化错误。
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
	// Trace 是内核附带的追踪号，用于把用户看到的错误和内核日志对上。
	Trace string `json:"trace,omitempty"`
}

func (e *RPCError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("rpc error %d: %s (data=%s trace=%s)", e.Code, e.Message, string(e.Data), e.Trace)
	}
	return fmt.Sprintf("rpc error %d: %s (trace=%s)", e.Code, e.Message, e.Trace)
}

// StartCore 启动真实内核子进程并完成 initialize 握手。
//
// env 必须由 Env.CoreEnv 构造：只包含隔离环境，不继承开发机凭据。
func StartCore(t testing.TB, core Core, env []string, extraArgs ...string) (*CoreConn, error) {
	t.Helper()
	args := append([]string{"--stdio"}, extraArgs...)
	cmd := exec.Command(core.Path, args...)
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("testsupport: stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("testsupport: stdout pipe: %v", err)
	}
	stderrBuf := &lockedBuffer{}
	cmd.Stderr = stderrBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("testsupport: start core %s: %v", core.Path, err)
	}

	c := &CoreConn{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReaderSize(stdout, 64*1024),
		pending:   map[int64]chan rawFrame{},
		stderrBuf: stderrBuf,
		nextID:    1,
	}
	c.eventChanged = sync.NewCond(&c.mu)
	go c.pumpLoop()
	t.Cleanup(c.Close)

	res, err := c.Call("initialize", map[string]any{
		"protocol_version": "1.0",
		"client":           "eos-cli-testsupport",
	})
	if err != nil {
		c.Close()
		t.Fatalf("BLOCKED: 真实内核 initialize 握手失败：%v（stderr=%s）", err, stderrBuf.String())
	}
	c.InitResult = res
	return c, nil
}

// pumpLoop 是 stdout 的唯一读取者：按 id 派发响应，其余视为通知。
func (c *CoreConn) pumpLoop() {
	for {
		msg, raw, err := c.readFrame()
		if err != nil {
			c.mu.Lock()
			c.readErr = err
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = map[int64]chan rawFrame{}
			c.eventChanged.Broadcast()
			c.mu.Unlock()
			return
		}

		idVal, hasID := msg["id"]
		if !hasID {
			c.mu.Lock()
			c.events = append(c.events, parseEvent(msg))
			c.appendTranscriptLocked("notification", 0, raw)
			c.eventChanged.Broadcast()
			c.mu.Unlock()
			continue
		}

		var id int64
		switch v := idVal.(type) {
		case float64:
			id = int64(v)
		case string:
			n, _ := strconv.ParseInt(v, 10, 64)
			id = n
		}
		kind := "response"
		if e, ok := msg["error"]; ok && e != nil {
			kind = "error"
			_ = e
		}
		c.mu.Lock()
		c.appendTranscriptLocked(kind, id, raw)
		ch, ok := c.pending[id]
		if ok {
			delete(c.pending, id)
			ch <- rawFrame{Msg: msg, Raw: raw}
			close(ch)
		} else {
			c.appendTranscriptLocked("stray-response", id, raw)
		}
		c.mu.Unlock()
	}
}

// readFrame 读一帧：先解析 header，再按 Content-Length 精确读取 payload。
func (c *CoreConn) readFrame() (map[string]any, string, error) {
	contentLength := -1
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("invalid header line %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n <= 0 {
				return nil, "", fmt.Errorf("invalid Content-Length %q", value)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, "", errors.New("frame missing Content-Length")
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(c.stdout, payload); err != nil {
		return nil, "", err
	}
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, string(payload), fmt.Errorf("decode payload (%d bytes): %w", contentLength, err)
	}
	return msg, string(payload), nil
}

// Call 发起一次请求并等待对应响应（带 deadline）。
func (c *CoreConn) Call(method string, params any) (json.RawMessage, error) {
	return c.CallTimeout(method, params, 90*time.Second)
}

// CallTimeout 是带显式超时的 Call。超时信息里附带 transcript 便于定位。
func (c *CoreConn) CallTimeout(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("core connection is closed")
	}
	if c.readErr != nil {
		err := c.readErr
		c.mu.Unlock()
		return nil, fmt.Errorf("core connection 已断开：%w", err)
	}
	id := c.nextID
	c.nextID++
	ch := make(chan rawFrame, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req := map[string]any{"id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if err := c.write(req); err != nil {
		c.forgetPending(id)
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case frame, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("%s: core 连接在等待响应时中断\ntranscript:\n%s",
				method, c.TranscriptString())
		}
		if e, ok := frame.Msg["error"]; ok && e != nil {
			return nil, decodeRPCError(e)
		}
		raw, err := json.Marshal(frame.Msg["result"])
		if err != nil {
			return nil, fmt.Errorf("marshal result: %w", err)
		}
		return raw, nil
	case <-timer.C:
		c.forgetPending(id)
		return nil, fmt.Errorf("TIMEOUT: %s (id=%d) 未在 %s 内返回\ntranscript:\n%s",
			method, id, timeout, c.TranscriptString())
	}
}

func (c *CoreConn) forgetPending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Notify 发送一条无响应期望的通知（event/subscribe 等）。
func (c *CoreConn) Notify(method string, params any) error {
	req := map[string]any{"method": method}
	if params != nil {
		req["params"] = params
	}
	return c.write(req)
}

// parseEvent 把真实通知解码成 Event。
//
// wire 形态（已对 eos-core 实际输出核对）：
//
//	{"method":"event","params":{"event_type":..., "payload":{...},
//	 "priority":..., "session_id":..., "source":..., "turn_id":...}}
func parseEvent(msg map[string]any) Event {
	params, _ := msg["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	payload, _ := params["payload"].(map[string]any)
	if payload == nil {
		payload = map[string]any{}
	}
	return Event{
		Type:      stringAt(params, "event_type"),
		Priority:  stringAt(params, "priority"),
		Source:    stringAt(params, "source"),
		SessionID: stringAt(params, "session_id"),
		TurnID:    stringAt(params, "turn_id"),
		Payload:   payload,
		Raw:       msg,
		At:        time.Now(),
	}
}

// SubscribeTurn 订阅某个会话/turn 的全部事件。
//
// 真相源：eos-core-protocol EventSubscribeRequest —— 空 event_types 表示
// 订阅全部；session_id / turn_id 为空表示不按该维度过滤。
// 不订阅就收不到通知，这是真实内核的行为，用例不得假设会自动推送。
func (c *CoreConn) SubscribeTurn(sessionID, turnID string) error {
	params := map[string]any{
		"event_types": []string{},
	}
	if sessionID != "" {
		params["session_id"] = sessionID
	}
	if turnID != "" {
		params["turn_id"] = turnID
	}
	_, err := c.Call("event/subscribe", params)
	return err
}

// UnsubscribeTurn 取消订阅。
func (c *CoreConn) UnsubscribeTurn(sessionID, turnID string) error {
	params := map[string]any{"event_types": []string{}}
	if sessionID != "" {
		params["session_id"] = sessionID
	}
	if turnID != "" {
		params["turn_id"] = turnID
	}
	_, err := c.Call("event/unsubscribe", params)
	return err
}

func (c *CoreConn) write(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// EOS wire 帧：Content-Length 按 UTF-8 字节数，且不带 jsonrpc 版本字段。
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := c.stdin.Write(payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	c.appendTranscriptLocked("request", 0, string(payload))
	return nil
}

func (c *CoreConn) appendTranscriptLocked(kind string, id int64, raw string) {
	c.transcript = append(c.transcript, TranscriptEntry{
		Kind: kind, ID: id, Raw: raw, At: time.Now().Format(time.RFC3339Nano),
	})
}

// RawResponses 返回真实内核写出的原始响应帧（不含 Content-Length 头）。
//
// 分帧相关的契约测试必须用它，而不是用 Call 返回的 result ——
// result 只是响应体的一部分，拿它造帧去喂 jsonrpc.Stream 是在自造样本。
func (c *CoreConn) RawResponses() []string {
	var out []string
	for _, e := range c.Transcript() {
		if e.Kind == "response" || e.Kind == "error" {
			out = append(out, e.Raw)
		}
	}
	return out
}

// RawResponseFor 返回匹配给定 id 的原始响应帧。
func (c *CoreConn) RawResponseFor(id int64) (string, bool) {
	for _, e := range c.Transcript() {
		if e.Kind != "response" && e.Kind != "error" {
			continue
		}
		if e.ID == id {
			return e.Raw, true
		}
	}
	return "", false
}

// Transcript 返回全部往来的副本。
func (c *CoreConn) Transcript() []TranscriptEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]TranscriptEntry, len(c.transcript))
	copy(out, c.transcript)
	return out
}

// TranscriptString 返回适合 t.Log 的可读 transcript。
func (c *CoreConn) TranscriptString() string {
	var b strings.Builder
	for _, e := range c.Transcript() {
		fmt.Fprintf(&b, "  [%s id=%d %s] %s\n", e.Kind, e.ID, e.At, e.Raw)
	}
	return b.String()
}

// Stderr 返回内核 stderr 全文。
func (c *CoreConn) Stderr() string { return c.stderrBuf.String() }

// Close 关闭连接并回收子进程。重复调用是幂等的。
func (c *CoreConn) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = map[int64]chan rawFrame{}
	c.mu.Unlock()

	_ = c.stdin.Close()
	if c.cmd.Process == nil {
		return
	}
	_ = c.cmd.Process.Kill()
	done := make(chan struct{})
	go func() {
		_, _ = c.cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func decodeRPCError(e any) error {
	bs, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("rpc error (undecodable): %v", e)
	}
	var out RPCError
	if err := json.Unmarshal(bs, &out); err != nil {
		return fmt.Errorf("rpc error (undecodable): %s", string(bs))
	}
	return &out
}
