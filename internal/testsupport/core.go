package testsupport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi/sidecar"
)

// Core 描述一次真实内核集成里实际加载的 eos-core。
//
// 证据要求：报告里必须同时给出 Path / SHA256 / ManifestTarget，
// 否则无法区分“测的是哪一份内核”。HEAD 提交号不能标识实际二进制。
type Core struct {
	Path           string
	SHA256         string
	ManifestPath   string
	ManifestTarget string
	ManifestAPI    string
	ManifestMinCLI string
	ManifestVer    string
	ManifestSig    string
	// Features 是 manifest 声明的能力集合。
	Features []string
	// Source 记录解析来源（vendored / embed / override）。
	Source string
}

// String 返回适合写进证据文件的一行摘要。
func (c Core) String() string {
	return fmt.Sprintf("core{path=%s sha256=%s target=%s api=%s source=%s sig=%q}",
		c.Path, c.SHA256, c.ManifestTarget, c.ManifestAPI, c.Source, c.ManifestSig)
}

// sha256File 计算文件摘要，失败时直接 t.Fatalf（证据缺失不允许继续）。
func sha256File(t testing.TB, path string) string {
	t.Helper()
	bs, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("testsupport: read %s for sha256: %v", path, err)
	}
	sum := sha256.Sum256(bs)
	return hex.EncodeToString(sum[:])
}

// LocateRealCore 按产品自己的解析规则定位真实内核。
//
// 优先使用 EOS_CORE_PATH（显式 override），否则走 sidecar.ResolveBinary
// 的默认搜索根 / 内嵌产物 —— 与 `eos` 实际启动路径同源。
//
// 关键：契约测试必须报告“实际加载的那一份”，因此这里返回真实路径与哈希，
// 而不是假定仓库里某个固定开发机路径。
func LocateRealCore(t testing.TB) Core {
	t.Helper()

	// 允许测试显式指定内核；但必须要求文件真实存在，不允许“找不到就跳过”。
	opts := sidecar.ResolveOptions{
		VerifyChecksum:      true,
		AllowDevPlaceholder: !sidecar.ReleaseArtifactCheck(),
	}
	if p := os.Getenv(sidecar.EnvCorePath); p != "" {
		opts.BinaryPath = p
	}
	resolved, err := sidecar.ResolveBinary(opts)
	if err != nil {
		t.Fatalf("BLOCKED: 无法定位真实 eos-core（%v）。P0 契约/集成用例不允许在缺内核时静默跳过；"+
			"请先构建/放置内核，或设置 %s 指向真实二进制。", err, sidecar.EnvCorePath)
	}

	core := Core{Path: resolved.Path, SHA256: sha256File(t, resolved.Path), Source: resolved.Source}
	if core.Source == "" {
		core.Source = "override"
	}

	if resolved.ManifestPath != "" {
		core.ManifestPath = resolved.ManifestPath
		bs, err := os.ReadFile(resolved.ManifestPath)
		if err != nil {
			t.Fatalf("testsupport: read manifest %s: %v", resolved.ManifestPath, err)
		}
		var man sidecar.Manifest
		if err := json.Unmarshal(bs, &man); err != nil {
			t.Fatalf("testsupport: parse manifest %s: %v", resolved.ManifestPath, err)
		}
		core.ManifestTarget = man.Target
		core.ManifestAPI = man.APIVersion
		core.ManifestMinCLI = man.MinCLIVersion
		core.ManifestVer = man.CoreVersion
		core.ManifestSig = man.Signature
		core.Features = man.Features
	}
	if core.ManifestTarget == "" {
		core.ManifestTarget = resolved.Target
	}
	return core
}

// VendoredManifestPath 返回仓库内嵌产物目录里的 manifest 路径。
// 仅用于“产物验收”类用例定位目标三元组，失败即 Fatal（不允许静默跳过）。
func VendoredManifestPath(t testing.TB) string {
	t.Helper()
	target := sidecar.TargetTriple(runtime.GOOS, runtime.GOARCH)
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("testsupport: runtime.Caller failed")
	}
	// internal/testsupport -> 仓库根
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	p := filepath.Join(repoRoot, "pkg", "coreapi", "sidecar", "core", target, "manifest.json")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("BLOCKED: 仓库内嵌产物 manifest 不存在：%s（%v）", p, err)
	}
	return p
}
