package architecture

// 本文件守护 engineprovider.Select 的失败路径（Rust-only，无回退）：
//   1. eos-core 二进制不存在：直接 error。
//   2. eos-core 子进程启动但 initialize 失败：直接 error。
//   3. 子进程 initialize 成功但缺少 required methods：直接 error。
//   4. 协议不匹配（initialize 报协议错误）：直接 error。
//   5. manifest 与 binary checksum 不匹配：直接 error。
//   6. manifest 签名错误：直接 error。
//   7. target / feature 不匹配：直接 error。
//
// 守护 invariant：任何 eos-core 解析/握手失败都必须 fail-fast，引擎已收敛为
// Rust-only，不再存在 legacy 回退路径。

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/coreapi/engineprovider"
	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	sidecarclient "github.com/eosaios/eos/pkg/coreapi/sidecar/client"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

func requiredMethodsForCutover() []string {
	return append([]string(nil), sidecarclient.RequiredMethods...)
}

// TestEngineProviderRejectsMissingRustBinary 验证当 eos-core 二进制不存在时，
// engineprovider.Select 直接返回 error。
func TestEngineProviderRejectsMissingRustBinary(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EOS_CORE_PATH", filepath.Join(dir, "no-such-binary"))

	_, err := engineprovider.Select(context.Background(), engineprovider.Options{
		Mode:            engineprovider.ModeAuto,
		Sidecar:         sidecar.ProcessOptions{},
		RequiredMethods: requiredMethodsForCutover(),
	})
	if err == nil {
		t.Fatal("Select() succeeded with missing Rust binary; expected error")
	}
}

// TestEngineProviderRejectsInitializeFailure 验证当 initialize 失败时，Select 直接
// 返回 error。
func TestEngineProviderRejectsInitializeFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	remote := sidecar.NewRemoteEngine(failureCaller{err: errors.New("fake initialize failure")})
	_, err := engineprovider.Select(ctx, engineprovider.Options{
		Mode:            engineprovider.ModeAuto,
		RequiredMethods: requiredMethodsForCutover(),
		StartRemote:     staticStartRemote(remote, nil),
	})
	if err == nil {
		t.Fatal("Select() succeeded when initialize failed; expected error")
	}
	if !strings.Contains(err.Error(), "fake initialize failure") {
		t.Logf("note: error message is %q", err.Error())
	}
}

// TestEngineProviderRejectsMissingRequiredMethods 通过 stub 验证当子进程启动
// 成功但 initialize 返回的 methods 不全时，Select 返回 ErrMissingMethods。
func TestEngineProviderRejectsMissingRequiredMethods(t *testing.T) {
	required := requiredMethodsForCutover()
	// 故意从 initialize 结果中删除一个 required method。
	truncated := required[:len(required)-1]
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	remote := sidecar.NewRemoteEngine(methodsCaller{methods: truncated})
	_, err := engineprovider.Select(ctx, engineprovider.Options{
		Mode:            engineprovider.ModeAuto,
		RequiredMethods: required,
		StartRemote:     staticStartRemote(remote, nil),
	})
	if err == nil {
		t.Fatal("Select() succeeded with truncated methods; expected ErrMissingMethods")
	}
	if !errors.Is(err, engineprovider.ErrMissingMethods) {
		t.Fatalf("Select() err = %v, want ErrMissingMethods", err)
	}
}

// TestEngineProviderRejectsProtocolMismatch 验证 required methods 列表中包含一个
// 不存在的 "eos/protocol/handshake" 时，Select 返回 error。
func TestEngineProviderRejectsProtocolMismatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	required := append([]string(nil), requiredMethodsForCutover()...)
	required = append(required, "eos/protocol/handshake/unknown")

	declared := append([]string(nil), requiredMethodsForCutover()...)
	remote := sidecar.NewRemoteEngine(methodsCaller{methods: declared})
	_, err := engineprovider.Select(ctx, engineprovider.Options{
		Mode:            engineprovider.ModeAuto,
		RequiredMethods: required,
		StartRemote:     staticStartRemote(remote, nil),
	})
	if err == nil {
		t.Fatal("Select() succeeded with protocol mismatch; expected error")
	}
	if !errors.Is(err, engineprovider.ErrMissingMethods) {
		t.Fatalf("Select() err = %v, want ErrMissingMethods", err)
	}
}

