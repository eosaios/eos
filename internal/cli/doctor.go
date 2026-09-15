package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/version"
	"github.com/eosaios/eos/pkg/coreapi/engineprovider"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	sidecarclient "github.com/eosaios/eos/pkg/coreapi/sidecar/client"
	"github.com/spf13/cobra"
)

// eos doctor：一键自检 CLI / 配置 / 内核分发 / 内核握手 / 日志。
// 输出设计为可直接粘贴进 issue；任何 FAIL 项使命令以非零码退出。
// 文案直接用中文（与 bridge/mcp/document 命令一致，诊断输出不进 i18n 表）。
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "诊断本地安装、配置与内核分发状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.OutOrStdout())
		},
	}
}

type doctorReporter struct {
	w      io.Writer
	failed int
}

func (r *doctorReporter) pass(format string, a ...any) {
	fmt.Fprintf(r.w, "  [✓] "+format+"\n", a...)
}

func (r *doctorReporter) warn(format string, a ...any) {
	fmt.Fprintf(r.w, "  [!] "+format+"\n", a...)
}

func (r *doctorReporter) fail(format string, a ...any) {
	r.failed++
	fmt.Fprintf(r.w, "  [✗] "+format+"\n", a...)
}

func (r *doctorReporter) section(name string) {
	fmt.Fprintf(r.w, "\n%s\n", name)
}

func runDoctor(w io.Writer) error {
	r := &doctorReporter{w: w}

	fmt.Fprintf(w, "eos doctor — %s\n", time.Now().Format("2006-01-02 15:04:05 MST"))

	doctorCLIEnv(r)
	doctorConfig(r)
	resolved := doctorCoreResolve(r)
	doctorCoreBoot(r)
	doctorLogs(r)

	if resolved != (sidecar.ResolvedBinary{}) {
		doctorHints(r, resolved)
	}

	fmt.Fprintln(w)
	if r.failed > 0 {
		fmt.Fprintf(w, "诊断完成：%d 项失败。请把以上完整输出附到 issue。\n", r.failed)
		return errors.New("doctor found problems")
	}
	fmt.Fprintln(w, "诊断完成：全部通过。")
	return nil
}

func doctorCLIEnv(r *doctorReporter) {
	r.section("== CLI 环境 ==")
	r.pass("eos %s (%s/%s, %s)", version.AppVersion, runtime.GOOS, runtime.GOARCH, runtime.Version())
	if sidecar.ReleaseArtifactCheck() {
		r.pass("release 门禁已启用（%s=1）：占位签名将被拒绝，生产公钥必需", sidecar.EnvReleaseArtifactCheck)
	}
}

func doctorConfig(r *doctorReporter) {
	r.section("== 配置 ==")
	path := config.Path()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			r.warn("配置文件不存在（%s）——首次使用请先完成模型配置", path)
		} else {
			r.fail("配置文件不可读 %s：%v", path, err)
		}
		return
	}
	if !json.Valid(raw) {
		r.fail("配置文件不是合法 JSON：%s（请手工修复或删除后重新配置）", path)
		return
	}
	r.pass("配置文件可解析：%s（%d 字节）", path, len(raw))

	cfg, _ := config.Load()
	if len(cfg.Models) == 0 {
		r.warn("未配置任何模型——TUI 首启会进入模型向导")
		return
	}
	masked := 0
	for i := range cfg.Models {
		if config.LooksMaskedAPIKey(cfg.Models[i].APIKey) {
			masked++
		}
	}
	active := "未设置"
	if m, ok := config.ActiveModel(cfg); ok {
		active = fmt.Sprintf("%s → %s", m.Name, m.Model)
	}
	r.pass("模型条目 %d 个，当前激活：%s", len(cfg.Models), active)
	if masked > 0 {
		r.warn("%d 个条目的 API Key 是掩码形态（形如 abcd...wxyz）——内核 masked 加载缺陷的残留，重启后可能 401；在设置里重存一次 Key 可修复", masked)
	}
}

func doctorCoreResolve(r *doctorReporter) sidecar.ResolvedBinary {
	r.section("== 内核（eos-core）解析 ==")
	resolved, err := sidecar.ResolveBinary(sidecar.ResolveOptions{
		VerifyChecksum:      true,
		RequireSignature:    true,
		AllowDevPlaceholder: true, // release 门禁启用时会被强制改写为 false
	})
	if err != nil {
		if errors.Is(err, sidecar.ErrCoreBinaryNotFound) {
			r.fail("未找到内核二进制：搜索根（exe 旁 core/、源码树、内嵌分发）均无 %s 目标", runtime.GOOS+"/"+runtime.GOARCH)
		} else {
			r.fail("内核解析失败：%v", err)
		}
		return sidecar.ResolvedBinary{}
	}
	m := resolved.Manifest
	r.pass("内核二进制：%s", resolved.Path)
	// EOS_CORE_PATH 显式指定二进制时 resolver 不读 manifest（ResolvedBinary.Manifest
	// 为 nil），版本/签名/最低 CLI 版本检查随之跳过——旁路本身即显式信任。
	if m == nil {
		r.pass("来源 %s，目标 %s（%s 旁路：无 manifest，跳过签名与版本检查）", resolved.Source, resolved.Target, sidecar.EnvCorePath)
		return resolved
	}
	r.pass("来源 %s，目标 %s，core %s / api %s", resolved.Source, resolved.Target, m.CoreVersion, m.APIVersion)

	switch {
	case sidecar.IsPlaceholderSignature(m.Signature):
		r.warn("签名为开发占位（unsigned-development-placeholder）——仅限本地开发，正式分发包不应出现")
	case strings.TrimSpace(m.Signature) == "":
		r.warn("manifest 无签名字段")
	default:
		r.pass("签名校验通过（sha256 + Ed25519）")
	}

	if m.MinCLIVersion != "" {
		if compareVersions(version.AppVersion, m.MinCLIVersion) < 0 {
			r.fail("CLI %s 低于内核要求的最低版本 %s——请运行 eos update", version.AppVersion, m.MinCLIVersion)
		} else {
			r.pass("满足内核最低 CLI 版本要求（%s）", m.MinCLIVersion)
		}
	}
	return resolved
}

