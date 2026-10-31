// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

package headless_test

import (
	"context"
	"testing"

	"github.com/eosaios/eos/internal/headless"
	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/coreapi/sidecar"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

func TestModelOutputFollowsEffectiveSessionSelection(t *testing.T) {
	for _, name := range []string{"global", "workspace", "session"} {
		t.Run(name, func(t *testing.T) {
			caller := &headlessSessionCaller{replies: map[string]any{
				jsonrpc.MethodSessionCurrent: coreapi.Session{ID: "current"},
				jsonrpc.MethodModelContext:   coreapi.ModelContextSnapshot{ResolvedModelName: name},
				jsonrpc.MethodModelList: []coreapi.ModelConfig{
					{Name: "global", Model: "global-wire", Active: true},
					{Name: "workspace", Model: "workspace-wire"},
					{Name: "session", Model: "session-wire"},
				},
			}}
			got, err := headless.ResolveCurrentModelName(context.Background(), sidecar.NewRemoteEngine(caller))
			if err != nil || got != name+"-wire" {
				t.Fatalf("effective model = %q, %v", got, err)
			}
		})
	}
}
