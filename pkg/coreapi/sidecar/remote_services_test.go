package sidecar

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"context"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
	"github.com/eosaios/eos/pkg/sandbox"
)

func TestRemoteServicesNilCaller(t *testing.T) {
	e := &RemoteEngine{}
	ctx := context.Background()

	// Sessions
	s := e.Sessions()
	if _, err := s.Current(ctx, coreapi.CurrentSessionRequest{}); err == nil {
		t.Fatal("current")
	}
	if _, err := s.Create(ctx, coreapi.CreateSessionRequest{}); err == nil {
		t.Fatal("create")
	}
	if _, err := s.List(ctx, coreapi.ListSessionsRequest{}); err == nil {
		t.Fatal("list")
	}
	if _, err := s.Resume(ctx, coreapi.ResumeSessionRequest{}); err == nil {
		t.Fatal("resume")
	}
	if err := s.SetCurrent(ctx, coreapi.SetCurrentSessionRequest{}); err == nil {
		t.Fatal("setcurrent")
	}
	if err := s.Delete(ctx, coreapi.DeleteSessionRequest{}); err == nil {
		t.Fatal("delete")
	}
	if _, err := s.Rename(ctx, coreapi.RenameSessionRequest{}); err == nil {
		t.Fatal("rename")
	}
	if _, err := s.SetMeta(ctx, coreapi.SetSessionMetaRequest{}); err == nil {
		t.Fatal("setmeta")
	}
	if _, err := s.LoadMessages(ctx, coreapi.LoadSessionMessagesRequest{}); err == nil {
		t.Fatal("loadmsgs")
	}
	if _, err := s.SaveMessages(ctx, coreapi.SaveSessionMessagesRequest{}); err == nil {
		t.Fatal("savemsgs")
	}

	// Turns
	tu := e.Turns()
	if _, err := tu.Start(ctx, coreapi.StartTurnRequest{}); err == nil {
		t.Fatal("turn start")
	}
	if err := tu.Interrupt(ctx, coreapi.TurnRef{}); err == nil {
		t.Fatal("interrupt")
	}
	if _, err := tu.Resume(ctx, coreapi.TurnRef{}); err == nil {
		t.Fatal("turn resume")
	}

	// MCP
	m := e.MCP()
	if _, err := m.List(ctx); err == nil {
		t.Fatal("mcp list")
	}
	if err := m.Upsert(ctx, coreapi.UpsertMCPRequest{}); err == nil {
		t.Fatal("mcp upsert")
	}
	if err := m.ImportJSON(ctx, coreapi.ImportMCPJSONRequest{}); err == nil {
		t.Fatal("mcp import")
	}
	if err := m.Delete(ctx, coreapi.MCPNameRequest{}); err == nil {
		t.Fatal("mcp delete")
	}
	if err := m.SetEnabled(ctx, coreapi.SetMCPEnabledRequest{}); err == nil {
		t.Fatal("mcp setenabled")
	}

	// LSP
	l := e.LSP()
	if _, err := l.List(ctx); err == nil {
		t.Fatal("lsp list")
	}
	if _, err := l.Detect(ctx, coreapi.LSPLanguageRequest{}); err == nil {
		t.Fatal("lsp detect")
	}
	if _, err := l.Start(ctx, coreapi.LSPLanguageRequest{}); err == nil {
		t.Fatal("lsp start")
	}
	if _, err := l.Install(ctx, coreapi.LSPLanguageRequest{}); err == nil {
		t.Fatal("lsp install")
	}
	if _, err := l.Diagnostics(ctx); err == nil {
		t.Fatal("lsp diag")
	}
	if _, err := l.DiagnosticsSummary(ctx); err == nil {
		t.Fatal("lsp diagsum")
	}

	// Sandbox
	sb := e.Sandbox()
	if _, err := sb.DerivePolicy(ctx, coreapi.DeriveSandboxPolicyRequest{}); err == nil {
		t.Fatal("derive")
	}
	if err := sb.SetPolicy(ctx, coreapi.SessionRef{}, sandbox.Policy{}); err == nil {
		t.Fatal("setpolicy")
	}
}

func TestRemoteServicesMoreNilCaller(t *testing.T) {
	e := &RemoteEngine{}
	ctx := context.Background()

	// Approvals / Inquiries
	if err := e.Approvals().Respond(ctx, coreapi.ApprovalResponse{}); err == nil {
		t.Fatal("approval")
	}
	if err := e.Inquiries().Respond(ctx, coreapi.InquiryResponse{}); err == nil {
		t.Fatal("inquiry")
	}

	// Agents
	a := e.Agents()
	if _, err := a.List(ctx, coreapi.ListAgentsRequest{}); err == nil {
		t.Fatal("agent list")
	}
	if err := a.Close(ctx, coreapi.AgentRef{}); err == nil {
		t.Fatal("agent close")
	}
}
