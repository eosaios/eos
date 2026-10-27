package webbridge

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func hasConfiguredModel(models []ModelCard) bool {
	envBase := strings.TrimSpace(os.Getenv("EOS_API_BASE"))
	envKey := strings.TrimSpace(os.Getenv("EOS_API_KEY"))
	envModel := strings.TrimSpace(os.Getenv("EOS_MODEL"))
	if envBase != "" && envKey != "" && envModel != "" {
		return true
	}
	for _, item := range models {
		if !item.Active {
			continue
		}
		if strings.TrimSpace(item.APIBase) == "" || strings.TrimSpace(item.Model) == "" {
			continue
		}
		if strings.TrimSpace(item.APIKeyMasked) == "" && envKey == "" {
			continue
		}
		return true
	}
	return false
}

func (s *BridgeService) loadModels() []ModelCard {
	models := s.listModelsReadOnly()
	out := make([]ModelCard, 0, len(models))
	for _, item := range models {
		out = append(out, ModelCard{
			Name:                    item.Name,
			APIBase:                 item.APIBase,
			APIKeyMasked:            item.APIKeyMasked,
			Model:                   item.Model,
			Source:                  item.Source,
			Active:                  item.Active,
			SupportsReasoningEffort: item.SupportsReasoningEffort,
			ReasoningLevels:         append([]string(nil), item.ReasoningLevels...),
			SupportsVision:          item.SupportsVision,
			SupportsTools:           item.SupportsTools,
			ProviderID:              item.ProviderID,
			Format:                  item.Format,
			PresetID:                item.PresetID,
			ContextWindow:           item.ContextWindow,
			EditKind:                item.EditKind,
			CanEdit:                 item.CanEdit,
			CanDelete:               item.CanDelete,
		})
	}
	return out
}

// modelCatalogUnavailableTitle is the user-facing name shown in the
// notifications list and the diagnostics ("resource checks") panel when the
// live model catalog could not be loaded. It deliberately avoids internal
// terminology.
const modelCatalogUnavailableTitle = "核心初始化失败"

// modelCatalogUnavailableMessage is the uniform user-facing explanation shown
// when the model catalog is unavailable. It states there is a problem without
// revealing internals (binary path, RPC error, reason go to slog only) and
// without the self-deceiving "please retry" — a real bug won't be fixed by a
// retry; surfacing it lets us fix it for real.
const modelCatalogUnavailableMessage = "模型目录数据不可用。"

// logModelCatalogDegradation records the engineering details behind a missing
// model catalog (binary path, manifest, raw RPC error, reason classification)
// to slog only — never to the UI. reason is a short stable token used to
// group log lines (empty_catalog / rpc_failed / legacy_runtime /
// core_unavailable).
func (s *BridgeService) logModelCatalogDegradation(reason string, err error) {
	args := []any{"reason", reason}
	if err != nil {
		args = append(args, "error", err)
	}
	if b := s.runtimeGatewayResolvedBinary; b.Path != "" || b.ManifestPath != "" || b.Source != "" || b.Target != "" {
		args = append(args,
			"resolved_path", strings.TrimSpace(b.Path),
			"resolved_manifest", strings.TrimSpace(b.ManifestPath),
			"resolved_source", strings.TrimSpace(b.Source),
			"resolved_target", strings.TrimSpace(b.Target),
		)
	}
	if root := strings.TrimSpace(s.runtimeGatewayCoreBinDir); root != "" {
		args = append(args, "root_dir", root)
	}
	slog.Warn("bridge.model_catalog.unavailable", args...)
}

func (s *BridgeService) loadModelCatalog() ModelCatalogState {
	// Reset the fallback reason at the start of every bootstrap so a stale
	// value does not leak into a later diagnostic. 本函数可从多个 emit
	// goroutine 的 loadBootstrap 并发进入：fallback 原因走局部变量，
	// 共享字段收口到 setModelCatalogFallback（stateMu）与只读快照。
	s.setModelCatalogFallback("")
	if s.coreReady() {
		if catalog, err := s.runtimeGatewayClient().CoreModelCatalogRPC(context.Background()); err == nil {
			if len(catalog.Providers) > 0 || len(catalog.Presets) > 0 {
				return s.buildModelCatalogState(catalog)
			}
			s.logModelCatalogDegradation("empty_catalog", nil)
		} else {
			s.logModelCatalogDegradation("rpc_failed", err)
		}
	} else {
		switch s.bridgeCoreMode() {
		case "legacy":
			s.logModelCatalogDegradation("legacy_runtime", nil)
		default:
			s.logModelCatalogDegradation("core_unavailable", nil)
		}
	}
	// User-facing copy is intentionally uniform and free of internal terminology
	// (binary paths, RPC errors, "fallback", etc.); those details live only in
	// slog via logModelCatalogDegradation. This branch should not normally be
	// reached — its presence indicates a core readiness bug to fix separately.
	s.setModelCatalogFallback(modelCatalogUnavailableMessage)
	if fallback := s.modelCatalogFallbackSnapshot(); fallback != "" {
		s.recordModelCatalogFallbackNotification(fallback)
	}
	return ModelCatalogState{
		AllowCustomProvider: true,
		AllowCustomModel:    true,
	}
}

