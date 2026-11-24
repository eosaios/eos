package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// 辅助 AI 任务（如提交信息）的三种模型使用方式，与界面选项一一对应。
const (
	AuxiliaryPolicyAuto    = "auto"
	AuxiliaryPolicyManaged = "managed"
	AuxiliaryPolicyByok    = "byok"
)

// auxiliarySettingsKey 是 ~/.eos.json 里这一段的名字。
const auxiliarySettingsKey = "ai_auxiliary"

// AuxiliaryAISettings 是辅助 AI 任务的全局设置：这类小任务不进对话，
// 用谁的模型、由谁付费单独决定。
type AuxiliaryAISettings struct {
	Policy string `json:"policy"`
	// 「自定义 BYOK 模型」所选的个人模型名。
	ByokModel string `json:"byokModel,omitempty"`
	// 无关联会话时已保存的选择（managed | byok），空 = 尚未选择。
	NoSessionMode string `json:"noSessionMode,omitempty"`
	// 用户已确认的托管告知版本，及其当时的单次费用上限（百万分之一积分）。
	Ack             string `json:"ack,omitempty"`
	AckCreditsMicro int64  `json:"ackCreditsMicro,omitempty"`
	// 上一次生成实际由谁付费（managed | byok）。
	LastPayer string `json:"lastPayer,omitempty"`
}

type AuxiliaryAISettingsResult struct {
	Ok       bool                 `json:"ok"`
	Error    string               `json:"error,omitempty"`
	Settings *AuxiliaryAISettings `json:"settings,omitempty"`
}

// auxiliaryDocument 是 ai_auxiliary 段的文件形态（snake_case，与
// ~/.eos.json 现有键风格一致）。
type auxiliaryDocument struct {
	Policy          string `json:"policy,omitempty"`
	ByokModel       string `json:"byok_model,omitempty"`
	NoSessionMode   string `json:"no_session_mode,omitempty"`
	Ack             string `json:"ack,omitempty"`
	AckCreditsMicro int64  `json:"ack_credits_micro,omitempty"`
	LastPayer       string `json:"last_payer,omitempty"`
}

// 同一段的读改写在进程内串行，两次相邻的保存不会互相覆盖。
var auxiliarySettingsMu sync.Mutex

