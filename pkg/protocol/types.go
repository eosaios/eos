package protocol

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "time"

type SessionInfo struct {
	SessionID        string         `json:"session_id"`
	ThreadID         string         `json:"thread_id,omitempty"`
	Workspace        string         `json:"workspace,omitempty"`
	Title            string         `json:"title,omitempty"`
	Preview          string         `json:"preview,omitempty"`
	Mode             string         `json:"mode,omitempty"`
	Status           string         `json:"status,omitempty"`
	CurrentRequestID string         `json:"current_request_id,omitempty"`
	PendingApprovals []string       `json:"pending_approvals,omitempty"`
	PendingInquiries []string       `json:"pending_inquiries,omitempty"`
	RunningTasks     []string       `json:"running_tasks,omitempty"`
	UpdatedAt        time.Time      `json:"updated_at,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type ApprovalRequest struct {
	ApprovalID string   `json:"approval_id"`
	Title      string   `json:"title,omitempty"`
	Message    string   `json:"message"`
	RiskLevel  string   `json:"risk_level,omitempty"`
	Options    []string `json:"options,omitempty"`
}

type ApprovalResolution struct {
	ApprovalID string `json:"approval_id"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
}

type InquiryRequest struct {
	InquiryID string   `json:"inquiry_id"`
	Question  string   `json:"question"`
	Options   []string `json:"options,omitempty"`
	AllowText bool     `json:"allow_text,omitempty"`
}

type InquiryResolution struct {
	InquiryID string `json:"inquiry_id"`
	Option    string `json:"option,omitempty"`
	Text      string `json:"text,omitempty"`
}

type TaskInfo struct {
	TaskID     string    `json:"task_id"`
	Kind       string    `json:"kind,omitempty"`
	Status     string    `json:"status,omitempty"`
	Label      string    `json:"label,omitempty"`
	CanCancel  bool      `json:"can_cancel,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

type ToolCall struct {
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	RiskLevel string         `json:"risk_level,omitempty"`
}

type ToolResult struct {
	ToolName string         `json:"tool_name"`
	Status   string         `json:"status,omitempty"`
	Display  string         `json:"display,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type TextPayload struct {
	Text             string `json:"text"`
	CollapsedDefault bool   `json:"collapsed_default,omitempty"`
}
