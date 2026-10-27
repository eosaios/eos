package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/i18n"
	"github.com/eosaios/eos/internal/update"
	"github.com/spf13/cobra"
)

// eos config 子命令：全局配置查看与修改。当前仅支持 update_proxy 一个配置项。
//
// 写入走原始 JSON 文档（map 级增删键）而非 Config 结构体回写——后者会把
// 结构体未声明的键（如桌面端写入的 gui_language/browser_profiles）整体丢掉，
// 这是跨壳层共享 ~/.eos.json 时必须避免的副作用。

const (
	updateProxyKey          = "update_proxy"
	updateProxyEnabledField = "update_proxy_enabled"
	updateProxyURLField     = "update_proxy_url"
	cfgUpdateProxyToggleOff = "off"
)

func newConfigCmd() *cobra.Command {
	cfg, _ := config.Load()
	lang := cfg.Language
	cmd := &cobra.Command{
		Use:   "config",
		Short: i18n.T("config.short", lang),
	}
	cmd.AddCommand(newConfigGetCmd(lang), newConfigSetCmd(lang))
	return cmd
}

func newConfigGetCmd(lang string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: i18n.T("config.get.short", lang),
		Long:  i18n.T("config.get.long", lang),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch strings.ToLower(strings.TrimSpace(args[0])) {
			case updateProxyKey:
				doc, err := loadRawConfigDoc(config.Path())
				if err != nil {
					return err
				}
				enabled := rawConfigDocBool(doc, updateProxyEnabledField)
				url := rawConfigDocString(doc, updateProxyURLField)
				if enabled {
					fmt.Printf(i18n.T("config.update_proxy.on", lang), url)
				} else {
					fmt.Print(i18n.T("config.update_proxy.off", lang))
				}
				return nil
			default:
				return fmt.Errorf(i18n.T("config.unknown_key", lang), args[0])
			}
		},
	}
}

func newConfigSetCmd(lang string) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: i18n.T("config.set.short", lang),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.ToLower(strings.TrimSpace(args[0]))
			value := strings.TrimSpace(args[1])
			switch key {
			case updateProxyKey:
				return setUpdateProxy(lang, value, config.Path())
			default:
				return fmt.Errorf(i18n.T("config.unknown_key", lang), args[0])
			}
		},
	}
}

func setUpdateProxy(lang, value, path string) error {
	doc, err := loadRawConfigDoc(path)
	if err != nil {
		return err
	}
	if strings.EqualFold(value, cfgUpdateProxyToggleOff) {
		if err := rawConfigDocSetBool(doc, updateProxyEnabledField, false); err != nil {
			return err
		}
		if err := saveRawConfigDoc(doc, path); err != nil {
			return err
		}
		fmt.Print(i18n.T("config.set.update_proxy.disabled", lang))
		return nil
	}
	// fail-fast：地址非法时在写入前报错（NewHTTPClient 同时承担运行期构造）。
	if _, err := update.NewHTTPClient(value); err != nil {
		return fmt.Errorf(i18n.T("config.set.update_proxy.invalid", lang), err.Error())
	}
	if err := rawConfigDocSetBool(doc, updateProxyEnabledField, true); err != nil {
		return err
	}
	if err := rawConfigDocSetString(doc, updateProxyURLField, value); err != nil {
		return err
	}
	if err := saveRawConfigDoc(doc, path); err != nil {
		return err
	}
	fmt.Printf(i18n.T("config.set.update_proxy.enabled", lang), value)
	return nil
}

// === 原始 JSON 文档读写（只增删本命令关注的键，保留其余所有键） ===

func loadRawConfigDoc(path string) (map[string]json.RawMessage, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	doc := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(b))) == 0 {
		return doc, nil
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}

func saveRawConfigDoc(doc map[string]json.RawMessage, path string) error {
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func rawConfigDocBool(doc map[string]json.RawMessage, key string) bool {
	raw, ok := doc[key]
	if !ok {
		return false
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return value
}

func rawConfigDocString(doc map[string]json.RawMessage, key string) string {
	raw, ok := doc[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func rawConfigDocSetBool(doc map[string]json.RawMessage, key string, value bool) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	doc[key] = encoded
	return nil
}

func rawConfigDocSetString(doc map[string]json.RawMessage, key, value string) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	doc[key] = encoded
	return nil
}