// TestEngineProviderModeAutoDefaultsToRustOnly 不传 Mode 时，必须解析为
// ModeAuto = Rust-only。
func TestEngineProviderModeAutoDefaultsToRustOnly(t *testing.T) {
	mode, err := engineprovider.ResolveMode("")
	if err != nil {
		t.Fatalf("ResolveMode(\"\") error = %v", err)
	}
	if mode != engineprovider.ModeAuto {
		t.Fatalf("ResolveMode(\"\") = %q, want %q", mode, engineprovider.ModeAuto)
	}
	dir := t.TempDir()
	t.Setenv("EOS_CORE_PATH", filepath.Join(dir, "no-such-binary"))
	_, err = engineprovider.Select(context.Background(), engineprovider.Options{
		// 不传 Mode → ModeAuto
		Sidecar:         sidecar.ProcessOptions{},
		RequiredMethods: requiredMethodsForCutover(),
	})
	if err == nil {
		t.Fatal("Select(ModeAuto) succeeded with missing binary; expected error")
	}
}

// TestEngineProviderResolveModeRejectsUnknown 验证 ResolveMode 拒绝未知模式，
// 防止 typo 走错路径；退役的 legacy/go/eino/parity/sidecar 也必须被拒绝。
func TestEngineProviderResolveModeRejectsUnknown(t *testing.T) {
	for _, value := range []string{"foo", "auto-but-also-legacy", "all", "legacy", "go", "eino", "parity", "sidecar"} {
		if _, err := engineprovider.ResolveMode(value); err == nil {
			t.Errorf("ResolveMode(%q) succeeded; expected error", value)
		}
	}
}

// --- resolution-level smoke tests (real manifest + binary) ---

// TestEngineProviderRejectsBadChecksum 验证当 manifest 声明的 sha256 与实际
// binary 不一致时，engineprovider 直接返回 ErrChecksumMismatch。
func TestEngineProviderRejectsBadChecksum(t *testing.T) {
	dir := t.TempDir()
	binaryName := eosCoreBinaryName()
	binaryPath := filepath.Join(dir, binaryName)
	if err := os.WriteFile(binaryPath, []byte("fake closed core binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	manifestPath := writeTestManifest(t, dir, sidecarManifestOptions{
		Binary:              binaryName,
		Target:              currentTargetTriple(t),
		Features:            fullFeatures(t),
		OverrideSHA256:      "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		AllowDevPlaceholder: true,
	})

	_, err := engineprovider.Select(context.Background(), engineprovider.Options{
		Mode: engineprovider.ModeAuto,
		Sidecar: sidecar.ProcessOptions{
			Resolve: sidecar.ResolveOptions{
				ManifestPath:        manifestPath,
				AllowDevPlaceholder: true,
			},
			VerifyChecksum:      true,
			AllowDevPlaceholder: true,
		},
		RequiredMethods: requiredMethodsForCutover(),
	})
	if err == nil {
		t.Fatal("Select() succeeded with bad checksum; expected ErrChecksumMismatch")
	}
	if !errors.Is(err, sidecar.ErrChecksumMismatch) {
		t.Fatalf("Select() err = %v, want sidecar.ErrChecksumMismatch", err)
	}
}

// TestEngineProviderRejectsBadSignature 验证当 manifest 签名与 embedded
// public key 不匹配时，engineprovider 直接返回 ErrSignatureInvalid。
func TestEngineProviderRejectsBadSignature(t *testing.T) {
	dir := t.TempDir()
	binaryName := eosCoreBinaryName()
	binaryPath := filepath.Join(dir, binaryName)
	if err := os.WriteFile(binaryPath, []byte("fake closed core binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	manifestPath := writeTestManifest(t, dir, sidecarManifestOptions{
		Binary:            binaryName,
		Target:            currentTargetTriple(t),
		Features:          []string{"initialize", "session/list"},
		OverrideSignature: "ed25519:" + randomBase64Signature(t),
		RequireSignature:  true,
	})

	_, err := engineprovider.Select(context.Background(), engineprovider.Options{
		Mode: engineprovider.ModeAuto,
		Sidecar: sidecar.ProcessOptions{
			Resolve: sidecar.ResolveOptions{
				ManifestPath: manifestPath,
			},
			RequireSignature: true,
		},
		RequiredMethods: requiredMethodsForCutover(),
	})
	if err == nil {
		t.Fatal("Select() succeeded with bad signature; expected ErrSignatureInvalid")
	}
	if !errors.Is(err, sidecar.ErrSignatureInvalid) {
		t.Fatalf("Select() err = %v, want sidecar.ErrSignatureInvalid", err)
	}
}

