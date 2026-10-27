package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/eosaios/eos/internal/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogComponent 定义日志组件
const (
	ComponentUser    = "USER"
	ComponentAI      = "AI"
	ComponentTool    = "TOOL"
	ComponentAgent   = "AGENT"
	ComponentSystem  = "SYSTEM"
	ComponentSummary = "SUMMARY"
	ComponentThought = "THOUGHT"
	ComponentUI      = "UI"
)

// EventType 定义事件类型
const (
	EventTypeUserInput      = "user_input"
	EventTypeAIResponse     = "ai_response"
	EventTypeToolCall       = "tool_call"
	EventTypeToolResult     = "tool_result"
	EventTypeAgentCall      = "agent_call"
	EventTypeAgentSkill     = "agent_skill"
	EventTypeThought        = "thought"
	EventTypeSummary        = "summary"
	EventTypeError          = "error"
	EventTypeWarning        = "warning"
	EventTypeInfo           = "info"
	EventTypeDebug          = "debug"
	EventTypeGeneral        = "general"
	EventTypePlanReady      = "plan_ready"
	EventTypeAssistantDelta = "assistant_delta"
	EventTypeAssistantFinal = "assistant_final"
)

func init() {
	NewLogger("")
}

func defaultLogDir() string {
	return filepath.Join(config.ConfiguredLogDir(), "cli")
}

func NewLogger(dir string) {
	if strings.TrimSpace(dir) == "" {
		dir = defaultLogDir()
	}
	_ = os.MkdirAll(dir, 0755)
	lj := &lumberjack.Logger{Filename: filepath.Join(dir, "eos-cli.log"), MaxSize: 10, MaxBackups: 7, MaxAge: 30, Compress: true}
	lvl := getLogLevel()
	h1 := slog.NewJSONHandler(lj, &slog.HandlerOptions{AddSource: true, Level: lvl})
	slog.SetDefault(slog.New(h1))
}

func getLogLevel() slog.Level {
	lvl := os.Getenv("EOS_LOG_LEVEL")
	if strings.TrimSpace(lvl) == "" {
		lvl = os.Getenv("LOG_LEVEL")
	}
	if strings.TrimSpace(lvl) != "" {
		switch strings.ToLower(lvl) {
		case "debug":
			return slog.LevelDebug
		case "info":
			return slog.LevelInfo
		case "warn", "warning":
			return slog.LevelWarn
		case "error":
			return slog.LevelError
		default:
			return slog.LevelInfo
		}
	}
	return slog.LevelDebug
}

// LogError 结构化错误日志辅助函数
// component: 组件名称（如 ComponentTool, ComponentUI）
// operation: 操作名称（如 "read_file", "parse_config"）
// err: 错误对象
// kv: 额外的键值对
func LogError(component, operation string, err error, kv ...any) {
	args := make([]any, 0, 4+len(kv))
	args = append(args, "component", component, "error", err.Error())
	args = append(args, kv...)
	slog.Error(operation+".error", args...)
}

// LogDebug 结构化调试日志辅助函数
// component: 组件名称
// operation: 操作名称
// kv: 键值对
func LogDebug(component, operation string, kv ...any) {
	args := make([]any, 0, 2+len(kv))
	args = append(args, "component", component)
	args = append(args, kv...)
	slog.Debug(operation+".debug", args...)
}

// LogInfo 结构化信息日志辅助函数
func LogInfo(component, operation string, kv ...any) {
	args := make([]any, 0, 2+len(kv))
	args = append(args, "component", component)
	args = append(args, kv...)
	slog.Info(operation+".info", args...)
}

// LogWarn 结构化警告日志辅助函数
func LogWarn(component, operation string, kv ...any) {
	args := make([]any, 0, 2+len(kv))
	args = append(args, "component", component)
	args = append(args, kv...)
	slog.Warn(operation+".warn", args...)
}
