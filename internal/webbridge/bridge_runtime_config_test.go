package webbridge

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
)

func TestBridgeRuntimeGatewayDefaultIsRustStdio(t *testing.T) {
	mode, err := normalizeBridgeRuntimeGatewayMode("")
	if err != nil {
		t.Fatalf("normalizeBridgeRuntimeGatewayMode(default) error = %v", err)
	}
	if mode != bridgeRuntimeGatewayModeRust {
		t.Fatalf("mode=%q, want %q", mode, bridgeRuntimeGatewayModeRust)
	}
}

func TestConfigureRuntimeGatewayStartsRustCore(t *testing.T) {
	oldStarter := startBridgeStdioGateway
	var got adapter.StdioClientOptions
	resolved := adapter.StdioResolvedBinary{
		Path:         `C:\bin\eos-core.exe`,
		ManifestPath: `C:\bin\manifest.json`,
		Source:       "test",
		Target:       "x86_64-pc-windows-gnu",
	}
	startBridgeStdioGateway = func(_ context.Context, opts adapter.StdioClientOptions) (bridgeRuntimeGateway, func() error, adapter.StdioResolvedBinary, error) {
		got = opts
		return adapter.NewStdioGateway(&adapter.StdioClient{}), func() error { return nil }, resolved, nil
	}
	t.Cleanup(func() { startBridgeStdioGateway = oldStarter })

	service := &BridgeService{}
	service.configureRuntimeGateway(BridgeServiceOptions{
		StartupWorkspace:  `C:\work\eos-gui`,
		CorePath:          `C:\bin\eos-core.exe`,
		CoreManifestPath:  `C:\bin\manifest.json`,
		StdioStartTimeout: time.Second,
	})

	if service.runtimeGatewayMode != bridgeRuntimeGatewayModeRust {
		t.Fatalf("runtimeGatewayMode=%q, want %q", service.runtimeGatewayMode, bridgeRuntimeGatewayModeRust)
	}
	if service.runtimeGateway == nil {
		t.Fatal("runtimeGateway is nil")
	}
	if got.CorePath != `C:\bin\eos-core.exe` {
		t.Fatalf("CorePath=%q", got.CorePath)
	}
	if got.ManifestPath != `C:\bin\manifest.json` {
		t.Fatalf("ManifestPath=%q", got.ManifestPath)
	}
	if service.runtimeGatewayResolvedBinary != resolved {
		t.Fatalf("runtimeGatewayResolvedBinary=%+v, want %+v", service.runtimeGatewayResolvedBinary, resolved)
	}
	if got.Workspace != `C:\work\eos-gui` {
		t.Fatalf("Workspace=%q", got.Workspace)
	}
	if !got.VerifyChecksum || !got.RequireSignature {
		t.Fatalf("sidecar verification disabled: %+v", got)
	}
	if !strings.HasSuffix(got.StoreDir, `.eos\core`) && !strings.HasSuffix(got.StoreDir, `.eos/core`) {
		t.Fatalf("StoreDir=%q, want workspace .eos/core", got.StoreDir)
	}
}

func TestConfigureRuntimeGatewayUsesGlobalStoreWithoutStartupWorkspace(t *testing.T) {
	oldStarter := startBridgeStdioGateway
	var got adapter.StdioClientOptions
	startBridgeStdioGateway = func(_ context.Context, opts adapter.StdioClientOptions) (bridgeRuntimeGateway, func() error, adapter.StdioResolvedBinary, error) {
		got = opts
		return adapter.NewStdioGateway(&adapter.StdioClient{}), func() error { return nil }, adapter.StdioResolvedBinary{}, nil
	}
	t.Cleanup(func() { startBridgeStdioGateway = oldStarter })

	service := &BridgeService{}
	service.configureRuntimeGateway(BridgeServiceOptions{
		StdioStartTimeout: time.Second,
	})

	// 没有 --workspace 启动参数时，Workspace 应回退到默认工作区
	// (~/.eos/workspace)，保证内核 EOS_SANDBOX_WORKSPACE_ROOT 非空，
	// 否则 workspace-write 模式会拒绝所有写操作。
	if got.Workspace == "" {
		t.Fatalf("Workspace empty, want default workspace fallback")
	}
	if got.Workspace != defaultWorkspacePathFromEnvironment() {
		t.Fatalf("Workspace=%q, want default %q", got.Workspace, defaultWorkspacePathFromEnvironment())
	}
	// StoreDir 仍走全局 ~/.eos/core（不随 workspace 回退而变）。
	if !strings.HasSuffix(got.StoreDir, `.eos\core`) && !strings.HasSuffix(got.StoreDir, `.eos/core`) {
		t.Fatalf("StoreDir=%q, want global .eos/core", got.StoreDir)
	}
}

func TestBridgeAllowDevPlaceholderEnabledForDevBuild(t *testing.T) {
	original := BuildVersion
	BuildVersion = "dev"
	t.Cleanup(func() {
		BuildVersion = original
	})
	t.Setenv(bridgeRuntimeGatewayAllowDevEnv, "")

	if !bridgeAllowDevPlaceholder() {
		t.Fatal("bridgeAllowDevPlaceholder() = false, want true for dev build")
	}
}

func TestConfigureRuntimeGatewayRejectsLegacyInDefaultBuild(t *testing.T) {
	service := &BridgeService{}
	service.configureRuntimeGateway(BridgeServiceOptions{
		RuntimeGateway:    "legacy",
		StdioStartTimeout: time.Second,
	})
	if service.runtimeGateway != nil {
		t.Fatalf("runtimeGateway=%T, want nil", service.runtimeGateway)
	}
	if service.runtimeGatewayMode != bridgeRuntimeGatewayModeLegacy {
		t.Fatalf("runtimeGatewayMode=%q, want legacy", service.runtimeGatewayMode)
	}
	if !strings.Contains(service.runtimeGatewayStartError, "legacy") {
		t.Fatalf("runtimeGatewayStartError=%q, want legacy message", service.runtimeGatewayStartError)
	}
}

func TestBridgeRuntimeGatewayCoreBinDirFindsVendoredSidecarFromEnv(t *testing.T) {
	root := t.TempDir()

	targets := sidecar.TargetTriples(runtime.GOOS, runtime.GOARCH)
	if len(targets) == 0 {
		t.Fatal("TargetTriples returned no targets")
	}
	triple := targets[len(targets)-1]
	coreDir := filepath.Join(root, "core")
	manifestPath := filepath.Join(coreDir, triple, sidecar.DefaultManifestName)
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(manifestPath dir) error = %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(`{"target":"`+triple+`"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(manifestPath) error = %v", err)
	}

	oldEnv := os.Getenv(bridgeRuntimeGatewayCoreBinDirEnv)
	t.Setenv(bridgeRuntimeGatewayCoreBinDirEnv, coreDir)
	t.Cleanup(func() {
		if oldEnv == "" {
			os.Unsetenv(bridgeRuntimeGatewayCoreBinDirEnv)
		} else {
			os.Setenv(bridgeRuntimeGatewayCoreBinDirEnv, oldEnv)
		}
	})

	if rootDir := bridgeRuntimeGatewayCoreBinDir(); rootDir != coreDir {
		t.Fatalf("bridgeRuntimeGatewayCoreBinDir()=%q, want %q", rootDir, coreDir)
	}
}
