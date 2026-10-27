package config

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"log/slog"
	"regexp"
	"strings"
)

// maskedKeyRe 匹配内核 mask_key 产出的掩码形态：`首4...尾4`（≤8 字符的 key
// 则为全 *）。eos.json 里 api_key 为该形态时是「显示占位」而非真实 key。
var maskedKeyRe = regexp.MustCompile(`^.{1,8}\.{3}.{1,8}$`)

// LooksMaskedAPIKey 判断 eos.json 里的 api_key 是否为掩码占位。
// 内核 model/save 持久化时会把 eos.json 的 api_key 写成 masked（真实 key
// 只进 OS 钥匙串）；而内核当前版本的 eos.json 加载路径会把非空 api_key
// 直接灌进内存 secrets 并屏蔽钥匙串恢复——masked 占位被当成真实 key 用，
// 重启后所有对话 401。壳层保存模型后需把明文 key 回补进 eos.json 规避。
func LooksMaskedAPIKey(key string) bool {
	k := strings.TrimSpace(key)
	if k == "" {
		return false
	}
	if strings.Trim(k, "*") == "" {
		return true
	}
	return maskedKeyRe.MatchString(k)
}

// SnapshotPlaintextAPIKeys 收集 eos.json 中仍是明文的 api_key（name → key）。
// 在调用内核 model/save / model/upsert 之前快照：保存成功后内核会把整份
// eos.json 的 api_key 覆写成 masked，快照用于回补。
func SnapshotPlaintextAPIKeys() map[string]string {
	cfg, _ := Load()
	keys := make(map[string]string, len(cfg.Models))
	for _, m := range cfg.Models {
		if k := strings.TrimSpace(m.APIKey); k != "" && !LooksMaskedAPIKey(k) {
			keys[m.Name] = m.APIKey
		}
	}
	return keys
}

// RestorePlaintextAPIKeys 把快照中的明文 key 回补进 eos.json 对应条目
// （仅覆盖条目当前值为 masked/空的情况，明文值不动）。内核修复「加载路径
// 跳过 masked」后本函数依然前向兼容：明文 api_key 是文档化的手填回退路径。
func RestorePlaintextAPIKeys(keys map[string]string) {
	if len(keys) == 0 {
		return
	}
	cfg, path := Load()
	changed := false
	for i := range cfg.Models {
		want, ok := keys[cfg.Models[i].Name]
		if !ok {
			continue
		}
		if LooksMaskedAPIKey(cfg.Models[i].APIKey) || strings.TrimSpace(cfg.Models[i].APIKey) == "" {
			cfg.Models[i].APIKey = want
			changed = true
		}
	}
	if !changed {
		return
	}
	if err := Save(cfg, path); err != nil {
		// 回补失败不阻断保存流程：key 仍在钥匙串（save 时内核已写入），
		// 仅在内核未修复 masked 加载 bug 的版本下重启后会 401。
		slog.Warn("config.apikey.restore.save.error", "path", path, "error", err.Error())
	}
}
