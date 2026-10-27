package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"log/slog"
)

// ErrorCode 错误码类型
type ErrorCode string

const (
	// ErrCodeUnknown 未知错误
	ErrCodeUnknown ErrorCode = "UNKNOWN"
	// ErrCodeInvalidInput 无效输入
	ErrCodeInvalidInput ErrorCode = "INVALID_INPUT"
	// ErrCodeNotFound 未找到
	ErrCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrCodePermission 权限错误
	ErrCodePermission ErrorCode = "PERMISSION_DENIED"
	// ErrCodeIO IO 错误
	ErrCodeIO ErrorCode = "IO_ERROR"
	// ErrCodeNetwork 网络错误
	ErrCodeNetwork ErrorCode = "NETWORK_ERROR"
	// ErrCodeTimeout 超时错误
	ErrCodeTimeout ErrorCode = "TIMEOUT"
	// ErrCodeConfig 配置错误
	ErrCodeConfig ErrorCode = "CONFIG_ERROR"
	// ErrCodeTool 工具错误
	ErrCodeTool ErrorCode = "TOOL_ERROR"
	// ErrCodeRuntime 运行时错误
	ErrCodeRuntime ErrorCode = "RUNTIME_ERROR"
)

// AppError 应用错误类型，包含错误码和上下文信息
type AppError struct {
	Code    ErrorCode      // 错误码
	Message string         // 错误消息
	Err     error          // 原始错误
	Op      string         // 操作名称
	Details map[string]any // 额外详情
}

// Error 实现 error 接口
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 实现 errors.Unwrap 接口
func (e *AppError) Unwrap() error {
	return e.Err
}

// LogAttrs 返回用于日志的属性
func (e *AppError) LogAttrs() []any {
	attrs := []any{"error_code", e.Code, "error_op", e.Op}
	if e.Message != "" {
		attrs = append(attrs, "error_message", e.Message)
	}
	for k, v := range e.Details {
		attrs = append(attrs, k, v)
	}
	return attrs
}

// NewError 创建新的应用错误
func NewError(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// WrapError 包装错误并添加上下文
func WrapError(err error, code ErrorCode, op string, message string) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
		Op:      op,
	}
}

// WrapErrorWithDetails 包装错误并添加详细上下文
func WrapErrorWithDetails(err error, code ErrorCode, op string, message string, details map[string]any) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
		Op:      op,
		Details: details,
	}
}

// LogAndWrapError 记录错误并返回包装后的错误
func LogAndWrapError(component, operation string, err error, code ErrorCode, message string) error {
	if err == nil {
		return nil
	}
	wrapped := WrapError(err, code, operation, message)
	slog.Error(component+"."+operation+".error",
		append([]any{"component", component}, wrapped.LogAttrs()...)...,
	)
	return wrapped
}

// IsErrorCode 检查错误是否为指定的错误码
func IsErrorCode(err error, code ErrorCode) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Code == code
	}
	return false
}

// GetErrorCode 获取错误码，如果不是 AppError 则返回 ErrCodeUnknown
func GetErrorCode(err error) ErrorCode {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Code
	}
	return ErrCodeUnknown
}
