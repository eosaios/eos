package cli

// internal/cli/bridge_test.go 验证 bridge manifest 的纯函数装配逻辑。
// （runBridgeManifest 需启动 sidecar，不在单元测试范围。）

import (
	"encoding/json"
	"strings"
	"testing"

	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
)

func TestBuildManifestUsesCoreInitResult(t *testing.T) {
	init := coreapijsonrpc.InitializeResult{
		ServerName:      "eos-core",
		ProtocolVersion: "2026.1",
		Methods:         []string{"initialize", "session/create", "tool/execute"},
		Capabilities:    map[string]any{"streaming": true},
	}
	m := buildManifest(init, bridgeManifestOptions{
		Workspace:    "/abs/ws",
		AccessMode:   "workspace-write",
		ApprovalMode: "on-request",
		Transport:    "stdio",
	})
	if m.ServerName != "eos-core" {
		t.Fatalf("server_name=%q, want eos-core", m.ServerName)
	}
	if m.ProtocolVersion != "2026.1" {
		t.Fatalf("protocol_version=%q, want 2026.1", m.ProtocolVersion)
	}
	if len(m.Methods) != 3 || m.Methods[1] != "session/create" {
		t.Fatalf("methods=%v, want init methods", m.Methods)
	}
	if m.Capabilities["streaming"] != true {
		t.Fatalf("capabilities not passed through: %v", m.Capabilities)
	}
}

func TestBuildManifestFallsBackToAllCoreMethods(t *testing.T) {
	m := buildManifest(coreapijsonrpc.InitializeResult{}, bridgeManifestOptions{})
	if len(m.Methods) == 0 {
		t.Fatal("empty init should fall back to AllCoreMethods, got empty methods")
	}
	found := map[string]bool{}
	for _, meth := range m.Methods {
		found[meth] = true
	}
	if !found["initialize"] || !found["tool/execute"] {
		t.Fatalf("fallback methods missing core ones: %v", m.Methods)
	}
}

func TestBuildManifestCommandAndDefaults(t *testing.T) {
	m := buildManifest(coreapijsonrpc.InitializeResult{}, bridgeManifestOptions{
		Workspace:    "/abs/ws",
		AccessMode:   "workspace-write",
		ApprovalMode: "on-request",
		Transport:    "stdio",
	})
	if !strings.Contains(m.Command, "eos serve --transport stdio") {
		t.Fatalf("command missing eos serve: %q", m.Command)
	}
	if !strings.Contains(m.Command, "--workspace") || !strings.Contains(m.Command, "/abs/ws") {
		t.Fatalf("command missing workspace: %q", m.Command)
	}
	if m.Transport != "stdio" {
		t.Fatalf("transport=%q, want stdio", m.Transport)
	}
	if m.Defaults["workspace"] != "/abs/ws" {
		t.Fatalf("defaults.workspace=%q", m.Defaults["workspace"])
	}
	if m.Defaults["access_mode"] != "workspace-write" {
		t.Fatalf("defaults.access_mode=%q", m.Defaults["access_mode"])
	}
	if m.Defaults["approval_mode"] != "on-request" {
		t.Fatalf("defaults.approval_mode=%q", m.Defaults["approval_mode"])
	}
}

func TestBuildManifestDefaultsTransportWhenEmpty(t *testing.T) {
	m := buildManifest(coreapijsonrpc.InitializeResult{}, bridgeManifestOptions{Transport: ""})
	if m.Transport != "stdio" {
		t.Fatalf("empty transport should default to stdio, got %q", m.Transport)
	}
	if !strings.Contains(m.Command, "--transport stdio") {
		t.Fatalf("command should use default stdio transport: %q", m.Command)
	}
}

func TestBuildManifestJSONRoundTrip(t *testing.T) {
	m := buildManifest(coreapijsonrpc.InitializeResult{
		ServerName:      "eos-core",
		ProtocolVersion: "1",
		Methods:         []string{"initialize"},
	}, bridgeManifestOptions{Workspace: "/w", Transport: "stdio"})
	bs, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(bs, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"command", "transport", "protocol_version", "server_name", "methods", "defaults"} {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("manifest JSON missing key %q: %s", key, bs)
		}
	}
}
