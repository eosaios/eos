package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func withFastRetry(t *testing.T) {
	t.Helper()
	origAttempts, origBackoff := downloadAttempts, downloadRetryBackoff
	downloadAttempts, downloadRetryBackoff = 4, time.Millisecond
	t.Cleanup(func() {
		downloadAttempts, downloadRetryBackoff = origAttempts, origBackoff
	})
}

// 首次请求传一半断开，重试带 Range 续传剩余部分——最终文件完整。
func TestDownloadToResumesAfterMidBodyDrop(t *testing.T) {
	withFastRetry(t)
	payload := strings.Repeat("eos-update-payload-", 4000) // ~72KB
	half := len(payload) / 2

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(payload[:half]))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			// 直接断开连接：客户端得到 unexpected EOF。
			if hj, ok := w.(http.Hijacker); ok {
				if conn, buf, err := hj.Hijack(); err == nil {
					_ = buf.Flush()
					_ = conn.Close()
				}
			}
			return
		}
		// 重试：校验 Range 并只回剩余部分。
		if rng := r.Header.Get("Range"); rng != fmt.Sprintf("bytes=%d-", half) {
			t.Errorf("retry Range = %q, want bytes=%d-", rng, half)
		}
		rest := payload[half:]
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rest)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(rest))
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "archive.bin")
	if err := downloadTo(context.Background(), srv.URL, dst, nil, nil); err != nil {
		t.Fatalf("downloadTo: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("expected exactly 2 requests, got %d", hits)
	}
}

// 服务端不支持 Range（重试仍回 200 全量）：客户端应整体重写而非拼接。
func TestDownloadToRetriesWithFullRewriteWhenRangeUnsupported(t *testing.T) {
	withFastRetry(t)
	payload := strings.Repeat("no-range-support ", 3000)

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(payload[:100]))
			if hj, ok := w.(http.Hijacker); ok {
				if conn, buf, err := hj.Hijack(); err == nil {
					_ = buf.Flush()
					_ = conn.Close()
				}
			}
			return
		}
		// 忽略 Range，直接 200 全量。
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "archive.bin")
	if err := downloadTo(context.Background(), srv.URL, dst, nil, nil); err != nil {
		t.Fatalf("downloadTo: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("payload mismatch after rewrite: got %d bytes", len(got))
	}
}

// 重试次数耗尽后报错，错误信息包含尝试次数与底层原因。
func TestDownloadToFailsAfterAttemptsExhausted(t *testing.T) {
	withFastRetry(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "archive.bin")
	err := downloadTo(context.Background(), srv.URL, dst, nil, nil)
	if err == nil {
		t.Fatal("expected error after attempts exhausted")
	}
	if !strings.Contains(err.Error(), "after 4 attempts") || !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("unexpected error: %v", err)
	}
}
