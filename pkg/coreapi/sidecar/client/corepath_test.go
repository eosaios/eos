package client_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi/sidecar"
)

// resolveE2ECoreBinary 定位供 E2E 测试驱动的真实 eos-core。
//
// 背景：这些用例最初写死了开发机路径（例如
// `C:/home/eos/eos-cli/pkg/coreapi/sidecar/binaries/...`），
// 在任何其它机器上都命中不了 os.Stat，于是整条用例被 t.Skip 静默跳过 ——
// “文件名带 E2E，实际从不执行”。
//
// 现在改为与 `eos` 实际启动同源地解析：
//  1. EOS_CORE_PATH 显式覆盖（便于对着本地构建的内核跑）；
//  2. 否则走 sidecar.ResolveBinary 的默认搜索根 / 内嵌产物。
//
// 找不到时返回空串，由调用方 t.Fatal —— 关键用例不允许在缺内核时
// 静默跳过并声称通过。
func resolveE2ECoreBinary(t *testing.T) string {
	t.Helper()

	// 显式 override 优先，但文件必须真实存在。
	if p := os.Getenv(sidecar.EnvCorePath); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s=%s 指向的内核不存在：%v", sidecar.EnvCorePath, p, err)
		}
		return p
	}

	resolved, err := sidecar.ResolveBinary(sidecar.ResolveOptions{
		VerifyChecksum:      true,
		AllowDevPlaceholder: !sidecar.ReleaseArtifactCheck(),
	})
	if err != nil {
		t.Fatalf("BLOCKED: 解析真实 eos-core 失败：%v。"+
			"P0 集成/E2E 用例不允许因缺内核而静默跳过；"+
			"请构建内核或设置 %s。", err, sidecar.EnvCorePath)
	}
	return resolved.Path
}

// vendoredManifestDir 返回仓库内嵌产物中当前目标的目录名。
//
// 仅用于诊断信息；测试驱动内核一律走 resolveE2ECoreBinary。
func vendoredManifestDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	target := sidecar.TargetTriple(runtime.GOOS, runtime.GOARCH)
	return filepath.Join(root, "pkg", "coreapi", "sidecar", "core", target)
}
