package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---------- json ----------

func TestJSONMarshalUnmarshal(t *testing.T) {
	var out map[string]any
	if err := UnmarshalWithErrorHandling([]byte(`{"a":1}`), &out, "test"); err != nil {
		t.Fatal(err)
	}
	if out["a"] == nil {
		t.Fatalf("out = %v", out)
	}
	if err := UnmarshalWithErrorHandling([]byte(`{`), &out, "test"); err == nil {
		t.Fatal("bad json should error")
	}
	if err := UnmarshalSilently([]byte(`{"b":2}`), &out); err != nil {
		t.Fatal(err)
	}

	data, err := MarshalWithErrorHandling(map[string]int{"x": 1}, "test")
	if err != nil || !strings.Contains(string(data), "x") {
		t.Fatalf("marshal = %q %v", data, err)
	}
	if _, err := MarshalWithErrorHandling(make(chan int), "test"); err == nil {
		t.Fatal("chan should fail marshal")
	}
	indented, err := MarshalIndent(map[string]int{"x": 1}, "", "  ")
	if err != nil || !strings.Contains(string(indented), "\n") {
		t.Fatalf("indent = %q %v", indented, err)
	}
}

func TestFixJSONEscapeSequences(t *testing.T) {
	// Windows 路径反斜杠被转义
	got := FixJSONEscapeSequences(`{"p":"C:\home\demo"}`)
	if !strings.Contains(got, `C:\\home\\demo`) {
		t.Fatalf("fix path = %q", got)
	}
	// 合法转义保留
	got = FixJSONEscapeSequences(`{"s":"a\"b\\c\nd"}`)
	if !strings.Contains(got, `a\"b\\c\nd`) {
		t.Fatalf("keep valid = %q", got)
	}
	// \uXXXX 保留
	got = FixJSONEscapeSequences(`{"u":"中"}`)
	if !strings.Contains(got, `中`) {
		t.Fatalf("unicode = %q", got)
	}
	got = FixJSONEscapeSequences(`{"u":"\u4e2d"}`)
	if !strings.Contains(got, `\u4e2d`) {
		t.Fatalf("uXXXX = %q", got)
	}
}

func TestUnmarshalWithEscapeFixAndCommonErrors(t *testing.T) {
	var out map[string]any
	// 含 Windows 路径转义
	if err := UnmarshalWithEscapeFix(`{"p":"C:\tmp\x"}`, &out); err != nil {
		t.Fatalf("escape fix: %v", err)
	}
	// 尾逗号
	out = nil
	if err := UnmarshalWithEscapeFix(`{"a":1,}`, &out); err != nil {
		t.Fatalf("trailing comma: %v", err)
	}
	// 真错误
	if err := UnmarshalWithEscapeFix(`{`, &out); err == nil {
		t.Fatal("bad json")
	}

	if got := FixCommonJSONErrors(`{"a":1,}`); !strings.Contains(got, `"a":1`) {
		t.Fatalf("common fix = %q", got)
	}
}

// ---------- params ----------

func TestGetParamHelpers(t *testing.T) {
	if GetParamString(nil, "k", "d") != "d" {
		t.Fatal("nil map")
	}
	p := map[string]any{"s": "v", "i": 3, "i64": int64(4), "f": 5.0, "f32": float32(6), "b": true, "bad": 1}
	if GetParamString(p, "s", "d") != "v" {
		t.Fatal("string")
	}
	if GetParamString(p, "missing", "d") != "d" || GetParamString(p, "bad", "d") != "d" {
		t.Fatal("string fallback")
	}
	if GetParamInt(p, "i", 0) != 3 || GetParamInt(p, "i64", 0) != 4 || GetParamInt(p, "f", 0) != 5 || GetParamInt(p, "f32", 0) != 6 {
		t.Fatal("int variants")
	}
	if GetParamInt(p, "s", -1) != -1 {
		t.Fatal("int fallback")
	}
	if !GetParamBool(p, "b", false) || GetParamBool(p, "bad", true) != true {
		t.Fatal("bool")
	}

	if GetParamStringSlice(nil, "k") != nil {
		t.Fatal("nil slice")
	}
	p2 := map[string]any{
		"ss":   []string{"a", "b"},
		"sa":   []any{"x", 1, "y"},
		"bad":  1,
		"none": []string{},
	}
	if got := GetParamStringSlice(p2, "ss"); len(got) != 2 {
		t.Fatalf("[]string = %v", got)
	}
	if got := GetParamStringSlice(p2, "sa"); len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Fatalf("[]any = %v", got)
	}
	if GetParamStringSlice(p2, "bad") != nil {
		t.Fatal("bad type")
	}
	if GetParamStringSlice(p2, "missing") != nil {
		t.Fatal("missing")
	}

	// ValidateParams
	missing, ok := ValidateParams(p, []string{"s", "nope", "i"})
	if ok || len(missing) != 1 || missing[0] != "nope" {
		t.Fatalf("validate = %v %v", missing, ok)
	}
	missing, ok = ValidateParams(nil, []string{"a", "b"})
	if ok || len(missing) != 2 {
		t.Fatalf("nil params = %v %v", missing, ok)
	}
	if _, ok := ValidateParams(p, []string{"s", "i"}); !ok {
		t.Fatal("all present")
	}
}

