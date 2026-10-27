package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func buildBridgeStdioClientOptions(opts BridgeServiceOptions, coreBinDir string) adapter.StdioClientOptions {
	return adapter.StdioClientOptions{
		CoreLogDir:   DefaultLogDir(),
		CorePath:     firstNonEmptyString(opts.CorePath, opts.AppServerPath),
		ManifestPath: opts.CoreManifestPath,
		CoreBinDir:   coreBinDir,
		ExtraArgs:    append([]string(nil), opts.AppServerArgs...),
		// 启动参数没带 --workspace 时回退到默认工作区 (~/.eos/workspace)，
		// 保证内核 spawn 时 EOS_SANDBOX_WORKSPACE_ROOT 一定非空。
		// 否则内核以 workspace-write 模式启动却无任何可写根，会拒绝所有写
		// 操作（"no writable root is configured"）。活动 workspace 在 session
		// 解析后还会经 sandbox/set_policy RPC 再同步一次（见 applySandboxPolicyRPC）。
		Workspace:           firstNonEmptyString(opts.StartupWorkspace, defaultWorkspacePathFromEnvironment()),
		SandboxMode:         "workspace-write",
		StoreDir:            bridgeCoreStoreDir(),
		VerifyChecksum:      true,
		RequireSignature:    true,
		AllowDevPlaceholder: bridgeAllowDevPlaceholder(),
	}
}

func bridgeCoreStoreDir() string {
	return bridgeDefaultCoreStoreDir()
}

func bridgeAllowDevPlaceholder() bool {
	if strings.EqualFold(strings.TrimSpace(BuildVersion), "dev") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(bridgeRuntimeGatewayAllowDevEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func bridgeDefaultCoreStoreDir() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".eos", "core")
	}
	return filepath.Join(".eos", "core")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