// setModelCatalogFallback 在 stateMu 内更新降级原因（多 emit goroutine 并发
// 写保护；写点收敛于此）。
func (s *BridgeService) setModelCatalogFallback(reason string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.modelCatalogFallback = strings.TrimSpace(reason)
}

// modelCatalogFallbackSnapshot 在 stateMu 内读取降级原因快照。
func (s *BridgeService) modelCatalogFallbackSnapshot() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return strings.TrimSpace(s.modelCatalogFallback)
}

// recordModelCatalogFallbackNotification pushes a one-shot warning when the
// live Rust model catalog could not be loaded.
// The notification is throttled to once per change in fallback reason so the
// user is told about the degradation without seeing a stack of duplicates.
func (s *BridgeService) recordModelCatalogFallbackNotification(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for _, item := range s.notifications {
		if strings.Contains(item.Title, modelCatalogUnavailableTitle) && strings.Contains(item.Message, reason) {
			return
		}
	}
	s.pushNotificationLocked(modelCatalogUnavailableTitle, reason, "warning")
}

func (s *BridgeService) buildModelCatalogState(catalog adapter.ModelCatalogState) ModelCatalogState {
	out := ModelCatalogState{
		Providers:           make([]ModelProviderOption, 0, len(catalog.Providers)),
		Presets:             make([]ModelPresetOption, 0, len(catalog.Presets)),
		AllowCustomProvider: catalog.AllowCustomProvider,
		AllowCustomModel:    catalog.AllowCustomModel,
	}
	for _, provider := range catalog.Providers {
		endpoints := make([]ProviderEndpoint, 0, len(provider.Endpoints))
		for _, ep := range provider.Endpoints {
			endpoints = append(endpoints, ProviderEndpoint{
				Plan:    ep.Plan,
				Format:  ep.Format,
				APIBase: ep.APIBase,
			})
		}
		out.Providers = append(out.Providers, ModelProviderOption{
			ID:            provider.ID,
			Name:          provider.Name,
			Website:       provider.Website,
			APIKeyEnv:     provider.APIKeyEnv,
			Endpoints:     endpoints,
			DefaultModels: append([]string(nil), provider.DefaultModels...),
		})
	}
	for _, preset := range catalog.Presets {
		planModels := make([]PlanModel, 0, len(preset.PlanModels))
		for _, pm := range preset.PlanModels {
			planModels = append(planModels, PlanModel{
				ModelID:                 pm.ModelID,
				Label:                   pm.Label,
				ContextWindow:           pm.ContextWindow,
				SupportsReasoningEffort: pm.SupportsReasoningEffort,
				SupportsVision:          pm.SupportsVision,
				SupportsTools:           pm.SupportsTools,
				ReasoningLevels:         append([]string(nil), pm.ReasoningLevels...),
			})
		}
		out.Presets = append(out.Presets, ModelPresetOption{
			ID:                      preset.ID,
			Name:                    preset.Name,
			ProviderID:              preset.ProviderID,
			ModelName:               preset.ModelName,
			Plan:                    preset.Plan,
			Format:                  preset.Format,
			ContextWindow:           preset.ContextWindow,
			Tags:                    append([]string(nil), preset.Tags...),
			Description:             preset.Description,
			SupportsReasoningEffort: preset.SupportsReasoningEffort,
			ReasoningLevels:         append([]string(nil), preset.ReasoningLevels...),
			SupportsVision:          preset.SupportsVision,
			SupportsTools:           preset.SupportsTools,
			PlanModels:              planModels,
		})
	}
	return out
}

func (s *BridgeService) loadReasoningLevel() string {
	level := strings.TrimSpace(s.reasoningLevelReadOnly())
	if level == "" {
		return "off"
	}
	return level
}
