package sidecar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 内嵌内核分发通道：`go install github.com/eosaios/eos@vX.Y.Z` 只把
// eos.exe 装进 GOBIN——旁边既没有 release 布局的 core/ 目录，也没有源码树，
// 原有两个解析根必然落空。本通道把当前平台的 vendored 内核（go:embed 进
// 二进制，随仓库一起分发）释放到用户缓存目录，作为第三条解析来源：
//
//	解析顺序：EOS_CORE_PATH / EOS_CORE_MANIFEST（显式指定）
//	         → exeDir/core、源码树/core（release 布局 / dev）
//	         → 内嵌释放缓存（go install 布局）
//
// 内嵌产物带 manifest（sha256 + Ed25519 签名），释放前先在内存校验内容
// 与 manifest 一致，落盘后走与文件分发完全相同的 resolveManifest 验签流程。

// embeddedCoreSidecar 由各平台的 embedded_*.go 按 GOOS/GOARCH 提供（仅当前
// 平台的产物会内嵌，控制体积）。nil 表示当前平台没有内嵌（如未来新平台）。
var embeddedCoreSidecar func() (bin, manifest []byte, ok bool)

// embeddedCacheDir 内嵌内核的释放根目录，测试可替换。
var embeddedCacheDir = defaultEmbeddedCacheDir

func defaultEmbeddedCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "eos", "core"), nil
}

// materializeEmbedded 将内嵌内核释放到缓存目录，返回 manifest 路径。
// 目录按 manifest sha256 前缀寻址（core/<triple>/<sha12>/）：新版本内核
// 写新目录，不与可能正在运行的旧内核 exe 产生 Windows 文件锁冲突；写入
// 成功后尽力清理同 triple 下的旧版本目录（失败忽略，下次再清）。
// 已释放且校验一致时直接复用（幂等）。
func materializeEmbedded(goos, goarch string) (string, error) {
	if embeddedCoreSidecar == nil {
		return "", fmt.Errorf("%w: no embedded core sidecar for %s/%s", ErrCoreBinaryNotFound, goos, goarch)
	}
	bin, manifestBytes, ok := embeddedCoreSidecar()
	if !ok || len(bin) == 0 || len(manifestBytes) == 0 {
		return "", fmt.Errorf("%w: embedded core sidecar empty", ErrCoreBinaryNotFound)
	}
	manifest, err := LoadManifestBytes(manifestBytes)
	if err != nil {
		return "", err
	}
	if err := manifest.VerifyBinaryBytes(bin); err != nil {
		return "", err
	}
	triples := TargetTriples(goos, goarch)
	if err := manifest.RequireTarget(triples); err != nil {
		return "", err
	}

	root, err := embeddedCacheDir()
	if err != nil {
		return "", err
	}
	want, err := parseSHA256(manifest.SHA256)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	// 缓存目录用 manifest 自身的 target（产物权威值；Windows 上宿主主选
	// msvc 而 vendored 产物是 gnu，按候选列表校验、按产物实际 triple 落盘）。
	dest := filepath.Join(root, manifest.Target, want[:12])
	manifestPath := filepath.Join(dest, DefaultManifestName)
	binPath := filepath.Join(dest, manifest.Binary)

	// 幂等复用：已释放且内容校验一致则不再写盘。
	if current, err := LoadManifest(manifestPath); err == nil {
		if current.SHA256 == manifest.SHA256 {
			if err := current.VerifyBinary(binPath); err == nil {
				return manifestPath, nil
			}
		}
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	binMode := os.FileMode(0o644)
	if isExecutableBinaryName(manifest.Binary) {
		binMode = 0o755
	}
	if err := writeFileAtomic(binPath, bin, binMode); err != nil {
		return "", err
	}
	if err := writeFileAtomic(manifestPath, manifestBytes, 0o644); err != nil {
		return "", err
	}
	cleanupStaleEmbedded(root, manifest.Target, want[:12])
	return manifestPath, nil
}

// cleanupStaleEmbedded 删除同 triple 下非当前 sha 前缀的旧目录（尽力而为：
// 旧内核正在运行被锁时删除失败，忽略即可，下次释放时再清）。
func cleanupStaleEmbedded(root, triple, keep string) {
	parent := filepath.Join(root, triple)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == keep {
			continue
		}
		_ = os.RemoveAll(filepath.Join(parent, e.Name()))
	}
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func isExecutableBinaryName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "eos-core" || name == "eos-core.exe"
}
