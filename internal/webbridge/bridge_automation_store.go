package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// automationStoreFile 是用户自定义/启用模板的持久化文件。
// 放在 ~/.eos/automation.json，重启后保留。
const automationStoreFileName = "automation.json"

// automationStoreRecord 是持久化到磁盘的单条记录。
// 复用 AutomationTemplateCard 的字段语义，但只持久化用户可控的部分
// （自定义模板 + 启用状态 + 绑定工作区）；预设模板每次从代码重建，不落盘。
type automationStoreRecord struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Prompt        string `json:"prompt"`
	Schedule      string `json:"schedule"`
	Enabled       bool   `json:"enabled"`
	Preset        bool   `json:"preset"`
	WorkspacePath string `json:"workspacePath"`
}

type automationStoreState struct {
	Templates []automationStoreRecord `json:"templates"`
	// PresetEnabled 记录用户对预设模板的启用状态（按 preset ID），
	// 这样预设模板不需要落盘整条记录，只记是否被启用。
	PresetEnabled map[string]bool `json:"presetEnabled"`
}

// automationStorePath 返回 ~/.eos/automation.json 的绝对路径。
func (s *BridgeService) automationStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return automationStoreFileName
	}
	return filepath.Join(home, ".eos", automationStoreFileName)
}

// maybeReloadAutomationFromFile 在检测到 automation.json 变更时重载定时调度。
// 「在对话中创建」流程里 AI 直接写该文件；模板列表经 bootstrap 自动刷新，
// 但已启用的 cron 调度需要这里显式重建才会生效。
func (s *BridgeService) maybeReloadAutomationFromFile(path string) {
	if !strings.EqualFold(filepath.Clean(strings.TrimSpace(path)), s.automationStorePath()) {
		return
	}
	s.reloadAutomationSchedules()
	slog.Info("bridge.automation.store_changed_reload", "path", path)
}

// loadAutomationStore 从磁盘读取持久化的自动化配置。
// 文件不存在或解析失败时返回空状态（不报错，向后兼容）。
func (s *BridgeService) loadAutomationStore() automationStoreState {
	path := s.automationStorePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("bridge.automation.load_failed", "path", path, "error", err)
		}
		return automationStoreState{PresetEnabled: map[string]bool{}}
	}
	var state automationStoreState
	if err := json.Unmarshal(raw, &state); err != nil {
		slog.Warn("bridge.automation.parse_failed", "path", path, "error", err)
		return automationStoreState{PresetEnabled: map[string]bool{}}
	}
	if state.PresetEnabled == nil {
		state.PresetEnabled = map[string]bool{}
	}
	return state
}

// saveAutomationStore 持久化自动化配置到磁盘。
// 调用方需已持有 stateMu（或确保无并发写）。
func (s *BridgeService) saveAutomationStore(state automationStoreState) error {
	path := s.automationStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return err
	}
	return nil
}

// hydrateAutomationTemplates 在 BridgeService 初始化时，从磁盘恢复用户模板
// 和预设启用状态到内存字段 customAutomationTemplates。
func (s *BridgeService) hydrateAutomationTemplates() {
	state := s.loadAutomationStore()
	s.customAutomationTemplates = nil
	for _, rec := range state.Templates {
		if strings.TrimSpace(rec.ID) == "" {
			continue
		}
		s.customAutomationTemplates = append(s.customAutomationTemplates, AutomationTemplateCard{
			ID:            rec.ID,
			Title:         rec.Title,
			Description:   rec.Description,
			Prompt:        rec.Prompt,
			Schedule:      rec.Schedule,
			Enabled:       rec.Enabled,
			Preset:        false, // 落盘的都是用户自定义模板
			WorkspacePath: rec.WorkspacePath,
		})
	}
}

// presetEnabledMapReadOnly 返回预设模板的启用状态（preset ID -> enabled）。
func (s *BridgeService) presetEnabledMapReadOnly() map[string]bool {
	state := s.loadAutomationStore()
	return state.PresetEnabled
}

// allAutomationTemplatesReadOnly 返回预设模板（合并启用状态/下次运行时间）
// 加上用户自定义模板的完整列表，供 bootstrap 投影使用。
//
// custom 模板每次从 store 文件重建（而非启动时 hydrate 的内存副本）：
// ~/.eos/automation.json 可能被会话里的 AI 用文件工具直接写入（「在对话中
// 创建」流程），文件是单一真相源；读一次盘的代价与原 presetEnabled 读取相同。
func (s *BridgeService) allAutomationTemplatesReadOnly() []AutomationTemplateCard {
	state := s.loadAutomationStore()

	presets := defaultAutomationTemplates()
	out := make([]AutomationTemplateCard, 0, len(presets)+len(state.Templates))
	for _, preset := range presets {
		preset.Enabled = state.PresetEnabled[preset.ID]
		preset.NextRunAt = computeNextRunAt(preset.Schedule, preset.Enabled)
		out = append(out, preset)
	}
	for _, rec := range state.Templates {
		if strings.TrimSpace(rec.ID) == "" {
			continue
		}
		out = append(out, AutomationTemplateCard{
			ID:            rec.ID,
			Title:         rec.Title,
			Description:   rec.Description,
			Prompt:        rec.Prompt,
			Schedule:      rec.Schedule,
			Enabled:       rec.Enabled,
			Preset:        false,
			WorkspacePath: rec.WorkspacePath,
			NextRunAt:     computeNextRunAt(rec.Schedule, rec.Enabled),
		})
	}
	return out
}

// automationTemplateByIDReadOnly 在预设 + 自定义模板里查找。
func (s *BridgeService) automationTemplateByIDReadOnly(templateID string) (AutomationTemplateCard, bool) {
	templateID = strings.TrimSpace(templateID)
	for _, item := range s.allAutomationTemplatesReadOnly() {
		if item.ID == templateID {
			return item, true
		}
	}
	return AutomationTemplateCard{}, false
}

// computeNextRunAt 根据 cron 表达式计算下次运行时间（RFC3339），无法解析或未启用时返回空。
func computeNextRunAt(schedule string, enabled bool) string {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" || !enabled {
		return ""
	}
	next, ok := nextCronRun(schedule)
	if !ok {
		return ""
	}
	return next.Format(time.RFC3339)
}
