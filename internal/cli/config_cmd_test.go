package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigGetSetCommands(t *testing.T) {
	setTestHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgPath := filepath.Join(home, ".eos.json")

	// get 未知 key
	cmd := newConfigGetCmd("zh")
	cmd.SetArgs([]string{"nope"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unknown key")
	}

	// get update-proxy 关闭
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = newConfigGetCmd("zh")
	cmd.SetArgs([]string{"update_proxy"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// get update-proxy 开启
	if err := os.WriteFile(cfgPath, []byte(`{"update_proxy_enabled":true,"update_proxy_url":"http://p"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = newConfigGetCmd("en")
	cmd.SetArgs([]string{"update_proxy"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// set 未知 key
	cmd = newConfigSetCmd("zh")
	cmd.SetArgs([]string{"nope", "x"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unknown set")
	}

	// set update-proxy 关
	cmd = newConfigSetCmd("zh")
	cmd.SetArgs([]string{"update_proxy", "off"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// set update-proxy 开 + URL
	cmd = newConfigSetCmd("zh")
	cmd.SetArgs([]string{"update_proxy", "http://proxy.local"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
