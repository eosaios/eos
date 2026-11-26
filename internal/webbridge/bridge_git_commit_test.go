package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// gitCommitGatewayStub 是最小内核 stub：只实现提交信息路由涉及的方法，并把
// 全局配置指到一个临时文件，测试不碰用户的 ~/.eos.json。
type gitCommitGatewayStub struct {
	bridgeRuntimeGateway
	configPath string
	supported  bool
	supportErr error
	result     coreapi.GitSuggestMessageResult
	rpcErr     error
	// 非空时等它关闭或 ctx 结束，模拟一次迟迟不回的生成。
	block <-chan struct{}

	mu   sync.Mutex
	wire []coreapi.GitSuggestMessageRequest
}

func (g *gitCommitGatewayStub) CoreConfigPath() string { return g.configPath }

func (g *gitCommitGatewayStub) CoreGitSuggestRoutingSupportedRPC(context.Context) (bool, error) {
	return g.supported, g.supportErr
}

func (g *gitCommitGatewayStub) CoreGitCommitMessageRPC(ctx context.Context, request coreapi.GitSuggestMessageRequest) (coreapi.GitSuggestMessageResult, error) {
	g.mu.Lock()
	g.wire = append(g.wire, request)
	g.mu.Unlock()
	if g.block != nil {
		select {
		case <-g.block:
		case <-ctx.Done():
			return coreapi.GitSuggestMessageResult{}, ctx.Err()
		}
	}
	return g.result, g.rpcErr
}

func (g *gitCommitGatewayStub) requests() []coreapi.GitSuggestMessageRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]coreapi.GitSuggestMessageRequest(nil), g.wire...)
}

func newCommitBridge(t *testing.T) (*BridgeService, *gitCommitGatewayStub) {
	t.Helper()
	stub := &gitCommitGatewayStub{
		configPath: filepath.Join(t.TempDir(), "eos.json"),
		supported:  true,
	}
	return &BridgeService{runtimeGateway: stub}, stub
}

func TestGenerateCommitMessageSendsTheRoutingAndReturnsTheCharge(t *testing.T) {
	bridge, stub := newCommitBridge(t)
	stub.result = coreapi.GitSuggestMessageResult{
		Message:      "  fix(git): tighten check  ",
		Payer:        "managed",
		CreditsMicro: 1736000,
		RequestID:    "req-1",
		Redactions:   2,
		Truncated:    true,
	}
	out := bridge.GenerateCommitMessage(GitCommitMessageRequest{
		WorkspaceRoot: " /repo ",
		SessionID:     "s-1",
		RequestKey:    "k-1",
		Policy:        "auto",
		ByokModel:     "mine",
		NoSessionMode: "byok",
		Ack:           "ack-9",
		ExpectedPayer: "managed",
	})
	want := GitCommitMessageResult{
		Ok:           true,
		Message:      "fix(git): tighten check",
		Payer:        "managed",
		CreditsMicro: 1736000,
		RequestID:    "req-1",
		Redactions:   2,
		Truncated:    true,
	}
	if out != want {
		t.Fatalf("result = %+v, want %+v", out, want)
	}
	requests := stub.requests()
	if len(requests) != 1 {
		t.Fatalf("kernel called %d times, want 1", len(requests))
	}
	routing := requests[0].Routing
	if routing == nil {
		t.Fatal("no routing reached the kernel")
	}
	if requests[0].WorkspaceRoot != "/repo" {
		t.Errorf("workspace root = %q", requests[0].WorkspaceRoot)
	}
	for field, got := range map[string]string{
		"policy":          routing.Policy,
		"byok model":      routing.ByokModel,
		"session":         routing.SessionID,
		"no-session mode": routing.NoSessionMode,
		"request key":     routing.RequestKey,
		"ack":             routing.Ack,
		"expected payer":  routing.ExpectedPayer,
	} {
		if got == "" {
			t.Errorf("%s did not reach the kernel", field)
		}
	}
	if routing.Probe {
		t.Error("a generation was sent as a probe")
	}
	// The payer of a real generation is what the interface compares against next time.
	section := readAuxiliarySection(t, stub.configPath)
	if section["last_payer"] != "managed" {
		t.Errorf("last payer = %v, want managed", section["last_payer"])
	}
}

