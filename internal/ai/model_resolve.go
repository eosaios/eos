package ai

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi"
)

// ModelResolution 描述用户输入解析到的目标。
type ModelResolution struct {
	// EntryName 命中的模型条目显示名（model/list 的 name，切换用唯一 key）。
	EntryName string
	// ProviderID / PresetID 命中条目的服务商与 preset 关联（套餐内切换
	// model/save 需要；非 preset 条目两者为空）。
	ProviderID string
	PresetID   string
	// PlanModelID 非空表示用户指到了套餐内某个具体模型（或条目当前模型）。
	PlanModelID string
	// NeedsPlanSwitch 条目当前 model 与 PlanModelID 不同、需要 model/save 换模型。
	NeedsPlanSwitch bool
}

func newResolution(e *coreapi.ModelConfig, planModelID string) *ModelResolution {
	if e == nil {
		return nil
	}
	return &ModelResolution{
		EntryName:   e.Name,
		ProviderID:  strings.TrimSpace(e.ProviderID),
		PresetID:    strings.TrimSpace(e.PresetID),
		PlanModelID: strings.TrimSpace(planModelID),
	}
}

// NormalizeModelKey 把模型标识归一化成可比对的形式：小写并剔除空格、
// 连字符、下划线、点、中点。"MiniMax M3" / "minimax-m3" / "MiniMax_M3"
// 都归一为 "minimaxm3"。桌面端曾把显示 label（含空格/中点）写进会话
// model_name，导致内核按条目名精确匹配 NotFound——归一化是容错的第一层。
func NormalizeModelKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch r {
		case ' ', '-', '_', '.', '·', '\t':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ResolveModelInput 把用户输入（条目名 / 模型 ID / 套餐模型 ID 或 label /
// preset ID 或名称，允许空格连字符混用）解析到已配置的模型条目。
//
// 匹配优先级（前者优先，均先精确后归一化）：
//  1. 条目 name
//  2. 条目 model ID（多条命中时优先激活条目）
//  3. 套餐内模型 ModelID / Label（经条目 preset_id 关联；旧版条目缺
//     preset_id 时按 api_base 与 provider 端点匹配兜底）
//  4. preset ID / preset 显示名
//
// catalog 允许传 nil（跳过 3/4 两级）。
// 解析失败返回错误，错误信息内嵌可选条目列表，可直接展示给用户。
func ResolveModelInput(input string, entries []coreapi.ModelConfig, catalog *coreapi.ModelCatalogState) (*ModelResolution, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("model name is required")
	}

	norm := NormalizeModelKey(input)

	// 1. 条目 name（精确，再归一化）
	for i := range entries {
		if entries[i].Name == input {
			return newResolution(&entries[i], entries[i].Model), nil
		}
	}
	for i := range entries {
		if NormalizeModelKey(entries[i].Name) == norm {
			return newResolution(&entries[i], entries[i].Model), nil
		}
	}

	// 2. 条目 model ID（激活条目优先）
	for i := range entries {
		if entries[i].Active && NormalizeModelKey(entries[i].Model) == norm {
			return newResolution(&entries[i], entries[i].Model), nil
		}
	}
	for i := range entries {
		if NormalizeModelKey(entries[i].Model) == norm {
			return newResolution(&entries[i], entries[i].Model), nil
		}
	}

	if catalog == nil {
		return nil, fmt.Errorf("unknown model %q. %s", input, AvailableModelsHint(entries))
	}
	presets := catalog.Presets

	// 3. 套餐内模型 ModelID / Label → 找到使用该套餐的条目
	for _, p := range presets {
		for _, pm := range p.PlanModels {
			if NormalizeModelKey(pm.ModelID) != norm && NormalizeModelKey(pm.Label) != norm {
				continue
			}
			if e := findEntryForPreset(entries, catalog, &p); e != nil {
				res := newResolution(e, pm.ModelID)
				applyPresetLinkage(res, e, &p)
				res.NeedsPlanSwitch = NormalizeModelKey(e.Model) != NormalizeModelKey(pm.ModelID)
				return res, nil
			}
		}
	}

	// 4. preset ID / 显示名 → 使用该 preset 的条目
	for _, p := range presets {
		if NormalizeModelKey(p.ID) != norm && NormalizeModelKey(p.Name) != norm {
			continue
		}
		if e := findEntryForPreset(entries, catalog, &p); e != nil {
			res := newResolution(e, e.Model)
			applyPresetLinkage(res, e, &p)
			return res, nil
		}
	}

	return nil, fmt.Errorf("unknown model %q. %s", input, AvailableModelsHint(entries))
}

// applyPresetLinkage 旧版条目缺 preset_id/provider_id 时，用命中的 preset
// 补齐关联（后续套餐内切换 model/save 需要）。
func applyPresetLinkage(res *ModelResolution, e *coreapi.ModelConfig, p *coreapi.ModelPresetOption) {
	if res.PresetID == "" {
		res.PresetID = strings.TrimSpace(p.ID)
	}
	if res.ProviderID == "" {
		res.ProviderID = strings.TrimSpace(p.ProviderID)
	}
}

