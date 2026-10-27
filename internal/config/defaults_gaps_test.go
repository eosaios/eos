package config

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// 批十四批测：DefaultLogDir 平台分支 / saveLocked 落盘失败臂（经 Save 触发）。

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultLogDirPlatformBranch(t *testing.T) {
	dir := DefaultLogDir()
	if dir == "" {
		t.Fatal("DefaultLogDir empty")
	}
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(filepath.ToSlash(dir), "Library/Logs/EOS") {
			t.Fatalf("darwin log dir = %q", dir)
		}
	case "windows":
		if !strings.Contains(dir, "EOS") || !strings.Contains(dir, "logs") {
			t.Fatalf("windows log dir = %q", dir)
		}
	default:
		if !strings.Contains(filepath.ToSlash(dir), "eos/logs") {
			t.Fatalf("linux log dir = %q", dir)
		}
	}
}

func TestSaveFailureArms(t *testing.T) {
	// 目标路径的父目录被同名文件占据 → MkdirAll 失败。
	blocker := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Active: "m1"}
	if err := Save(cfg, filepath.Join(blocker, "sub", "config.json")); err == nil {
		t.Fatal("Save under blocked dir error = nil")
	}

	// tmp 写失败：目标本身是目录（WriteFile 落在目录上失败）。
	dirTarget := filepath.Join(t.TempDir(), "as-dir")
	if err := os.MkdirAll(dirTarget+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Save(cfg, dirTarget); err == nil {
		t.Fatal("Save onto dir-backed path error = nil")
	}
}
