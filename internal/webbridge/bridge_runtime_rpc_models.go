package webbridge

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

func (s *BridgeService) listModelsReadOnly() []adapter.ModelConfig {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return nil
	}
	return coreOnlyValue(
		gateway,
		[]adapter.ModelConfig(nil),
		func(g bridgeRuntimeGateway) ([]adapter.ModelConfig, error) {
			return g.CoreListModelsRPC(coreCtx())
		},
	)
}

type bridgeRuntimeGatewayModelContextReader interface {
	CoreModelContextRPC(context.Context, adapter.ModelContextRequest) (adapter.ModelContextSnapshot, error)
}

type bridgeRuntimeGatewayModelContextWriter interface {
	CoreSetWorkspaceModelRPC(context.Context, string, string) error
	CoreSetSessionModelRPC(context.Context, string, string) error
}

func modelContextFromAdapter(snapshot adapter.ModelContextSnapshot) ModelContextSnapshot {
	return ModelContextSnapshot{
		WorkspaceRoot:      strings.TrimSpace(snapshot.WorkspaceRoot),
		SessionID:          strings.TrimSpace(snapshot.SessionID),
		GlobalDefaultName:  strings.TrimSpace(snapshot.GlobalDefaultName),
		WorkspaceModelName: strings.TrimSpace(snapshot.WorkspaceModelName),
		SessionModelName:   strings.TrimSpace(snapshot.SessionModelName),
		ResolvedModelName:  strings.TrimSpace(snapshot.ResolvedModelName),
		ResolvedScope:      strings.TrimSpace(snapshot.ResolvedScope),
	}
}

func (s *BridgeService) loadModelContext(workspaceRoot, sessionID string) ModelContextSnapshot {
	if s == nil || s.runtimeGatewayClient() == nil {
		return ModelContextSnapshot{}
	}
	reader, ok := s.runtimeGatewayClient().(bridgeRuntimeGatewayModelContextReader)
	if !ok {
		return ModelContextSnapshot{}
	}
	snapshot, err := reader.CoreModelContextRPC(coreCtx(), adapter.ModelContextRequest{
		WorkspaceRoot: strings.TrimSpace(workspaceRoot),
		SessionID:     strings.TrimSpace(sessionID),
	})
	if err != nil {
		return ModelContextSnapshot{}
	}
	return modelContextFromAdapter(snapshot)
}

func (s *BridgeService) selectCurrentModelRPC(workspaceRoot, sessionID, modelName string) (string, error) {
	if s == nil || s.runtimeGatewayClient() == nil {
		return "", errors.New("runtime core unavailable")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", errors.New("model name is required")
	}
	writer, ok := s.runtimeGatewayClient().(bridgeRuntimeGatewayModelContextWriter)
	if strings.TrimSpace(sessionID) != "" && ok {
		if err := writer.CoreSetSessionModelRPC(coreCtx(), strings.TrimSpace(sessionID), modelName); err != nil {
			return "", err
		}
		// 会话内切换模型时同步刷新 workspace 默认模型：新会话没有会话级模型，
		// 会按 workspace -> global 顺序回落，因此这里让新会话继承「最近一次选择」，
		// 而不是一直停留在旧的全局默认。
		if workspaceRoot = strings.TrimSpace(workspaceRoot); workspaceRoot != "" {
			if err := writer.CoreSetWorkspaceModelRPC(coreCtx(), workspaceRoot, modelName); err != nil {
				slog.Warn("bridge.select_current_model.workspace_default_failed", "workspace", workspaceRoot, "error", err)
			}
		}
		return "session", nil
	}
	if strings.TrimSpace(workspaceRoot) != "" && ok {
		if err := writer.CoreSetWorkspaceModelRPC(coreCtx(), strings.TrimSpace(workspaceRoot), modelName); err != nil {
			return "", err
		}
		return "workspace", nil
	}
	if err := s.activateModelRPC(modelName); err != nil {
		return "", err
	}
	return "global", nil
}

func (s *BridgeService) upsertModelRPC(name, base, keyMasked, model string) error {
	name = strings.TrimSpace(name)
	base = strings.TrimSpace(base)
	keyMasked = strings.TrimSpace(keyMasked)
	model = strings.TrimSpace(model)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error {
			return g.CoreUpsertModelRPC(coreCtx(), name, base, keyMasked, model)
		},
	)
}

func (s *BridgeService) saveModelRPC(req adapter.ModelSaveRequest) error {
	req.OriginalName = strings.TrimSpace(req.OriginalName)
	req.Mode = strings.TrimSpace(req.Mode)
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	req.PresetID = strings.TrimSpace(req.PresetID)
	req.Name = strings.TrimSpace(req.Name)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.APIBase = strings.TrimSpace(req.APIBase)
	req.Model = strings.TrimSpace(req.Model)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreSaveModelRPC(coreCtx(), req) },
	)
}

func (s *BridgeService) verifyModelRPC(req adapter.ModelSaveRequest) (coreapi.ModelVerifyResponse, error) {
	req.OriginalName = strings.TrimSpace(req.OriginalName)
	req.Mode = strings.TrimSpace(req.Mode)
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	req.PresetID = strings.TrimSpace(req.PresetID)
	req.Name = strings.TrimSpace(req.Name)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.APIBase = strings.TrimSpace(req.APIBase)
	req.Model = strings.TrimSpace(req.Model)
	return coreValueOrRequire(
		s,
		func(g bridgeRuntimeGateway) (coreapi.ModelVerifyResponse, error) {
			return g.CoreVerifyModelRPC(coreCtx(), req)
		},
	)
}

func (s *BridgeService) activateModelRPC(name string) error {
	name = strings.TrimSpace(name)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreActivateModelRPC(coreCtx(), name) },
	)
}

func (s *BridgeService) deleteModelRPC(name string) error {
	name = strings.TrimSpace(name)
	return coreErrOrRequire(
		s,
		func(g bridgeRuntimeGateway) error { return g.CoreDeleteModelRPC(coreCtx(), name) },
	)
}
