package engineprovider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi"
	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
)

const EnvCoreEngine = "EOS_CORE_ENGINE"

// Mode 唯一可选 Rust 内核：通过 sidecar 拉起 eos-core。legacy/parity 已退役，
// 不再有 Go 内核回退路径。
type Mode string

const (
	// ModeAuto 与 ModeRust 等价：均启动 eos-core sidecar。保留两者仅为语义清晰。
	ModeAuto Mode = "auto"
	ModeRust Mode = "rust"
)

// Kind 标识返回的引擎实现类型。Rust-only 之后唯一取值为 RustSidecar。
type Kind string

const (
	KindRustSidecar Kind = "rust-sidecar"
)

var (
	ErrMissingMethods = errors.New("core engine is missing required methods")
)

type RemoteEngine interface {
	coreapi.Engine
	Initialize(context.Context) (coreapijsonrpc.InitializeResult, error)
	Close() error
}

type StartRemoteFunc func(context.Context, sidecar.ProcessOptions) (RemoteEngine, error)

type Options struct {
	Mode            Mode
	Sidecar         sidecar.ProcessOptions
	RequiredMethods []string
	StartRemote     StartRemoteFunc
}

type Selection struct {
	Kind       Kind
	Engine     coreapi.Engine
	Initialize coreapijsonrpc.InitializeResult
	Missing    []string
	close      func() error
}

// Select 解析 mode 并启动 eos-core sidecar。Rust-only：失败即返回 error，
// 不做任何静默回退。
func Select(ctx context.Context, opts Options) (Selection, error) {
	if _, err := ResolveMode(string(opts.Mode)); err != nil {
		return Selection{}, err
	}
	return selectRust(ctx, opts)
}

// ResolveMode 仅接受空/auto/rust（均视为 Rust-only）；其余字符串报 unsupported。
func ResolveMode(value string) (Mode, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = strings.TrimSpace(os.Getenv(EnvCoreEngine))
	}
	switch strings.ToLower(value) {
	case "", "auto", "rust":
		return ModeAuto, nil
	default:
		return "", fmt.Errorf("unsupported core engine mode %q", value)
	}
}

func MissingMethods(available []string, required []string) []string {
	if len(required) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(available))
	for _, method := range available {
		method = strings.TrimSpace(method)
		if method != "" {
			seen[method] = struct{}{}
		}
	}
	var missing []string
	for _, method := range required {
		method = strings.TrimSpace(method)
		if method == "" {
			continue
		}
		if _, ok := seen[method]; !ok {
			missing = append(missing, method)
		}
	}
	return missing
}

func (s Selection) Close() error {
	if s.close == nil {
		return nil
	}
	return s.close()
}

func selectRust(ctx context.Context, opts Options) (Selection, error) {
	start := opts.StartRemote
	if start == nil {
		start = func(ctx context.Context, processOpts sidecar.ProcessOptions) (RemoteEngine, error) {
			return sidecar.StartRemoteEngine(ctx, processOpts)
		}
	}
	processOpts := opts.Sidecar
	if len(processOpts.RequiredFeatures) == 0 && len(opts.RequiredMethods) > 0 {
		processOpts.RequiredFeatures = append([]string(nil), opts.RequiredMethods...)
	}
	remote, err := start(ctx, processOpts)
	if err != nil {
		return Selection{}, err
	}
	closeRemote := func() error { return remote.Close() }
	initResult, err := remote.Initialize(ctx)
	if err != nil {
		_ = closeRemote()
		return Selection{}, err
	}
	missing := MissingMethods(initResult.Methods, opts.RequiredMethods)
	if len(missing) > 0 {
		_ = closeRemote()
		return Selection{Kind: KindRustSidecar, Initialize: initResult, Missing: missing}, fmt.Errorf("%w: %s", ErrMissingMethods, strings.Join(missing, ", "))
	}
	return Selection{
		Kind:       KindRustSidecar,
		Engine:     remote,
		Initialize: initResult,
		close:      closeRemote,
	}, nil
}
