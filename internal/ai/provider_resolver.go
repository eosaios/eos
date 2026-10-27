package ai

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
)

// SupportsVisionFromCatalog reports whether the named model advertises vision
// support in the catalog populated from Rust core. The Go TUI consults this
// when deciding whether to attach images to a request.
func SupportsVisionFromCatalog(modelName string) bool {
	id := strings.ToLower(strings.TrimSpace(modelName))
	if entry := GetModelEntry(id); entry != nil {
		return entry.SupportsVision
	}
	return false
}

// SupportsToolsFromCatalog reports whether the named model advertises tool
// calling support in the catalog populated from Rust core.
func SupportsToolsFromCatalog(modelName string) bool {
	id := strings.ToLower(strings.TrimSpace(modelName))
	if entry := GetModelEntry(id); entry != nil {
		return entry.SupportsTools
	}
	return false
}

// GetCatalogContextWindow returns the best known context window for a catalog
// entry, preferring runtime overrides and then any sibling preset that maps to
// the same underlying provider model.
func GetCatalogContextWindow(entry *ModelCatalogEntry) int {
	if entry == nil {
		return 0
	}
	if v, ok := getContextWindowOverride(entry.ModelName); ok && v > 0 {
		return v
	}
	if v, ok := getContextWindowOverride(entry.ID); ok && v > 0 {
		return v
	}
	if entry.ContextWindow > 0 {
		return entry.ContextWindow
	}

	modelName := strings.ToLower(strings.TrimSpace(entry.ModelName))
	entryID := strings.ToLower(strings.TrimSpace(entry.ID))
	for _, candidate := range globalCatalog.GetAll() {
		switch {
		case modelName != "" && strings.EqualFold(candidate.ModelName, modelName) && candidate.ContextWindow > 0:
			return candidate.ContextWindow
		case entryID != "" && strings.EqualFold(candidate.ID, entryID) && candidate.ContextWindow > 0:
			return candidate.ContextWindow
		}
	}
	return 0
}

// findCatalogEntryByKey looks up a catalog entry by its ID or ModelName.
func findCatalogEntryByKey(key string) *ModelCatalogEntry {
	if key == "" {
		return nil
	}
	if entry := GetModelEntry(key); entry != nil {
		return entry
	}
	for _, entry := range globalCatalog.GetAll() {
		if strings.EqualFold(strings.TrimSpace(entry.ModelName), key) {
			return entry
		}
	}
	return nil
}
