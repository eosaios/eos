package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func defaultBridgeServiceOptions(logFile, startupWorkspace string) BridgeServiceOptions {
	return BridgeServiceOptions{
		LogFile:           strings.TrimSpace(logFile),
		StartupWorkspace:  strings.TrimSpace(startupWorkspace),
		RuntimeGateway:    os.Getenv(bridgeRuntimeGatewayEnv),
		AppServerPath:     os.Getenv(bridgeRuntimeGatewayAppServerEnv),
		CorePath:          os.Getenv(bridgeRuntimeGatewayCorePathEnv),
		CoreManifestPath:  os.Getenv(bridgeRuntimeGatewayCoreManifestEnv),
		AppServerArgs:     splitBridgeRuntimeGatewayArgs(os.Getenv(bridgeRuntimeGatewayAppServerArgsEnv)),
		StdioStartTimeout: bridgeRuntimeGatewayStartTimeoutFromEnv(),
	}
}

func normalizeBridgeServiceOptions(opts BridgeServiceOptions) BridgeServiceOptions {
	opts.LogFile = strings.TrimSpace(opts.LogFile)
	opts.StartupWorkspace = strings.TrimSpace(opts.StartupWorkspace)
	opts.RuntimeGateway = strings.TrimSpace(opts.RuntimeGateway)
	opts.AppServerPath = strings.TrimSpace(opts.AppServerPath)
	opts.CorePath = strings.TrimSpace(opts.CorePath)
	opts.CoreManifestPath = strings.TrimSpace(opts.CoreManifestPath)
	opts.AppServerArgs = compactStrings(opts.AppServerArgs)
	if opts.StdioStartTimeout <= 0 {
		opts.StdioStartTimeout = defaultBridgeRuntimeGatewayStartTimeout
	}
	return opts
}

func splitBridgeRuntimeGatewayArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}

func bridgeRuntimeGatewayStartTimeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(bridgeRuntimeGatewayStartTimeoutEnv))
	if raw == "" {
		return defaultBridgeRuntimeGatewayStartTimeout
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return defaultBridgeRuntimeGatewayStartTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

func normalizeBridgeRuntimeGatewayMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "default", "rust", "rust-stdio", "rust_stdio", "stdio", "core", "eos-core", "external":
		return bridgeRuntimeGatewayModeRust, nil
	case "legacy", "runtime", "inprocess", "in-process", "in_process":
		return bridgeRuntimeGatewayModeLegacy, nil
	default:
		return bridgeRuntimeGatewayModeRust, fmt.Errorf("unsupported %s=%q", bridgeRuntimeGatewayEnv, strings.TrimSpace(mode))
	}
}
