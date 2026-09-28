//go:build darwin

package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacOSAppBundlePath(t *testing.T) {
	cases := []struct {
		name string
		exe  string
		want string
		ok   bool
	}{
		{
			name: "典型布局 /Applications",
			exe:  "/Applications/EOS.app/Contents/MacOS/EOS",
			want: "/Applications/EOS.app",
			ok:   true,
		},
		{
			name: "用户目录布局",
			exe:  "/Users/you/Applications/EOS.app/Contents/MacOS/EOS",
			want: "/Users/you/Applications/EOS.app",
			ok:   true,
		},
		{
			name: "裸二进制无 bundle",
			exe:  "/usr/local/bin/eos-app",
			want: "",
			ok:   false,
		},
		{
			name: "go run 临时目录无 bundle",
			exe:  "/var/folders/x/GOTMP/eos-app",
			want: "",
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := macOSAppBundlePath(tc.exe)
			if ok != tc.ok || got != tc.want {
				t.Errorf("macOSAppBundlePath(%q) = (%q, %v), want (%q, %v)", tc.exe, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestHdiutilMountPoint(t *testing.T) {
	plist := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><array><dict>
<key>blocksize</key><integer>512</integer>
<key>mount-point</key><string>/Volumes/EOS 1.0</string>
<key>dev-entry</key><string>/dev/disk4s1</string>
</dict></array></dict></plist>`
	if got := hdiutilMountPoint(plist); got != "/Volumes/EOS 1.0" {
		t.Errorf("hdiutilMountPoint = %q, want /Volumes/EOS 1.0", got)
	}

	escaped := `<key>mount-point</key><string>/Volumes/EOS &amp; Co</string>`
	if got := hdiutilMountPoint(escaped); got != "/Volumes/EOS & Co" {
		t.Errorf("XML 转义未解码: %q", got)
	}

	if got := hdiutilMountPoint("<plist><dict/></plist>"); got != "" {
		t.Errorf("缺少 mount-point 时应返回空串, got %q", got)
	}
}

func TestFindMacOSAppBundle(t *testing.T) {
	volume := t.TempDir()
	if err := os.MkdirAll(filepath.Join(volume, "EOS.app", "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volume, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := findMacOSAppBundle(volume)
	if err != nil {
		t.Fatalf("findMacOSAppBundle: %v", err)
	}
	if got != filepath.Join(volume, "EOS.app") {
		t.Errorf("findMacOSAppBundle = %q, want %q", got, filepath.Join(volume, "EOS.app"))
	}

	empty := t.TempDir()
	if _, err := findMacOSAppBundle(empty); err == nil {
		t.Error("无 .app 时应报错")
	}
}

func TestReplaceMacOSBundle(t *testing.T) {
	parent := t.TempDir()
	source := filepath.Join(parent, "src", "EOS.app")
	target := filepath.Join(parent, "Applications", "EOS.app")
	for _, dir := range []string{
		filepath.Join(source, "Contents", "MacOS"),
		filepath.Join(target, "Contents", "MacOS"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExe := func(root, version string) error {
		return os.WriteFile(filepath.Join(root, "Contents", "MacOS", "EOS"), []byte(version), 0o755)
	}
	if err := writeExe(source, "v2"); err != nil {
		t.Fatal(err)
	}
	if err := writeExe(target, "v1"); err != nil {
		t.Fatal(err)
	}

	if err := replaceMacOSBundle(source, target); err != nil {
		t.Fatalf("replaceMacOSBundle: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(target, "Contents", "MacOS", "EOS"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2" {
		t.Errorf("替换后版本 = %q, want v2", string(got))
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".EOS.app.") {
			t.Errorf("暂存/备份目录未清理: %s", entry.Name())
		}
	}
}

func TestReplaceMacOSBundleRollback(t *testing.T) {
	parent := t.TempDir()
	source := filepath.Join(parent, "missing", "EOS.app") // 不存在的源 → 复制必败
	target := filepath.Join(parent, "EOS.app")
	if err := os.MkdirAll(filepath.Join(target, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceMacOSBundle(source, target); err == nil {
		t.Fatal("源不存在时应报错")
	}
	if _, err := os.Stat(filepath.Join(target, "Contents")); err != nil {
		t.Errorf("失败后旧 bundle 应原样保留: %v", err)
	}
}
