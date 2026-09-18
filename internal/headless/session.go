// Package headless 提供无 UI 场景（print/exec/mcp serve）共享的会话与
// turn 驱动原语：会话解析、模型覆盖、turn 事件订阅与统一事件泵。
//
// 提取自 internal/cli/print.go（2026-09）：eos mcp serve 的委托工具
//（eos_chat 等）与 CLI headless 模式共用同一套语义，避免复制粘贴漂移。
package headless

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/eosaios/eos/internal/ai"
	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/pkg/coreapi"
)

// EnsureSession 解析 headless 上下文的当前会话：Current 成功则复用（含
// 会话模型覆盖自愈），否则创建新会话。workspaceRoot 来自 WorkspaceRoot。
func EnsureSession(ctx context.Context, engine coreapi.Engine) (coreapi.Session, error) {
	if engine == nil {
		return coreapi.Session{}, fmt.Errorf("core engine unavailable")
	}
	workspaceRoot := WorkspaceRoot(ctx, engine)
	session, err := engine.Sessions().Current(ctx, coreapi.CurrentSessionRequest{WorkspaceRoot: workspaceRoot})
	if err == nil && strings.TrimSpace(session.ID) != "" {
		// 会话自愈：历史版本（桌面端 GUI）曾把目录 label（如 "MiniMax M3"）
		// 写进 model_name，内核按条目名精确匹配会 NotFound，每次对话必炸。
		// 这里归一化修复或清除无效覆盖，避免复用旧会话时阻断。
		if note := ai.HealSessionModelOverride(ctx, engine.Models(), session); note != "" {
			slog.Warn("headless.session.model.healed", "session_id", session.ID, "note", note)
		}
		return session, nil
	}
	session, err = engine.Sessions().Create(ctx, coreapi.CreateSessionRequest{
		WorkspaceRoot: workspaceRoot,
		Title:         "Headless session",
		Metadata:      map[string]any{"source": "cli"},
	})
	if err != nil {
		return coreapi.Session{}, fmt.Errorf("create headless session: %w", err)
	}
	if strings.TrimSpace(session.ID) == "" {
		return coreapi.Session{}, fmt.Errorf("create headless session returned empty id")
	}
	return session, nil
}

// WorkspaceRoot 取 headless 上下文的工作区根：优先内核前台工作区，
// 回退进程 cwd。
func WorkspaceRoot(ctx context.Context, engine coreapi.Engine) string {
	if engine != nil {
		if snapshot, err := engine.State().Snapshot(ctx, coreapi.StateSnapshotRequest{}); err == nil {
			if root := strings.TrimSpace(snapshot.ForegroundWorkspace); root != "" {
				return root
			}
		}
	}
	return cwdWorkspaceRoot()
}

// ApplyModelOverride 把 --model 的值解析并写入会话模型覆盖。
// EOS_MODEL_OVERRIDE 环境变量内核并不消费（历史遗留），--model 必须走
// model/session/set 才真正生效；输入支持条目名/模型 ID/套餐模型 label。
func ApplyModelOverride(ctx context.Context, engine coreapi.Engine, session coreapi.Session, override string) error {
	override = strings.TrimSpace(override)
	if override == "" {
		return nil
	}
	entries, err := engine.Models().List(ctx)
	if err != nil {
		return fmt.Errorf("--model: list models: %w", err)
	}
	var catalog *coreapi.ModelCatalogState
	if c, catErr := engine.Models().Catalog(ctx); catErr == nil {
		catalog = &c
	}
	res, err := ai.ResolveModelInput(override, entries, catalog)
	if err != nil {
		return fmt.Errorf("--model: %v", err)
	}
	if res.NeedsPlanSwitch {
		// 与 adapter.SaveModel 相同的明文 key 维护：内核保存会把 eos.json
		// api_key 覆写成 masked，旧内核加载时会把 masked 当真实 key（401）。
		plaintext := config.SnapshotPlaintextAPIKeys()
		if err := engine.Models().Save(ctx, coreapi.ModelSaveRequest{
			OriginalName: res.EntryName,
			Mode:         "preset",
			ProviderID:   res.ProviderID,
			PresetID:     res.PresetID,
			Name:         res.EntryName,
			Model:        res.PlanModelID,
		}); err != nil {
			return fmt.Errorf("--model: switch plan model: %w", err)
		}
		config.RestorePlaintextAPIKeys(plaintext)
	}
	if err := engine.Models().SetSession(ctx, coreapi.SetSessionModelRequest{
		SessionID: session.ID,
		ModelName: res.EntryName,
	}); err != nil {
		return fmt.Errorf("--model: set session model: %w", err)
	}
	return nil
}

// ResolveActiveModelName 返回当前激活模型名（列表第一个 Active 项）。
func ResolveActiveModelName(ctx context.Context, engine coreapi.Engine) (string, error) {
	if engine == nil {
		return "", nil
	}
	items, err := engine.Models().List(ctx)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Active {
			return strings.TrimSpace(item.Model), nil
		}
	}
	return "", nil
}

// cwdWorkspaceRoot 进程工作目录兜底（os.Getwd 失败返回空串）。
func cwdWorkspaceRoot() string {
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}