// TestEngineProviderRejectsManifestTargetMismatch 验证当 manifest 的 target
// 与运行平台不一致时，engineprovider 直接返回 ErrTargetMismatch。
func TestEngineProviderRejectsManifestTargetMismatch(t *testing.T) {
	dir := t.TempDir()
	binaryName := eosCoreBinaryName()
	binaryPath := filepath.Join(dir, binaryName)
	if err := os.WriteFile(binaryPath, []byte("fake closed core binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	manifestPath := writeTestManifest(t, dir, sidecarManifestOptions{
		Binary:              binaryName,
		Target:              mismatchingTargetTriple(t),
		Features:            []string{"initialize", "session/list"},
		AllowDevPlaceholder: true,
	})

	_, err := engineprovider.Select(context.Background(), engineprovider.Options{
		Mode: engineprovider.ModeAuto,
		Sidecar: sidecar.ProcessOptions{
			Resolve: sidecar.ResolveOptions{
				ManifestPath: manifestPath,
			},
			AllowDevPlaceholder: true,
		},
		RequiredMethods: requiredMethodsForCutover(),
	})
	if err == nil {
		t.Fatal("Select() succeeded with target mismatch; expected ErrTargetMismatch")
	}
	if !errors.Is(err, sidecar.ErrTargetMismatch) {
		t.Fatalf("Select() err = %v, want sidecar.ErrTargetMismatch", err)
	}
}

// TestEngineProviderRejectsManifestFeatureMissing 验证 manifest 缺少 required
// feature 时，engineprovider 直接返回 ErrFeatureMissing。
func TestEngineProviderRejectsManifestFeatureMissing(t *testing.T) {
	dir := t.TempDir()
	binaryName := eosCoreBinaryName()
	binaryPath := filepath.Join(dir, binaryName)
	if err := os.WriteFile(binaryPath, []byte("fake closed core binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	manifestPath := writeTestManifest(t, dir, sidecarManifestOptions{
		Binary:              binaryName,
		Target:              currentTargetTriple(t),
		Features:            []string{"initialize"},
		AllowDevPlaceholder: true,
	})

	_, err := engineprovider.Select(context.Background(), engineprovider.Options{
		Mode: engineprovider.ModeAuto,
		Sidecar: sidecar.ProcessOptions{
			Resolve: sidecar.ResolveOptions{
				ManifestPath:        manifestPath,
				AllowDevPlaceholder: true,
			},
			RequiredFeatures:    []string{"initialize", "session/list"},
			AllowDevPlaceholder: true,
		},
		RequiredMethods: requiredMethodsForCutover(),
	})
	if err == nil {
		t.Fatal("Select() succeeded with feature missing; expected ErrFeatureMissing")
	}
	if !errors.Is(err, sidecar.ErrFeatureMissing) {
		t.Fatalf("Select() err = %v, want sidecar.ErrFeatureMissing", err)
	}
}

// --- helpers ---

func staticStartRemote(remote engineprovider.RemoteEngine, err error) engineprovider.StartRemoteFunc {
	return func(context.Context, sidecar.ProcessOptions) (engineprovider.RemoteEngine, error) {
		return remote, err
	}
}

// fullFeatures 覆盖整个 required method set，作为 happy-path 用的 baseline。
func fullFeatures(t *testing.T) []string {
	t.Helper()
	return append([]string(nil), requiredMethodsForCutover()...)
}

