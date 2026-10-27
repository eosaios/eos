package ai

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestDetectThinkingCapabilityMatrix(t *testing.T) {
	cases := map[string]ThinkingCapability{
		"":                  ThinkingNone,
		"gpt-4o":            ThinkingNone,
		"o1":                ThinkingHigh,
		"o1-mini":           ThinkingMedium,
		"o1-preview":        ThinkingMedium,
		"deepseek-r1":       ThinkingHigh,
		"deepseek-reasoner": ThinkingHigh,
		"kimi-k2.5":         ThinkingMedium,
		"kimi-thinking":     ThinkingMedium,
		"glm-4.7":           ThinkingMedium,
		"glm-thinking":      ThinkingMedium,
		"qwen-thinking":     ThinkingMedium,
		"qwen-reasoning":    ThinkingHigh,
		"qwen-qwq":          ThinkingHigh,
		"doubao-thinking":   ThinkingMedium,
		"claude-3":          ThinkingNone,
	}
	for name, want := range cases {
		if got := DetectThinkingCapability(name); got != want {
			t.Fatalf("Detect(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestHealSessionModelOverride(t *testing.T) {
	// nil models
	if got := HealSessionModelOverride(context.Background(), nil, coreapi.Session{ID: "s"}); got != "" {
		t.Fatal("nil models")
	}
	// 空 session ID
	ms := &fakeModelSvc{}
	if got := HealSessionModelOverride(context.Background(), ms, coreapi.Session{}); got != "" {
		t.Fatal("empty id")
	}
	// 无 metadata
	if got := HealSessionModelOverride(context.Background(), ms, coreapi.Session{ID: "s"}); got != "" {
		t.Fatal("no metadata")
	}
	// 有效覆盖（entry 存在）
	ms.entries = []coreapi.ModelConfig{{Name: "m1"}}
	sess := coreapi.Session{ID: "s", Metadata: map[string]any{"model_name": "m1"}}
	if got := HealSessionModelOverride(context.Background(), ms, sess); got != "" {
		t.Fatalf("valid = %q", got)
	}
	// 无效覆盖 → 归一化
	ms.entries = []coreapi.ModelConfig{{Name: "good", Model: "good-model"}}
	sess2 := coreapi.Session{ID: "s", Metadata: map[string]any{"model_name": "bad-label"}}
	got := HealSessionModelOverride(context.Background(), ms, sess2)
	_ = got
}

type fakeModelSvc struct {
	entries []coreapi.ModelConfig
}

func (s *fakeModelSvc) List(context.Context) ([]coreapi.ModelConfig, error) {
	return s.entries, nil
}
func (s *fakeModelSvc) Catalog(context.Context) (coreapi.ModelCatalogState, error) {
	return coreapi.ModelCatalogState{}, nil
}
func (s *fakeModelSvc) Upsert(context.Context, coreapi.UpsertModelRequest) error { return nil }
func (s *fakeModelSvc) Save(context.Context, coreapi.ModelSaveRequest) error     { return nil }
func (s *fakeModelSvc) Delete(context.Context, coreapi.ModelNameRequest) error   { return nil }
func (s *fakeModelSvc) Activate(context.Context, coreapi.ModelNameRequest) error {
	return nil
}
func (s *fakeModelSvc) SyncEnv(context.Context) error { return nil }
func (s *fakeModelSvc) Context(context.Context, coreapi.ModelContextRequest) (coreapi.ModelContextSnapshot, error) {
	return coreapi.ModelContextSnapshot{}, nil
}
func (s *fakeModelSvc) SetWorkspace(context.Context, coreapi.SetWorkspaceModelRequest) error {
	return nil
}
func (s *fakeModelSvc) ClearWorkspace(context.Context, coreapi.ClearWorkspaceModelRequest) error {
	return nil
}
func (s *fakeModelSvc) SetSession(context.Context, coreapi.SetSessionModelRequest) error {
	return nil
}
func (s *fakeModelSvc) ClearSession(context.Context, coreapi.ClearSessionModelRequest) error {
	return nil
}
