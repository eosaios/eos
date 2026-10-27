package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

// externalAppCatalog 的基础约束：files 恒可用且置顶，terminal 次之，
// 第三方应用 id 唯一。
func TestExternalAppCatalog(t *testing.T) {
	apps := externalAppCatalog()
	if len(apps) < 2 {
		t.Fatalf("externalAppCatalog() = %v, want at least files and terminal", apps)
	}
	if apps[0].ID != "files" || !apps[0].Installed {
		t.Errorf("apps[0] = %+v, want files installed", apps[0])
	}
	if apps[1].ID != "terminal" {
		t.Errorf("apps[1] = %+v, want terminal", apps[1])
	}
	seen := map[string]bool{}
	for _, app := range apps {
		if app.ID == "" || app.Name == "" {
			t.Errorf("app = %+v, want non-empty id and name", app)
		}
		if seen[app.ID] {
			t.Errorf("duplicate app id %q", app.ID)
		}
		seen[app.ID] = true
	}
}

// 未知应用必须报错而不是启动进程；未安装的第三方应用返回哨兵错误。
// 该路径只做查表，不会真的 exec。
func TestOpenInExternalAppErrors(t *testing.T) {
	dir := t.TempDir()

	if err := OpenInExternalApp("definitely-not-an-app", dir); err == nil {
		t.Fatal("OpenInExternalApp(unknown app) = nil, want error")
	}

	// iterm 在非 darwin 平台必然未提供；即使装了 iTerm 的 darwin 上，
	// 该断言也只在探测不到包时成立，所以按平台分支断言。
	if runtime.GOOS != "darwin" {
		err := OpenInExternalApp("iterm", dir)
		if !errors.Is(err, errExternalAppUnavailable) {
			t.Fatalf("OpenInExternalApp(iterm) on %s = %v, want errExternalAppUnavailable", runtime.GOOS, err)
		}
	}
}

// 路径必须存在：不存在的路径报错，不创建。
func TestOpenInExternalAppMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := OpenInExternalApp("files", missing); err == nil {
		t.Fatal("OpenInExternalApp(missing path) = nil, want error")
	}
}
