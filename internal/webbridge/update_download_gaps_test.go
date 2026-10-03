package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// update_download 余臂批测：StartUpdateDownload 的全前置校验族（检查
// 失败/无更新/缺资产/缺 digest/代理配置错误）、ready 态文件缺失重下、
// Cancel 幂等、InstallUpdate 的文件缺失/就绪校验臂。

import (
	"os"
	"path/filepath"
	"testing"
)

// withUpdateCheckResult 注入 CheckForUpdates 的返回。
func withUpdateCheckResult(t *testing.T, result *UpdateCheckResult) {
	t.Helper()
	withUpdateCache(t, result)
}

func TestStartUpdateDownloadPreconditions(t *testing.T) {
	newBridge := func(t *testing.T) *BridgeService {
		return &BridgeService{runtimeGateway: &chatSessionsGatewayStub{}}
	}

	t.Run("ready 且文件在：直接返回", func(t *testing.T) {
		svc := newBridge(t)
		path := filepath.Join(t.TempDir(), "pkg.bin")
		os.WriteFile(path, []byte("x"), 0o644)
		svc.updateDownload = UpdateDownloadState{Stage: updateStageReady, LocalPath: path}
		got := svc.StartUpdateDownload()
		if got.Stage != updateStageReady || got.LocalPath != path {
			t.Fatalf("ready 复用 = %+v", got)
		}
	})

	t.Run("ready 但文件丢：走检查链后失败", func(t *testing.T) {
		withUpdateCheckResult(t, nil)
		t.Setenv("HOME", t.TempDir())
		svc := newBridge(t)
		svc.updateDownload = UpdateDownloadState{Stage: updateStageReady, LocalPath: "/gone/pkg.bin"}
		got := svc.StartUpdateDownload()
		if got.Stage != updateStageFailed {
			t.Fatalf("文件丢失后应失败: %+v", got)
		}
	})

	t.Run("检查失败", func(t *testing.T) {
		withUpdateCheckResult(t, &UpdateCheckResult{Error: "网络不通"})
		svc := newBridge(t)
		got := svc.StartUpdateDownload()
		if got.Stage != updateStageFailed || got.Error == "" {
			t.Fatalf("检查失败 = %+v", got)
		}
	})

	t.Run("已是最新", func(t *testing.T) {
		withUpdateCheckResult(t, &UpdateCheckResult{HasUpdate: false})
		svc := newBridge(t)
		got := svc.StartUpdateDownload()
		if got.Stage != updateStageFailed {
			t.Fatalf("无更新 = %+v", got)
		}
	})

	t.Run("缺下载资产", func(t *testing.T) {
		withUpdateCheckResult(t, &UpdateCheckResult{HasUpdate: true, LatestVersion: "v2"})
		svc := newBridge(t)
		got := svc.StartUpdateDownload()
		if got.Stage != updateStageFailed {
			t.Fatalf("缺资产 = %+v", got)
		}
	})

	t.Run("缺 digest 拒绝", func(t *testing.T) {
		withUpdateCheckResult(t, &UpdateCheckResult{
			HasUpdate: true, LatestVersion: "v2",
			DownloadURL: "http://x/p.bin", AssetName: "p.bin",
		})
		svc := newBridge(t)
		got := svc.StartUpdateDownload()
		if got.Stage != updateStageFailed {
			t.Fatalf("缺 digest = %+v", got)
		}
	})
}

func TestStartUpdateDownloadProxyConfigError(t *testing.T) {
	// 代理配置读失败：configPath 指向目录。
	dir := t.TempDir()
	withUpdateCheckResult(t, &UpdateCheckResult{
		HasUpdate: true, LatestVersion: "v2",
		DownloadURL: "http://x/p.bin", AssetName: "p.bin", AssetDigest: "sha256:" + "a",
	})
	svc := &BridgeService{runtimeGateway: &chatSessionsGatewayStub{configPathOverride: dir}}
	got := svc.StartUpdateDownload()
	if got.Stage != updateStageFailed {
		t.Fatalf("代理配置读失败 = %+v", got)
	}

	// 代理地址非法。
	cfg := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfg, []byte(`{"update_proxy_enabled": true, "update_proxy_url": "ftp://bad"}`), 0o644)
	svc2 := &BridgeService{runtimeGateway: &chatSessionsGatewayStub{configPathOverride: cfg}}
	got = svc2.StartUpdateDownload()
	if got.Stage != updateStageFailed {
		t.Fatalf("非法代理 = %+v", got)
	}
}

func TestCancelUpdateDownloadIdempotent(t *testing.T) {
	svc := &BridgeService{}
	// 无下载：幂等返回当前态。
	if got := svc.CancelUpdateDownload(); got.Stage != "" {
		t.Fatalf("无下载取消 = %+v", got)
	}
	// 有 cancel：触发并复位。
	var cancelled bool
	svc.updateCancel = func() { cancelled = true }
	_ = svc.CancelUpdateDownload()
	if !cancelled {
		t.Fatal("cancel 未触发")
	}
}

func TestInstallUpdateGuards(t *testing.T) {
	svc := &BridgeService{}

	// 非 ready 态拒绝。
	if err := svc.InstallUpdate(); err == nil {
		t.Fatal("非 ready 应拒绝")
	}

	// ready 但文件缺失。
	path := filepath.Join(t.TempDir(), "gone.bin")
	svc.updateMu.Lock()
	svc.updateDownload = UpdateDownloadState{Stage: updateStageReady, LocalPath: path}
	svc.updateMu.Unlock()
	if err := svc.InstallUpdate(); err == nil {
		t.Fatal("文件缺失应报错")
	}
}