// findEntryForPreset 返回使用指定 preset 的条目：先按 preset_id 精确关联；
// 旧版条目（无 preset_id）按 api_base 与 provider 该 (plan, format) 端点
// 匹配兜底。激活条目优先。
func findEntryForPreset(entries []coreapi.ModelConfig, catalog *coreapi.ModelCatalogState, p *coreapi.ModelPresetOption) *coreapi.ModelConfig {
	presetID := strings.TrimSpace(p.ID)
	if presetID != "" {
		if e := findEntryByPreset(entries, presetID); e != nil {
			return e
		}
	}
	base := presetEndpointBase(catalog, p)
	if base == "" {
		return nil
	}
	var fallback *coreapi.ModelConfig
	for i := range entries {
		e := &entries[i]
		if e.Active && sameBase(e.APIBase, base) {
			return e
		}
		if fallback == nil && sameBase(e.APIBase, base) {
			fallback = e
		}
	}
	return fallback
}

// presetEndpointBase 从 provider 端点表里解析 preset 对应 (plan, format) 的
// api_base（plan 匹配容忍 code/coding 两种拼写）。
func presetEndpointBase(catalog *coreapi.ModelCatalogState, p *coreapi.ModelPresetOption) string {
	if catalog == nil || p == nil {
		return ""
	}
	providerID := strings.TrimSpace(p.ProviderID)
	plan := strings.ToLower(strings.TrimSpace(p.Plan))
	format := strings.ToLower(strings.TrimSpace(p.Format))
	for _, prov := range catalog.Providers {
		if strings.TrimSpace(prov.ID) != providerID {
			continue
		}
		for _, ep := range prov.Endpoints {
			epPlan := strings.ToLower(strings.TrimSpace(ep.Plan))
			if epPlan == "code" {
				epPlan = "coding"
			}
			if plan == "code" {
				plan = "coding"
			}
			if epPlan == plan && strings.ToLower(strings.TrimSpace(ep.Format)) == format {
				return strings.TrimSpace(ep.APIBase)
			}
		}
	}
	return ""
}

func sameBase(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/")) }
	return norm(a) != "" && norm(a) == norm(b)
}

// findEntryByPreset 返回使用指定 preset 的条目（激活条目优先）。
func findEntryByPreset(entries []coreapi.ModelConfig, presetID string) *coreapi.ModelConfig {
	presetID = strings.TrimSpace(presetID)
	if presetID == "" {
		return nil
	}
	var fallback *coreapi.ModelConfig
	for i := range entries {
		e := &entries[i]
		if strings.TrimSpace(e.PresetID) != presetID {
			continue
		}
		if e.Active {
			return e
		}
		if fallback == nil {
			fallback = e
		}
	}
	return fallback
}

// AvailableModelsHint 渲染可选条目列表（"name (model)"，按名排序），
// 附加在解析错误后帮助用户自助纠正。
func AvailableModelsHint(entries []coreapi.ModelConfig) string {
	if len(entries) == 0 {
		return "no configured models; run /model to add one"
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if m := strings.TrimSpace(e.Model); m != "" && m != e.Name {
			names = append(names, fmt.Sprintf("%s (%s)", e.Name, m))
		} else {
			names = append(names, e.Name)
		}
	}
	sort.Strings(names)
	return "available models: " + strings.Join(names, ", ")
}

// HealSessionModelOverride 校正会话 metadata.model_name 里的无效覆盖。
//
// 背景：历史版本（桌面端 GUI）曾把目录显示 label（如 "MiniMax M3"）直接写进
// 会话 model_name，而内核按条目 name 精确匹配，导致该会话每次对话都
// NotFound。本函数在会话加载时自愈：覆盖有效→不动；能归一化解析→改写为
// 条目名；彻底无法解析→清除覆盖回落默认模型。返回给用户看的提示文本
// （空串表示无需处理），所有失败均不阻断对话。
func HealSessionModelOverride(ctx context.Context, models coreapi.ModelService, session coreapi.Session) string {
	if models == nil || strings.TrimSpace(session.ID) == "" {
		return ""
	}
	raw, ok := session.Metadata["model_name"]
	if !ok {
		return ""
	}
	name, _ := raw.(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	entries, err := models.List(ctx)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.Name == name {
			return "" // 覆盖有效
		}
	}

	var catalog *coreapi.ModelCatalogState
	if c, catErr := models.Catalog(ctx); catErr == nil {
		catalog = &c
	}
	if res, resErr := ResolveModelInput(name, entries, catalog); resErr == nil {
		if setErr := models.SetSession(ctx, coreapi.SetSessionModelRequest{
			SessionID: session.ID,
			ModelName: res.EntryName,
		}); setErr == nil {
			return fmt.Sprintf("session model %q normalized to %q", name, res.EntryName)
		}
	}

	if clearErr := models.ClearSession(ctx, coreapi.ClearSessionModelRequest{SessionID: session.ID}); clearErr == nil {
		return fmt.Sprintf("session model %q not found; cleared to fall back to default model", name)
	}
	return ""
}
