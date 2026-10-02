package adapter

// 表驱动网关往返：scripted 服务器按 method 回放预设 result JSON，
// 批量点亮 StdioGateway 的薄委托方法（marshal→Call→unmarshal 全链）。

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// newReplayGateway 起一个 net.Pipe 回放服务器：按 method 查表回 result。
func newReplayGateway(t *testing.T, replies map[string]any) *StdioGateway {
	t.Helper()
	client, serverConn := newPipeStdioClient(t)
	go func() {
		stream := protocoljsonrpc.NewStream(serverConn, serverConn)
		_ = serverConn
		for {
			msg, err := stream.ReadMessage()
			if err != nil {
				return
			}
			if msg.Request == nil {
				continue
			}
			result, ok := replies[msg.Request.Method]
			if !ok {
				result = map[string]any{}
			}
			raw, _ := json.Marshal(result)
			if err := stream.WriteMessage(protocoljsonrpc.Response{
				ID:     msg.Request.ID,
				Result: raw,
			}); err != nil {
				return
			}
		}
	}()
	return NewStdioGateway(client)
}

func TestGatewayBrowserRoundtrips(t *testing.T) {
	tabInfo := map[string]any{
		"index": 2, "url": "https://eos.example", "title": "EOS", "active": true,
	}
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodBrowserTabNew:        tabInfo,
		protocoljsonrpc.MethodBrowserTabSwitch:     tabInfo,
		protocoljsonrpc.MethodBrowserTabs:          []any{tabInfo},
		protocoljsonrpc.MethodBrowserProfiles:      []any{},
		protocoljsonrpc.MethodBrowserProfileUpsert: []any{},
	})
	ctx := context.Background()

	steps := []struct {
		name string
		fn   func() error
	}{
		{"takeover", func() error {
			return g.CoreBrowserControlTakeoverRPC(ctx, coreapi.BrowserControlTakeoverRequest{Reason: "test"})
		}},
		{"confirm", func() error { return g.CoreBrowserControlConfirmRPC(ctx) }},
		{"resume", func() error { return g.CoreBrowserControlResumeRPC(ctx) }},
		{"upload provide", func() error {
			return g.CoreBrowserUploadProvideRPC(ctx, "req-1", []string{"/tmp/a.png"})
		}},
		{"focus", func() error {
			return g.CoreBrowserFocusRPC(ctx, coreapi.BrowserFocusRequest{Profile: "default"})
		}},
		{"set default profile", func() error { return g.CoreBrowserSetDefaultProfileRPC(ctx, "default") }},
		{"navigate", func() error { return g.CoreBrowserNavigateRPC(ctx, "https://eos.example") }},
		{"tab close", func() error { return g.CoreBrowserTabCloseRPC(ctx, intPtr(3)) }},
		{"live start", func() error {
			return g.CoreBrowserLiveStartRPC(ctx, coreapi.BrowserLiveStartRequest{})
		}},
		{"live stop", func() error { return g.CoreBrowserLiveStopRPC(ctx) }},
		{"input", func() error {
			return g.CoreBrowserInputRPC(ctx, coreapi.BrowserInputRequest{Kind: "mouse", Action: "move"})
		}},
		{"history", func() error { return g.CoreBrowserHistoryRPC(ctx, "back") }},
		{"pick start", func() error { return g.CoreBrowserPickStartRPC(ctx) }},
		{"pick stop", func() error { return g.CoreBrowserPickStopRPC(ctx) }},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}

	if _, err := g.CoreBrowserCopySelectionRPC(ctx); err != nil {
		t.Fatalf("copy selection: %v", err)
	}
	if _, err := g.CoreBrowserProfilesRPC(ctx); err != nil {
		t.Fatalf("profiles: %v", err)
	}
	if _, err := g.CoreBrowserCredentialsImportRPC(ctx, coreapi.BrowserCredentialsImportRequest{
		Profile: "default", Domains: []string{"eosaios.com"},
	}); err != nil {
		t.Fatalf("credentials import: %v", err)
	}
	if _, err := g.CoreBrowserProfileUpsertRPC(ctx, map[string]any{
		"name": "default", "headless": false,
	}); err != nil {
		t.Fatalf("profile upsert: %v", err)
	}
}

func intPtr(i int) *int { return &i }

