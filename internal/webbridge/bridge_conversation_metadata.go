package webbridge

import (
	"encoding/json"
	"fmt"
	"strings"
)

func metadataChangeSet(value any) *MessageChangeSet {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out MessageChangeSet
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	out.Files = nonNilSlice(out.Files)
	if strings.TrimSpace(out.ID) == "" || len(out.Files) == 0 {
		return nil
	}
	return &out
}

func metadataTurnRollback(value any) *TurnRollback {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out TurnRollback
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	out.Files = nonNilSlice(out.Files)
	if strings.TrimSpace(out.AssistantMessageID) == "" && strings.TrimSpace(out.UserMessageID) == "" && len(out.Files) == 0 {
		return nil
	}
	return &out
}

func metadataMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func metadataString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func metadataStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return compactStrings(append([]string(nil), typed...))
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := metadataString(item); text != "" {
				out = append(out, text)
			}
		}
		return compactStrings(out)
	default:
		return nil
	}
}

func metadataRuntimeEvents(value any) []RuntimeEvent {
	switch typed := value.(type) {
	case []RuntimeEvent:
		return append([]RuntimeEvent(nil), typed...)
	case []map[string]any:
		out := make([]RuntimeEvent, 0, len(typed))
		for _, item := range typed {
			out = append(out, runtimeEventFromMetadata(item))
		}
		return out
	case []any:
		out := make([]RuntimeEvent, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := metadataMap(item); ok {
				out = append(out, runtimeEventFromMetadata(mapped))
			}
		}
		return out
	default:
		return nil
	}
}

func runtimeEventFromMetadata(metadata map[string]any) RuntimeEvent {
	return RuntimeEvent{
		ID:         metadataString(metadata["id"]),
		Type:       metadataString(metadata["type"]),
		Title:      metadataString(metadata["title"]),
		Detail:     metadataString(metadata["detail"]),
		Status:     metadataString(metadata["status"]),
		Timestamp:  metadataString(metadata["timestamp"]),
		DurationMS: metadataInt64(metadata["durationMs"]),
	}
}

func metadataInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case json.Number:
		n, _ := typed.Int64()
		return n
	default:
		return 0
	}
}

// metadataBreakdown 解析内核 turn.token_usage 里的 context_breakdown 对象
// （上下文构成占比）。字段缺失/类型不符按 0 计；旧内核无此字段时返回 nil。
func metadataBreakdown(value any) *ContextBreakdown {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return &ContextBreakdown{
		Messages:     metadataInt64(obj["messages"]),
		SystemTools:  metadataInt64(obj["system_tools"]),
		McpTools:     metadataInt64(obj["mcp_tools"]),
		Skills:       metadataInt64(obj["skills"]),
		SystemPrompt: metadataInt64(obj["system_prompt"]),
		Other:        metadataInt64(obj["other"]),
	}
}

func runtimeStringValue(payload map[string]any, keys ...string) string {
	if payload == nil {
		return ""
	}
	for _, key := range keys {
		raw, ok := payload[key]
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

func runtimeNestedStringValue(payload map[string]any, keys ...string) string {
	if value := runtimeStringValue(payload, keys...); value != "" {
		return value
	}
	result, ok := metadataMap(payload["result"])
	if !ok {
		return ""
	}
	return runtimeStringValue(result, keys...)
}

func runtimeNestedStringSliceValue(payload map[string]any, keys ...string) []string {
	if payload == nil {
		return nil
	}
	for _, key := range keys {
		if values := metadataStringSlice(payload[key]); len(values) > 0 {
			return values
		}
	}
	result, ok := metadataMap(payload["result"])
	if !ok {
		return nil
	}
	for _, key := range keys {
		if values := metadataStringSlice(result[key]); len(values) > 0 {
			return values
		}
	}
	return nil
}
