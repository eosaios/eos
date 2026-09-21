package ui

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"strings"
	"time"

	uiadapter "github.com/eosaios/eos/internal/ui/adapter"
	"github.com/eosaios/eos/internal/update"
	"github.com/eosaios/eos/pkg/protocol"
)

// Msg 是所有消息类型的接口
type Msg interface {
	msgType()
}

// ConvertEvent 将 runtime event 转换为 UI Msg
func ConvertEvent(e uiadapter.RuntimeEvent) Msg {
	switch e.Type {
	case string(protocol.EventTypeItemStarted):
		return convertItemStarted(e)
	case string(protocol.EventTypeItemDelta):
		return ItemDeltaMsg{
			ItemID:    eventString(e.Data, "item_id"),
			DeltaType: eventString(e.Data, "delta_type"),
			Delta:     eventText(e, "delta", "text", "message"),
			RID:       e.RID,
		}
	case string(protocol.EventTypeItemCompleted):
		return convertItemCompleted(e)
	case "final", string(protocol.EventTypeTextFinal):
		return AIResponseMsg{Type: "final", Content: eventText(e, "text", "message"), RID: e.RID}
	case string(protocol.EventTypeRequestDone):
		return InvokeDoneMsg{Content: eventString(e.Data, "text")}
	case "reasoning", "phase.note", string(protocol.EventTypeTextReasoning),
		string(protocol.EventTypeTaskStarted), string(protocol.EventTypeTaskUpdated), string(protocol.EventTypeTaskDone):
		return ThinkingMsg{RID: e.RID, Content: eventText(e, "message", "text", "label", "title"), Done: false}
	case "agent.task":
		// 调度agent给子agent分配任务
		sourceName, sourceID := eventAgentSource(e.Data)
		return AgentTaskMsg{
			AgentName:       firstNonEmpty(strings.TrimSpace(e.RID), eventString(e.Data, "agent_name")),
			AgentID:         eventString(e.Data, "agent_id"),
			SourceAgentName: sourceName,
			SourceAgentID:   sourceID,
			Event:           "dispatch",
			Task:            eventText(e, "task", "message", "text"),
			Goal:            eventString(e.Data, "goal"),
		}
	case string(protocol.EventTypeAgentStarted), string(protocol.EventTypeAgentProgress):
		sourceName, sourceID := eventAgentSource(e.Data)
		return AgentTaskMsg{
			AgentName:       firstNonEmpty(strings.TrimSpace(e.RID), eventString(e.Data, "agent_name")),
			AgentID:         eventString(e.Data, "agent_id"),
			SourceAgentName: sourceName,
			SourceAgentID:   sourceID,
			Event:           agentEventKind(e.Type),
			Task:            eventText(e, "task", "message", "text"),
			Goal:            eventString(e.Data, "goal"),
		}
	case "agent.final", string(protocol.EventTypeAgentDone):
		// 子agent的最终输出
		sourceName, sourceID := eventAgentSource(e.Data)
		return AgentFinalMsg{
			AgentName:       firstNonEmpty(strings.TrimSpace(e.RID), eventString(e.Data, "agent_name")),
			AgentID:         eventString(e.Data, "agent_id"),
			SourceAgentName: sourceName,
			SourceAgentID:   sourceID,
			Event:           "result",
			Content:         eventText(e, "text", "message"),
		}
	case "prompt.request", string(protocol.EventTypeApprovalReq), string(protocol.EventTypeInquiryReq):
		return convertPromptEvent(e)
	case string(protocol.EventTypeModeChanged):
		return ModeChangedMsg{Mode: eventString(e.Data, "new_mode"), PreviousMode: eventString(e.Data, "old_mode")}
	case string(protocol.EventTypeAgentFailed), string(protocol.EventTypeAgentCancelled):
		sourceName, sourceID := eventAgentSource(e.Data)
		return AgentFinalMsg{
			AgentName:       firstNonEmpty(strings.TrimSpace(e.RID), eventString(e.Data, "agent_name")),
			AgentID:         eventString(e.Data, "agent_id"),
			SourceAgentName: sourceName,
			SourceAgentID:   sourceID,
			Event:           agentEventKind(e.Type),
			Content:         eventText(e, "error", "reason", "message", "text"),
		}
	case "browser.takeover.confirm":
		return BrowserTakeoverConfirmMsg{Reason: payloadString(e, "reason"), Note: payloadString(e, "note")}
	case "browser.takeover.started":
		return BrowserTakeoverStartedMsg{Reason: payloadString(e, "reason"), Note: payloadString(e, "note")}
	case "browser.takeover.ended":
		return BrowserTakeoverEndedMsg{Result: payloadString(e, "result")}
	case "browser.action":
		return BrowserActionMsg{Action: payloadString(e, "action"), Target: payloadString(e, "target"), Result: payloadString(e, "result")}
	case "browser.download.completed":
		return BrowserDownloadDoneMsg{Filename: payloadString(e, "filename"), Path: payloadString(e, "path")}
	case "browser.pick.selected":
		return BrowserPickSelectedMsg{Ref: payloadString(e, "ref"), Selector: payloadString(e, "selector"), Role: payloadString(e, "role"), Name: payloadString(e, "name")}
	case "error", string(protocol.EventTypeRequestFailed), string(protocol.EventTypeTaskFailed):
		return AIResponseMsg{Type: "error", Content: eventText(e, "error", "message", "text"), RID: e.RID}
	default:
		return nil
	}
}

