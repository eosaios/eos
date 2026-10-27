package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

const (
	bridgeRuntimeGatewayEnv              = "EOS_GUI_RUNTIME_GATEWAY"
	bridgeRuntimeGatewayAppServerEnv     = "EOS_GUI_APP_SERVER_PATH"
	bridgeRuntimeGatewayAppServerArgsEnv = "EOS_GUI_APP_SERVER_ARGS"
	bridgeRuntimeGatewayStartTimeoutEnv  = "EOS_GUI_APP_SERVER_START_TIMEOUT_MS"
	bridgeRuntimeGatewayCorePathEnv      = "EOS_GUI_CORE_PATH"
	bridgeRuntimeGatewayCoreManifestEnv  = "EOS_GUI_CORE_MANIFEST"
	bridgeRuntimeGatewayCoreBinDirEnv    = "EOS_GUI_CORE_BIN_DIR"
	bridgeRuntimeGatewayAllowDevEnv      = "EOS_GUI_ALLOW_DEV_PLACEHOLDER"

	bridgeRuntimeGatewayModeRust   = "rust-stdio"
	bridgeRuntimeGatewayModeLegacy = "legacy"

	defaultBridgeRuntimeGatewayStartTimeout = 5 * time.Second
)

// BridgeServiceOptions controls runtime wiring for the Wails bridge. The
// default desktop path starts eos-core --stdio.
type BridgeServiceOptions struct {
	LogFile           string
	StartupWorkspace  string
	RuntimeGateway    string
	AppServerPath     string
	CorePath          string
	CoreManifestPath  string
	AppServerArgs     []string
	StdioStartTimeout time.Duration
}

type bridgeStdioGatewayStarter func(context.Context, adapter.StdioClientOptions) (bridgeRuntimeGateway, func() error, adapter.StdioResolvedBinary, error)

var startBridgeStdioGateway bridgeStdioGatewayStarter = startBridgeStdioGatewayProcess

func NewBridgeServiceWithOptions(opts BridgeServiceOptions) *BridgeService {
	opts = normalizeBridgeServiceOptions(opts)
	return newBridgeServiceWithDefaults(opts)
}

func (s *BridgeService) configureRuntimeGateway(opts BridgeServiceOptions) {
	if s == nil {
		return
	}
	mode, modeErr := normalizeBridgeRuntimeGatewayMode(opts.RuntimeGateway)
	s.runtimeGatewayMode = mode
	if modeErr != nil {
		s.runtimeGatewayStartError = modeErr.Error()
		slog.Warn("bridge.runtime_gateway.mode.unsupported", "mode", opts.RuntimeGateway, "error", modeErr)
		return
	}
	if mode == bridgeRuntimeGatewayModeLegacy {
		s.runtimeGatewayStartError = "legacy Go runtime is not available in default builds; rebuild with -tags legacy to use EOS_GUI_RUNTIME_GATEWAY=legacy"
		slog.Warn("bridge.runtime_gateway.legacy_unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.StdioStartTimeout)
	defer cancel()

	coreBinDir := bridgeRuntimeGatewayCoreBinDir()
	s.runtimeGatewayCoreBinDir = coreBinDir
	gateway, closeGateway, resolvedBinary, err := startBridgeStdioGateway(ctx, buildBridgeStdioClientOptions(opts, coreBinDir))
	s.runtimeGatewayResolvedBinary = resolvedBinary
	if err != nil {
		s.runtimeGatewayStartError = err.Error()
		slog.Warn("bridge.runtime_gateway.rust.start_failed",
			"core", firstNonEmptyString(opts.CorePath, opts.AppServerPath),
			"root_dir", strings.TrimSpace(coreBinDir),
			"resolved_path", strings.TrimSpace(resolvedBinary.Path),
			"resolved_manifest", strings.TrimSpace(resolvedBinary.ManifestPath),
			"resolved_source", strings.TrimSpace(resolvedBinary.Source),
			"workspace", opts.StartupWorkspace,
			"error", err,
		)
		return
	}
	if gateway == nil {
		s.runtimeGatewayStartError = "rust stdio gateway starter returned nil gateway"
		slog.Warn("bridge.runtime_gateway.rust.start_failed", "error", s.runtimeGatewayStartError)
		return
	}

	s.runtimeGateway = gateway
	s.runtimeGatewayClose = closeGateway
	s.runtimeGatewayMode = bridgeRuntimeGatewayModeRust
	s.runtimeGatewayStartError = ""
	slog.Info("bridge.runtime_gateway.rust.ready",
		"core", firstNonEmptyString(opts.CorePath, opts.AppServerPath),
		"root_dir", strings.TrimSpace(coreBinDir),
		"resolved_path", strings.TrimSpace(resolvedBinary.Path),
		"resolved_manifest", strings.TrimSpace(resolvedBinary.ManifestPath),
		"resolved_source", strings.TrimSpace(resolvedBinary.Source),
		"workspace", opts.StartupWorkspace,
	)
}
