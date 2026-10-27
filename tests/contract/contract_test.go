// Package contract 是 EOS 跨语言契约测试（TEST-STRATEGY §3）。
//
// 真相源规则（不可协商）：
//   - 方法/能力集合：Rust 导出的 schema.json + 真实 eos-core 的 initialize 响应。
//   - DTO / wire：以真实 eos-core 实际 stdout 字节为准。
//   - 工具目录：真实 eos-core 的 tool/catalog 响应。
//
// 禁止从 Go 消费侧常量构造 expected —— 那只会证明 Go 自己和自己一致。
//
// build tag=eoscontract：该包需要真实 eos-core，不参与默认 `go test ./...`。
package contract

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/testsupport"
	"github.com/eosaios/eos/pkg/coreapi/generated"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// workspaceRoot 从本文件位置推导多仓工作区根目录，
// 避免在任何测试里写死开发机绝对路径。
func workspaceRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("contract: runtime.Caller failed")
	}
	// tests/contract -> eos-cli -> workspace root
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

// rustSchemaPath 指向 eos-core-rs 已提交的 schema 导出物。
//
// 之所以按需查找而非硬编码：eos-core-rs 可能不在同一工作区布局里。
// 找不到必须 BLOCKED，不允许跳过。
func rustSchemaPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("contract: runtime.Caller failed")
	}
	// tests/contract -> 仓库根
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	// 候选顺序：
	//  1) EOS_CORE_RS_DIR（CI 检出到 $RUNNER_TEMP 后显式注入）
	//  2) 仓库内 third_party 快照（离线/无 PAT 时的契约锚点）
	//  3) 同级目录 ../eos-core-rs/（本地多仓工作区布局）
	candidates := []string{
		filepath.Join(repoRoot, "third_party", "eos-core-protocol", "schema.json"),
		filepath.Join(workspaceRoot(t), "eos-core-rs", "crates", "eos-core-protocol", "schema.json"),
	}
	if env := os.Getenv("EOS_CORE_RS_DIR"); env != "" {
		candidates = append([]string{
			filepath.Join(env, "crates", "eos-core-protocol", "schema.json"),
		}, candidates...)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatalf("BLOCKED: 找不到 eos-core-rs 导出的 schema.json，已尝试：%v。"+
		"请把 eos-core-rs 放在同级目录，或设置 EOS_CORE_RS_DIR 指向它。", candidates)
	return ""
}

// rustSchema 是 eos-core-protocol 导出的 schema.json 结构。
//
// 注意：schema.json 里的 methods[].name 已经是 wire 方法名（Rust 的
// schema_snapshot() 直接取 FULL_CORE_METHODS 的元素），group 只是分组标注。
type rustSchema struct {
	Version int `json:"version"`
	Methods []struct {
		Name  string `json:"name"`
		Group string `json:"group"`
	} `json:"methods"`
	Types []struct {
		Name   string `json:"name"`
		Fields []struct {
			JSON string `json:"json"`
			Go   string `json:"go"`
			O    int    `json:"o"`
		} `json:"fields"`
	} `json:"types"`
}

// loadRustSchema 读取已提交的 Rust 导出物。
//
// 它是 Rust 侧的唯一真相源：Rust 的 write_schema_json 测试会写这个文件。
func loadRustSchema(t *testing.T) rustSchema {
	t.Helper()
	p := rustSchemaPath(t)
	bs, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读不到 Rust 导出 schema：%s（%v）", p, err)
	}
	var s rustSchema
	if err := json.Unmarshal(bs, &s); err != nil {
		t.Fatalf("解析 Rust schema 失败：%v", err)
	}
	if len(s.Methods) == 0 {
		t.Fatalf("BLOCKED: Rust schema 没有方法列表：%s", p)
	}
	return s
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func setDiff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func toSet(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, s := range in {
		out[s] = true
	}
	return out
}