func TestCommitMessageRouteOnlyAsks(t *testing.T) {
	bridge, stub := newCommitBridge(t)
	stub.result = coreapi.GitSuggestMessageResult{Payer: "byok"}
	out := bridge.CommitMessageRoute(GitCommitMessageRequest{ExpectedPayer: "managed", RequestKey: "k-2"})
	want := CommitMessageRouteResult{Ok: true, Payer: "byok"}
	if out != want {
		t.Fatalf("route = %+v, want %+v", out, want)
	}
	requests := stub.requests()
	if len(requests) != 1 || requests[0].Routing == nil {
		t.Fatalf("kernel asked %d times", len(requests))
	}
	routing := requests[0].Routing
	if !routing.Probe {
		t.Error("the route question was not marked as a probe")
	}
	if routing.ExpectedPayer != "" {
		t.Errorf("a probe has nothing to check its payer against: %+v", routing)
	}
	if routing.Policy != AuxiliaryPolicyAuto {
		t.Errorf("an unstated policy = %q, want auto", routing.Policy)
	}
	if _, err := os.Stat(stub.configPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("asking who pays wrote the config file: %v", err)
	}
}

func TestCommitMessagesNeedAKernelThatRoutes(t *testing.T) {
	bridge, stub := newCommitBridge(t)
	stub.supported = false
	out := bridge.GenerateCommitMessage(GitCommitMessageRequest{RequestKey: "k-3"})
	if out.Ok || out.Code != "AI_CORE_UNSUPPORTED" {
		t.Fatalf("old kernel: %+v", out)
	}
	if len(stub.requests()) != 0 {
		t.Fatal("an old kernel must not be asked to generate: it would use the global default model")
	}
	// No kernel at all.
	bare := (&BridgeService{}).GenerateCommitMessage(GitCommitMessageRequest{})
	if bare.Code != "AI_CORE_UNSUPPORTED" {
		t.Errorf("no kernel: %+v", bare)
	}
	// The handshake itself failed.
	stub.supportErr = errors.New("kernel exited")
	if out := bridge.CommitMessageRoute(GitCommitMessageRequest{}); out.Ok || out.Code != "" || !out.Retryable {
		t.Errorf("handshake failure: %+v", out)
	}
}

func TestCommitFailuresKeepTheStableReason(t *testing.T) {
	bridge, stub := newCommitBridge(t)
	cases := []struct {
		name      string
		err       error
		code      string
		detail    string
		retryable bool
	}{
		{"sign-in", &protocoljsonrpc.RPCError{Reason: "AI_LOGIN_REQUIRED: Sign in to EOS"}, "AI_LOGIN_REQUIRED", "Sign in to EOS", false},
		{"subscription", &protocoljsonrpc.RPCError{Reason: "AI_SUBSCRIPTION_REQUIRED: no plan"}, "AI_SUBSCRIPTION_REQUIRED", "no plan", false},
		{"credits", &protocoljsonrpc.RPCError{Reason: "AI_CREDITS_EXHAUSTED: 0 left"}, "AI_CREDITS_EXHAUSTED", "0 left", false},
		{"timeout", &protocoljsonrpc.RPCError{Reason: "AI_BYOK_TIMEOUT: slow model"}, "AI_BYOK_TIMEOUT", "slow model", true},
		{"nothing staged", &protocoljsonrpc.RPCError{Reason: "AI_NOTHING_STAGED: no staged changes to describe"}, "AI_NOTHING_STAGED", "no staged changes to describe", false},
		{"payer moved", &protocoljsonrpc.RPCError{Reason: "AI_ROUTE_CHANGED: the model would now be provided by managed"}, "AI_ROUTE_CHANGED", "the model would now be provided by managed", false},
	}
	for _, test := range cases {
		stub.rpcErr = test.err
		out := bridge.GenerateCommitMessage(GitCommitMessageRequest{RequestKey: "k-4-" + test.name})
		if out.Ok || out.Code != test.code || out.Error != test.detail || out.Retryable != test.retryable {
			t.Errorf("%s: %+v", test.name, out)
		}
	}
	stub.rpcErr = errors.New("pipe closed")
	out := bridge.GenerateCommitMessage(GitCommitMessageRequest{RequestKey: "k-4-plain"})
	if out.Ok || out.Code != "" || out.Error != "pipe closed" || !out.Retryable {
		t.Errorf("an error without a reason stays retryable: %+v", out)
	}
	if section := readAuxiliarySection(t, stub.configPath); section["last_payer"] != nil {
		t.Errorf("a failed generation must not record a payer: %v", section)
	}
}

