package webbridge

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/sandbox"
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

func (s *BridgeService) setSandboxModeRPC(mode string) error {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return errors.New("runtime core unavailable")
	}
	mode = NormalizeSandboxMode(mode)
	return coreOnlyErr(
		gateway,
		func(g bridgeRuntimeGateway) error { return g.CoreSetSandboxModeRPC(coreCtx(), mode) },
	)
}

// enterFullAccessRPC 委托内核 permission/enter_full_access：原子推进
// approval=never + sandbox=danger_full_access。壳层不再自己拼双轴。
func (s *BridgeService) enterFullAccessRPC(workspace string) error {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return errors.New("runtime core unavailable")
	}
	resolvedWorkspace := firstNonEmptyString(strings.TrimSpace(workspace), defaultWorkspacePathFromEnvironment())
	return coreOnlyErr(
		gateway,
		func(g bridgeRuntimeGateway) error {
			_, err := g.CoreEnterFullAccessRPC(coreCtx(), resolvedWorkspace)
			return err
		},
	)
}

// applySandboxModeSemantics 是壳层所有沙箱写入路径（composer 下拉 / 设置保存 /
// 会话与工作区恢复）的单一入口，维护「完全访问」复合不变式：
//   - danger-full-access：走内核 permission/enter_full_access 原子推进 approval=never +
//     danger policy，并立即收口本地待审卡片——单推沙箱轴会让审批仍处 on-request，
//     完全访问态下继续弹审批卡（用户可感知的 bug）。
//   - 其余（read-only / workspace-write）：推沙箱轴 + 把审批轴复位内核默认
//     on-request——进入完全访问时审批被置 never，离开时若不复位，中高风险工具
//     会被 Never→Deny 静默拒绝。
//
// 调用方不得持有 stateMu（danger-full-access 分支收口待审卡需要拿锁）。
func (s *BridgeService) applySandboxModeSemantics(workspace, mode string) error {
	mode = NormalizeSandboxMode(mode)
	if mode == "danger-full-access" {
		if err := s.enterFullAccessRPC(workspace); err != nil {
			return err
		}
		s.stateMu.Lock()
		s.syncApprovedPromptsAfterFullAccessLocked()
		s.stateMu.Unlock()
		return nil
	}
	if err := s.setSandboxModeRPC(mode); err != nil {
		return err
	}
	if err := s.applySandboxPolicyRPC(workspace, mode); err != nil {
		return err
	}
	return s.setApprovalModeRPC("on-request")
}

// 注：壳层 normalizeApprovalMode 已删除——内核 ApprovalMode::parse（eos-core-tools）
// 是单一真相源。壳层写入侧直接透传用户输入（让内核拒绝 yolo/always-allow 等危险
// 别名），读取侧只 trim 空白（内核 serde 只输出标准 kebab-case）。AGENTS.md §3。

// setApprovalModeRPC 把审批模式推进内核（permission/approval_mode/set），
// 立即影响后续工具调用的审批裁决。这是前端"本次会话全部允许"开关的
// 内核侧同步路径——always-allow 模式下，非只读工具直接 Allow，dangerous 仍拦。
//
// 不在壳层归一化：内核 ApprovalMode::parse 是单一真相源（untrusted/on-request/
// never + 受控历史别名 on-failure/unless-trusted），yolo/always-allow 等危险别名
// 会被内核拒绝。壳层只透传用户输入，让内核报错暴露问题（AGENTS.md §3 + 原则 1）。
func (s *BridgeService) setApprovalModeRPC(mode string) error {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return err
	}
	return coreOnlyErr(
		gateway,
		func(g bridgeRuntimeGateway) error { return g.CoreSetApprovalModeRPC(coreCtx(), mode) },
	)
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

// applySandboxPolicyRPC 把沙箱策略同步进内核真正用于裁决的 SandboxPolicy。
//
// 壳层不再手组装 Policy（含 AllowNetwork 这种 mode-scoped 派生）——改调
// sandbox/derive_policy，让内核按单一真相源派生完整 Policy（workspace-write
// 自动放行网络等规则都在内核 SandboxPolicy::derive 里）。壳层只传 mode +
// workspace_root（AGENTS.md §3：壳层不做业务裁决）。
//
// 历史背景：runtime/sandbox_mode/set 只更新显示快照不碰裁决 Policy，所以这里
// 必须走 sandbox/derive_policy + sandbox/set_policy 两步，让裁决路径真正生效。
func (s *BridgeService) applySandboxPolicyRPC(workspace, uiMode string) error {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return errors.New("runtime core unavailable")
	}
	// workspace_root 必须非空：内核 workspace-write 模式下空 root 会拒绝所有写。
	// 回退到默认工作区，与 spawn 时 (bridge_runtime_gateway_options.go) 的回退一致。
	resolvedWorkspace := firstNonEmptyString(strings.TrimSpace(workspace), defaultWorkspacePathFromEnvironment())
	// 内核派生 Policy（含 AllowNetwork 等 mode-scoped 默认值）。
	policy, err := gateway.CoreDeriveSandboxPolicyRPC(coreCtx(), string(sandbox.NormalizeMode(uiMode)), resolvedWorkspace)
	if err != nil {
		return fmt.Errorf("derive sandbox policy: %w", err)
	}
	if err := gateway.CoreSetSandboxPolicyRPC(coreCtx(), "", policy); err != nil {
		return fmt.Errorf("set sandbox policy: %w", err)
	}
	return nil
}