// BrowserTakeoverConfirmMsg AI 请求人工接管，等人确认（browser.takeover.confirm）。
type BrowserTakeoverConfirmMsg struct {
	Reason string
	Note   string
}

// BrowserTakeoverStartedMsg 内核请求/人接管了浏览器（browser.takeover.started）。
type BrowserTakeoverStartedMsg struct {
	Reason string
	Note   string
}

// BrowserTakeoverEndedMsg 接管结束（resumed/timeout/cancelled/unconfirmed/declined）。
type BrowserTakeoverEndedMsg struct {
	Result string
}

// BrowserActionMsg AI 浏览器动作（步骤日志行）。
type BrowserActionMsg struct {
	Action string
	Target string
	Result string
}

// BrowserDownloadDoneMsg 下载完成（含落盘路径）。
type BrowserDownloadDoneMsg struct {
	Filename string
	Path     string
}

// BrowserPickSelectedMsg 选取器捕获元素（结构化引用插入输入行）。
type BrowserPickSelectedMsg struct {
	Ref      string
	Selector string
	Role     string
	Name     string
}

// FormatPickChip 结构化引用的输入行文案（e12 · button "登录"）。
func (msg BrowserPickSelectedMsg) FormatPickChip() string {
	ref := msg.Ref
	if ref == "" {
		ref = msg.Selector
	}
	if ref == "" {
		ref = "element"
	}
	if msg.Role == "" {
		return ref
	}
	name := msg.Name
	if name != "" {
		name = ` "` + name + `"`
	}
	return ref + " · " + msg.Role + name
}

func convertPromptEvent(e uiadapter.RuntimeEvent) PromptRequestMsg {
	msg := PromptRequestMsg{
		ID: eventID(e, "approval_id", "inquiry_id"),
	}

	switch e.Type {
	case string(protocol.EventTypeApprovalReq):
		msg.Kind = "permission"
	case string(protocol.EventTypeInquiryReq):
		msg.Kind = "inquiry"
	default:
		msg.Kind = eventString(e.Data, "kind")
	}
	msg.Title = eventString(e.Data, "title")
	msg.Question = eventText(e, "question", "message", "summary", "text")
	msg.Options = eventStringSlice(e.Data, "options")
	msg.Category = eventString(e.Data, "category")
	msg.Summary = firstNonEmpty(eventString(e.Data, "summary"), eventString(e.Data, "message"))
	msg.Diff = eventString(e.Data, "diff")
	msg.DiffPath = eventString(e.Data, "diff_path")
	msg.AllowText = eventBool(e.Data, "allow_text")
	msg.TextHint = eventString(e.Data, "text_hint")
	if msg.Kind == "" {
		msg.Kind = "permission"
	}
	return msg
}

// convertItemStarted turns an item.started event into a Msg. The payload
// nests the TurnItem under payload.item with a "kind" discriminator
// ("agent_message" or "tool_call").

func payloadString(e uiadapter.RuntimeEvent, key string) string {
	if e.Data == nil {
		return ""
	}
	if value, ok := e.Data[key]; ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	return ""
}

func convertItemStarted(e uiadapter.RuntimeEvent) Msg {
	item, _ := e.Data["item"].(map[string]any)
	kind, _ := item["kind"].(string)
	id, _ := item["id"].(string)
	switch kind {
	case "tool_call":
		name, _ := item["name"].(string)
		args, _ := item["arguments"].(string)
		return ToolCallMsg{ID: id, Name: name, Params: parseToolParams(args)}
	case "reasoning":
		// Reasoning item: a dedicated thinking block. Routed to its own
		// branch in AppModel so it isn't mistaken for an agent_message.
		return ItemStartedMsg{ItemID: id, ItemType: "reasoning", RID: e.RID}
	default:
		// agent_message (or unknown): start a new text segment.
		return ItemStartedMsg{ItemID: id, ItemType: kind, RID: e.RID}
	}
}

