package webbridge

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func (s *BridgeService) resolveForegroundWorkspaceWithSnapshotAndLast(preferred string, runtimeSnapshot adapter.RuntimeSnapshot, persistedLastWorkspace string) string {
	target := strings.TrimSpace(preferred)
	source := ""
	if target != "" {
		source = "preferred"
	}
	if target == "" {
		target = strings.TrimSpace(persistedLastWorkspace)
		if target != "" {
			source = "last_workspace"
		}
	}
	if strings.TrimSpace(target) == "" {
		target = strings.TrimSpace(runtimeSnapshot.ForegroundWorkspace)
		if target != "" {
			source = "runtime_snapshot"
		}
	}
	target = strings.TrimSpace(target)
	if target == "" {
		var err error
		target, err = s.ensureDefaultWorkspaceReady(WorkspaceActivationForeground)
		if err != nil {
			slog.Warn("bridge.default_workspace.foreground.error", "path", target, "error", err)
		}
		if target != "" {
			source = "default"
		}
	}
	if target == "" {
		return ""
	}
	switch source {
	case "preferred", "last_workspace", "default":
		s.tryCoreRPC("activate-workspace", target, "", func() error {
			return s.activateWorkspaceRPC(target)
		})
	}
	return target
}

func (s *BridgeService) persistedLastWorkspace() string {
	path := strings.TrimSpace(s.coreConfigPathReadOnly())
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var state workspacePersistenceState
	if err := json.Unmarshal(raw, &state); err != nil {
		return ""
	}
	return strings.TrimSpace(state.LastWorkspace)
}

// persistedLastSession 读上次停留的会话 ID（从内核配置文件的 last_session 字段）。
// 与 persistedLastWorkspace 同文件，由 persistWorkspaceAndSession 写入。
func (s *BridgeService) persistedLastSession() string {
	path := strings.TrimSpace(s.coreConfigPathReadOnly())
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var state workspacePersistenceState
	if err := json.Unmarshal(raw, &state); err != nil {
		return ""
	}
	return strings.TrimSpace(state.LastSession)
}

// persistWorkspaceAndSession 把当前工作区 + 会话写入内核配置文件（JSON 追加字段，
// 不碰内核其他字段）。桌面端启动时读回，默认停留到上次的工作区会话。
func (s *BridgeService) persistWorkspaceAndSession(workspace, sessionID string) {
	configPath := strings.TrimSpace(s.coreConfigPathReadOnly())
	if configPath == "" {
		return
	}
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	if workspace == "" && sessionID == "" {
		return
	}
	// 读现有配置（保留内核的字段），只更新 last_workspace/last_session。
	existing := map[string]any{}
	if raw, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(raw, &existing)
	}
	if workspace != "" {
		existing["last_workspace"] = workspace
	}
	if sessionID != "" {
		existing["last_session"] = sessionID
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(configPath, data, 0o644)
}
