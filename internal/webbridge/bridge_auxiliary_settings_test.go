package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// auxiliaryConfigBridge 把全局配置指到临时文件。涉及设置读写的用例必须用它：
// 没有内核的桥（空 BridgeService）会把路径回落到用户真实的 ~/.eos.json，
// 11-24 有一次测试就这样往正式配置里写了一个键。
func auxiliaryConfigBridge(t *testing.T) (*BridgeService, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eos.json")
	stub := &gitCommitGatewayStub{configPath: path}
	return &BridgeService{runtimeGateway: stub}, path
}

// readAuxiliarySection 读出文件里 ai_auxiliary 段（JSON 解码后的形态）。
// 文件还不存在时返回空：没有任何保存发生过。
func readAuxiliarySection(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}
	}
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	section := map[string]any{}
	if raw, ok := doc[auxiliarySettingsKey]; ok {
		if err := json.Unmarshal(raw, &section); err != nil {
			t.Fatalf("parse %s: %v", auxiliarySettingsKey, err)
		}
	}
	return section
}

func TestAuxiliarySettingsStartOutAsAuto(t *testing.T) {
	bridge, path := auxiliaryConfigBridge(t)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the fixture must start without a config file: %v", err)
	}
	out := bridge.AuxiliaryAISettings()
	want := AuxiliaryAISettings{Policy: AuxiliaryPolicyAuto}
	if !out.Ok || out.Settings == nil || *out.Settings != want {
		t.Fatalf("settings = %+v, want %+v", out, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("reading created the config file: %v", err)
	}
}

func TestSaveAuxiliarySettingsKeepsOtherKeysAndUsesTheFileSpelling(t *testing.T) {
	bridge, path := auxiliaryConfigBridge(t)
	original := "{\n  \"theme\": \"dark\",\n  \"ai_auxiliary\": {\n    \"policy\": \"byok\"\n  }\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	out := bridge.SaveAuxiliaryAISettings(AuxiliaryAISettings{
		Policy:          " MANAGED ",
		ByokModel:       " mine ",
		NoSessionMode:   "later",
		Ack:             " ack-9 ",
		AckCreditsMicro: 25000000,
		LastPayer:       "BYOK",
	})
	want := AuxiliaryAISettings{
		Policy:          "managed",
		ByokModel:       "mine",
		Ack:             "ack-9",
		AckCreditsMicro: 25000000,
		LastPayer:       "byok",
	}
	if !out.Ok || out.Settings == nil || *out.Settings != want {
		t.Fatalf("saved = %+v, want %+v", out, want)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["theme"] != "dark" {
		t.Errorf("a neighbouring key was lost: %s", raw)
	}
	section, _ := doc[auxiliarySettingsKey].(map[string]any)
	for key, value := range map[string]any{
		"policy":            "managed",
		"byok_model":        "mine",
		"ack":               "ack-9",
		"ack_credits_micro": float64(25000000),
		"last_payer":        "byok",
	} {
		if section[key] != value {
			t.Errorf("%s = %v, want %v", key, section[key], value)
		}
	}
	for _, key := range []string{"no_session_mode", "policy_x", "byokModel"} {
		if _, present := section[key]; present {
			t.Errorf("%s should not be written", key)
		}
	}
	reloaded := bridge.AuxiliaryAISettings()
	if !reloaded.Ok || *reloaded.Settings != want {
		t.Errorf("reloaded = %+v", reloaded.Settings)
	}
}

func TestAnAcknowledgementCannotOutliveItsDisclosure(t *testing.T) {
	bridge, _ := auxiliaryConfigBridge(t)
	// A saved choice that drops the acknowledgement drops its cost limit too.
	out := bridge.SaveAuxiliaryAISettings(AuxiliaryAISettings{Policy: "byok", AckCreditsMicro: 900})
	if out.Settings.AckCreditsMicro != 0 {
		t.Errorf("a cost limit without an acknowledgement was kept: %+v", out.Settings)
	}
	// An unknown policy cannot be stored: the Core refuses to run it.
	out = bridge.SaveAuxiliaryAISettings(AuxiliaryAISettings{Policy: "cheap", NoSessionMode: "maybe"})
	if out.Settings.Policy != AuxiliaryPolicyAuto || out.Settings.NoSessionMode != "" {
		t.Errorf("unknown values were stored: %+v", out.Settings)
	}
	// Confirming requires the service's token.
	if out := bridge.ConfirmCommitDisclosure(CommitMessageConsent{MaxCreditsMicro: 5}); out.Ok || out.Error == "" {
		t.Errorf("an empty token was accepted: %+v", out)
	}
}

func TestACorruptSectionIsReportedAndLeftAlone(t *testing.T) {
	bridge, path := auxiliaryConfigBridge(t)
	original := "{\n  \"ai_auxiliary\": \"garbage\",\n  \"theme\": \"light\"\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := bridge.AuxiliaryAISettings(); out.Ok || out.Settings != nil {
		t.Errorf("a corrupt section read as settings: %+v", out)
	}
	if out := bridge.SaveAuxiliaryAISettings(AuxiliaryAISettings{Policy: "managed"}); out.Ok {
		t.Errorf("a corrupt section was overwritten: %+v", out)
	}
	if raw, _ := os.ReadFile(path); string(raw) != original {
		t.Errorf("the file changed:\n%s", raw)
	}
}
