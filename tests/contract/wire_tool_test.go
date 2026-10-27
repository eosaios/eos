package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/eosaios/eos/internal/testsupport"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// CT-05（P0）真实 EOS wire 帧：Unicode 与任意字节边界。
//
// 规格来源：eos-core-protocol 的 stdio 分帧实现 + Go 侧 jsonrpc.Stream。
// 精确预期：
//   - Content-Length 按 UTF-8 字节数计，不是 rune 数、也不是字符数；
//   - 帧内不出现 jsonrpc 版本字段（EOS wire 不是标准 JSON-RPC 2.0 封装）；
//   - 任意字节边界拆分后仍能还原完整消息；连续两帧不得串位。
//
// 载荷来源：真实内核的 tool/catalog 响应，其中必然含中文与 ASCII 混合。
func TestCT05_RealWireFramingUnicode(t *testing.T) {
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	conn, err := testsupport.StartCore(t, core, env.CoreEnv(nil))
	if err != nil {
		t.Fatalf("启动真实内核失败：%v", err)
	}

	raw, err := conn.Call("tool/catalog", map[string]any{})
	if err != nil {
		t.Fatalf("tool/catalog: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("真实内核返回空载荷，无法验证分帧")
	}

	t.Run("content_length_is_byte_based", func(t *testing.T) {
		frame := buildFrame(raw)
		wantPrefix := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(raw))
		if !bytes.HasPrefix(frame, []byte(wantPrefix)) {
			t.Fatalf("帧头未按字节长度编码：want prefix %q, got %q", wantPrefix, truncate(string(frame), 80))
		}
		gotRunes := len([]rune(string(raw)))
		if len(raw) > gotRunes {
			t.Logf("payload 字节数=%d rune 数=%d（多 %d 字节证明按字节而非字符分帧）",
				len(raw), gotRunes, len(raw)-gotRunes)
		} else {
			t.Errorf("载荷全为 ASCII，无法证明按字节分帧；需要含多字节字符的真实样本")
		}
	})

	t.Run("no_jsonrpc_version_field", func(t *testing.T) {
		if bytes.Contains(raw, []byte(`"jsonrpc"`)) {
			t.Fatalf("真实响应里出现 jsonrpc 版本字段，与 EOS wire 约定不符：%s", truncate(string(raw), 200))
		}
	})

	t.Run("producer_writes_byte_based_content_length", func(t *testing.T) {
		// 必须走**生产写路径**：只验证解码端或自己拼帧，
		// 抓不住“写入时把字节数写成字符数”这类实现错误。
		// 载荷用合法的响应信封（含多字节字符），保证能被 Decode 的校验通过。
		payload := []byte(`{"id":1,"result":{"中文":"🌍 内容","ascii":"tail"}}`)
		var buf bytes.Buffer
		stream := jsonrpc.NewStream(nil, &buf)
		if err := stream.WriteFrame(payload); err != nil {
			t.Fatalf("WriteFrame: %v", err)
		}
		wantLen := len(payload)
		gotRunes := len([]rune(string(payload)))
		if gotRunes == wantLen {
			t.Fatalf("载荷没有多字节字符，无法区分字节数与字符数")
		}
		wantHeader := fmt.Sprintf("Content-Length: %d\r\n\r\n", wantLen)
		if !bytes.HasPrefix(buf.Bytes(), []byte(wantHeader)) {
			t.Errorf("生产写路径的 Content-Length 不是字节数：\n  want %q\n  got  %q\n"+
				"  (payload 字节数=%d 字符数=%d)",
				wantHeader, trunc(string(buf.Bytes()), 60), wantLen, gotRunes)
		}
		// 写出来的帧必须能被读回来（含中文与 emoji 往返无损）。
		// 比较对象是 result 部分：payload 是完整信封，ReadMessage 返回信封。
		back := jsonrpc.NewStream(bytes.NewReader(buf.Bytes()), nil)
		msg, err := back.ReadMessage()
		if err != nil {
			t.Fatalf("回读自己写出的帧失败：%v", err)
		}
		if msg.Response == nil {
			t.Fatalf("回读结果不是响应：%v", msg.Kind)
		}
		var envelope struct {
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			t.Fatalf("解析原始 payload 失败：%v", err)
		}
		var want, got any
		if err := json.Unmarshal(envelope.Result, &want); err != nil {
			t.Fatalf("解析原始 result 失败：%v", err)
		}
		if err := json.Unmarshal(msg.Response.Result, &got); err != nil {
			t.Fatalf("解析回读 result 失败：%v", err)
		}
		if fmt.Sprintf("%v", want) != fmt.Sprintf("%v", got) {
			t.Errorf("写出→读回后语义不一致：\\n  want %v\\n  got  %v", want, got)
		}
	})

	t.Run("split_at_arbitrary_byte_boundaries", func(t *testing.T) {
		// 用内核真实写出的响应帧（含 id/result 外壳），不是 Call 的 result。
		frames := conn.RawResponses()
		if len(frames) == 0 {
			t.Fatal("没有捕获到真实响应帧")
		}
		full := buildFrame([]byte(frames[0]))
		for _, chunk := range []int{1, 2, 3, 7, 13, 64, 1024} {
			feeder := &chunkedReader{data: full, chunk: chunk}
			st := jsonrpc.NewStream(feeder, nil)
			if _, err := st.ReadMessage(); err != nil {
				t.Fatalf("chunk=%d 逐块喂入解码失败：%v", chunk, err)
			}
		}
	})

	t.Run("two_messages_back_to_back", func(t *testing.T) {
		// 连续两帧不得互相串位：用真实帧验证解码边界。
		frames := conn.RawResponses()
		if len(frames) < 2 {
			t.Fatalf("需要至少 2 个真实响应帧，实际 %d", len(frames))
		}
		buf := bytes.NewReader(append(buildFrame([]byte(frames[0])), buildFrame([]byte(frames[1]))...))
		st := jsonrpc.NewStream(buf, nil)
		m1, err := st.ReadMessage()
		if err != nil {
			t.Fatalf("读取第一帧失败：%v", err)
		}
		m2, err := st.ReadMessage()
		if err != nil {
			t.Fatalf("读取第二帧失败：%v", err)
		}
		// 每帧都必须被解成独立响应，且 id 与真实帧一致。
		if m1.Response == nil || m2.Response == nil {
			t.Fatalf("连续帧解码结果不是响应：%#v / %#v", m1.Kind, m2.Kind)
		}
		if m1.Response.ID.String() == m2.Response.ID.String() {
			t.Errorf("两帧解出同一个 id %v，说明帧边界错位", m1.Response.ID)
		}
	})
}

