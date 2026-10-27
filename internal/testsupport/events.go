package testsupport

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

// Event 是内核主动推送的一条通知。
//
// 真实 wire 形态（来自 eos-core 实际 stdout，已在 probe 中核对）：
//
//	{"method":"event","params":{
//	  "event_type":"turn.completed","payload":{...},
//	  "priority":"droppable","session_id":"sess_1",
//	  "source":"turn_runner","turn_id":"turn_1"}}
type Event struct {
	// Type 取自 params.event_type，例如 turn.started / turn.item_delta。
	Type string
	// Priority 取自 params.priority。
	Priority string
	// Source 取自 params.source。
	Source string
	// SessionID / TurnID 取自 params 同名字段，不做任何推测。
	SessionID string
	TurnID    string
	// Payload 是 params.payload 的解码结果。
	Payload map[string]any
	// Raw 是整条通知。
	Raw map[string]any
	// At 是本地接收时间。
	At time.Time
}

// PayloadString 取 payload 里的字符串字段。
func (e Event) PayloadString(key string) string { return stringAt(e.Payload, key) }

// PayloadMap 取 payload 里的嵌套对象。
func (e Event) PayloadMap(key string) map[string]any {
	m, _ := e.Payload[key].(map[string]any)
	return m
}

// ItemText 返回 item 事件里 item.text；非 item 事件返回 ""。
func (e Event) ItemText() string {
	item := e.PayloadMap("item")
	return stringAt(item, "text")
}

// terminalTypes 是被认定为 turn 终态的 event_type。
//
// 真相源：真实内核推送的终态事件名。终态唯一是 CASE-CATALOG CT-09 的要求；
// 若内核新增终态类型而此处未登记，AssertTerminal 会因“找不到终态”失败，
// 从而暴露契约漂移，而不是静默放行。
var terminalTypes = map[string]bool{
	"turn.completed": true,
	"turn.failed":    true,
	"turn.cancelled": true,
	"turn.canceled":  true,
	"turn.error":     true,
}

// IsTerminalEvent 报告该事件类型是否为 turn 终态。
func IsTerminalEvent(typ string) bool { return terminalTypes[typ] }

// Events 返回已收集的全部事件副本。
func (c *CoreConn) Events() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Event, len(c.events))
	copy(out, c.events)
	return out
}