// eosCoreBinaryName 返回当前平台下可被 exec.Cmd 接受的 eos-core binary 名。
func eosCoreBinaryName() string {
	if runtime.GOOS == "windows" {
		return "eos-core.exe"
	}
	return "eos-core"
}

type sidecarManifestOptions struct {
	Binary              string
	Target              string
	Features            []string
	OverrideSHA256      string
	OverrideSignature   string
	AllowDevPlaceholder bool
	RequireSignature    bool
}

func writeTestManifest(t *testing.T, dir string, opts sidecarManifestOptions) string {
	t.Helper()
	binaryPath := filepath.Join(dir, opts.Binary)
	content, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	sum := sha256.Sum256(content)
	sha := "sha256:" + hex.EncodeToString(sum[:])
	if strings.TrimSpace(opts.OverrideSHA256) != "" {
		sha = opts.OverrideSHA256
	}
	signature := sidecar.SignaturePlaceholder
	if strings.TrimSpace(opts.OverrideSignature) != "" {
		signature = opts.OverrideSignature
	}
	manifest := sidecar.Manifest{
		SchemaVersion: sidecar.ManifestSchemaVersion,
		CoreVersion:   "0.1.0",
		APIVersion:    sidecar.DefaultAPIVersion,
		Target:        opts.Target,
		Binary:        opts.Binary,
		SHA256:        sha,
		Signature:     signature,
		MinCLIVersion: "v0.3.0",
		Features:      append([]string(nil), opts.Features...),
	}
	if opts.RequireSignature {
		if !strings.HasPrefix(signature, "ed25519:") {
			manifest.Signature = "ed25519:" + randomBase64Signature(t)
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	path := filepath.Join(dir, sidecar.DefaultManifestName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func randomBase64Signature(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 64)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	if decoded, err := base64.StdEncoding.DecodeString(encoded); err != nil || len(decoded) != 64 {
		t.Fatalf("randomBase64Signature sanity: decoded=%d err=%v", len(decoded), err)
	}
	return encoded
}

func currentTargetTriple(t *testing.T) string {
	t.Helper()
	out := sidecar.TargetTriple(goEnv(t, "GOOS"), goEnv(t, "GOARCH"))
	if out == "" {
		out = sidecar.TargetTriple(runtime.GOOS, runtime.GOARCH)
	}
	if out == "" {
		t.Skip("unable to determine current target triple")
	}
	return out
}

// mismatchingTargetTriple 返回一个与当前平台必然不同的 triple。
// 原实现硬编码 aarch64-apple-darwin 当「不匹配」目标，在 arm64 mac 上恰好
// 等于当前平台，走不到 mismatch 分支（校验顺延到 feature 检查而报错）。
func mismatchingTargetTriple(t *testing.T) string {
	t.Helper()
	current := currentTargetTriple(t)
	for _, candidate := range []string{
		"x86_64-apple-darwin",
		"aarch64-apple-darwin",
		"x86_64-unknown-linux-musl",
	} {
		if candidate != current {
			return candidate
		}
	}
	t.Skip("unable to determine mismatching target triple")
	return ""
}

func goEnv(t *testing.T, key string) string {
	t.Helper()
	if v := os.Getenv(key); v != "" {
		return v
	}
	return os.Getenv(strings.ToUpper(key))
}

// methodsCaller 返回 initialize 响应时带 methods 列表，标志 initialize 成功。
type methodsCaller struct {
	methods []string
}

func (m methodsCaller) Call(_ context.Context, method string, _ any, out any) error {
	switch method {
	case protocoljsonrpc.MethodInitialize:
		if target, ok := out.(*coreapijsonrpc.InitializeResult); ok {
			*target = coreapijsonrpc.InitializeResult{
				ServerName:      "eos-core-fake",
				ProtocolVersion: "v1",
				Methods:         append([]string(nil), m.methods...),
			}
		}
		return nil
	default:
		return coreapi.ErrUnsupported
	}
}

// failureCaller 在 initialize 阶段直接返回 error。
type failureCaller struct {
	err error
}

func (f failureCaller) Call(_ context.Context, method string, _ any, _ any) error {
	if method == protocoljsonrpc.MethodInitialize {
		return f.err
	}
	return coreapi.ErrUnsupported
}
