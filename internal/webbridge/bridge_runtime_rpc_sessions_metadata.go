package webbridge

import (
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

func sessionMetasFromCoreAPI(items []coreapi.Session, fallbackWorkspace string) []adapter.SessionMeta {
	out := make([]adapter.SessionMeta, 0, len(items))
	for _, item := range items {
		meta := adapter.SessionMeta{
			ID:          strings.TrimSpace(item.ID),
			SavedAt:     firstNonZeroTime(item.UpdatedAt, item.CreatedAt),
			Model:       rpcMetadataString(item.Metadata, "model"),
			Summary:     rpcMetadataString(item.Metadata, "summary"),
			Preview:     rpcMetadataString(item.Metadata, "preview"),
			Title:       rpcMetadataString(item.Metadata, "title"),
			Rounds:      rpcMetadataInt(item.Metadata, "rounds"),
			Tokens:      rpcMetadataInt(item.Metadata, "tokens"),
			SandboxMode: rpcMetadataString(item.Metadata, "sandbox_mode"),
		}
		if meta.ID == "" {
			continue
		}
		if meta.Title == "" {
			meta.Title = meta.ID
		}
		if strings.TrimSpace(item.WorkspaceRoot) == "" {
			item.WorkspaceRoot = strings.TrimSpace(fallbackWorkspace)
		}
		out = append(out, meta)
	}
	return out
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func rpcMetadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	return metadataString(metadata[key])
}

func rpcMetadataInt(metadata map[string]any, key string) int {
	if len(metadata) == 0 {
		return 0
	}
	return int(metadataInt64(metadata[key]))
}