// EventsOfType 返回指定类型的事件。
func (c *CoreConn) EventsOfType(typ string) []Event {
	var out []Event
	for _, e := range c.Events() {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

// EventCount 返回指定类型的事件数量。
func (c *CoreConn) EventCount(typ string) int { return len(c.EventsOfType(typ)) }

// EventsOfTurn 返回属于某个 turn 的事件。
func (c *CoreConn) EventsOfTurn(turnID string) []Event {
	var out []Event
	for _, e := range c.Events() {
		if e.TurnID == turnID {
			out = append(out, e)
		}
	}
	return out
}

// TextDeltaEvent 是 turn.item_delta 中文本增量的结构。
type TextDeltaEvent struct {
	ItemID   string `json:"item_id"`
	Delta    string `json:"delta"`
	DeltaTyp string `json:"delta_type"`
}

// waitFor 等待条件满足，带 deadline。
//
// 关键：基于连接内部条件变量（通知到达时广播），不用固定 sleep，
// 既能证明事件真的到达，又不靠拉长超时掩盖竞态。
func (c *CoreConn) waitFor(timeout time.Duration, match func([]Event) bool) {
	deadline := time.Now().Add(timeout)
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if match(c.events) {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		// cond 无超时版本：用短定时器定期唤醒做超时检查。
		timer := time.AfterFunc(20*time.Millisecond, func() {
			c.mu.Lock()
			c.eventChanged.Broadcast()
			c.mu.Unlock()
		})
		c.eventChanged.Wait()
		timer.Stop()
	}
}

// WaitForEvents 等待至少出现 n 个指定类型的事件。
func (c *CoreConn) WaitForEvents(t testing.TB, typ string, n int, timeout time.Duration) []Event {
	t.Helper()
	c.waitFor(timeout, func(evs []Event) bool { return countType(evs, typ) >= n })
	got := c.EventsOfType(typ)
	if len(got) < n {
		t.Fatalf("TIMEOUT: 等待 %d 个 %s 事件超时（%s，实际 %d 个）\n已收到事件类型：\n%s",
			n, typ, timeout, len(got), c.TypeHistogram())
	}
	return got
}

// WaitForMatch 等待出现第一个满足条件的事件。
func (c *CoreConn) WaitForMatch(t testing.TB, timeout time.Duration, match func(Event) bool) []Event {
	t.Helper()
	c.waitFor(timeout, func(evs []Event) bool {
		for _, e := range evs {
			if match(e) {
				return true
			}
		}
		return false
	})
	var out []Event
	for _, e := range c.Events() {
		if match(e) {
			out = append(out, e)
		}
	}
	return out
}

// WaitQuiet 在给定窗口内不做任何事，只用于“观察一段时间确认没有新事件”。
//
// 否定断言需要它：证明“之后什么都没发生”，而不是只看某一瞬间的状态。
// match 恒返回 false 时就是一个纯粹的等待窗口。
func (c *CoreConn) WaitQuiet(window time.Duration) {
	c.waitFor(window, func([]Event) bool { return false })
}

// AssertNoEventIn 断言窗口内不出现满足条件的事件，并返回出现次数。
func (c *CoreConn) AssertNoEventIn(t testing.TB, window time.Duration, match func(Event) bool, what string) {
	t.Helper()
	c.waitFor(window, func(evs []Event) bool {
		for _, e := range evs {
			if match(e) {
				return true
			}
		}
		return false
	})
	for _, e := range c.Events() {
		if match(e) {
			t.Fatalf("观察窗口 %s 内不应出现%s，却收到 %s（payload=%v）", window, what, e.Type, e.Payload)
		}
	}
}

func countType(evs []Event, typ string) int {
	n := 0
	for _, e := range evs {
		if e.Type == typ {
			n++
		}
	}
	return n
}

// terminalsIn 返回事件切片中属于 turnID 的终态事件。
//
// 注意：调用方可能已持有 c.mu（例如在 waitFor 的回调里），
// 因此这里只接受切片、不回读连接状态，避免二次加锁造成死锁。
func terminalsIn(evs []Event, turnID string) []Event {
	var out []Event
	for _, e := range evs {
		if turnID != "" && e.TurnID != turnID {
			continue
		}
		if terminalTypes[e.Type] {
			out = append(out, e)
		}
	}
	return out
}

// AssertTerminal 断言该 turn 恰好出现一个终态事件。
//
// 规则（CT-09 / IT-08）：终态唯一。没有终态或出现矛盾终态都算失败 ——
// 两者都会让上层误判任务是否完成。
func (c *CoreConn) AssertTerminal(t testing.TB, turnID string, timeout time.Duration) Event {
	t.Helper()
	c.waitFor(timeout, func(evs []Event) bool {
		return len(terminalsIn(evs, turnID)) >= 1
	})
	terminals := terminalsIn(c.Events(), turnID)
	if len(terminals) == 0 {
		t.Fatalf("TIMEOUT: turn %s 在 %s 内没有出现任何终态事件。\n已收到事件类型：\n%s\ntranscript:\n%s",
			turnID, timeout, c.TypeHistogram(), c.TranscriptString())
	}
	if len(terminals) > 1 {
		// 再观察一小段，确认不是“迟到的第二个终态”而是真重复。
		c.waitFor(500*time.Millisecond, func([]Event) bool { return false })
		terminals = terminalsIn(c.Events(), turnID)
		if len(terminals) > 1 {
			t.Fatalf("EVENT SEMANTICS: turn %s 出现 %d 个终态事件（要求恰好 1 个）：%s",
				turnID, len(terminals), describeEvents(terminals))
		}
	}
	return terminals[0]
}

// AssertNoTerminal 断言在给定窗口内不出现终态（用于“取消后不得再完成”）。
func (c *CoreConn) AssertNoTerminal(t testing.TB, turnID string, window time.Duration) {
	t.Helper()
	c.waitFor(window, func(evs []Event) bool {
		return len(terminalsIn(evs, turnID)) > 0
	})
	for _, e := range terminalsIn(c.Events(), turnID) {
		t.Fatalf("EVENT SEMANTICS: 期望 turn %s 不再出现终态，却收到 %s（payload=%v）",
			turnID, e.Type, e.Payload)
	}
}

func describeEvents(evs []Event) string {
	parts := make([]string, 0, len(evs))
	for _, e := range evs {
		parts = append(parts, fmt.Sprintf("%s(turn=%s session=%s at=%s)",
			e.Type, e.TurnID, e.SessionID, e.At.Format(time.RFC3339Nano)))
	}
	return strings.Join(parts, ", ")
}

// TypeHistogram 返回“事件类型 → 次数”清单，失败信息里用得上。
func (c *CoreConn) TypeHistogram() string {
	counts := map[string]int{}
	for _, e := range c.Events() {
		counts[e.Type]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "  %-34s x%d\n", k, counts[k])
	}
	if len(keys) == 0 {
		return "  (无事件)\n"
	}
	return b.String()
}

// JoinedText 把某 turn 的所有 text 增量按到达顺序拼接。
func (c *CoreConn) JoinedText(turnID string) string {
	var b strings.Builder
	for _, e := range c.EventsOfTurn(turnID) {
		if e.Type != "turn.item_delta" {
			continue
		}
		if e.PayloadString("delta_type") == "text" {
			b.WriteString(e.PayloadString("delta"))
		}
	}
	return b.String()
}

// EventTypeNames 返回出现过的全部事件类型名（去重、排序）。
func (c *CoreConn) EventTypeNames() []string {
	seen := map[string]bool{}
	for _, e := range c.Events() {
		seen[e.Type] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stringAt 取 map 里的字符串字段。
func stringAt(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// DecodeInto 把事件的 Payload 解码到强类型结构体，失败即 Fatal。
func DecodeInto(t testing.TB, e Event, out any) {
	t.Helper()
	bs, err := json.Marshal(e.Payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := json.Unmarshal(bs, out); err != nil {
		t.Fatalf("decode event %s payload into %T: %v (raw=%s)", e.Type, out, err, string(bs))
	}
}
