package webbridge

import (
	"encoding/json"
	"github.com/eosaios/eos/pkg/attachments"
	"log/slog"
	"strings"
)

func fileReferenceSessionContent(message ChatMessage) string {
	if message.Role != "user" {
		return message.Content
	}
	var paths []string
	for _, file := range message.Attachments {
		if file.Kind != "image" && !isImageAttachmentPath(file.Path) && strings.TrimSpace(file.Path) != "" {
			paths = append(paths, file.Path)
		}
	}
	return attachments.ReferenceInput(message.Content, paths)
}

func preserveFileReferenceMetadata(gui map[string]any, message ChatMessage) {
	if message.Role == "user" && len(message.Attachments) > 0 {
		gui["fileReferenceContent"] = message.Content
		gui["fileReferences"] = message.Attachments
	}
}

func restoreFileReferenceMetadata(message *ChatMessage, gui map[string]any) {
	if message.Role != "user" {
		return
	}
	content, ok := gui["fileReferenceContent"].(string)
	if !ok {
		return
	}
	encoded, err := json.Marshal(gui["fileReferences"])
	if err != nil {
		slog.Warn("file_references.metadata_encode_failed", "error", err)
		return
	}
	var refs []AttachmentRef
	if err := json.Unmarshal(encoded, &refs); err != nil {
		slog.Warn("file_references.metadata_decode_failed", "error", err)
		return
	}
	message.Content = content
	message.Attachments = refs
}