// chunkedReader 按固定大小切块返回数据，模拟任意字节边界拆分。
type chunkedReader struct {
	data  []byte
	chunk int
	pos   int
}

func (c *chunkedReader) Read(p []byte) (int, error) {
	if c.pos >= len(c.data) {
		return 0, io.EOF
	}
	n := c.chunk
	if n > len(p) {
		n = len(p)
	}
	if c.pos+n > len(c.data) {
		n = len(c.data) - c.pos
	}
	copy(p, c.data[c.pos:c.pos+n])
	c.pos += n
	return n, nil
}

// buildFrame 按 EOS wire 规则构造一帧：只有 Content-Length，没有版本字段。
func buildFrame(payload []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n", len(payload))
	b.Write(payload)
	return b.Bytes()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// CT-07（P0）真实工具目录：全量枚举 + 路由可达性。
//
// 规格来源：真实 eos-core 的 tool/catalog 响应。
// 精确预期：
//   - 目录非空；每项都有 name / params_schema / description / invocable；
//   - params_schema 必须是 type=object 的 JSON Schema；
//   - tool/execute 对未知工具名明确报错（路由层确实校验工具名）。
//
// 不会在这里盲调有副作用的工具 —— 全量执行按 TEST-STRATEGY §3 拆成
// “目录/schema/route 全量检查” + “按工具族的安全执行 fixture”两步。
func TestCT07_LiveToolCatalogShape(t *testing.T) {
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	conn, err := testsupport.StartCore(t, core, env.CoreEnv(nil))
	if err != nil {
		t.Fatalf("启动真实内核失败：%v", err)
	}

	raw, err := conn.Call("tool/catalog", map[string]any{})
	if err != nil {
		t.Fatalf("tool/catalog: %v", err)
	}
	var catalog []map[string]any
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("tool/catalog 返回的不是数组：%v (raw=%s)", err, truncate(string(raw), 200))
	}
	if len(catalog) == 0 {
		t.Fatal("真实内核返回空工具目录，P0 工具调用链路无从验证")
	}
	t.Logf("真实工具目录条目数 = %d", len(catalog))

	seen := map[string]bool{}
	invocable := 0
	for i, entry := range catalog {
		name, _ := entry["name"].(string)
		if name == "" {
			t.Errorf("目录第 %d 项缺少 name：%v", i, entry)
			continue
		}
		if seen[name] {
			t.Errorf("工具目录出现重复工具名 %q，路由会产生歧义", name)
		}
		seen[name] = true

		schema, ok := entry["params_schema"].(map[string]any)
		if !ok {
			t.Errorf("工具 %q 缺少 params_schema 对象", name)
			continue
		}
		if typ, _ := schema["type"].(string); typ != "object" {
			t.Errorf("工具 %q 的 params_schema.type = %q，want object", name, typ)
		}
		if _, ok := schema["properties"].(map[string]any); ok {
			// object schema 应当能枚举出属性，便于消费者做补全。
			for prop, def := range schema["properties"].(map[string]any) {
				defMap, ok := def.(map[string]any)
				if !ok {
					t.Errorf("工具 %q 的属性 %q 定义不是对象", name, prop)
					continue
				}
				if _, hasType := defMap["type"]; !hasType {
					t.Errorf("工具 %q 的属性 %q 缺少 type", name, prop)
				}
			}
		}
		if _, ok := entry["description"].(string); !ok {
			t.Errorf("工具 %q 缺少 description，前端无法展示用途", name)
		}
		if iv, ok := entry["invocable"].(bool); ok && iv {
			invocable++
		}
	}
	t.Logf("invocable 工具数 = %d", invocable)

	// 路由验证：未知工具必须被明确拒绝，且不得产生任何副作用。
	//
	// 真实内核的约定：未知工具不是 RPC 错误，而是 status="denied" 的
	// ToolResult（RPC 层本身是通的）。断言必须落在结果语义上。
	sessID := createSessionForContract(t, conn, env.WorkspaceRoot, "ct07")
	beforeAll := testsupport.SnapshotDir(t, env.WorkspaceRoot)

	unknownRaw, err := conn.Call("tool/execute", map[string]any{
		"name":           "definitely_not_a_real_tool",
		"args":           map[string]any{},
		"workspace_root": env.WorkspaceRoot,
		"session_id":     sessID,
		"request_id":     "ct07_unknown",
	})
	if err != nil {
		t.Fatalf("未知工具应返回拒绝结果而不是传输错误，实际 err=%v", err)
	}
	var res struct {
		Status  string `json:"status"`
		Name    string `json:"name"`
		Error   string `json:"error"`
		Display string `json:"display"`
	}
	if err := json.Unmarshal(unknownRaw, &res); err != nil {
		t.Fatalf("解析 tool/execute 结果失败：%v (raw=%s)", err, truncate(string(unknownRaw), 200))
	}
	if res.Status != "denied" {
		t.Errorf("未知工具 status = %q，want denied", res.Status)
	}
	if res.Error == "" && res.Display == "" {
		t.Errorf("未知工具既无 error 也无 display，用户看不到原因：%s", truncate(string(unknownRaw), 200))
	}
	if res.Name != "definitely_not_a_real_tool" {
		t.Errorf("拒绝结果回显的工具名 = %q，want definitely_not_a_real_tool", res.Name)
	}
	if after := testsupport.SnapshotDir(t, env.WorkspaceRoot); len(beforeAll.Diff(after)) != 0 {
		t.Errorf("未知工具调用产生了副作用：%v", beforeAll.Diff(after))
	}
	t.Logf("未知工具结果 = %s", truncate(string(unknownRaw), 240))

	// 缺 name 属于参数错误，必须是 RPC 层 invalid params。
	_, err = conn.Call("tool/execute", map[string]any{
		"workspace_root": env.WorkspaceRoot,
		"session_id":     sessID,
		"request_id":     "ct07_noname",
	})
	if err == nil {
		t.Fatal("缺少 name 的 tool/execute 竟然成功，参数校验缺失")
	}
	var rpcErr *testsupport.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("缺 name 错误类型 = %T (%v)，want *RPCError", err, err)
	}
	if rpcErr.Code != -32602 {
		t.Errorf("缺 name 错误 code = %d，want -32602 (invalid params)", rpcErr.Code)
	}
	t.Logf("缺 name 错误 = %v", err)
}

