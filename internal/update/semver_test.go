package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		// 基本数值比较（字符串比较会翻车的场景全部覆盖）
		{"1.0.0-beta.3", "1.0.0-beta.10", -1},
		{"1.0.0-beta.10", "1.0.0-beta.3", 1},
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.9.0", "1.10.0", -1}, // 字符串比较会把 10 判小于 9
		{"2.0.0", "1.99.99", 1},
		// prerelease 语义：正式版 > 预发布
		{"1.0.0-beta.3", "1.0.0", -1},
		{"1.0.0", "1.0.0-beta.3", 1},
		{"1.0.0-rc.1", "1.0.0-beta.9", 1},
		// 数字标识 < 字母标识；段数多者大
		{"1.0.0-beta.2", "1.0.0-beta.alpha", -1},
		{"1.0.0-beta", "1.0.0-beta.1", -1},
		// 宽容输入
		{"1.0", "1.0.0", 0},
		{"1.0.0+build.5", "1.0.0", 0}, // build metadata 不参与比较
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !isNewer("v1.0.0-beta.3", "v1.0.0-beta.10") {
		t.Error("beta.10 should be newer than beta.3")
	}
	if isNewer("v1.0.0-beta.10", "v1.0.0-beta.3") {
		t.Error("beta.3 must not be newer than beta.10")
	}
	if isNewer("v1.0.0", "v1.0.0") {
		t.Error("same version must not be newer")
	}
	if isNewer("", "v1.0.0") || isNewer("v1.0.0", "") {
		t.Error("empty version must not count as newer")
	}
}

func TestPlatformAssetName(t *testing.T) {
	cases := []struct {
		tag, goos, goarch string
		want              string
	}{
		{"v1.0.0-beta.3", "darwin", "arm64", "eos-cli_v1.0.0-beta.3_darwin-arm64.tar.gz"},
		{"v1.0.0-beta.3", "darwin", "amd64", "eos-cli_v1.0.0-beta.3_darwin-amd64.tar.gz"},
		{"v1.0.0-beta.3", "linux", "amd64", "eos-cli_v1.0.0-beta.3_linux-amd64.tar.gz"},
		{"v1.0.0-beta.3", "windows", "amd64", "eos-cli_v1.0.0-beta.3_windows-amd64.zip"},
	}
	for _, c := range cases {
		got, _ := platformAssetName(c.tag, c.goos, c.goarch)
		if got != c.want {
			t.Errorf("platformAssetName(%q,%q,%q) = %q, want %q", c.tag, c.goos, c.goarch, got, c.want)
		}
	}
}
