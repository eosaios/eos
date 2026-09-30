package webbridge

// 目标模式（goal mode）桥接层：goal/set|get|pause|resume|clear 的 Wails 入口
// + BootstrapState 的 Goal 快照投影。
//
// 语义（对齐内核 GoalEngine）：设定目标后进入 active 并立即触发空闲自驱——
// agent 持续朝目标工作，跨 turn 自动续跑，直到模型 update_goal(complete/
// blocked)、token 预算耗尽（budgetLimited 粘性终态）或用户 pause/clear。
// goal.updated / goal.cleared 事件到达时 emitBootstrap，前端目标指示器随动。

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi"
)

// GoalSnapshot 是 BootstrapState 的目标快照（Goal 为 nil 表示当前会话无目标）。
type GoalSnapshot struct {
	SessionID         string  `json:"sessionId"`
	Objective         string  `json:"objective"`
	Status            string  `json:"status"` // active|paused|blocked|usageLimited|budgetLimited|complete
	TokenBudget       *int64  `json:"tokenBudget,omitempty"`
	TokensUsed        int64   `json:"tokensUsed"`
	TimeUsedSeconds   int64   `json:"timeUsedSeconds"`
	GoalID            string  `json:"goalId"`
	EstimatedProgress float64 `json:"estimatedProgress,omitempty"`
}

// goalSnapshotReadOnly 投影当前会话的目标状态（无 runtime / 无会话 → 空快照）。
func (s *BridgeService) goalSnapshotReadOnly() GoalSnapshot {
	s.stateMu.RLock()
	sessionID := strings.TrimSpace(s.currentSessionID)
	s.stateMu.RUnlock()
	if sessionID == "" {
		return GoalSnapshot{}
	}
	resp := coreValueOrNil(
		s,
		coreapi.GoalGetResponse{},
		func(g bridgeRuntimeGateway) (coreapi.GoalGetResponse, error) {
			return g.CoreGoalGetRPC(coreCtx(), sessionID)
		},
	)
	if resp.Goal == nil {
		return GoalSnapshot{SessionID: sessionID}
	}
	goal := *resp.Goal
	return GoalSnapshot{
		SessionID:       sessionID,
		Objective:       goal.Objective,
		Status:          goal.Status,
		TokenBudget:     goal.TokenBudget,
		TokensUsed:      goal.TokensUsed,
		TimeUsedSeconds: goal.TimeUsedSeconds,
		GoalID:          goal.GoalID,
	}
}

// requireGoalSession 取当前会话 id；goal 按会话键控，无会话时明确报错。
func (s *BridgeService) requireGoalSession() (string, error) {
	sessionID := strings.TrimSpace(s.currentSessionID)
	if sessionID == "" {
		return "", errors.New("no active session")
	}
	return sessionID, nil
}

// SetGoal 是 /goal set 的桌面端入口：设定（或替换）目标并返回最新 bootstrap。
func (s *BridgeService) SetGoal(objective string, tokenBudget *int64) (BootstrapState, error) {
	sessionID, err := s.requireGoalSession()
	if err != nil {
		return BootstrapState{}, err
	}
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return BootstrapState{}, errors.New("objective must not be empty")
	}
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return BootstrapState{}, err
	}
	goal, err := gateway.CoreGoalSetRPC(coreCtx(), coreapi.GoalSetRequest{
		SessionID:   sessionID,
		Objective:   objective,
		TokenBudget: tokenBudget,
	})
	if err != nil {
		return BootstrapState{}, fmt.Errorf("set goal: %w", err)
	}
	slog.Info("bridge.goal.set", "session_id", sessionID, "goal_id", goal.GoalID, "status", goal.Status)
	return s.LoadBootstrap(), nil
}

// PauseGoal 暂停目标（停止自驱；进行中的回合不打断）。
func (s *BridgeService) PauseGoal() (BootstrapState, error) {
	sessionID, err := s.requireGoalSession()
	if err != nil {
		return BootstrapState{}, err
	}
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return BootstrapState{}, err
	}
	goal, err := gateway.CoreGoalPauseRPC(coreCtx(), sessionID)
	if err != nil {
		return BootstrapState{}, fmt.Errorf("pause goal: %w", err)
	}
	slog.Info("bridge.goal.pause", "session_id", sessionID, "status", goal.Status)
	return s.LoadBootstrap(), nil
}

// ResumeGoal 恢复目标并立即触发自驱（budgetLimited 终态会被内核拒绝）。
func (s *BridgeService) ResumeGoal() (BootstrapState, error) {
	sessionID, err := s.requireGoalSession()
	if err != nil {
		return BootstrapState{}, err
	}
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return BootstrapState{}, err
	}
	goal, err := gateway.CoreGoalResumeRPC(coreCtx(), sessionID)
	if err != nil {
		return BootstrapState{}, fmt.Errorf("resume goal: %w", err)
	}
	slog.Info("bridge.goal.resume", "session_id", sessionID, "status", goal.Status)
	return s.LoadBootstrap(), nil
}

// ClearGoal 清除目标（幂等）。
func (s *BridgeService) ClearGoal() (BootstrapState, error) {
	sessionID, err := s.requireGoalSession()
	if err != nil {
		return BootstrapState{}, err
	}
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return BootstrapState{}, err
	}
	if err := gateway.CoreGoalClearRPC(coreCtx(), sessionID); err != nil {
		return BootstrapState{}, fmt.Errorf("clear goal: %w", err)
	}
	slog.Info("bridge.goal.clear", "session_id", sessionID)
	return s.LoadBootstrap(), nil
}
