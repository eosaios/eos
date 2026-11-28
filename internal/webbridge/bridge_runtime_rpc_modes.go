package webbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// 运行模式 RPC：执行模式 / 沙箱模式 / 推理等级的只读快照与写入。
// 这三者共享 CoreModeSnapshotRPC，独立于 workspace 写操作。

func (s *BridgeService) executionModeReadOnly() string {
	snapshot := coreValueOrNil(
		s,
		coreapi.ModeSnapshot{},
		func(g bridgeRuntimeGateway) (coreapi.ModeSnapshot, error) {
			return g.CoreModeSnapshotRPC(coreCtx())
		},
	)
	return normalizeExecutionMode(snapshot.ExecutionMode)
}

func (s *BridgeService) sandboxModeReadOnly() string {
	snapshot := coreValueOrNil(
		s,
		coreapi.ModeSnapshot{},
		func(g bridgeRuntimeGateway) (coreapi.ModeSnapshot, error) {
			return g.CoreModeSnapshotRPC(coreCtx())
		},
	)
	return NormalizeSandboxMode(snapshot.SandboxMode)
}

func (s *BridgeService) reasoningLevelReadOnly() string {
	snapshot := coreValueOrNil(
		s,
		coreapi.ModeSnapshot{},
		func(g bridgeRuntimeGateway) (coreapi.ModeSnapshot, error) {
			return g.CoreModeSnapshotRPC(coreCtx())
		},
	)
	return strings.TrimSpace(snapshot.ReasoningLevel)
}

func (s *BridgeService) setExecutionModeRPC(mode string) {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return
	}
	mode = normalizeExecutionMode(mode)
	_ = coreOnlyErr(
		gateway,
		func(g bridgeRuntimeGateway) error { return g.CoreSetExecutionModeRPC(coreCtx(), mode) },
	)
}

// The same session-scoped transaction changes both enforcement axes for every preset.
func (s *BridgeService) applySandboxModeSemantics(workspace, mode string) error {
	return s.applySandboxModeForSession(workspace, mode, s.currentSessionValue())
}
func (s *BridgeService) applySandboxModeForSession(workspace, mode, sessionID string) error {
	gateway, ok := runtimeGatewayOrNil(s).(interface {
		CoreSetPermissionPresetRPC(context.Context, coreapi.PermissionPresetRequest) (adapter.PermissionSnapshot, error)
	})
	if !ok {
		return errors.New("Core does not support scoped permission presets")
	}
	req := coreapi.PermissionPresetRequest{SessionID: strings.TrimSpace(sessionID), WorkspaceRoot: firstNonEmptyString(strings.TrimSpace(workspace), defaultWorkspacePathFromEnvironment()), Mode: parseSandboxMode(mode)}
	if req.Mode == "" {
		return fmt.Errorf("unknown permission preset %q", mode)
	}
	if req.SessionID == "" {
		return errors.New("select a conversation before changing its permissions")
	}
	actual, err := gateway.CoreSetPermissionPresetRPC(coreCtx(), req)
	if err != nil {
		return err
	}
	expectedApproval := "never"
	if req.Mode == "workspace-write" {
		expectedApproval = "on-request"
	}
	if actual.SandboxMode != req.Mode || actual.ApprovalMode != expectedApproval || actual.AllowAll != (req.Mode == "danger-full-access") {
		return fmt.Errorf("Core confirmed permission %s/%s (allow_all=%t), requested %s/%s", actual.SandboxMode, actual.ApprovalMode, actual.AllowAll, req.Mode, expectedApproval)
	}
	return nil
}

// Compatibility entry for explicit advanced-policy edits; keep them session-scoped.
func (s *BridgeService) setApprovalModeRPC(mode string) error {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return err
	}
	id := s.currentSessionValue()
	if id == "" {
		return errors.New("select a conversation before changing its permissions")
	}
	raw, err := json.Marshal(map[string]string{"session_id": id, "mode": mode})
	if err != nil {
		return err
	}
	_, err = gateway.CoreCallRPC(coreCtx(), "permission/approval_mode/set", raw)
	return err
}

// SetApprovalModeForUI 是前端"本次会话全部允许"开关的 Wails 桥接入口。
// 把审批模式推进内核，并返回最新 bootstrap（含更新后的 permission.approvalMode
// 回显）。失败时也返回 bootstrap，前端可据此回滚开关状态。
func (svc *CommandService) SetApprovalModeForUI(mode string) BootstrapState {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}
	}
	if err := s.setApprovalModeRPC(mode); err != nil {
		slog.Warn("bridge.approval_mode.set_failed", "mode", mode, "error", err)
	}
	return s.LoadBootstrap()
}

func (s *BridgeService) setReasoningLevelRPC(level string) error {
	level = strings.TrimSpace(level)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreSetReasoningLevelRPC(coreCtx(), level) },
	)
}