func doctorCoreBoot(r *doctorReporter) {
	r.section("== 内核握手 ==")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	selection, err := startRustOnlyEngine(ctx, "doctor", nil)
	if err != nil {
		if errors.Is(err, sidecar.ErrSignaturePlaceholder) && !sidecar.ReleaseArtifactCheck() {
			// 生产路径拒绝占位签名是设计行为；开发场景（本地 dev-rebuild 产物）
			// 再用放行占位的方式验证内核本身能否启动，把问题定位到签名而非二进制。
			r.warn("生产握手路径拒绝开发占位签名（eos --print / eos exec 同样会失败）")
			doctorCoreBootDevPlaceholder(r, ctx)
			return
		}
		r.fail("内核进程启动/initialize 握手失败：%v", err)
		r.warn("常见原因：二进制与 manifest 不配对（checksum mismatch）、单实例锁被占用、缺失运行时库（Linux 下 libxdo/libgbm）")
		return
	}
	defer selection.Close()
	doctorReportHandshake(r, selection)
}

// doctorCoreBootDevPlaceholder 用 AllowDevPlaceholder 再握手一次，
// 区分「内核二进制本身能跑，只是签名是占位」与「内核根本起不来」。
func doctorCoreBootDevPlaceholder(r *doctorReporter, ctx context.Context) {
	selection, err := engineprovider.Select(ctx, engineprovider.Options{
		Mode: engineprovider.ModeAuto,
		Sidecar: sidecar.ProcessOptions{
			VerifyChecksum:      true,
			RequireSignature:    true,
			AllowDevPlaceholder: true,
			Stderr:              coreStderrWriter(),
		},
		RequiredMethods: sidecarclient.RequiredMethods,
	})
	if err != nil {
		r.fail("放行占位签名后握手仍失败——内核二进制本身无法启动：%v", err)
		return
	}
	defer selection.Close()
	doctorReportHandshake(r, selection)
	r.pass("放行占位签名后内核可正常启动——本地开发请用 dev-rebuild 流程重签，正式包必须生产签名")
}

func doctorReportHandshake(r *doctorReporter, selection engineprovider.Selection) {
	init := selection.Initialize
	methodCount := len(init.Methods)
	if methodCount == 0 {
		r.fail("握手成功但方法列表为空：%s", init.ServerName)
		return
	}
	if len(selection.Missing) > 0 {
		r.fail("内核缺少 CLI 必需方法 %d 个（如 %s）——内核与 CLI 版本不配套，请同步升级", len(selection.Missing), strings.Join(selection.Missing[:min(3, len(selection.Missing))], ", "))
		return
	}
	r.pass("%s 协议 %s，方法 %d 个，CLI 必需方法全部具备", init.ServerName, init.ProtocolVersion, methodCount)
}

func doctorLogs(r *doctorReporter) {
	r.section("== 日志 ==")
	base := config.ConfiguredLogDir()
	for _, sub := range []string{"cli", "core"} {
		dir := filepath.Join(base, sub)
		probe := filepath.Join(dir, ".doctor_probe")
		if err := os.WriteFile(probe, nil, 0o644); err != nil {
			r.fail("日志目录不可写 %s：%v", dir, err)
			continue
		}
		_ = os.Remove(probe)
		r.pass("日志目录可写：%s", dir)
	}
}

func doctorHints(r *doctorReporter, resolved sidecar.ResolvedBinary) {
	r.section("== 环境覆盖 ==")
	printed := false
	for _, env := range []string{sidecar.EnvCorePath, sidecar.EnvCoreManifest, sidecar.EnvCoreBinDir, sidecar.EnvSignaturePublicKey} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			r.warn("%s=%s（覆盖默认解析链，请确认指向有效文件）", env, v)
			printed = true
		}
	}
	if !printed {
		r.pass("无内核相关环境变量覆盖，走默认解析链（来源 %s）", resolved.Source)
	}
}

// compareVersions 按点分段比较版本号（v 前缀忽略）：数字段数值比较，
// 非数字段（如 beta.22 的 beta）退化为字典序。返回 -1/0/1。
func compareVersions(a, b string) int {
	norm := func(s string) string { return strings.TrimPrefix(strings.TrimSpace(s), "v") }
	as := strings.Split(norm(a), ".")
	bs := strings.Split(norm(b), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := "", ""
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		an, aerr := strconv.Atoi(av)
		bn, berr := strconv.Atoi(bv)
		switch {
		case aerr == nil && berr == nil:
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
		default:
			if av != bv {
				if av < bv {
					return -1
				}
				return 1
			}
		}
	}
	return 0
}