func TestCancelCommitMessageStopsWaiting(t *testing.T) {
	bridge, stub := newCommitBridge(t)
	release := make(chan struct{})
	stub.block = release
	defer close(release)
	done := make(chan GitCommitMessageResult, 1)
	go func() {
		done <- bridge.GenerateCommitMessage(GitCommitMessageRequest{RequestKey: " k-5 "})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !commitMessageWaiting("k-5") {
		if time.Now().After(deadline) {
			t.Fatal("the generation was never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if out := bridge.CancelCommitMessage("k-5"); !out.Cancelled {
		t.Fatalf("cancel = %+v", out)
	}
	select {
	case out := <-done:
		if out.Code != "AI_CANCELLED" || out.Ok {
			t.Errorf("cancelled generation = %+v", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling did not end the wait")
	}
	if out := bridge.CancelCommitMessage("k-5"); out.Cancelled {
		t.Errorf("a finished generation cannot be cancelled again: %+v", out)
	}
}

func commitMessageWaiting(key string) bool {
	gitCommitCalls.Lock()
	defer gitCommitCalls.Unlock()
	_, waiting := gitCommitCalls.cancel[key]
	return waiting
}

func TestConsentComesBackWithoutRecordingAPayer(t *testing.T) {
	bridge, stub := newCommitBridge(t)
	stub.result = coreapi.GitSuggestMessageResult{
		Payer: "managed",
		Consent: &coreapi.GitSuggestConsent{
			Disclosure:             "暂存区代码变更将发送到 EOS 托管模型。",
			Ack:                    "ack-9",
			MaxRequestCreditsMicro: 25000000,
		},
	}
	out := bridge.GenerateCommitMessage(GitCommitMessageRequest{RequestKey: "k-6", Policy: "managed"})
	want := &CommitMessageConsent{
		Disclosure:      "暂存区代码变更将发送到 EOS 托管模型。",
		Ack:             "ack-9",
		MaxCreditsMicro: 25000000,
	}
	if !out.Ok || out.Message != "" || out.Payer != "managed" || out.CreditsMicro != 0 {
		t.Fatalf("result = %+v", out)
	}
	if out.Consent == nil || *out.Consent != *want {
		t.Fatalf("consent = %+v, want %+v", out.Consent, want)
	}
	if section := readAuxiliarySection(t, stub.configPath); section["last_payer"] != nil {
		t.Errorf("nothing was generated, so no payer is recorded: %v", section)
	}
	// The confirmed disclosure is kept for later generations, without touching the rest.
	confirmed := bridge.ConfirmCommitDisclosure(*out.Consent)
	if !confirmed.Ok || confirmed.Settings.Ack != "ack-9" || confirmed.Settings.AckCreditsMicro != 25000000 {
		t.Fatalf("confirm = %+v", confirmed)
	}
	if confirmed.Settings.LastPayer != "" {
		t.Errorf("confirming is not using: %+v", confirmed.Settings)
	}
	if again := bridge.AuxiliaryAISettings(); !again.Ok || again.Settings.Ack != "ack-9" {
		t.Errorf("reload = %+v", again)
	}
	if section := readAuxiliarySection(t, stub.configPath); section["ack_credits_micro"] != float64(25000000) {
		t.Errorf("file = %v", section)
	}
}