// convertItemCompleted turns an item.completed event into a Msg. For
// agent_message items it carries the full segment text; for tool_call items
// it carries the result.
func convertItemCompleted(e uiadapter.RuntimeEvent) Msg {
	item, _ := e.Data["item"].(map[string]any)
	kind, _ := item["kind"].(string)
	id, _ := item["id"].(string)
	switch kind {
	case "tool_call":
		name, _ := item["name"].(string)
		result, _ := item["result"].(map[string]any)
		status, _ := result["status"].(string)
		if status == "" {
			status = "success"
		}
		output, _ := result["display"].(string)
		if output == "" {
			output, _ = result["error"].(string)
		}
		return ToolResultMsg{ID: id, Name: name, Status: status, Output: output}
	case "reasoning":
		// Reasoning content ships as a Vec<String> under "content"; join it
		// into a single string so the shell can archive the thinking block.
		reasoning := strings.Join(eventStringSlice(item, "content"), "\n")
		return ItemCompletedMsg{ItemID: id, ItemType: "reasoning", Reasoning: reasoning, RID: e.RID}
	default:
		text, _ := item["text"].(string)
		reasoning, _ := item["reasoning"].(string)
		return ItemCompletedMsg{ItemID: id, ItemType: kind, Text: text, Reasoning: reasoning, RID: e.RID}
	}
}

// parseToolParams parses a JSON arguments string into a params map.
func parseToolParams(args string) map[string]any {
	params := map[string]any{}
	if strings.TrimSpace(args) == "" {
		return params
	}
	_ = json.Unmarshal([]byte(args), &params)
	return params
}

func eventID(e uiadapter.RuntimeEvent, keys ...string) string {
	for _, key := range keys {
		if v := eventString(e.Data, key); v != "" {
			return v
		}
	}
	return strings.TrimSpace(e.RID)
}

func eventText(e uiadapter.RuntimeEvent, keys ...string) string {
	if text := eventString(e.Data, keys...); text != "" {
		return text
	}
	return strings.TrimSpace(e.Content)
}

func eventParams(e uiadapter.RuntimeEvent) map[string]any {
	if e.Data == nil {
		return nil
	}
	// 按 arguments -> input/parameters/params 的顺序解析工具参数。
	// 绝不能在找不到参数时把整个信封 Data 当参数返回——那会让 UI 把
	// event_id/session_id/turn_id 等元数据渲染成工具参数。
	for _, key := range []string{"arguments", "input", "parameters", "params"} {
		raw, ok := e.Data[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case map[string]any:
			if len(v) > 0 {
				return v
			}
		case string:
			// JSON 字符串是多数模型适配器的承载形态。
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			var out map[string]any
			if err := json.Unmarshal([]byte(v), &out); err == nil && len(out) > 0 {
				return out
			}
			// 非 JSON 字符串：忽略，不回退到信封。
		}
	}
	return nil
}

