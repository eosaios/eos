package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestClientErrorMessages(t *testing.T) {
	if (*ClientError)(nil).Error() != "" {
		t.Fatal("nil Error")
	}
	if (*ClientError)(nil).Unwrap() != nil {
		t.Fatal("nil Unwrap")
	}

	cases := []struct {
		e    *ClientError
		want string
	}{
		{&ClientError{Kind: ErrInvalidURL, URL: "bad"}, "invalid URL: bad"},
		{&ClientError{Kind: ErrNonHTTPSURL, URL: "http://x"}, "non-HTTPS URL is not allowed: http://x"},
		{&ClientError{Kind: ErrTooManyRedirects, URL: "u"}, "too many redirects for u"},
		{&ClientError{Kind: ErrCrossHostRedirect, URL: "a", RedirectURL: "b"}, "cross-host redirect from a to b is not allowed"},
		{&ClientError{Kind: ErrHTTPStatus, StatusCode: 500}, "HTTP request failed with status 500"},
		{&ClientError{Kind: ErrHTTPStatus, Message: "custom"}, "custom"},
		{&ClientError{Kind: "other", Message: "m"}, "m"},
		{&ClientError{Kind: "other", WrappedError: errors.New("inner")}, "inner"},
		{&ClientError{Kind: "other"}, "http client error"},
	}
	for _, tc := range cases {
		if got := tc.e.Error(); got != tc.want {
			t.Fatalf("Error(%v) = %q, want %q", tc.e.Kind, got, tc.want)
		}
	}

	inner := errors.New("root")
	e := &ClientError{WrappedError: inner}
	if e.Unwrap() != inner {
		t.Fatal("Unwrap")
	}
}

func TestShouldRetry(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 3}
	spec := RequestSpec{Method: http.MethodGet, RetryPolicy: policy}

	// 用尽次数
	if shouldRetry(spec, nil, nil, 3) {
		t.Fatal("exhausted")
	}
	// MaxAttempts<=1
	if shouldRetry(RequestSpec{Method: http.MethodGet, RetryPolicy: RetryPolicy{MaxAttempts: 1}}, nil, nil, 1) {
		t.Fatal("single attempt")
	}
	// 非 GET/HEAD
	if shouldRetry(RequestSpec{Method: http.MethodPost, RetryPolicy: policy}, nil, nil, 1) {
		t.Fatal("post")
	}
	// ClientError 不重试
	if shouldRetry(spec, nil, &ClientError{Kind: ErrHTTPStatus}, 1) {
		t.Fatal("client error")
	}
	// 网络错误重试
	var netErr net.Error = timeoutErr{}
	if !shouldRetry(spec, nil, netErr, 1) {
		t.Fatal("net error")
	}
	// 普通错误走 IsRetryableError
	if shouldRetry(spec, nil, errors.New("plain"), 1) {
		t.Fatal("plain error")
	}
	// resp nil
	if shouldRetry(spec, nil, nil, 1) {
		t.Fatal("nil resp")
	}
	// 可重试状态码
	if !shouldRetry(spec, &ResponseSpec{StatusCode: 503}, nil, 1) {
		t.Fatal("503")
	}
	if shouldRetry(spec, &ResponseSpec{StatusCode: 200}, nil, 1) {
		t.Fatal("200")
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestReadLimited(t *testing.T) {
	// 无限制
	body, truncated, err := readLimited(strings.NewReader("hello"), 0)
	if err != nil || truncated || string(body) != "hello" {
		t.Fatalf("unlimited = %q %v %v", body, truncated, err)
	}
	// 未超限
	body, truncated, err = readLimited(strings.NewReader("hi"), 10)
	if err != nil || truncated || string(body) != "hi" {
		t.Fatalf("under = %q %v %v", body, truncated, err)
	}
	// 超限截断
	body, truncated, err = readLimited(strings.NewReader("0123456789"), 4)
	if err != nil || !truncated || string(body) != "0123" {
		t.Fatalf("over = %q %v %v", body, truncated, err)
	}
}

func TestRedirectAndHostHelpers(t *testing.T) {
	for _, code := range []int{301, 302, 307, 308} {
		if !isRedirectStatus(code) {
			t.Fatalf("code %d should be redirect", code)
		}
	}
	if isRedirectStatus(200) || isRedirectStatus(500) {
		t.Fatal("non-redirect")
	}

	u := func(s string) *url.URL {
		out, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if !sameOriginOrWWW(u("https://example.com/a"), u("https://www.example.com/b")) {
		t.Fatal("www sibling")
	}
	if sameOriginOrWWW(u("https://example.com"), u("http://example.com")) {
		t.Fatal("scheme mismatch")
	}
	if sameOriginOrWWW(u("https://example.com:443"), u("https://example.com:8443")) {
		t.Fatal("port mismatch")
	}
	if sameOriginOrWWW(nil, u("https://x")) || sameOriginOrWWW(u("https://x"), nil) {
		t.Fatal("nil url")
	}
	if stripWWW("WWW.Example.COM") != "example.com" {
		t.Fatalf("stripWWW = %q", stripWWW("WWW.Example.COM"))
	}

	if !isLocalHost("localhost") || !isLocalHost("127.0.0.1") || !isLocalHost("::1") || !isLocalHost(" LOCALHOST ") {
		t.Fatal("local hosts")
	}
	if isLocalHost("example.com") || isLocalHost("127.0.0.2") {
		t.Fatal("remote hosts")
	}
}

func TestIsAllowedScheme(t *testing.T) {
	// 默认只允许 https
	if !isAllowedScheme(mustURL(t, "https://example.com"), RequestSpec{}) {
		t.Fatal("https should pass")
	}
	if isAllowedScheme(mustURL(t, "http://example.com"), RequestSpec{}) {
		t.Fatal("http should fail by default")
	}
	if !isAllowedScheme(mustURL(t, "http://example.com"), RequestSpec{AllowHTTP: true}) {
		t.Fatal("AllowHTTP")
	}
	if !isAllowedScheme(mustURL(t, "http://localhost/x"), RequestSpec{AllowLocalHTTP: true}) {
		t.Fatal("AllowLocalHTTP")
	}
	if isAllowedScheme(mustURL(t, "ftp://example.com"), RequestSpec{AllowHTTP: true}) {
		t.Fatal("ftp should fail")
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	out, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// 保证 io.Reader 接口在 readLimited 错误路径上也可测
var _ io.Reader = strings.NewReader("")
