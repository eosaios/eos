package webbridge

import (
	"context"
	"log/slog"
	"strings"
)

func coreCtx() context.Context {
	return context.Background()
}

func (s *BridgeService) warnCoreRPCWriteFailure(domain, workspace, sessionID string, err error) {
	if err == nil {
		return
	}
	args := []any{"domain", strings.TrimSpace(domain), "error", err}
	if workspace = strings.TrimSpace(workspace); workspace != "" {
		args = append(args, "workspace", workspace)
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		args = append(args, "session_id", sessionID)
	}
	slog.Warn("bridge.core_rpc.write_failed", args...)
}

func (s *BridgeService) tryCoreRPC(domain, workspace, sessionID string, rpc func() error) {
	if rpc == nil {
		return
	}
	if err := rpc(); err != nil {
		s.warnCoreRPCWriteFailure(domain, workspace, sessionID, err)
	}
}
