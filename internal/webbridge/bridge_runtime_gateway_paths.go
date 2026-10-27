package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi/sidecar"
)

func bridgeRuntimeGatewayCoreBinDir() string {
	if envRoot := strings.TrimSpace(os.Getenv(bridgeRuntimeGatewayCoreBinDirEnv)); envRoot != "" {
		if root := firstExistingBridgeCoreBinDir([]string{envRoot}); root != "" {
			return root
		}
	}

	// 候选按优先级排列：
	//   1. exe 同级 core/——dev（wails3 dev，exe 在 output/）→ output/core/ 命中；
	//      便携归档（exe 和 core 同目录）→ exeDir/core 命中；
	//   2. exe/../Resources/core/——macOS .app bundle 布局（可执行文件在
	//      Contents/MacOS/，内核随资源进 Contents/Resources/core/；
	//      core/ 不放 MacOS/ 是 bundle 规范要求，否则 codesign 无法封包）。
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		return firstExistingBridgeCoreBinDir([]string{
			filepath.Join(exeDir, "core"),
			filepath.Join(exeDir, "..", "Resources", "core"),
		})
	}
	return ""
}

func firstExistingBridgeCoreBinDir(candidates []string) string {
	targets := sidecar.TargetTriples(runtime.GOOS, runtime.GOARCH)
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		root := strings.TrimSpace(candidate)
		if root == "" {
			continue
		}
		root = filepath.Clean(root)
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		if sidecar.ExistingTargetTriple(root, targets) != "" {
			return root
		}
	}
	return ""
}
