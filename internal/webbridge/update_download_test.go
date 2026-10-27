package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVerifyFileDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.bin")
	content := []byte("eos update asset payload")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	good := "sha256:" + hex.EncodeToString(sum[:])

	if err := verifyFileDigest(path, good); err != nil {
		t.Errorf("verifyFileDigest(good) = %v, want nil", err)
	}
	if err := verifyFileDigest(path, "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Error("verifyFileDigest(不匹配 digest) 应报错")
	}
	if err := verifyFileDigest(path, "sha256:zz"); err == nil {
		t.Error("verifyFileDigest(非法长度 digest) 应报错")
	}
	if err := verifyFileDigest(path, ""); err == nil {
		t.Error("verifyFileDigest(空 digest) 应报错")
	}
}

func TestDownloadHTTPToFileProgressAndCleanup(t *testing.T) {
	payload := strings.Repeat("a", 200*1024) // > 64KB 触发多轮进度回调
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "sub", "asset.bin")

	var mu sync.Mutex
	var lastReceived, lastTotal int64
	callbacks := 0
	// httptest 响应为 chunked（无 Content-Length），总量走 totalHint 分支
	err := downloadHTTPToFile(context.Background(), nil, server.URL, dest, int64(len(payload)), func(received, total int64) {
		mu.Lock()
		callbacks++
		lastReceived, lastTotal = received, total
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("downloadHTTPToFile = %v, want nil", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Errorf("下载内容不一致：长度 %d，want %d", len(got), len(payload))
	}
	mu.Lock()
	defer mu.Unlock()
	if callbacks == 0 || lastReceived != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Errorf("进度回调异常：callbacks=%d last=%d/%d", callbacks, lastReceived, lastTotal)
	}

	// 失败路径：半成品文件必须被清理
	failDest := filepath.Join(dir, "fail.bin")
	if err := downloadHTTPToFile(context.Background(), nil, server.URL+"/missing", failDest, 0, nil); err == nil {
		t.Error("404 下载应报错")
	} else if _, statErr := os.Stat(failDest); statErr == nil {
		t.Error("失败下载的半成品文件应被清理")
	}
}

func TestRunUpdateDownloadHappyPathAndDigestMismatch(t *testing.T) {
	payload := []byte("fake-installer-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	sum := sha256.Sum256(payload)
	goodDigest := "sha256:" + hex.EncodeToString(sum[:])

	t.Run("校验通过进入 ready", func(t *testing.T) {
		svc := &BridgeService{}
		var mu sync.Mutex
		var stages []string
		svc.emitEvent = func(name string, _ any) {
			if name != updateDownloadEventName {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			stages = append(stages, svc.GetUpdateDownloadState().Stage)
		}
		dest := filepath.Join(t.TempDir(), "setup.exe")
		done := make(chan struct{})
		go func() {
			svc.runUpdateDownload(context.Background(), nil, UpdateCheckResult{
				DownloadURL:    server.URL,
				AssetDigest:    goodDigest,
				AssetSizeBytes: int64(len(payload)),
			}, dest)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("下载超时")
		}
		final := svc.GetUpdateDownloadState()
		if final.Stage != updateStageReady || final.LocalPath != dest || final.Percent != 100 {
			t.Errorf("最终状态异常: %+v", final)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(stages) == 0 || stages[len(stages)-1] != updateStageReady {
			t.Errorf("事件流异常：stages=%v", stages)
		}
	})

	t.Run("digest 不匹配丢弃文件并失败", func(t *testing.T) {
		svc := &BridgeService{}
		dest := filepath.Join(t.TempDir(), "setup-bad.exe")
		svc.runUpdateDownload(context.Background(), nil, UpdateCheckResult{
			DownloadURL: server.URL,
			AssetDigest: "sha256:" + strings.Repeat("1", 64),
		}, dest)
		final := svc.GetUpdateDownloadState()
		if final.Stage != updateStageFailed || final.Error == "" {
			t.Errorf("期望 failed 带错误信息: %+v", final)
		}
		if _, err := os.Stat(dest); err == nil {
			t.Error("校验失败的文件应被删除")
		}
	})
}

func TestStartUpdateDownloadIdempotentWhileDownloading(t *testing.T) {
	svc := &BridgeService{}
	svc.updateDownload = UpdateDownloadState{
		Stage:      updateStageDownloading,
		Version:    "v9.9.9",
		TotalBytes: 100,
	}
	got := svc.StartUpdateDownload()
	if got.Stage != updateStageDownloading || got.Version != "v9.9.9" {
		t.Errorf("下载中重复启动应原样返回当前进度: %+v", got)
	}
}