func TestGatewayWorkspaceSessionModelRoundtrips(t *testing.T) {
	g := newReplayGateway(t, map[string]any{
		protocoljsonrpc.MethodSessionCreate:           map[string]any{"id": "s-1", "title": "T"},
		protocoljsonrpc.MethodSessionMessagesSave:     map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionMessagesLoad:     []any{},
		protocoljsonrpc.MethodSessionRename:           map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionCurrent:          map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionResume:           map[string]any{"id": "s-1"},
		protocoljsonrpc.MethodSessionList:             []any{},
		protocoljsonrpc.MethodModelList:               []any{},
		protocoljsonrpc.MethodModelCatalog:            map[string]any{"providers": []any{}, "presets": []any{}},
		protocoljsonrpc.MethodModelContext:            map[string]any{},
		protocoljsonrpc.MethodStateSnapshot:           map[string]any{},
		protocoljsonrpc.MethodRuntimeModesGet:         map[string]any{},
		protocoljsonrpc.MethodWorkspaceList:           []any{},
		protocoljsonrpc.MethodMCPList:                 []any{},
		protocoljsonrpc.MethodWorkspaceWorktreeList:   []any{},
		protocoljsonrpc.MethodRemoteWorkspaceList:     []any{},
		protocoljsonrpc.MethodWorkspaceWorktreeCreate: map[string]any{"name": "wt"},
		protocoljsonrpc.MethodUsageSummary:            map[string]any{"rounds": 0},
		protocoljsonrpc.MethodModelVerify:             map[string]any{"ok": true},
	})
	ctx := context.Background()

	simple := []struct {
		name string
		fn   func() error
	}{
		{"add workspace", func() error { return g.CoreAddWorkspaceRPC(ctx, "/tmp/ws") }},
		{"remove workspace", func() error { return g.CoreRemoveWorkspaceRPC(ctx, "/tmp/ws") }},
		{"use workspace", func() error { return g.CoreUseWorkspaceRPC(ctx, "/tmp/ws") }},
		{"trust workspace", func() error { return g.CoreTrustWorkspaceRPC(ctx, "/tmp/ws") }},
		{"remember workspace", func() error { return g.CoreRememberWorkspaceRPC(ctx, "/tmp/ws", false) }},
		{"set execution mode", func() error { return g.CoreSetExecutionModeRPC(ctx, "auto") }},
		{"set reasoning level", func() error { return g.CoreSetReasoningLevelRPC(ctx, "high") }},
		{"set session model", func() error { return g.CoreSetSessionModelRPC(ctx, "s-1", "glm-5.3") }},
		{"clear session model", func() error { return g.CoreClearSessionModelRPC(ctx, "s-1") }},
		{"set workspace model", func() error { return g.CoreSetWorkspaceModelRPC(ctx, "/tmp/ws", "glm-5.3") }},
		{"clear workspace model", func() error { return g.CoreClearWorkspaceModelRPC(ctx, "/tmp/ws") }},
		{"upsert model", func() error { return g.CoreUpsertModelRPC(ctx, "m1", "https://api", "sk", "glm") }},
		{"upsert mcp", func() error { return g.CoreUpsertMCPRPC(ctx, "srv", "stdio", "/bin/x", true) }},
		{"set session sandbox", func() error { return g.CoreSetSessionSandboxModeRPC(ctx, "s-1", "workspace-write") }},
	}
	for _, s := range simple {
		if err := s.fn(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}

	if _, err := g.CoreCreateSessionRPC(ctx, "/tmp/ws", "T", "chat", nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := g.CoreSaveSessionMessagesRPC(ctx, "/tmp/ws", "s-1", nil); err != nil {
		t.Fatalf("save messages: %v", err)
	}
	if _, err := g.CoreRenameSessionRPC(ctx, "/tmp/ws", "s-1", "新标题"); err != nil {
		t.Fatalf("rename session: %v", err)
	}
	if _, err := g.CoreListModelsRPC(ctx); err != nil {
		t.Fatalf("list models: %v", err)
	}
	if _, err := g.CoreModelCatalogRPC(ctx); err != nil {
		t.Fatalf("model catalog: %v", err)
	}
	if _, err := g.CoreCreateWorktreeRPC(ctx, "wt"); err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if _, err := g.CoreUsageSummaryRPC(ctx); err != nil {
		t.Fatalf("usage summary: %v", err)
	}
	if _, err := g.CoreListMCPRPC(ctx); err != nil {
		t.Fatalf("list mcp: %v", err)
	}
}
