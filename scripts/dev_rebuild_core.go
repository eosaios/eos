//go:build ignore

package main

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// dev_rebuild_core.go：本地开发内核重建（macOS/Linux 版，Windows 用
// scripts/dev-rebuild.ps1）——改了 eos-core-rs 但还没发版时，在本机编译
// 新内核并 stage 到本仓库 pkg/coreapi/sidecar/core/<triple>/（resolver 的
// 源码树搜索根），`go run .` 直接用上新内核，无需任何环境变量。
//
// 与 eos-app 仓库同名脚本同构，差异仅在产物位置：桌面端写 output/core/
// （wails3 dev 读路径），CLI 写 vendored core/（go run . / go build 的
// 源码树搜索根；该目录 git 跟踪 CI 签名产物，本地 stage 后显示为工作区
// 修改——勿提交，CI sync-vendored-sidecar 会用签名版还原）。
//
// 签名说明：本机无私钥（生产 Ed25519 私钥只在 GitHub Secrets），写入
// unsigned-development-placeholder 占位签名。占位仅 dev 模式（未设
// EOS_RELEASE_ARTIFACT_CHECK）被放行：TUI/exec/print 启动路径的
// AllowDevPlaceholder 绑定 release 门禁，release 产物强制拒绝占位。
// sha256 校验仍然强制——manifest 里的 sha256 必须与新二进制一致。
//
// 用法（在 eos-cli 仓库根执行）：
//
//	go run ./scripts/dev_rebuild_core.go                     # debug 内核
//	go run ./scripts/dev_rebuild_core.go -release            # release 内核
//	EOS_CORE_REPO=../eos-core-rs go run ./scripts/dev_rebuild_core.go

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi/sidecar"
)

const devPlaceholderSignature = "unsigned-development-placeholder"

func fail(format string, args ...any) {
	fmt.Printf("dev_rebuild_core: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	release := flag.Bool("release", false, "构建 release 内核（默认 debug）")
	coreRepo := flag.String("core-repo", os.Getenv("EOS_CORE_REPO"), "eos-core-rs 仓库路径（默认 ../eos-core-rs）")
	flag.Parse()

	repoRoot, err := os.Getwd()
	if err != nil {
		fail("解析仓库根失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		fail("请在 eos-cli 仓库根执行（缺 go.mod）: %s", repoRoot)
	}
	coreRepoPath := strings.TrimSpace(*coreRepo)
	if coreRepoPath == "" {
		coreRepoPath = filepath.Join("..", "eos-core-rs")
	}
	coreRepoPath, err = filepath.Abs(coreRepoPath)
	if err != nil {
		fail("解析内核仓库路径失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(coreRepoPath, "Cargo.toml")); err != nil {
		fail("内核仓库不存在（%s）: 缺 Cargo.toml", coreRepoPath)
	}

	// 1) 本地编译内核。
	profile := "debug"
	cargoArgs := []string{"build", "-p", "eos-core-bin"}
	if *release {
		profile = "release"
		cargoArgs = append(cargoArgs, "--release")
	}
	fmt.Printf("[1/3] cargo %s ...\n", strings.Join(cargoArgs, " "))
	cargo := exec.Command("cargo", cargoArgs...)
	cargo.Dir = coreRepoPath
	cargo.Stdout, cargo.Stderr = os.Stdout, os.Stderr
	if err := cargo.Run(); err != nil {
		fail("cargo build 失败: %v", err)
	}
	binPath := filepath.Join(coreRepoPath, "target", profile, coreBinaryName())

	// 2) 以 vendored core/<triple>/manifest.json 为模板（保留 core_version/
	//    api_version/features 等），更新 sha256 并置 dev placeholder 签名。
	triples := sidecar.TargetTriples(runtime.GOOS, runtime.GOARCH)
	if len(triples) == 0 {
		fail("无法映射 host target %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	vendoredDir := filepath.Join(repoRoot, "pkg", "coreapi", "sidecar", "core")
	baseManifest := findBaseManifest(vendoredDir, triples)
	if baseManifest == "" {
		fail("找不到模板 manifest（%s/<triple>/manifest.json）", vendoredDir)
	}
	manifest, err := readManifestJSON(baseManifest)
	if err != nil {
		fail("读取模板 manifest 失败: %v", err)
	}
	binBytes, err := os.ReadFile(binPath)
	if err != nil {
		fail("读取新内核失败: %v", err)
	}
	sum := sha256.Sum256(binBytes)
	manifest["sha256"] = "sha256:" + hex.EncodeToString(sum[:])
	// 模板可能来自另一 target 目录，target 字段必须改写为本目录 triple。
	manifest["target"] = triples[0]
	manifest["signature"] = devPlaceholderSignature
	manifest["signature_algorithm"] = ""

	// 3) 写回 vendored core/<triple>/（go run . / go build 的源码树搜索根）。
	outDir := filepath.Join(vendoredDir, triples[0])
	outBin := filepath.Join(outDir, coreBinaryName())
	outManifest := filepath.Join(outDir, "manifest.json")
	if err := os.WriteFile(outBin, binBytes, 0o755); err != nil {
		fail("写入内核失败: %v", err)
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fail("序列化 manifest 失败: %v", err)
	}
	if err := os.WriteFile(outManifest, append(manifestBytes, '\n'), 0o644); err != nil {
		fail("写入 manifest 失败: %v", err)
	}

	fmt.Printf("[2/3] 内核已写入 %s\n", outBin)
	fmt.Printf("[3/3] 完成（%s，sha256 已更新，dev placeholder 签名）。`go run .` 直接使用新内核。\n", profile)
	fmt.Printf("注意：vendored core/ 是 git 跟踪目录，本次 stage 会显示为工作区修改——勿提交，\n")
	fmt.Printf("      CI sync-vendored-sidecar 会用签名版内核还原该目录。\n")
}

func coreBinaryName() string {
	if runtime.GOOS == "windows" {
		return "eos-core.exe"
	}
	return "eos-core"
}

// findBaseManifest 在 vendored 目录下按 host 的 target 优先级找模板 manifest。
func findBaseManifest(vendoredDir string, triples []string) string {
	for _, triple := range triples {
		p := filepath.Join(vendoredDir, triple, "manifest.json")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func readManifestJSON(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}
