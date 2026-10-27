package protocol

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Version string

const (
	VersionV1 Version = "v1"
)

type Source string

const (
	SourceCore   Source = "core"
	SourceCLI    Source = "cli"
	SourceGUI    Source = "gui"
	SourceServe  Source = "serve"
	SourceAgent  Source = "agent"
	SourceTool   Source = "tool"
	SourceSystem Source = "system"
)

type EventType string

const (
	EventTypeItemStarted      EventType = "item.started"
	EventTypeItemDelta        EventType = "item.delta"
	EventTypeItemCompleted    EventType = "item.completed"
	EventTypeTextFinal        EventType = "text.final"
	EventTypeTextReasoning    EventType = "text.reasoning"
	EventTypeApprovalReq      EventType = "approval.required"
	EventTypeApprovalDone     EventType = "approval.resolved"
	EventTypeInquiryReq       EventType = "inquiry.required"
	EventTypeInquiryDone      EventType = "inquiry.resolved"
	EventTypeRequestStarted   EventType = "request.started"
	EventTypeRequestDone      EventType = "request.completed"
	EventTypeRequestFailed    EventType = "request.failed"
	EventTypeSessionUpdated   EventType = "session.updated"
	EventTypeAgentStarted     EventType = "agent.started"
	EventTypeAgentProgress    EventType = "agent.progress"
	EventTypeAgentDone        EventType = "agent.completed"
	EventTypeAgentFailed      EventType = "agent.failed"
	EventTypeAgentCancelled   EventType = "agent.cancelled"
	EventTypeTaskStarted      EventType = "task.started"
	EventTypeTaskUpdated      EventType = "task.updated"
	EventTypeTaskDone         EventType = "task.completed"
	EventTypeTaskFailed       EventType = "task.failed"
	EventTypeLoopBlocked      EventType = "loop.block"
	EventTypeTurnWrapUp       EventType = "turn.wrap_up"
	EventTypeModeChanged      EventType = "mode.changed"
	EventTypeBudgetUpdated    EventType = "budget.updated"
	EventTypeMemorySuggestion EventType = "memory.suggestion"

	EventTypeTurnStarted               EventType = "turn.started"
	EventTypeTurnItemStarted           EventType = "turn.item_started"
	EventTypeTurnItemDelta             EventType = "turn.item_delta"
	EventTypeTurnItemCompleted         EventType = "turn.item_completed"
	EventTypeTurnCompleted             EventType = "turn.completed"
	EventTypeTurnError                 EventType = "turn.error"
	EventTypeTurnCancelled             EventType = "turn.cancelled"
	EventTypeTurnInterrupted           EventType = "turn.interrupted"
	EventTypeTurnWaitingApproval       EventType = "turn.waiting_approval"
	EventTypeTurnPreCompact            EventType = "turn.pre_compact"
	EventTypeTurnMidCompact            EventType = "turn.mid_compact"
	EventTypeTurnModelDownshift        EventType = "turn.model_downshift"
	EventTypeTurnContextWindowExceeded EventType = "turn.context_window_exceeded"
	EventTypeTurnToolLoopExhausted     EventType = "turn.tool_loop_exhausted"
	// EventTypeTurnTokenUsage 内核每步模型响应后广播的累计 usage 快照（Droppable
	// 计量事件），壳层转发给前端实时刷新上下文用量/输出速率。
	EventTypeTurnTokenUsage EventType = "turn.token_usage"
)

type Envelope struct {
	Version       Version        `json:"version"`
	EventID       string         `json:"event_id"`
	EventType     EventType      `json:"event_type"`
	SessionID     string         `json:"session_id,omitempty"`
	ThreadID      string         `json:"thread_id,omitempty"`
	TurnID        string         `json:"turn_id,omitempty"`
	AgentID       string         `json:"agent_id,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
	Source        Source         `json:"source"`
	Payload       map[string]any `json:"payload,omitempty"`
}

type EventOptions struct {
	EventID       string
	SessionID     string
	ThreadID      string
	TurnID        string
	AgentID       string
	RequestID     string
	CorrelationID string
	Timestamp     time.Time
	Source        Source
	Payload       map[string]any
}

func NewEvent(eventType EventType, opts EventOptions) Envelope {
	eventID := strings.TrimSpace(opts.EventID)
	if eventID == "" {
		eventID = "evt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}

	ts := opts.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	src := opts.Source
	if strings.TrimSpace(string(src)) == "" {
		src = SourceCore
	}

	return Envelope{
		Version:       VersionV1,
		EventID:       eventID,
		EventType:     eventType,
		SessionID:     strings.TrimSpace(opts.SessionID),
		ThreadID:      strings.TrimSpace(opts.ThreadID),
		TurnID:        strings.TrimSpace(opts.TurnID),
		AgentID:       strings.TrimSpace(opts.AgentID),
		RequestID:     strings.TrimSpace(opts.RequestID),
		CorrelationID: strings.TrimSpace(opts.CorrelationID),
		Timestamp:     ts,
		Source:        src,
		Payload:       ClonePayload(opts.Payload),
	}
}

func NormalizeEventType(eventType EventType) EventType {
	switch eventType {
	case EventTypeTurnStarted:
		return EventTypeRequestStarted
	case EventTypeTurnItemStarted:
		return EventTypeItemStarted
	case EventTypeTurnItemDelta:
		return EventTypeItemDelta
	case EventTypeTurnItemCompleted:
		return EventTypeItemCompleted
	case EventTypeTurnCompleted:
		return EventTypeRequestDone
	case EventTypeTurnError, EventTypeTurnCancelled, EventTypeTurnInterrupted:
		return EventTypeRequestFailed
	case EventTypeTurnWaitingApproval:
		return EventTypeApprovalReq
	case EventTypeTurnPreCompact, EventTypeTurnMidCompact, EventTypeTurnModelDownshift:
		return EventTypeTextReasoning
	case EventTypeTurnContextWindowExceeded, EventTypeTurnToolLoopExhausted:
		return EventTypeTextReasoning
	default:
		return eventType
	}
}

func ClonePayload(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