// syncApprovedPromptsAfterFullAccessLocked 把「进入完全访问」完整投影到前端。
// 内核 permission/enter_full_access 会 accept pending 表里的全部**审批**条目
// （request_user_input 问询卡除外——问计划问题不是权限审批，完全访问不能代替
// 用户作答，内核与壳层同语义），壳层必须把同一状态转移落到消息 items——审批
// 浮层横幅从 item.Approval.State 投影（workbench-approvals-logic.ts），不从
// s.prompts 投影。
//
// 两步收口，缺一不可：
//  1. 按 s.prompts 反向索引走 settlePromptLocked 权威路径（删 prompt + 状态行收尾 +
//     item 翻转 + delta）。链接健全时这步已完整。
//  2. 兜底扫全部会话消息里仍 pending 的审批 item（真相源）直接翻转 + delta。
//     prompt 被其它路径提前删除（停止 fallback / 回滚 / 会话加载清理历史上都是只删
//     prompt 不翻 item）或 prompt.CallID 链接失配时，第 1 步够不到 item，横幅永久
//     卡在「等待确认」且点按钮因 prompt 已删而幂等空转——第 2 步保证壳层投影与
//     内核「全部放行」语义严格对齐。
//
// 必须在 stateMu 锁内调用（settlePromptLocked/sweep 读写 s.prompts/s.sessions 并
// emit delta）。
func (s *BridgeService) syncApprovedPromptsAfterFullAccessLocked() {
	var pending []*promptState
	for _, prompt := range s.prompts {
		if prompt == nil || strings.TrimSpace(prompt.Kind) != "approval" {
			continue
		}
		pending = append(pending, prompt)
	}
	text, level := s.resolutionStatusTextAndLevel("tool.approval_approved")
	for _, prompt := range pending {
		// 复用 settlePromptLocked 权威收口路径：删除 prompt、用确定性 statusKey
		// 精确定位并翻转等待行、emit status delta、翻转 item.Approval.State、
		// emit item 快照 delta。session 传 nil，内部按 prompt.SessionID 补全。
		s.settlePromptLocked(nil, prompt, text, level, "streaming")
	}
	s.sweepPendingApprovalItemsLocked()
}

// sweepPendingApprovalItemsLocked 翻转消息 items 里仍 pending 的审批态并 delta
// 推送（含状态行收尾）。这是 item 真相源层面的收口：不依赖 s.prompts 反向索引与
// CallID 链接是否健全，任何路径漏翻的 pending 审批卡在这里与内核状态对齐。
// request_user_input 问询卡不在收口范围（完全访问不回答计划问题）。
// 必须在 stateMu 锁内调用。
func (s *BridgeService) sweepPendingApprovalItemsLocked() {
	for _, session := range s.sessions {
		if session == nil {
			continue
		}
		for i := range session.Messages {
			msg := &session.Messages[i]
			for j := range msg.Items {
				item := &msg.Items[j]
				approval := item.Approval
				if approval == nil || strings.TrimSpace(approval.Kind) == "request_user_input" {
					continue
				}
				if !strings.EqualFold(strings.TrimSpace(approval.State), "pending") {
					continue
				}
				approval.State = "approved"
				approval.ResolvedAt = time.Now().Format(time.RFC3339)
				statusKey := promptStatusKey(approval.ApprovalID)
				s.setMessageStatusWithItemStateKey(session, msg.ID, statusKey, s.t("approval.resolved.allowed"), "success", "streaming", "completed")
				s.emitPromptStatusDeltaLocked(session, msg.ID, statusKey)
				s.emitConversationDelta(ConversationDeltaPayload{
					SessionID: session.ID,
					MessageID: msg.ID,
					ItemID:    item.ID,
					Kind:      item.Kind,
					Status:    item.Status,
					Item:      item,
				})
			}
		}
	}
}

func (s *BridgeService) setReasoningLevelRPC(level string) error {
	level = strings.TrimSpace(level)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreSetReasoningLevelRPC(coreCtx(), level) },
	)
}
