package testsupport

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// Env 是一个完全隔离的测试环境：所有目录都由测试自己创建，
// 任何子进程只能看到这里列出的路径。
type Env struct {
	// Root 是本次用例的根目录，t.TempDir() 提供，自动清理。
	Root string
	// Home 扮演 HOME / USERPROFILE，Core 的 ~/.eos 会落在这里。
	Home string
	// WorkspaceRoot 是 EOS_WORKSPACE_ROOT / EOS_SANDBOX_WORKSPACE_ROOT。
	WorkspaceRoot string
	// StoreDir 是 EOS_CORE_STORE_DIR，Core 的会话与消息持久化位置。
	StoreDir string
	// ConfigDir / CacheDir / LogDir 分别对应 XDG_*，CLI 自身会写这些地方。
	ConfigDir string
	CacheDir  string
	LogDir    string
	// OutsideRoot 位于 WorkspaceRoot 之外，用于放置 sentinel，
	// 断言“越权写入没有发生”。
	OutsideRoot string

	// OutsideSentinel 是 OutsideRoot 下固定内容的哨兵文件。
	// 用它的哈希证明工作区外的文件没有被测试改写。
	OutsideSentinel string
}

// NewEnv 建立隔离环境并写入 sentinel。
//
// 前置条件：调用方传入 testing.TB（通常是 *testing.T）。
// 副作用：只写 t.TempDir() 下的目录和 sentinel 文件。
func NewEnv(t testing.TB) *Env {
	t.Helper()
	root := t.TempDir()
	e := &Env{
		Root:          root,
		Home:          filepath.Join(root, "home"),
		WorkspaceRoot: filepath.Join(root, "workspace"),
		StoreDir:      filepath.Join(root, "store"),
		ConfigDir:     filepath.Join(root, "config"),
		CacheDir:      filepath.Join(root, "cache"),
		LogDir:        filepath.Join(root, "log"),
		OutsideRoot:   filepath.Join(root, "outside"),
	}
	for _, d := range []string{
		e.Home, e.WorkspaceRoot, e.StoreDir,
		e.ConfigDir, e.CacheDir, e.LogDir, e.OutsideRoot,
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("testsupport: mkdir %s: %v", d, err)
		}
	}
	e.OutsideSentinel = filepath.Join(e.OutsideRoot, "sentinel.txt")
	if err := os.WriteFile(e.OutsideSentinel, []byte(SentinelContent), 0o644); err != nil {
		t.Fatalf("testsupport: write sentinel: %v", err)
	}
	return e
}

// SentinelContent 是哨兵文件的固定内容。
// 它故意包含中文和 emoji，确保哈希比较同时覆盖 UTF-8 字节。
const SentinelContent = "EOS 测试哨兵：不允许被越权写入 🌍\n"

// credentialEnvPrefix / credentialEnvExact 列出必须在子进程里剔除的
// 凭据类变量。不剔除会让测试继承开发机的真实 token，违反隔离要求。
var (
	credentialEnvExact = map[string]bool{
		"EOS_SIGNATURE_PUBLIC_KEY":   true, // 信任根属于被测策略，测试自己下发
		"EOS_RELEASE_ARTIFACT_CHECK": true,
	}
	credentialEnvPrefix = []string{
		"EOS_API_KEY",
		"EOS_TOKEN",
		"EOS_MODEL_KEY",
		"EOS_CORE_PATH", // Core override 必须由测试显式控制，见 CoreEnv
		"EOS_CORE_MANIFEST",
		"EOS_CORE_BIN_DIR",
		"OPENAI_",
		"ANTHROPIC_",
		"MINIMAX_",
		"ARK_",
		"VOLC_",
	}
)

// IsCredentialEnv 判断某个环境变量名是否属于必须剔除的凭据/override 类。
func IsCredentialEnv(key string) bool {
	upper := strings.ToUpper(key)
	if credentialEnvExact[upper] {
		return true
	}
	for _, p := range credentialEnvPrefix {
		if strings.HasPrefix(upper, p) {
			return true
		}
	}
	return false
}

// CoreEnv 返回传给 eos-core 子进程的环境变量。
//
// 只保留 PATH / SystemRoot 等系统启动必需项，其余全部由测试显式下发，
// 避免子进程看到开发机上的 EOS_* 配置。
func (e *Env) CoreEnv(extra map[string]string) []string {
	env := map[string]string{
		"HOME":                       e.Home,
		"USERPROFILE":                e.Home,
		"XDG_CONFIG_HOME":            e.ConfigDir,
		"XDG_CACHE_HOME":             e.CacheDir,
		"XDG_DATA_HOME":              e.StoreDir,
		"XDG_STATE_HOME":             e.LogDir,
		"EOS_HOME":                   filepath.Join(e.Home, ".eos"),
		"EOS_WORKSPACE_ROOT":         e.WorkspaceRoot,
		"EOS_SANDBOX_WORKSPACE_ROOT": e.WorkspaceRoot,
		"EOS_CORE_STORE_DIR":         e.StoreDir,
		"EOS_LOG_LEVEL":              "warn",
	}
	if runtime.GOOS == "windows" {
		// Windows 子进程没有 SystemRoot 会加载失败。
		if wd, ok := os.LookupEnv("SystemRoot"); ok {
			env["SystemRoot"] = wd
		}
		env["TEMP"] = e.CacheDir
		env["TMP"] = e.CacheDir
	}
	for k, v := range extra {
		env[k] = v
	}
	return FlattenEnv(env)
}

// FlattenEnv 把 map 转成排序稳定的 K=V 切片，便于失败时 diff 定位。
func FlattenEnv(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

// SandboxMode 是测试要下发的沙箱档位，直接对应 EOS_SANDBOX_MODE 的 wire 值。
type SandboxMode string

const (
	// SandboxReadOnly 只读，写工作区外一律拒绝。
	SandboxReadOnly SandboxMode = "read-only"
	// SandboxWorkspaceWrite 允许写工作区内。
	SandboxWorkspaceWrite SandboxMode = "workspace-write"
	// SandboxFullAccess 全权限，仅用于明确要验证该档位语义的用例。
	SandboxFullAccess SandboxMode = "full-access"
)
