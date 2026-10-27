package webbridge

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// SaveAutomationTemplate 新增或编辑用户自定义模板。
// 校验 cron 表达式、标题非空；持久化后刷新调度器。
func (svc *AutomationService) SaveAutomationTemplate(req AutomationSaveRequest) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return s.LoadBootstrap(), errors.New(s.t("error.automation.template_title_required"))
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return s.LoadBootstrap(), errors.New(s.t("error.automation.template_prompt_required"))
	}
	schedule := strings.TrimSpace(req.Schedule)
	if err := validateCronSchedule(schedule); err != nil {
		return s.LoadBootstrap(), fmt.Errorf("cron 表达式不合法：%v", err)
	}
	// 有表达式但未启用时同样允许保存（用户可稍后启用），无需额外校验。

	id := strings.TrimSpace(req.OriginalID)
	isEdit := id != ""
	if !isEdit {
		id = newID("automation-template")
	}

	// 不允许通过此接口改预设模板。
	if isEdit {
		if existing, ok := s.findCustomAutomationTemplateLocked(id); ok {
			_ = existing
		} else {
			// 编辑的 ID 不在自定义模板里——可能是预设模板 ID，拒绝。
			return s.LoadBootstrap(), errors.New(s.t("error.automation.template_preset_edit_forbidden"))
		}
	}

	record := AutomationTemplateCard{
		ID:            id,
		Title:         title,
		Description:   strings.TrimSpace(req.Description),
		Prompt:        prompt,
		Schedule:      schedule,
		Enabled:       req.Enabled,
		Preset:        false,
		WorkspacePath: strings.TrimSpace(req.WorkspacePath),
	}

	s.stateMu.Lock()
	replaced := false
	for index, item := range s.customAutomationTemplates {
		if item.ID == id {
			s.customAutomationTemplates[index] = record
			replaced = true
			break
		}
	}
	if !replaced {
		s.customAutomationTemplates = append(s.customAutomationTemplates, record)
	}
	persistErr := s.persistAutomationTemplatesLocked()
	s.stateMu.Unlock()

	s.reloadAutomationSchedules()
	s.emitShellUpdated()
	if persistErr != nil {
		slog.Warn("bridge.automation.persist_failed", "error", persistErr)
	}
	return s.LoadBootstrap(), nil
}

// DeleteAutomationTemplate 删除用户自定义模板（预设模板不可删除）。
func (svc *AutomationService) DeleteAutomationTemplate(id string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return s.LoadBootstrap(), errors.New(s.t("error.automation.template_id_required"))
	}
	s.stateMu.Lock()
	found := false
	next := s.customAutomationTemplates[:0]
	for _, item := range s.customAutomationTemplates {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	s.customAutomationTemplates = next
	persistErr := s.persistAutomationTemplatesLocked()
	s.stateMu.Unlock()

	if !found {
		return s.LoadBootstrap(), errors.New(s.t("error.automation.template_custom_delete_only"))
	}
	s.reloadAutomationSchedules()
	s.emitShellUpdated()
	if persistErr != nil {
		slog.Warn("bridge.automation.persist_failed", "error", persistErr)
	}
	return s.LoadBootstrap(), nil
}

// ToggleAutomationTemplate 启用/停用一个模板（自定义或预设均可）。
// 自定义模板直接改 Enabled；预设模板记录到 presetEnabled。
func (svc *AutomationService) ToggleAutomationTemplate(id string, enabled bool) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return s.LoadBootstrap(), errors.New(s.t("error.automation.template_id_required"))
	}

	template, ok := s.automationTemplateByIDReadOnly(id)
	if !ok {
		return s.LoadBootstrap(), errors.New(s.t("error.automation.template_not_found"))
	}

	// 启用定时调度必须有合法 cron 表达式。
	if enabled && strings.TrimSpace(template.Schedule) == "" {
		return s.LoadBootstrap(), errors.New(s.t("error.automation.template_schedule_required"))
	}
	if err := validateCronSchedule(template.Schedule); err != nil {
		return s.LoadBootstrap(), fmt.Errorf("cron 表达式不合法：%v", err)
	}

	// 自定义模板：改内存字段；预设模板：改 presetEnabled 持久化。
	isCustom := false
	s.stateMu.Lock()
	for index, item := range s.customAutomationTemplates {
		if item.ID == id {
			s.customAutomationTemplates[index].Enabled = enabled
			isCustom = true
			break
		}
	}
	state := s.loadAutomationStore()
	if state.PresetEnabled == nil {
		state.PresetEnabled = map[string]bool{}
	}
	if !isCustom {
		// 预设模板：把启用状态写入 presetEnabled。
		if enabled {
			state.PresetEnabled[id] = true
		} else {
			delete(state.PresetEnabled, id)
		}
	} else {
		// 自定义模板：把整条记录刷新到 store。
		state.Templates = recordsFromCustomTemplatesLocked(s)
	}
	persistErr := s.saveAutomationStore(state)
	s.stateMu.Unlock()

	s.reloadAutomationSchedules()
	s.emitShellUpdated()
	if persistErr != nil {
		slog.Warn("bridge.automation.persist_failed", "error", persistErr)
	}
	return s.LoadBootstrap(), nil
}

// findCustomAutomationTemplateLocked 在自定义模板里查找（需持有 stateMu）。
func (s *BridgeService) findCustomAutomationTemplateLocked(id string) (AutomationTemplateCard, bool) {
	for _, item := range s.customAutomationTemplates {
		if item.ID == id {
			return item, true
		}
	}
	return AutomationTemplateCard{}, false
}

// persistAutomationTemplatesLocked 把当前自定义模板写入磁盘（需持有 stateMu）。
// 预设启用状态也一并保留。
func (s *BridgeService) persistAutomationTemplatesLocked() error {
	state := s.loadAutomationStore()
	state.Templates = recordsFromCustomTemplatesLocked(s)
	return s.saveAutomationStore(state)
}

// recordsFromCustomTemplatesLocked 把内存中的自定义模板转成持久化记录。
func recordsFromCustomTemplatesLocked(s *BridgeService) []automationStoreRecord {
	out := make([]automationStoreRecord, 0, len(s.customAutomationTemplates))
	for _, item := range s.customAutomationTemplates {
		out = append(out, automationStoreRecord{
			ID:            item.ID,
			Title:         item.Title,
			Description:   item.Description,
			Prompt:        item.Prompt,
			Schedule:      item.Schedule,
			Enabled:       item.Enabled,
			Preset:        false,
			WorkspacePath: item.WorkspacePath,
		})
	}
	return out
}
