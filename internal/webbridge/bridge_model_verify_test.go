package webbridge

import (
	"context"
	"errors"
	"testing"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

type modelVerifyGatewayStub struct {
	bridgeRuntimeGateway
	requests []adapter.ModelSaveRequest
	response coreapi.ModelVerifyResponse
	err      error
}

func (g *modelVerifyGatewayStub) CoreVerifyModelRPC(_ context.Context, req adapter.ModelSaveRequest) (coreapi.ModelVerifyResponse, error) {
	g.requests = append(g.requests, req)
	return g.response, g.err
}

func TestVerifyModelForwardsRequestAndMapsResult(t *testing.T) {
	gateway := &modelVerifyGatewayStub{response: coreapi.ModelVerifyResponse{Ok: true, LatencyMs: 826}}
	service := &BridgeService{runtimeGateway: gateway}
	svc := NewCapabilityService(service)

	result, err := svc.VerifyModel(ModelSaveRequest{
		Mode:         "custom_provider",
		Name:         "  DeepSeek V4 Pro  ",
		APIKey:       "  sk-test  ",
		APIBase:      "  https://api.deepseek.com  ",
		Model:        "  deepseek-v4-pro  ",
		OriginalName: " old name ",
	})
	if err != nil {
		t.Fatalf("VerifyModel returned error: %v", err)
	}
	if len(gateway.requests) != 1 {
		t.Fatalf("expected exactly one gateway call, got %d", len(gateway.requests))
	}
	sent := gateway.requests[0]
	if sent.Mode != "custom_provider" || sent.Name != "DeepSeek V4 Pro" || sent.APIBase != "https://api.deepseek.com" || sent.Model != "deepseek-v4-pro" || sent.OriginalName != "old name" {
		t.Fatalf("gateway request fields not trimmed as expected: %+v", sent)
	}
	if sent.APIKey != "sk-test" {
		t.Fatalf("API key should be trimmed, got %q", sent.APIKey)
	}
	if !result.OK || result.LatencyMS != 826 || result.Message != "" {
		t.Fatalf("unexpected verify result: %+v", result)
	}
}

func TestVerifyModelFailedCheckIsResultNotError(t *testing.T) {
	gateway := &modelVerifyGatewayStub{response: coreapi.ModelVerifyResponse{
		Ok:      false,
		Message: "invalid api key",
	}}
	service := &BridgeService{runtimeGateway: gateway}
	svc := NewCapabilityService(service)

	result, err := svc.VerifyModel(ModelSaveRequest{Mode: "custom_provider", Name: "x"})
	if err != nil {
		t.Fatalf("failed check must surface as result, not transport error: %v", err)
	}
	if result.OK {
		t.Fatal("expected ok=false to propagate")
	}
	if result.Message != "invalid api key" {
		t.Fatalf("expected failure reason to propagate, got %q", result.Message)
	}
}

func TestVerifyModelSurfacesTransportError(t *testing.T) {
	gateway := &modelVerifyGatewayStub{err: errors.New("core unavailable")}
	service := &BridgeService{runtimeGateway: gateway}
	svc := NewCapabilityService(service)

	if _, err := svc.VerifyModel(ModelSaveRequest{Mode: "preset"}); err == nil {
		t.Fatal("expected transport error to propagate")
	}
}