// CT-09（P0）事件消费语义：真实事件可解码、ID 关联正确、终态一致。
//
// 规格来源：真实 eos-core 推送的 event 通知。
// 精确预期：
//   - 每条通知的 method 必须是 "event"，params.event_type 非空；
//   - turn 相关事件的 session_id/turn_id 必须与请求的一致；
//   - 每个 turn 恰好一个终态，且终态里的 items 与增量拼接一致。
func TestCT09_EventEnvelopeSemantics(t *testing.T) {
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)

	model := testsupport.NewModelServer(t,
		testsupport.NewTextScript("main", nil, "确定性", "回答🌍"),
		testsupport.NewBackgroundScript(),
	)
	conn, err := testsupport.StartCore(t, core, env.CoreEnv(nil))
	if err != nil {
		t.Fatalf("启动真实内核失败：%v", err)
	}
	if _, err := conn.Call("model/upsert", map[string]any{
		"name": "ct09", "api_base": model.BaseURL, "api_key": model.Token, "model": "eos-test-model",
	}); err != nil {
		t.Fatalf("model/upsert: %v", err)
	}

	sessID := createSessionForContract(t, conn, env.WorkspaceRoot, "ct09")
	if err := conn.SubscribeTurn(sessID, ""); err != nil {
		t.Fatalf("event/subscribe: %v", err)
	}

	raw, err := conn.Call("turn/start", map[string]any{"session_id": sessID, "input": "确定性回答"})
	if err != nil {
		t.Fatalf("turn/start: %v", err)
	}
	var started struct {
		ID      string `json:"id"`
		TurnID  string `json:"turn_id"`
		Session string `json:"session_id"`
	}
	_ = json.Unmarshal(raw, &started)
	turnID := firstNonEmptyString(started.TurnID, started.ID)

	terminal := conn.AssertTerminal(t, turnID, 60*second)

	events := conn.EventsOfTurn(turnID)
	if len(events) == 0 {
		t.Fatalf("turn %s 没有收到任何事件", turnID)
	}
	// envelope 结构：method 必须是 event，且 params 里要有 event_type。
	for _, e := range events {
		if e.Type == "" {
			t.Errorf("事件缺少 event_type：%v", e.Raw)
		}
		if e.SessionID != sessID {
			t.Errorf("事件 %s 的 session_id = %q，want %q", e.Type, e.SessionID, sessID)
		}
		if e.TurnID != turnID {
			t.Errorf("事件 %s 的 turn_id = %q，want %q", e.Type, e.TurnID, turnID)
		}
		if e.Priority == "" {
			t.Errorf("事件 %s 缺少 priority，背压策略无法判断", e.Type)
		}
	}

	// 终态与增量一致。
	joined := conn.JoinedText(turnID)
	items, _ := terminal.Payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("终态 items = %d，want 1", len(items))
	}
	item, _ := items[0].(map[string]any)
	if got, _ := item["text"].(string); got != joined {
		t.Errorf("终态 item.text = %q，增量拼接 = %q，两者不一致", got, joined)
	}
	if joined != "确定性回答🌍" {
		t.Errorf("增量拼接 = %q，want %q", joined, "确定性回答🌍")
	}

	// 终态唯一（AssertTerminal 已断言），这里再确认没有重复终态类型混流。
	seen := map[string]int{}
	for _, e := range conn.EventsOfTurn(turnID) {
		seen[e.Type]++
	}
	if seen["turn.completed"] != 1 {
		t.Errorf("turn.completed 出现 %d 次，want 1", seen["turn.completed"])
	}
	t.Logf("事件序列:\n%s", conn.TypeHistogram())
}

func createSessionForContract(t *testing.T, conn *testsupport.CoreConn, workspaceRoot, title string) string {
	t.Helper()
	raw, err := conn.Call("session/create", map[string]any{
		"workspace_root": workspaceRoot,
		"title":          title,
	})
	if err != nil {
		t.Fatalf("session/create: %v\ntranscript:\n%s", err, conn.TranscriptString())
	}
	var sess struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &sess); err != nil {
		t.Fatalf("解析 session/create 失败：%v (raw=%s)", err, truncate(string(raw), 200))
	}
	if sess.ID == "" {
		t.Fatalf("session/create 未返回 id (raw=%s)", truncate(string(raw), 200))
	}
	return sess.ID
}
