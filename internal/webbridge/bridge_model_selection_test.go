package webbridge

import (
	"context"
	"errors"
	"testing"
)

type modelSelectionGatewayStub struct {
	bridgeRuntimeGateway
	sessionCalls   []string
	workspaceCalls []string
	workspaceErr   error
}

func (g *modelSelectionGatewayStub) CoreSetSessionModelRPC(_ context.Context, sessionID, modelName string) error {
	g.sessionCalls = append(g.sessionCalls, sessionID+":"+modelName)
	return nil
}

func (g *modelSelectionGatewayStub) CoreSetWorkspaceModelRPC(_ context.Context, workspaceRoot, modelName string) error {
	g.workspaceCalls = append(g.workspaceCalls, workspaceRoot+":"+modelName)
	return g.workspaceErr
}

func TestSelectCurrentModelRPCSessionScopeUpdatesWorkspaceDefault(t *testing.T) {
	gateway := &modelSelectionGatewayStub{}
	service := &BridgeService{runtimeGateway: gateway}

	scope, err := service.selectCurrentModelRPC("/ws/project", "session-1", "gpt-5")
	if err != nil {
		t.Fatalf("selectCurrentModelRPC returned error: %v", err)
	}
	if scope != "session" {
		t.Fatalf("expected session scope, got %q", scope)
	}
	if len(gateway.sessionCalls) != 1 || gateway.sessionCalls[0] != "session-1:gpt-5" {
		t.Fatalf("unexpected session calls: %v", gateway.sessionCalls)
	}
	if len(gateway.workspaceCalls) != 1 || gateway.workspaceCalls[0] != "/ws/project:gpt-5" {
		t.Fatalf("expected workspace default to follow latest selection, got %v", gateway.workspaceCalls)
	}
}

func TestSelectCurrentModelRPCSessionScopeWithoutWorkspaceSkipsDefault(t *testing.T) {
	gateway := &modelSelectionGatewayStub{}
	service := &BridgeService{runtimeGateway: gateway}

	scope, err := service.selectCurrentModelRPC("  ", "session-1", "gpt-5")
	if err != nil {
		t.Fatalf("selectCurrentModelRPC returned error: %v", err)
	}
	if scope != "session" {
		t.Fatalf("expected session scope, got %q", scope)
	}
	if len(gateway.sessionCalls) != 1 {
		t.Fatalf("unexpected session calls: %v", gateway.sessionCalls)
	}
	if len(gateway.workspaceCalls) != 0 {
		t.Fatalf("expected no workspace calls, got %v", gateway.workspaceCalls)
	}
}

func TestSelectCurrentModelRPCWorkspaceScope(t *testing.T) {
	gateway := &modelSelectionGatewayStub{}
	service := &BridgeService{runtimeGateway: gateway}

	scope, err := service.selectCurrentModelRPC("/ws/project", "", "gpt-5")
	if err != nil {
		t.Fatalf("selectCurrentModelRPC returned error: %v", err)
	}
	if scope != "workspace" {
		t.Fatalf("expected workspace scope, got %q", scope)
	}
	if len(gateway.workspaceCalls) != 1 || gateway.workspaceCalls[0] != "/ws/project:gpt-5" {
		t.Fatalf("unexpected workspace calls: %v", gateway.workspaceCalls)
	}
	if len(gateway.sessionCalls) != 0 {
		t.Fatalf("expected no session calls, got %v", gateway.sessionCalls)
	}
}

func TestSelectCurrentModelRPCWorkspaceFailureIsBestEffort(t *testing.T) {
	gateway := &modelSelectionGatewayStub{workspaceErr: errors.New("workspace write failed")}
	service := &BridgeService{runtimeGateway: gateway}

	scope, err := service.selectCurrentModelRPC("/ws/project", "session-1", "gpt-5")
	if err != nil {
		t.Fatalf("session selection should not fail when workspace default update fails: %v", err)
	}
	if scope != "session" {
		t.Fatalf("expected session scope, got %q", scope)
	}
	if len(gateway.sessionCalls) != 1 {
		t.Fatalf("unexpected session calls: %v", gateway.sessionCalls)
	}
}
