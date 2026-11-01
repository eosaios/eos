package webbridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDarwinInstallChainWithFakeBins(t *testing.T) {
	fakeBin := t.TempDir()
	mountPoint := t.TempDir()
	appBundle := filepath.Join(mountPoint, "EOS.app")
	if err := os.MkdirAll(appBundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appBundle, "new-marker"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 假 hdiutil：attach 弹 mount-point plist，detach 记账。
	hdiutilCalls := filepath.Join(fakeBin, "hdiutil-calls")
	hdiutil := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + hdiutilCalls + "\n" +
		"if [ \"$1\" = attach ]; then\n" +
		"  printf '<?xml><dict><key>mount-point</key><string>" + mountPoint + "</string></dict>'\n" +
		"fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "hdiutil"), []byte(hdiutil), 0o755); err != nil {
		t.Fatal(err)
	}
	// 假 ditto：目录级复制（真实 ditto 语义的足够子集）。
	ditto := "#!/bin/sh\nmkdir -p \"$2\" && cp -R \"$1\"/. \"$2\"/\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "ditto"), []byte(ditto), 0o755); err != nil {
		t.Fatal(err)
	}
	// 假 open：relaunch 调度断言。
	openCalls := filepath.Join(fakeBin, "open-calls")
	openScript := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + openCalls + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "open"), []byte(openScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	// 挂载：plist 解析回假挂载点。
	got, err := mountMacOSDmg("/fake/eos.dmg")
	if err != nil || got != mountPoint {
		t.Fatalf("mountMacOSDmg = %q, %v", got, err)
	}
	detachMacOSDmg(mountPoint)

	// swap 全链：旧 bundle 原地替换为 dmg 内新 bundle。
	target := filepath.Join(t.TempDir(), "EOS.app")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old-marker"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := swapMacOSBundle("/fake/eos.dmg", target); err != nil {
		t.Fatalf("swapMacOSBundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "new-marker")); err != nil {
		t.Fatalf("替换后应存在新标记: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "old-marker")); !os.IsNotExist(err) {
		t.Fatal("旧内容不应残留")
	}

	// relaunch 调度：1s 后经 PATH 命中假 open。
	bundle := filepath.Join(t.TempDir(), "EOS.app")
	scheduleMacOSRelaunch(bundle)
	// Include system validation of the fresh fake open after the 1s schedule.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(openCalls); err == nil && strings.Contains(string(data), bundle) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("scheduleMacOSRelaunch 未触发假 open")
}