func eventString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok {
			continue
		}
		if text, ok := raw.(string); ok {
			text = strings.TrimSpace(text)
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func eventStringSlice(data map[string]any, key string) []string {
	if data == nil {
		return nil
	}
	raw, ok := data[key]
	if !ok {
		return nil
	}
	switch items := raw.(type) {
	case []string:
		out := make([]string, len(items))
		copy(out, items)
		return out
	case []interface{}:
		out := make([]string, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			out = append(out, text)
		}
		return out
	default:
		return nil
	}
}

func eventBool(data map[string]any, key string) bool {
	if data == nil {
		return false
	}
	raw, ok := data[key]
	if !ok {
		return false
	}
	v, _ := raw.(bool)
	return v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func agentEventKind(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case "agent.task", string(protocol.EventTypeAgentStarted):
		return "dispatch"
	case string(protocol.EventTypeAgentProgress):
		return "update"
	case "agent.final", string(protocol.EventTypeAgentDone):
		return "result"
	case string(protocol.EventTypeAgentFailed):
		return "failed"
	case string(protocol.EventTypeAgentCancelled):
		return "cancelled"
	default:
		return ""
	}
}

func eventAgentSource(data map[string]any) (string, string) {
	name := firstNonEmpty(
		eventString(data, "source_agent_name"),
		eventString(data, "caller_agent_name"),
		eventString(data, "parent_agent_name"),
	)
	id := firstNonEmpty(
		eventString(data, "source_agent_id"),
		eventString(data, "caller_agent_id"),
		eventString(data, "parent_agent_id"),
	)
	if name == "" {
		name = "assistant"
	}
	return name, id
}

// 系统消息
type WindowSizeMsg struct {
	Width, Height int
}

type TickMsg struct {
	Time time.Time
}

// 用户输入消息
type KeyMsg struct {
	Key string
}

type MouseMsg struct {
	X, Y   int
	Action string
}

// AI 相关消息
type AIRequestMsg struct {
	Query  string
	Images []string
	Mode   string
}

type AIResponseMsg struct {
	Type    string // "delta", "final", "error"
	Content string
	RID     string
}

// ItemStartedMsg signals the start of a new output item (a text segment or a
// tool call). The shell creates a new history entry for this item_id.
type ItemStartedMsg struct {
	ItemID   string
	ItemType string // "agent_message" or "tool_call"
	RID      string
}

// ItemDeltaMsg carries an incremental chunk for an in-progress item.
// DeltaType is "text", "reasoning", or "tool_args".
type ItemDeltaMsg struct {
	ItemID    string
	DeltaType string
	Delta     string
	RID       string
}

// ItemCompletedMsg signals an item is finished. For agent_message items, Text
// holds the full segment text. The shell finalizes the history entry.
type ItemCompletedMsg struct {
	ItemID    string
	ItemType  string
	Text      string
	Reasoning string
	RID       string
}

type InvokeDoneMsg struct {
	Content string
}

type PredictionUpdateMsg struct {
	Text  string
	Seq   int
	Draft string
}

type ThinkingMsg struct {
	RID     string
	Content string
	Done    bool
}

// 工具消息
type ToolCallMsg struct {
	ID     string
	Name   string
	Params map[string]any
}

type ToolResultMsg struct {
	ID     string
	Name   string
	Status string
	Output string
}

// Agent 消息
type AgentTaskMsg struct {
	AgentName       string
	AgentID         string
	SourceAgentName string
	SourceAgentID   string
	Event           string
	Task            string
	Goal            string
}

type AgentFinalMsg struct {
	AgentName       string
	AgentID         string
	SourceAgentName string
	SourceAgentID   string
	Event           string
	Content         string
}

type ModeChangedMsg struct {
	Mode         string
	PreviousMode string
}

type PromptRequestMsg struct {
	ID        string
	Kind      string
	Title     string
	Question  string
	Options   []string
	Category  string
	Summary   string
	Diff      string
	DiffPath  string
	AllowText bool
	TextHint  string
}

type PromptResultMsg struct {
	ID          string
	Kind        string
	Decision    string
	Option      string
	OptionIndex int
	Text        string
}

// 面板消息
type PanelOpenMsg struct {
	Panel string
}

type PanelCloseMsg struct{}

type PanelActionMsg struct {
	Panel  string
	Action string
	Data   any
}

// 设置消息
type SettingsUpdateMsg struct {
	Key   string
	Value any
}

// 错误消息
type ErrorMsg struct {
	Err   error
	Fatal bool
}

// VersionCheckMsg 版本检查结果
type VersionCheckMsg struct {
	Result *update.CheckResult
}

// 实现 msgType 方法以满足接口要求
func (BrowserTakeoverConfirmMsg) msgType() {}
func (BrowserTakeoverStartedMsg) msgType() {}
func (BrowserTakeoverEndedMsg) msgType()   {}
func (BrowserActionMsg) msgType()          {}
func (BrowserDownloadDoneMsg) msgType()    {}
func (BrowserPickSelectedMsg) msgType()    {}
func (WindowSizeMsg) msgType()       {}
func (TickMsg) msgType()             {}
func (KeyMsg) msgType()              {}
func (MouseMsg) msgType()            {}
func (AIRequestMsg) msgType()        {}
func (AIResponseMsg) msgType()       {}
func (ItemStartedMsg) msgType()      {}
func (ItemDeltaMsg) msgType()        {}
func (ItemCompletedMsg) msgType()    {}
func (InvokeDoneMsg) msgType()       {}
func (PredictionUpdateMsg) msgType() {}
func (ThinkingMsg) msgType()         {}
func (ToolCallMsg) msgType()         {}
func (ToolResultMsg) msgType()       {}
func (AgentTaskMsg) msgType()        {}
func (AgentFinalMsg) msgType()       {}
func (ModeChangedMsg) msgType()      {}
func (PromptRequestMsg) msgType()    {}
func (PromptResultMsg) msgType()     {}
func (PanelOpenMsg) msgType()        {}
func (PanelCloseMsg) msgType()       {}
func (PanelActionMsg) msgType()      {}
func (SettingsUpdateMsg) msgType()   {}
func (VersionCheckMsg) msgType()     {}
func (ErrorMsg) msgType()            {}
