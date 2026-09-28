package settings

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "settings.json")
	m := NewManager(path)

	// 路径不存在 → Load 失败
	if _, err := m.Load(); err == nil {
		t.Fatal("missing file")
	}

	s := &Settings{Language: "en", Theme: "light", MaxInjectKB: 64}
	if err := m.Save(s); err != nil {
		t.Fatal(err)
	}
	got, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Language != "en" || got.Theme != "light" || got.MaxInjectKB != 64 {
		t.Fatalf("roundtrip = %+v", got)
	}

	// SetPath
	m.SetPath(filepath.Join(t.TempDir(), "other.json"))
	if _, err := m.Load(); err == nil {
		t.Fatal("new path missing")
	}

	// 空 path
	empty := NewManager("")
	if _, err := empty.Load(); err == nil {
		t.Fatal("empty path load")
	}
	if err := empty.Save(s); err == nil {
		t.Fatal("empty path save")
	}

	// 坏 JSON
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(bad).Load(); err == nil {
		t.Fatal("bad json")
	}
}

func TestLoadMergedAndMerge(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "user.json")
	project := filepath.Join(root, "proj")

	// 全缺 → 默认
	s := LoadMerged(user, project)
	if s == nil {
		t.Fatal("merged nil")
	}

	// user 层
	if err := os.WriteFile(user, []byte(`{"language":"en","theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s = LoadMerged(user, project)
	if s.Language != "en" {
		t.Fatalf("user lang = %q", s.Language)
	}

	// project 覆盖
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	projPath := filepath.Join(project, ".eos", "settings.json")
	if err := os.MkdirAll(filepath.Dir(projPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projPath, []byte(`{"theme":"light"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s = LoadMerged(user, project)
	if s.Theme != "light" {
		t.Fatalf("proj theme = %q", s.Theme)
	}
	if s.Language != "en" {
		t.Fatalf("keep user lang = %q", s.Language)
	}
}