// ---------- log ----------

func TestGetLogLevelAndLogHelpers(t *testing.T) {
	t.Setenv("EOS_LOG_LEVEL", "")
	t.Setenv("LOG_LEVEL", "")
	if getLogLevel() != slog.LevelDebug {
		t.Fatal("default debug")
	}
	t.Setenv("EOS_LOG_LEVEL", "INFO")
	if getLogLevel() != slog.LevelInfo {
		t.Fatal("info")
	}
	t.Setenv("EOS_LOG_LEVEL", "warning")
	if getLogLevel() != slog.LevelWarn {
		t.Fatal("warning")
	}
	t.Setenv("EOS_LOG_LEVEL", "ERROR")
	if getLogLevel() != slog.LevelError {
		t.Fatal("error")
	}
	t.Setenv("EOS_LOG_LEVEL", "nope")
	if getLogLevel() != slog.LevelInfo {
		t.Fatal("unknown → info")
	}
	t.Setenv("EOS_LOG_LEVEL", "")
	t.Setenv("LOG_LEVEL", "debug")
	if getLogLevel() != slog.LevelDebug {
		t.Fatal("LOG_LEVEL fallback")
	}

	// 日志辅助不 panic
	LogError(ComponentSystem, "op", errors.New("e"), "k", 1)
	LogDebug(ComponentSystem, "op", "k", 1)
	LogInfo(ComponentSystem, "op")
	LogWarn(ComponentSystem, "op", "k", "v")
}

// ---------- retry ----------

func TestIsRetryable(t *testing.T) {
	if !IsRetryableHTTPError(429) || !IsRetryableHTTPError(503) || !IsRetryableHTTPError(502) {
		t.Fatal("retryable status")
	}
	if IsRetryableHTTPError(200) || IsRetryableHTTPError(404) {
		t.Fatal("non-retryable status")
	}
	if !IsRetryableError(timeoutErr{}) {
		t.Fatal("timeout should retry")
	}
	if IsRetryableError(errors.New("plain")) {
		t.Fatal("plain")
	}
	if IsRetryableError(&ClientError{Kind: ErrHTTPStatus}) {
		t.Fatal("client error")
	}
	// 回归：旧 contains 只匹配前后缀，中间命中检不出
	if !IsRetryableError(errors.New("dial tcp 127.0.0.1:80: connection refused")) {
		t.Fatal("mid-string connection refused should retry")
	}
	if !IsRetryableError(errors.New("Connection Reset By Peer")) {
		t.Fatal("case-insensitive match")
	}
	// contains 子串语义
	if !contains("hello world", "lo wo") {
		t.Fatal("mid substring")
	}
	if contains("hello", "xyz") {
		t.Fatal("missing")
	}
}

func TestCalculateDelayAndDoRetry(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, Multiplier: 2}
	if d := calculateDelay(0, policy); d < 0 {
		t.Fatalf("delay = %v", d)
	}

	// DoRetry 成功
	calls := 0
	err := DoRetry(context.Background(), func() error {
		calls++
		return nil
	}, policy)
	if err != nil || calls != 1 {
		t.Fatalf("success = %v calls=%d", err, calls)
	}

	// 可重试错误耗尽次数
	calls = 0
	err = DoRetry(context.Background(), func() error {
		calls++
		return timeoutErr{}
	}, RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond})
	if err == nil || calls != 2 {
		t.Fatalf("exhausted = %v calls=%d", err, calls)
	}

	// 不可重试错误立即停
	calls = 0
	err = DoRetry(context.Background(), func() error {
		calls++
		return errors.New("plain")
	}, policy)
	if err == nil || calls != 1 {
		t.Fatalf("non-retryable = %v calls=%d", err, calls)
	}
}

func TestDoHTTPRetryFunc(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond}
	res := DoHTTPRetry(context.Background(), func() (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	}, policy)
	if res.Error != nil || res.Response == nil || res.Response.StatusCode != 200 || !res.Succeeded {
		t.Fatalf("ok = %+v", res)
	}

	res = DoHTTPRetry(context.Background(), func() (*http.Response, error) {
		return nil, errors.New("net")
	}, policy)
	if res.Error == nil || res.Succeeded {
		t.Fatalf("should fail: %+v", res)
	}
}