// auxiliaryModeName 只认 managed 与 byok 两个取值，其余视为未设置。策略、
// 无会话选择与费用承担方用的是同一对名字：选谁，就是谁付费。
func auxiliaryModeName(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case AuxiliaryPolicyManaged, AuxiliaryPolicyByok:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

// normalizeAuxiliarySettings 归一化输入。无法识别的策略回落为「自动」：
// 内核对未知策略一律拒绝调用，存一个调用不了的值没有意义。
func normalizeAuxiliarySettings(settings AuxiliaryAISettings) AuxiliaryAISettings {
	normalized := AuxiliaryAISettings{
		Policy:          auxiliaryModeName(settings.Policy),
		ByokModel:       strings.TrimSpace(settings.ByokModel),
		NoSessionMode:   auxiliaryModeName(settings.NoSessionMode),
		Ack:             strings.TrimSpace(settings.Ack),
		AckCreditsMicro: max(settings.AckCreditsMicro, 0),
		LastPayer:       auxiliaryModeName(settings.LastPayer),
	}
	if normalized.Policy == "" {
		normalized.Policy = AuxiliaryPolicyAuto
	}
	// 没有已确认的告知版本时，费用上限无所依附。
	if normalized.Ack == "" {
		normalized.AckCreditsMicro = 0
	}
	return normalized
}

func auxiliaryDocumentOf(settings AuxiliaryAISettings) auxiliaryDocument {
	return auxiliaryDocument{
		Policy:          settings.Policy,
		ByokModel:       settings.ByokModel,
		NoSessionMode:   settings.NoSessionMode,
		Ack:             settings.Ack,
		AckCreditsMicro: settings.AckCreditsMicro,
		LastPayer:       settings.LastPayer,
	}
}

func (document auxiliaryDocument) settings() AuxiliaryAISettings {
	return AuxiliaryAISettings{
		Policy:          document.Policy,
		ByokModel:       document.ByokModel,
		NoSessionMode:   document.NoSessionMode,
		Ack:             document.Ack,
		AckCreditsMicro: document.AckCreditsMicro,
		LastPayer:       document.LastPayer,
	}
}

// auxiliarySection 取出文件里的 ai_auxiliary 段；没有这一段时是零值。
func auxiliarySection(doc map[string]json.RawMessage) (auxiliaryDocument, error) {
	section := auxiliaryDocument{}
	raw, ok := doc[auxiliarySettingsKey]
	if !ok {
		return section, nil
	}
	if err := json.Unmarshal(raw, &section); err != nil {
		return auxiliaryDocument{}, fmt.Errorf("parse auxiliary AI settings: %w", err)
	}
	return section, nil
}

// readAuxiliarySettings 只读 ai_auxiliary 段，不改动文件。
func (s *BridgeService) readAuxiliarySettings() (AuxiliaryAISettings, error) {
	auxiliarySettingsMu.Lock()
	defer auxiliarySettingsMu.Unlock()
	doc, err := loadJSONDocument(resolveGlobalConfigPath(s.configPathReadOnly()))
	if err != nil {
		return AuxiliaryAISettings{}, fmt.Errorf("read auxiliary AI settings: %w", err)
	}
	section, err := auxiliarySection(doc)
	if err != nil {
		return AuxiliaryAISettings{}, err
	}
	return normalizeAuxiliarySettings(section.settings()), nil
}

// updateAuxiliarySettings 读改写 ai_auxiliary 段，文件里的其余键原样保留。
func (s *BridgeService) updateAuxiliarySettings(change func(*auxiliaryDocument)) error {
	auxiliarySettingsMu.Lock()
	defer auxiliarySettingsMu.Unlock()
	path := resolveGlobalConfigPath(s.configPathReadOnly())
	doc, err := loadJSONDocument(path)
	if err != nil {
		return fmt.Errorf("read auxiliary AI settings: %w", err)
	}
	section, err := auxiliarySection(doc)
	if err != nil {
		return err
	}
	change(&section)
	if err := setJSONValue(doc, auxiliarySettingsKey, section); err != nil {
		return err
	}
	if err := writeJSONDocument(path, doc); err != nil {
		return fmt.Errorf("write auxiliary AI settings: %w", err)
	}
	return nil
}

// AuxiliaryAISettings 读取辅助 AI 任务设置；文件里还没有这一段时返回默认值。
func (s *BridgeService) AuxiliaryAISettings() AuxiliaryAISettingsResult {
	if s == nil {
		return AuxiliaryAISettingsResult{}
	}
	settings, err := s.readAuxiliarySettings()
	if err != nil {
		return AuxiliaryAISettingsResult{Error: err.Error()}
	}
	return AuxiliaryAISettingsResult{Ok: true, Settings: &settings}
}

// SaveAuxiliaryAISettings 保存辅助 AI 任务设置。
func (s *BridgeService) SaveAuxiliaryAISettings(settings AuxiliaryAISettings) AuxiliaryAISettingsResult {
	if s == nil {
		return AuxiliaryAISettingsResult{}
	}
	normalized := normalizeAuxiliarySettings(settings)
	err := s.updateAuxiliarySettings(func(section *auxiliaryDocument) {
		*section = auxiliaryDocumentOf(normalized)
	})
	if err != nil {
		return AuxiliaryAISettingsResult{Error: err.Error()}
	}
	return AuxiliaryAISettingsResult{Ok: true, Settings: &normalized}
}

// ConfirmCommitDisclosure 记下用户已确认的托管告知：这一版告知下的托管
// 生成不再逐次确认，直到服务端的告知文案或费用上限变化。
func (s *BridgeService) ConfirmCommitDisclosure(consent CommitMessageConsent) AuxiliaryAISettingsResult {
	if s == nil {
		return AuxiliaryAISettingsResult{}
	}
	ack := strings.TrimSpace(consent.Ack)
	if ack == "" {
		return AuxiliaryAISettingsResult{Error: "告知版本为空"}
	}
	normalized := AuxiliaryAISettings{}
	err := s.updateAuxiliarySettings(func(section *auxiliaryDocument) {
		section.Ack = ack
		section.AckCreditsMicro = max(consent.MaxCreditsMicro, 0)
		normalized = normalizeAuxiliarySettings(section.settings())
	})
	if err != nil {
		return AuxiliaryAISettingsResult{Error: err.Error()}
	}
	return AuxiliaryAISettingsResult{Ok: true, Settings: &normalized}
}

// rememberCommitPayer 记住上一次生成实际由谁付费。费用承担方从 BYOK 变为
// 托管时，界面据此要求用户重新确认。写入失败只记日志：提交信息已经生成
// 并已结算，不能因为记不下这一笔而丢弃结果。
func (s *BridgeService) rememberCommitPayer(payer string) {
	payer = auxiliaryModeName(payer)
	if payer == "" {
		return
	}
	err := s.updateAuxiliarySettings(func(section *auxiliaryDocument) {
		section.LastPayer = payer
	})
	if err != nil {
		slog.Error("commit.payer", "error", err)
	}
}
