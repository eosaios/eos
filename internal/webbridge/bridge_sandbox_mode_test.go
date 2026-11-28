package webbridge

import (
	"context"
	"sync"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// sandboxModeGatewayStub 是一个最小 gateway stub，用于隔离测试沙箱模式的
// per-session 恢复与持久化逻辑。它只实现 syncSandboxModeForSession /
// persistSessionSandboxMode / applySandboxModeSemantics 调用链涉及的方法。
type sandboxModeGatewayStub struct {
	bridgeRuntimeGateway
	mu           sync.Mutex
	snapshotMode string
	requests     []coreapi.PermissionPresetRequest

	// sessions 返回给 CoreListSessionsRPC（含 sandbox_mode metadata）。
	sessions []coreapi.Session
}

func (g *sandboxModeGatewayStub) CoreConfigPath() string { return "" }

func (g *sandboxModeGatewayStub) CoreDefaultWorkspaceRPC(context.Context) (string, error) {
	return "", nil
}

func (g *sandboxModeGatewayStub) ResolveSessionWorkspace(string) (string, error) {
	return "", nil
}

func (g *sandboxModeGatewayStub) CoreRuntimeSnapshotRPC(context.Context) (adapter.RuntimeSnapshot, error) {
	return adapter.RuntimeSnapshot{}, nil
}

func (g *sandboxModeGatewayStub) CoreListSessionsRPC(_ context.Context, _ string) ([]coreapi.Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]coreapi.Session, len(g.sessions))
	copy(out, g.sessions)
	return out, nil
}

func (g *sandboxModeGatewayStub) CoreGetSettingsRPC(context.Context) (adapter.GUISettings, error) {
	return adapter.GUISettings{}, nil
}

func (g *sandboxModeGatewayStub) CorePermissionSessionSnapshotRPC(_ context.Context, id string) (adapter.PermissionSnapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return adapter.PermissionSnapshot{SandboxMode: g.snapshotMode}, nil
}
func (g *sandboxModeGatewayStub) CoreSetPermissionPresetRPC(_ context.Context, req coreapi.PermissionPresetRequest) (adapter.PermissionSnapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, req)
	approval := "never"
	if req.Mode == "workspace-write" {
		approval = "on-request"
	}
	return adapter.PermissionSnapshot{SandboxMode: req.Mode, ApprovalMode: approval, AllowAll: req.Mode == "danger-full-access"}, nil
}

func TestExistingSessionUsesCorePermissionMigration(t *testing.T) {
	for _, tc := range []struct {
		saved  map[string]any
		actual string
	}{
		{map[string]any{"sandbox_mode": "full_access"}, "workspace-write"},
		{map[string]any{"tool_permission_scope": map[string]any{}}, "read-only"},
	} {
		gw := &sandboxModeGatewayStub{sessions: []coreapi.Session{{ID: "s-1", Metadata: tc.saved}}, snapshotMode: tc.actual}
		s := &BridgeService{runtimeGateway: gw}
		if got := s.syncSandboxModeForSession("/ws", "s-1"); got != tc.actual || len(gw.requests) != 0 {
			t.Fatalf("attempted to overwrite Core's migrated state: %q, %v", got, gw.requests)
		}
	}
}

func TestPermissionPresetWritesOneScopedTransaction(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"workspace", "workspace-write"}, {"full_access", "danger-full-access"}, {"read-only", "read-only"}} {
		gw := &sandboxModeGatewayStub{}
		s := &BridgeService{runtimeGateway: gw, currentSessionID: "sess-42"}
		if err := s.applySandboxModeSemantics("/ws", tc.input); err != nil {
			t.Fatal(err)
		}
		if len(gw.requests) != 1 || gw.requests[0] != (coreapi.PermissionPresetRequest{SessionID: "sess-42", WorkspaceRoot: "/ws", Mode: tc.want}) {
			t.Fatalf("unexpected permission calls %v", gw.requests)
		}
	}
}

func TestNewSessionPermissionIsScopedAndUnknownSessionCannotEscalate(t *testing.T) {
	gw := &sandboxModeGatewayStub{sessions: []coreapi.Session{{ID: "new"}}}
	s := &BridgeService{runtimeGateway: gw}
	if mode := s.syncSandboxModeForSession("/ws", "new"); mode != "workspace-write" {
		t.Fatal(mode)
	}
	if mode := s.syncSandboxModeForSession("/ws", "missing"); mode != "unknown" {
		t.Fatal(mode)
	}
	if err := s.applySandboxModeForSession("/ws", "danger-full-access", ""); err == nil {
		t.Fatal("unscoped mode change succeeded")
	}
	if len(gw.requests) != 1 || gw.requests[0].SessionID != "new" {
		t.Fatal(gw.requests)
	}
}
