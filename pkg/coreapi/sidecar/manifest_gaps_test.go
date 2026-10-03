package sidecar

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// manifest 余臂批测：Load/LoadManifestBytes 错误族与 Validate 九臂矩阵。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validManifestJSON() string {
	return `{"schema_version":"` + ManifestSchemaVersion + `","core_version":"0.1.0",` +
		`"api_version":"` + DefaultAPIVersion + `","target":"aarch64-apple-darwin",` +
		`"binary":"eos-core","sha256":"` + strings.Repeat("ab", 32) + `"}`
}

func TestLoadManifestErrors(t *testing.T) {
	// 空 path。
	if _, err := LoadManifest("  "); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("空 path = %v", err)
	}
	// 文件不可读。
	if _, err := LoadManifest(filepath.Join(t.TempDir(), "gone.json")); err == nil {
		t.Fatal("缺文件应报错")
	}
	// 经文件的成功链。
	path := filepath.Join(t.TempDir(), "m.json")
	os.WriteFile(path, []byte(validManifestJSON()), 0o644)
	if m, err := LoadManifest(path); err != nil || m.Binary != "eos-core" {
		t.Fatalf("成功链 = %+v, %v", m, err)
	}
}

func TestLoadManifestBytesErrors(t *testing.T) {
	if _, err := LoadManifestBytes([]byte("{bad json")); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("坏 JSON = %v", err)
	}
	if _, err := LoadManifestBytes([]byte(validManifestJSON())); err != nil {
		t.Fatalf("合法清单 = %v", err)
	}
}

func TestManifestValidateMatrix(t *testing.T) {
	valid := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   "0.1.0",
		APIVersion:    DefaultAPIVersion,
		Target:        "x",
		Binary:        "eos-core",
		SHA256:        strings.Repeat("ab", 32),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("基线应合法: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"缺 schema", func(m *Manifest) { m.SchemaVersion = "" }, "schema_version is required"},
		{"schema 不符", func(m *Manifest) { m.SchemaVersion = "9.9.9" }, "unsupported schema_version"},
		{"缺 core", func(m *Manifest) { m.CoreVersion = " " }, "core_version is required"},
		{"缺 api", func(m *Manifest) { m.APIVersion = "" }, "api_version is required"},
		{"缺 target", func(m *Manifest) { m.Target = "" }, "target is required"},
		{"缺 binary", func(m *Manifest) { m.Binary = "" }, "binary is required"},
		{"缺 sha", func(m *Manifest) { m.SHA256 = " " }, "sha256 is required"},
		{"sha 非法", func(m *Manifest) { m.SHA256 = "nope" }, "invalid"},
	}
	for _, tc := range cases {
		m := valid
		tc.mutate(&m)
		err := m.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.Is(err, ErrManifestInvalid) {
			t.Fatalf("%s = %v（want 含 %q）", tc.name, err, tc.want)
		}
	}
}
