package webbridge

import (
	"context"
	"sync"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/sandbox"
)

// sandboxModeGatewayStub 是一个最小 gateway stub，用于隔离测试沙箱模式的
// per-session 恢复与持久化逻辑。它只实现 syncSandboxModeForSession /
// persistSessionSandboxMode / applySandboxModeSemantics 调用链涉及的方法。
type sandboxModeGatewayStub struct {
	bridgeRuntimeGateway
	mu sync.Mutex

	// sessions 返回给 CoreListSessionsRPC（含 sandbox_mode metadata）。
	sessions []coreapi.Session
	// appliedSandboxMode 捕获 setSandboxModeRPC 推进内核的 mode。
	appliedSandboxMode string
	// persistedSandboxMode 捕获 CoreSetSessionSandboxModeRPC 写入的 (sessionID, mode)。
	persistedSandboxMode map[string]string
	// enterFullAccessCalls 捕获 CoreEnterFullAccessRPC 调用次数（完全访问复合入口）。
	enterFullAccessCalls int
	// derivedPolicyModes 捕获 sandbox/derive_policy + set_policy 推进的策略模式序列。
	derivedPolicyModes []string
	// approvalModeSets 捕获 CoreSetApprovalModeRPC 推进的审批轴值序列。
	approvalModeSets []string
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

func (g *sandboxModeGatewayStub) CoreSetSandboxModeRPC(_ context.Context, mode string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.appliedSandboxMode = mode
	return nil
}

func (g *sandboxModeGatewayStub) CoreDeriveSandboxPolicyRPC(_ context.Context, mode, _ string) (sandbox.Policy, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.derivedPolicyModes = append(g.derivedPolicyModes, mode)
	return sandbox.Policy{Mode: sandbox.NormalizeMode(mode)}, nil
}

func (g *sandboxModeGatewayStub) CoreSetSandboxPolicyRPC(context.Context, string, sandbox.Policy) error {
	return nil
}

func (g *sandboxModeGatewayStub) CoreSetSessionSandboxModeRPC(_ context.Context, sessionID, mode string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.persistedSandboxMode == nil {
		g.persistedSandboxMode = map[string]string{}
	}
	g.persistedSandboxMode[sessionID] = mode
	return nil
}

func (g *sandboxModeGatewayStub) CoreEnterFullAccessRPC(_ context.Context, _ string) (sandbox.Policy, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enterFullAccessCalls++
	return sandbox.Policy{}, nil
}

func (g *sandboxModeGatewayStub) CoreSetApprovalModeRPC(_ context.Context, mode string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.approvalModeSets = append(g.approvalModeSets, mode)
	return nil
}

// TestSyncSandboxModeForSessionPrefersSessionMetadata 验证恢复时优先用会话 metadata
// 记录的 sandbox_mode（per-session 持久化），而非全局默认。metadata 里可能是
// 历史词表值（full_access），读取侧归一化为内核 kebab-case 规范值。
func TestSyncSandboxModeForSessionPrefersSessionMetadata(t *testing.T) {
	gateway := &sandboxModeGatewayStub{
		sessions: []coreapi.Session{
			{ID: "sess-full", Metadata: map[string]any{"sandbox_mode": "full_access"}},
		},
	}
	s := &BridgeService{runtimeGateway: gateway}

	got := s.syncSandboxModeForSession("", "sess-full")
	if got != "danger-full-access" {
		t.Fatalf("syncSandboxModeForSession = %q, want danger-full-access (legacy full_access normalized)", got)
	}
	// danger-full-access 恢复必须走内核复合入口（approval=never + danger policy 原子推进），
	// 不再是壳层单推沙箱轴——否则完全访问态下审批卡继续弹（回归）。
	if gateway.enterFullAccessCalls != 1 {
		t.Errorf("enterFullAccessCalls = %d, want 1 (compound kernel entry)", gateway.enterFullAccessCalls)
	}
}

// TestApplySandboxModeSemanticsFullAccessSettlesPrompts 验证进入完全访问的复合
// 语义：走内核 enter_full_access，并立即收口本地待审 prompt。
func TestApplySandboxModeSemanticsFullAccessSettlesPrompts(t *testing.T) {
	gateway := &sandboxModeGatewayStub{}
	s := &BridgeService{
		runtimeGateway: gateway,
		prompts:        map[string]*promptState{},
		sessions:       map[string]*sessionState{},
		emitEvent:      func(string, any) {},
	}
	s.prompts["approval_1"] = &promptState{
		PromptCard:         PromptCard{ID: "approval_1", Kind: "approval", SessionID: "session-1"},
		AssistantMessageID: "msg-1",
	}

	if err := s.applySandboxModeSemantics("c:/ws", "danger-full-access"); err != nil {
		t.Fatalf("applySandboxModeSemantics(danger-full-access) error = %v", err)
	}
	if gateway.enterFullAccessCalls != 1 {
		t.Errorf("enterFullAccessCalls = %d, want 1", gateway.enterFullAccessCalls)
	}
	// 完全访问由内核 enter_full_access 原子推进双轴，壳层不再单推沙箱/审批轴。
	if gateway.appliedSandboxMode != "" {
		t.Errorf("appliedSandboxMode = %q, want empty (kernel compound entry owns both axes)", gateway.appliedSandboxMode)
	}
	if len(gateway.approvalModeSets) != 0 {
		t.Errorf("approvalModeSets = %v, want empty (kernel compound entry owns both axes)", gateway.approvalModeSets)
	}
	if len(s.prompts) != 0 {
		t.Fatalf("prompts = %d items, want 0 (settled after full access)", len(s.prompts))
	}
}

// TestApplySandboxModeSemanticsResetsApprovalOnExit 验证离开完全访问（切回
// workspace-write，含历史值 workspace 输入）时审批轴复位 on-request——否则中高
// 风险工具会被 Never→Deny 静默拒绝。
func TestApplySandboxModeSemanticsResetsApprovalOnExit(t *testing.T) {
	gateway := &sandboxModeGatewayStub{}
	s := &BridgeService{runtimeGateway: gateway}

	if err := s.applySandboxModeSemantics("c:/ws", "workspace"); err != nil {
		t.Fatalf("applySandboxModeSemantics(workspace) error = %v", err)
	}
	if gateway.appliedSandboxMode != "workspace-write" {
		t.Errorf("appliedSandboxMode = %q, want workspace-write (legacy workspace normalized)", gateway.appliedSandboxMode)
	}
	if len(gateway.approvalModeSets) != 1 || gateway.approvalModeSets[0] != "on-request" {
		t.Errorf("approvalModeSets = %v, want [on-request] (approval axis reset on exit)", gateway.approvalModeSets)
	}
	if gateway.enterFullAccessCalls != 0 {
		t.Errorf("enterFullAccessCalls = %d, want 0", gateway.enterFullAccessCalls)
	}
}

// TestApplySandboxModeSemanticsReadOnlyDerivesReadOnlyPolicy 验证只读档：推快照 +
// 内核派生 read-only 策略 + 审批复位 on-request，与 workspace-write 一样走
// derive_policy/set_policy 真实裁决路径。
func TestApplySandboxModeSemanticsReadOnlyDerivesReadOnlyPolicy(t *testing.T) {
	gateway := &sandboxModeGatewayStub{}
	s := &BridgeService{runtimeGateway: gateway}

	if err := s.applySandboxModeSemantics("c:/ws", "read-only"); err != nil {
		t.Fatalf("applySandboxModeSemantics(read-only) error = %v", err)
	}
	if gateway.appliedSandboxMode != "read-only" {
		t.Errorf("appliedSandboxMode = %q, want read-only", gateway.appliedSandboxMode)
	}
	if len(gateway.derivedPolicyModes) != 1 || gateway.derivedPolicyModes[0] != "read-only" {
		t.Errorf("derivedPolicyModes = %v, want [read-only] (enforcement policy derived from kernel)", gateway.derivedPolicyModes)
	}
	if len(gateway.approvalModeSets) != 1 || gateway.approvalModeSets[0] != "on-request" {
		t.Errorf("approvalModeSets = %v, want [on-request]", gateway.approvalModeSets)
	}
	if gateway.enterFullAccessCalls != 0 {
		t.Errorf("enterFullAccessCalls = %d, want 0", gateway.enterFullAccessCalls)
	}
}

// TestSyncSandboxModeForSessionFallsBackToDefaultWhenNoMetadata 验证会话无
// sandbox_mode 记录（新会话）时回落到默认 workspace-write。
func TestSyncSandboxModeForSessionFallsBackToDefaultWhenNoMetadata(t *testing.T) {
	gateway := &sandboxModeGatewayStub{
		sessions: []coreapi.Session{
			{ID: "sess-new", Metadata: map[string]any{"title": "New Chat"}},
		},
	}
	s := &BridgeService{runtimeGateway: gateway}

	got := s.syncSandboxModeForSession("", "sess-new")
	if got != "workspace-write" {
		t.Fatalf("syncSandboxModeForSession = %q, want workspace-write (default)", got)
	}
	if gateway.appliedSandboxMode != "workspace-write" {
		t.Errorf("appliedSandboxMode = %q, want workspace-write", gateway.appliedSandboxMode)
	}
}

// TestSyncSandboxModeForSessionFallsBackToDefaultWhenNoSession 验证 sessionID 为空
// （会话尚未建立）时回落到默认 workspace-write。
func TestSyncSandboxModeForSessionFallsBackToDefaultWhenNoSession(t *testing.T) {
	gateway := &sandboxModeGatewayStub{}
	s := &BridgeService{runtimeGateway: gateway}

	got := s.syncSandboxModeForSession("", "")
	if got != "workspace-write" {
		t.Fatalf("syncSandboxModeForSession = %q, want workspace-write (default)", got)
	}
}

// TestPersistSessionSandboxModeWritesMetadata 验证 persistSessionSandboxMode 把
// 模式写入内核 session metadata。
func TestPersistSessionSandboxModeWritesMetadata(t *testing.T) {
	gateway := &sandboxModeGatewayStub{}
	s := &BridgeService{runtimeGateway: gateway}

	s.persistSessionSandboxMode("sess-1", "danger-full-access")
	if got := gateway.persistedSandboxMode["sess-1"]; got != "danger-full-access" {
		t.Fatalf("persistedSandboxMode[sess-1] = %q, want danger-full-access", got)
	}
}

// TestPersistSessionSandboxModeSkipsWhenNoSession 验证 sessionID 为空时跳过持久化
// （会话尚未建立时是正常态，仅运行时生效）。
func TestPersistSessionSandboxModeSkipsWhenNoSession(t *testing.T) {
	gateway := &sandboxModeGatewayStub{}
	s := &BridgeService{runtimeGateway: gateway}

	s.persistSessionSandboxMode("", "danger-full-access")
	if len(gateway.persistedSandboxMode) != 0 {
		t.Fatalf("persistedSandboxMode = %v, want empty (no session)", gateway.persistedSandboxMode)
	}
}
