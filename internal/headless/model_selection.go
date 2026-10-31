// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

package headless

import (
	"context"
	"fmt"
	"strings"

	"github.com/eosaios/eos/pkg/coreapi"
)

// ResolveCurrentModelName reports the session's effective wire model, including
// session/workspace overrides. The global default does not describe a turn.
func ResolveCurrentModelName(ctx context.Context, engine coreapi.Engine) (string, error) {
	if engine == nil {
		return "", fmt.Errorf("core engine unavailable")
	}
	workspace := WorkspaceRoot(ctx, engine)
	session, err := engine.Sessions().Current(ctx, coreapi.CurrentSessionRequest{WorkspaceRoot: workspace})
	if err != nil {
		return "", fmt.Errorf("resolve current model session: %w", err)
	}
	selection, err := engine.Models().Context(ctx, coreapi.ModelContextRequest{WorkspaceRoot: workspace, SessionID: session.ID})
	if err != nil {
		return "", fmt.Errorf("resolve current model: %w", err)
	}
	if selection.ResolvedModelName == "" {
		return "", nil
	}
	models, err := engine.Models().List(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve current model list: %w", err)
	}
	for _, model := range models {
		if model.Name == selection.ResolvedModelName {
			return strings.TrimSpace(model.Model), nil
		}
	}
	return "", fmt.Errorf("resolved model %q is missing from configuration", selection.ResolvedModelName)
}
