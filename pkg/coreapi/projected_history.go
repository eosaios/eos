package coreapi

import (
	"context"
	"fmt"
)

// ToolResultPresentationKey marks a shell's display projection. Content for
// these messages is owned by the core; the display text must not replace it.
const ToolResultPresentationKey = "eos_gui_tool_result"

type SessionMessageLoader func(context.Context, LoadSessionMessagesRequest) ([]SessionMessage, error)

// PreserveProjectedToolReceipts keeps opaque model observations and image
// attachments from the same session/turn/call while saving shell metadata.
// Unmarked messages retain the ordinary session/messages/save contract.
func PreserveProjectedToolReceipts(ctx context.Context, req SaveSessionMessagesRequest, load SessionMessageLoader) (SaveSessionMessagesRequest, error) {
	projected := false
	for _, msg := range req.Messages {
		if isProjectedToolReceipt(msg) {
			projected = true
			break
		}
	}
	if !projected {
		return req, nil
	}
	canonical, err := load(ctx, LoadSessionMessagesRequest{SessionID: req.SessionID, WorkspaceRoot: req.WorkspaceRoot})
	if err != nil {
		return SaveSessionMessagesRequest{}, fmt.Errorf("load canonical tool receipts: %w", err)
	}
	type receiptKey struct{ turn, call string }
	key := func(msg SessionMessage) receiptKey {
		turn, _ := msg.Metadata["turn_id"].(string)
		call, _ := msg.Metadata["tool_call_id"].(string)
		return receiptKey{turn, call}
	}
	receipts := make(map[receiptKey]SessionMessage)
	for _, msg := range canonical {
		if msg.Role != "tool" {
			continue
		}
		k := key(msg)
		if k.call == "" {
			continue
		}
		if _, exists := receipts[k]; exists {
			return SaveSessionMessagesRequest{}, fmt.Errorf("ambiguous canonical tool receipt for turn %q call %q", k.turn, k.call)
		}
		receipts[k] = msg
	}
	// Do not partially mutate the caller's messages when a later receipt fails.
	result := req
	result.Messages = append([]SessionMessage(nil), req.Messages...)
	for i, msg := range result.Messages {
		if !isProjectedToolReceipt(msg) {
			continue
		}
		k := key(msg)
		original, ok := receipts[k]
		if k.call == "" || !ok {
			return SaveSessionMessagesRequest{}, fmt.Errorf("canonical tool receipt unavailable for turn %q call %q", k.turn, k.call)
		}
		result.Messages[i].Content = original.Content
		result.Messages[i].ImagePaths = append([]string(nil), original.ImagePaths...)
	}
	return result, nil
}

func isProjectedToolReceipt(msg SessionMessage) bool {
	_, marked := msg.Metadata[ToolResultPresentationKey]
	return msg.Role == "tool" && marked
}