// CT-01（P0）三方一致性：Rust 导出 schema ↔ Go 生成物 ↔ 真实打包 Core。
//
// 规格来源：TEST-STRATEGY §3「方法/版本」行 + §8「三方校验要分别覆盖
// 源码导出↔已提交生成物 和 生成物↔实际打包 Core」。
//
// 前置条件：仓库内存在 eos-core-protocol/schema.json；能定位到真实 eos-core。
// 精确预期：
//   - Go 生成物 CoreMethods() 的集合 == Rust FULL_CORE_METHODS 的集合；
//   - 真实 Core initialize 返回的 methods ⊆ Rust 导出集合
//     （内核可能尚未实现全部迁移目标方法，这属于已知缺口，必须显式列出而不是隐藏）；
//   - manifest 声明的 features 集合 == Rust 导出集合（发布物自述必须与源码一致）。
func TestCT01_ThreeWayMethodSetConsistency(t *testing.T) {
	schema := loadRustSchema(t)

	rustSet := map[string]bool{}
	for _, m := range schema.Methods {
		rustSet[m.Name] = true
	}
	t.Logf("Rust 导出方法数 = %d", len(rustSet))

	t.Run("rust_export_vs_go_generated", func(t *testing.T) {
		goSet := toSet(generated.CoreMethods())
		if missing := setDiff(rustSet, goSet); len(missing) > 0 {
			t.Errorf("Go 生成物缺少 Rust 导出的方法（%d）：%v", len(missing), missing)
		}
		if extra := setDiff(goSet, rustSet); len(extra) > 0 {
			t.Errorf("Go 生成物多出 Rust 未导出的方法（%d）：%v", len(extra), extra)
		}
		if len(rustSet) != len(schema.Methods) {
			t.Errorf("Rust schema 内存在重复方法名：声明 %d 条，去重后 %d 条", len(schema.Methods), len(rustSet))
		}
	})

	t.Run("rust_export_vs_vendored_manifest", func(t *testing.T) {
		core := testsupport.LocateRealCore(t)
		if len(core.Features) == 0 {
			t.Fatalf("manifest 未声明 features（%s）", core.ManifestPath)
		}
		manSet := toSet(core.Features)
		if missing := setDiff(rustSet, manSet); len(missing) > 0 {
			t.Errorf("manifest features 缺少 Rust 导出的方法（%d）：%v", len(missing), missing)
		}
		if extra := setDiff(manSet, rustSet); len(extra) > 0 {
			t.Errorf("manifest features 多出 Rust 未导出的方法（%d）：%v", len(extra), extra)
		}
	})

	t.Run("go_required_methods_subsets_of_live_core", func(t *testing.T) {
		env := testsupport.NewEnv(t)
		core := testsupport.LocateRealCore(t)
		conn, err := testsupport.StartCore(t, core, env.CoreEnv(nil))
		if err != nil {
			t.Fatalf("启动真实内核失败：%v", err)
		}
		live := liveMethods(t, conn)

		// Go 启动路径声明的必需方法，必须全部被真实内核**公示**。
		// 能力探测走 initialize.methods；漏公示会让 TUI/headless 主路径
		// 在真实使用中报 method not found。
		var missing []string
		for _, m := range jsonrpcAllCoreMethods() {
			if !live[m] {
				missing = append(missing, m)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			// 区分两种失败，避免把“公示缺失”和“根本没实现”混为一谈：
			// 对每个漏公示的方法直接调用一次，看内核是否真的路由。
			routable, unroutable := probeRoutability(t, conn, env, missing)
			t.Errorf("DEFECT/CT-01: 真实内核未在 initialize.methods 公示的方法（%d 个）：%v\n"+
				"其中内核实际可路由但未公示: %v\n"+
				"内核实际不可路由（未实现）: %v\n实际内核: %s",
				len(missing), missing, routable, unroutable, core.String())
		}
	})
}

// probeRoutability 直接调用一组方法，判断内核是“未公示”还是“未实现”。
//
// 两者对使用者的影响完全不同：
//   - 未实现 → 依赖该能力的功能真的不可用；
//   - 已实现但未公示 → 能力探测会误判，调用本身却能成功。
func probeRoutability(t *testing.T, conn *testsupport.CoreConn, env *testsupport.Env, methods []string) (routable, unroutable []string) {
	t.Helper()
	sessID := createSessionForContract(t, conn, env.WorkspaceRoot, "ct01-probe")
	return probeRoutabilityWith(t, conn, env, sessID, methods)
}

// liveMethods 从真实内核的 initialize 响应取出方法集合。
func liveMethods(t *testing.T, conn *testsupport.CoreConn) map[string]bool {
	t.Helper()
	var init struct {
		Methods      []string `json:"methods"`
		Capabilities struct {
			MigrationTarget int  `json:"migration_target_method_count"`
			FullCoreMigrate bool `json:"full_core_migration_surface"`
		} `json:"capabilities"`
		ProtocolVersion string `json:"protocol_version"`
		ServerName      string `json:"server_name"`
	}
	if err := json.Unmarshal(conn.InitResult, &init); err != nil {
		t.Fatalf("解析 initialize 响应失败：%v (raw=%s)", err, string(conn.InitResult))
	}
	if len(init.Methods) == 0 {
		t.Fatalf("真实内核 initialize 未返回方法列表")
	}
	if init.ProtocolVersion != "v1" {
		t.Errorf("真实内核 protocol_version = %q，want v1（与 manifest api_version 对齐）", init.ProtocolVersion)
	}
	out := toSet(init.Methods)
	t.Logf("真实内核声明方法数 = %d（capabilities.migration_target_method_count = %d, full_core_migration_surface = %v）",
		len(init.Methods), init.Capabilities.MigrationTarget, init.Capabilities.FullCoreMigrate)
	return out
}

func jsonrpcAllCoreMethods() []string { return jsonrpc.AllCoreMethods() }

// CT-02（P0）真实打包 Core 的 initialize 能力面。
//
// 精确预期：真实内核自述的 methods 必须是 manifest 声明 features 的子集，
// 且 capabilities 里的迁移目标计数必须与 manifest features 数一致。
//
// 这里的“子集”关系是已确认的产品现状（内核按实现进度暴露方法），
// 但 manifest 若声明了内核不提供的方法，发布物自述就是错的 —— 单独断言。
func TestCT02_LiveCoreInitializeMatchesManifest(t *testing.T) {
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	conn, err := testsupport.StartCore(t, core, env.CoreEnv(nil))
	if err != nil {
		t.Fatalf("启动真实内核失败：%v", err)
	}

	var init struct {
		Methods      []string `json:"methods"`
		Capabilities struct {
			MigrationTarget int  `json:"migration_target_method_count"`
			FullCoreMigrate bool `json:"full_core_migration_surface"`
			SessionCreate   bool `json:"session_create"`
			ToolCatalog     bool `json:"tool_catalog"`
			ToolExecute     bool `json:"tool_execute"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(conn.InitResult, &init); err != nil {
		t.Fatalf("解析 initialize 失败：%v", err)
	}

	// P0 链路依赖的能力位必须为 true，否则 TUI/headless 主路径不成立。
	for name, got := range map[string]bool{
		"session_create":              init.Capabilities.SessionCreate,
		"tool_catalog":                init.Capabilities.ToolCatalog,
		"tool_execute":                init.Capabilities.ToolExecute,
		"full_core_migration_surface": init.Capabilities.FullCoreMigrate,
	} {
		if !got {
			t.Errorf("capabilities.%s = false，P0 主链路依赖该能力", name)
		}
	}

	if len(core.Features) == 0 {
		t.Fatalf("manifest 未声明 features，无法做 initialize 对齐检查")
	}
	if got, want := init.Capabilities.MigrationTarget, len(core.Features); got != want {
		t.Errorf("capabilities.migration_target_method_count = %d，manifest features 数 = %d", got, want)
	}

	live := toSet(init.Methods)
	manSet := toSet(core.Features)
	notAdvertised := setDiff(manSet, live)
	if len(notAdvertised) > 0 {
		// 记录为显式缺陷：manifest 声明了、initialize 却没公示的方法。
		// 注意这不等于“未实现”——用 probeRoutability 区分两者，
		// 否则报告会把能力探测缺陷夸大成功能缺失。
		sessID := createSessionForContract(t, conn, env.WorkspaceRoot, "ct02-probe")
		routable, unroutable := probeRoutabilityWith(t, conn, env, sessID, notAdvertised)
		t.Errorf("DEFECT/CT-02: manifest 声明但 initialize.methods 未公示的方法（%d 个）：%v\n"+
			"  内核实际可路由（能力探测会误判不可用）: %v\n"+
			"  内核实际返回 method not found（真未实现）: %v\n"+
			"  capabilities 仍自述 full_core_migration_surface=true, migration_target_method_count=%d\n"+
			"内核=%s",
			len(notAdvertised), notAdvertised, routable, unroutable,
			init.Capabilities.MigrationTarget, core.String())
	}
	if extra := setDiff(live, manSet); len(extra) > 0 {
		t.Errorf("DEFECT/CT-02: 真实内核公示但 manifest 未声明的方法（%d 个）：%v", len(extra), extra)
	}
}

func probeRoutabilityWith(t *testing.T, conn *testsupport.CoreConn, env *testsupport.Env, sessID string, methods []string) (routable, unroutable []string) {
	t.Helper()
	for _, m := range methods {
		_, err := conn.CallTimeout(m, map[string]any{
			"session_id": sessID,
			"path":       env.WorkspaceRoot,
		}, 8*second)
		if err == nil {
			routable = append(routable, m)
			continue
		}
		var rpcErr *testsupport.RPCError
		if errors.As(err, &rpcErr) && rpcErr.Code == -32601 {
			unroutable = append(unroutable, m)
			continue
		}
		routable = append(routable, m)
	}
	return routable, unroutable
}

// CT-03（P0）错误对象的消费语义：真实失败样本的错误形状。
//
// 规格来源：TEST-STRATEGY §3「错误」行 —— 比较约定 code/message/data。
// 精确预期：未知方法返回 -32601；错误对象含 message；data 至少能定位到方法名。
func TestCT03_RealErrorObjectShape(t *testing.T) {
	env := testsupport.NewEnv(t)
	core := testsupport.LocateRealCore(t)
	conn, err := testsupport.StartCore(t, core, env.CoreEnv(nil))
	if err != nil {
		t.Fatalf("启动真实内核失败：%v", err)
	}

	cases := []struct {
		name       string
		method     string
		wantCode   int
		wantInMsg  string
		dataChecks func(t *testing.T, data map[string]any)
	}{
		{
			name:      "unknown_method",
			method:    "no/such/method",
			wantCode:  -32601,
			wantInMsg: "not found",
			dataChecks: func(t *testing.T, data map[string]any) {
				if got := data["method"]; got != "no/such/method" {
					t.Errorf("错误 data.method = %v，want no/such/method", got)
				}
			},
		},
		{
			name:      "session_not_found",
			method:    "session/messages/load",
			wantCode:  0, // 具体码不预设，只断言结构与可定位性
			wantInMsg: "",
			dataChecks: func(t *testing.T, data map[string]any) {
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := conn.Call(tc.method, map[string]any{"session_id": "sess_does_not_exist"})
			if err == nil {
				t.Fatalf("%s 期望返回错误，实际成功 —— 说明错误路径没被触达", tc.method)
			}
			rpcErr, ok := err.(*testsupport.RPCError)
			if !ok {
				t.Fatalf("错误类型 = %T (%v)，want *testsupport.RPCError；"+
					"消费者必须能区分结构化错误与传输错误", err, err)
			}
			if rpcErr.Message == "" {
				t.Errorf("错误 message 为空，用户无法据此定位问题")
			}
			if tc.wantCode != 0 && rpcErr.Code != tc.wantCode {
				t.Errorf("错误 code = %d，want %d", rpcErr.Code, tc.wantCode)
			}
			if tc.wantInMsg != "" && !contains(rpcErr.Message, tc.wantInMsg) {
				t.Errorf("错误 message = %q，未包含 %q", rpcErr.Message, tc.wantInMsg)
			}
			if len(rpcErr.Data) > 0 {
				var data map[string]any
				if err := json.Unmarshal(rpcErr.Data, &data); err != nil {
					t.Errorf("错误 data 不是对象，无法被消费者稳定解码：%v (%s)", err, string(rpcErr.Data))
				} else {
					tc.dataChecks(t, data)
				}
			}
			t.Logf("%s -> code=%d message=%q data=%s trace=%s",
				tc.method, rpcErr.Code, rpcErr.Message, string(rpcErr.Data), rpcErr.Trace)
		})
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// second 是常用超时的简写，保持用例可读。
const second = time.Second

// firstNonEmptyString 返回第一个非空字符串。
func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// trunc 截断长字符串，用于失败信息保持可读。
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
