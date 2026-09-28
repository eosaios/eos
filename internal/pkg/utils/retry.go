package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

// RetryPolicy 重试策略配置
type RetryPolicy struct {
	MaxAttempts int           // 最大尝试次数（包括首次）
	BaseDelay   time.Duration // 基础延迟
	MaxDelay    time.Duration // 最大延迟
	Multiplier  float64       // 延迟倍数
}

// DefaultRetryPolicy 默认重试策略
var DefaultRetryPolicy = RetryPolicy{
	MaxAttempts: 3,
	BaseDelay:   500 * time.Millisecond,
	MaxDelay:    10 * time.Second,
	Multiplier:  2.0,
}

// NoRetry 不重试策略
var NoRetry = RetryPolicy{
	MaxAttempts: 1,
	BaseDelay:   0,
	MaxDelay:    0,
	Multiplier:  1.0,
}

// RetryableFunc 可重试的函数类型
type RetryableFunc func() (*http.Response, error)

// RetryResult 重试结果
type RetryResult struct {
	Response   *http.Response // HTTP 响应
	Error      error          // 最后一次错误
	Attempts   int            // 尝试次数
	TotalDelay time.Duration  // 总延迟时间
	Succeeded  bool           // 是否成功
}

// IsRetryableHTTPError 检查 HTTP 错误是否可重试
func IsRetryableHTTPError(statusCode int) bool {
	// 可重试的状态码：
	// 408 Request Timeout
	// 429 Too Many Requests
	// 500 Internal Server Error
	// 502 Bad Gateway
	// 503 Service Unavailable
	// 504 Gateway Timeout
	switch statusCode {
	case 408, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// IsRetryableError 检查错误是否可重试
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 超时错误可重试
	if IsErrorCode(err, ErrCodeTimeout) {
		return true
	}

	// 网络错误可重试
	if IsErrorCode(err, ErrCodeNetwork) {
		return true
	}

	errStr := err.Error()
	// 检查常见的临时错误模式
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"timeout",
		"deadline exceeded",
		"temporary failure",
		"try again",
	}

	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// contains 检查字符串是否包含子串（不区分大小写）。
// 旧实现只匹配前缀/后缀且区分大小写，导致
// "dial tcp ...: connection refused" 这类中间命中模式检不出来。
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// calculateDelay 计算指数退避延迟
func calculateDelay(attempt int, policy RetryPolicy) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// 指数退避：delay = base * (multiplier ^ (attempt - 1))
	delay := float64(policy.BaseDelay) * math.Pow(policy.Multiplier, float64(attempt-1))

	// 限制最大延迟
	if delay > float64(policy.MaxDelay) {
		delay = float64(policy.MaxDelay)
	}

	return time.Duration(delay)
}

// DoHTTPRetry 执行带重试的 HTTP 请求
// 使用指数退避策略，仅对可重试的错误进行重试
func DoHTTPRetry(ctx context.Context, fn RetryableFunc, policy RetryPolicy) *RetryResult {
	if policy.MaxAttempts <= 0 {
		policy = DefaultRetryPolicy
	}

	result := &RetryResult{
		Attempts: 0,
	}

	var lastErr error

	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		result.Attempts++

		// 执行请求
		resp, err := fn()

		if err == nil && resp != nil {
			// 请求成功
			result.Response = resp
			result.Succeeded = true

			// 检查状态码是否需要重试
			if IsRetryableHTTPError(resp.StatusCode) {
				slog.Debug("retry.http.status_retryable",
					"component", ComponentSystem,
					"attempt", attempt+1,
					"max_attempts", policy.MaxAttempts,
					"status_code", resp.StatusCode,
				)

				// 关闭响应体
				_ = resp.Body.Close()

				// 如果不是最后一次尝试，继续重试
				if attempt < policy.MaxAttempts-1 {
					delay := calculateDelay(attempt+1, policy)
					result.TotalDelay += delay
					time.Sleep(delay)
					continue
				}
			} else {
				// 成功且不需要重试
				return result
			}
		}

		// 记录错误
		if err != nil {
			lastErr = err
			result.Error = err
		}

		// 检查是否可重试
		if !IsRetryableError(err) {
			// 不可重试的错误，直接返回
			slog.Debug("retry.http.not_retryable",
				"component", ComponentSystem,
				"error", err.Error(),
			)
			return result
		}

		slog.Debug("retry.http.attempt_failed",
			"component", ComponentSystem,
			"attempt", attempt+1,
			"max_attempts", policy.MaxAttempts,
			"error", err.Error(),
		)

		// 如果不是最后一次尝试，等待后重试
		if attempt < policy.MaxAttempts-1 {
			delay := calculateDelay(attempt+1, policy)
			result.TotalDelay += delay

			slog.Debug("retry.http.waiting",
				"component", ComponentSystem,
				"delay", delay.String(),
			)

			select {
			case <-time.After(delay):
				// 继续重试
			case <-ctx.Done():
				// 上下文取消
				result.Error = ctx.Err()
				return result
			}
		}
	}

	result.Error = lastErr
	return result
}

// DoHTTPRetryWithClient 使用自定义客户端执行带重试的 HTTP 请求
func DoHTTPRetryWithClient(ctx context.Context, client *http.Client, req *http.Request, policy RetryPolicy) *RetryResult {
	fn := func() (*http.Response, error) {
		return client.Do(req.Clone(ctx))
	}
	return DoHTTPRetry(ctx, fn, policy)
}

// DoHTTPRetrySimple 简化的 HTTP 重试，使用默认客户端和策略
func DoHTTPRetrySimple(req *http.Request) *RetryResult {
	return DoHTTPRetryWithClient(context.Background(), http.DefaultClient, req, DefaultRetryPolicy)
}

// RetryableOperation 通用可重试操作
type RetryableOperation func() error

// DoRetry 执行通用重试操作
func DoRetry(ctx context.Context, fn RetryableOperation, policy RetryPolicy) error {
	if policy.MaxAttempts <= 0 {
		policy = DefaultRetryPolicy
	}

	var lastErr error

	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// 检查是否可重试
		if !IsRetryableError(err) {
			return err
		}

		slog.Debug("retry.operation.attempt_failed",
			"component", ComponentSystem,
			"attempt", attempt+1,
			"max_attempts", policy.MaxAttempts,
			"error", err.Error(),
		)

		// 如果不是最后一次尝试，等待后重试
		if attempt < policy.MaxAttempts-1 {
			delay := calculateDelay(attempt+1, policy)

			select {
			case <-time.After(delay):
				// 继续重试
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return lastErr
}
