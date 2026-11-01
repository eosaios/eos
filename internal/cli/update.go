package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"fmt"
	"time"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/i18n"
	"github.com/eosaios/eos/internal/update"
	"github.com/eosaios/eos/internal/version"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	cfg, _ := config.Load()
	lang := cfg.Language
	checkOnly := false
	cmd := &cobra.Command{
		Use:   "update",
		Short: i18n.T("update.short", lang),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf(i18n.T("update.current_version", lang), version.AppVersion)
			if channel, err := update.VersionChannel(version.AppVersion); err == nil {
				fmt.Printf(i18n.T("update.channel", lang), channel)
			}
			fmt.Println(i18n.T("update.checking", lang))

			// 代理开关（config set update_proxy）生效：地址非法时构造期
			// fail-fast——配置被手工改坏时立即报错，不静默直连掩盖。
			proxyClient, err := update.NewHTTPClient(config.EffectiveUpdateProxyURL(&cfg))
			if err != nil {
				return err
			}

			result, err := update.CheckLatestWithClient(context.Background(), proxyClient)
			if err != nil {
				return fmt.Errorf(i18n.T("update.check_failed", lang), err)
			}

			if !result.HasUpdate {
				fmt.Printf(i18n.T("update.up_to_date", lang), result.LatestVersion)
				return nil
			}

			fmt.Printf(i18n.T("update.new_version", lang), result.LatestVersion)

			// 没有当前平台的归档（如未发布的平台组合）时退化为指引手动下载
			if result.DownloadURL == "" {
				fmt.Println(i18n.T("update.asset_missing", lang))
				fmt.Printf("  %s\n", result.ReleaseURL)
				return nil
			}

			if checkOnly {
				return nil
			}

			// 下载 → SHA256 校验 → 解压 → 替换二进制与 core/ 整树
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			fmt.Printf(i18n.T("update.downloading", lang), result.AssetName)
			outcome, err := update.ApplyWithClient(ctx, result, func(done, total int64) {
				if total > 0 {
					pct := int(float64(done) / float64(total) * 100)
					fmt.Printf(i18n.T("update.progress", lang), pct,
						float64(done)/1024/1024, float64(total)/1024/1024)
				}
			}, proxyClient)
			if err != nil {
				return fmt.Errorf(i18n.T("update.apply_failed", lang), err)
			}
			fmt.Println()

			fmt.Printf(i18n.T("update.success", lang), result.LatestVersion)
			if outcome.CoreUpdated {
				fmt.Print(i18n.T("update.core_updated", lang))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, i18n.T("update.check_flag", lang))
	return cmd
}
