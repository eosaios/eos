package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

func TestRemoteEngineWithRealSidecar(t *testing.T) {
	binaryPath := os.Getenv(EnvCorePath)
	if binaryPath == "" {
		t.Skipf("%s is not set", EnvCorePath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	engine, err := StartRemoteEngine(ctx, ProcessOptions{BinaryPath: binaryPath})
	if err != nil {
		t.Fatalf("StartRemoteEngine() error = %v", err)
	}
	defer engine.Close()

	created, err := engine.Sessions().Create(ctx, coreapi.CreateSessionRequest{
		WorkspaceRoot: "C:/work",
		Title:         "sidecar integration",
	})
	if err != nil {
		t.Fatalf("Sessions().Create() error = %v", err)
	}
	if created.ID == "" {
		t.Fatalf("Sessions().Create().ID is empty")
	}

	sessions, err := engine.Sessions().List(ctx, coreapi.ListSessionsRequest{WorkspaceRoot: "C:/work"})
	if err != nil {
		t.Fatalf("Sessions().List() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != created.ID {
		t.Fatalf("Sessions().List()=%+v, want created session", sessions)
	}

	snapshot, err := engine.State().Snapshot(ctx, coreapi.StateSnapshotRequest{})
	if err != nil {
		t.Fatalf("State().Snapshot() error = %v", err)
	}
	if snapshot.ForegroundWorkspace != "C:/work" {
		t.Fatalf("ForegroundWorkspace=%q, want C:/work", snapshot.ForegroundWorkspace)
	}

	turn, err := engine.Turns().Start(ctx, coreapi.StartTurnRequest{SessionID: created.ID, Input: "hello"})
	if err != nil {
		t.Fatalf("Turns().Start() error = %v", err)
	}
	if turn.SessionID != created.ID || turn.Status == "" {
		t.Fatalf("Turns().Start()=%+v, want session %q with status", turn, created.ID)
	}

	status := engine.Sandbox().BackendStatus(ctx)
	if status.GOOS == "" || status.Backend == "" {
		t.Fatalf("Sandbox().BackendStatus()=%+v, want populated status", status)
	}
	if !status.Degraded || status.Enforced {
		t.Fatalf("Sandbox().BackendStatus()=%+v, want visible degraded status", status)
	}

	// The frozen Rust core supports tool/execute. An unregistered tool is a
	// structured denial, rather than a missing-method transport error.
	tool, err := engine.Tools().Execute(ctx, coreapi.ToolRequest{Name: "shell"})
	if err != nil {
		t.Fatalf("Tools().Execute() error = %v", err)
	}
	wantTool := coreapi.ToolResult{
		Name: "shell", Status: "denied", Display: "tool 'shell' is not registered",
		Output: json.RawMessage("null"), Error: "tool 'shell' is not registered",
	}
	if !reflect.DeepEqual(tool, wantTool) {
		t.Fatalf("Tools().Execute() = %#v, want %#v", tool, wantTool)
	}

	// Use a dedicated context for Shutdown so it is not racing
	// with the test-level deadline that may cancel the shared ctx.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := engine.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestRemoteEngineWithVendoredSidecar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	engine, err := StartRemoteEngine(ctx, ProcessOptions{VerifyChecksum: true})
	if err != nil {
		if errors.Is(err, ErrCoreBinaryNotFound) {
			t.Skipf("no vendored sidecar binary for this target: %v", err)
		}
		t.Fatalf("StartRemoteEngine() error = %v", err)
	}
	defer engine.Close()

	init, err := engine.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if init.ServerName == "" {
		t.Fatalf("Initialize().ServerName is empty")
	}

	status := engine.Sandbox().BackendStatus(ctx)
	if status.GOOS == "" || status.Backend == "" {
		t.Fatalf("Sandbox().BackendStatus()=%+v, want populated status", status)
	}
	// Use a dedicated context for Shutdown so it is not racing
	// with the test-level deadline that may cancel the shared ctx.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := engine.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestRemoteEngineAgentRunPersistsSessionTurnWithVendoredSidecar(t *testing.T) {
	// 这个测试驱动 agent 真实跑一轮（Spawn → SendInput → Run），需要内核连上
	// 真实 model provider 才能产出 completed 状态的 turn。CI 上没有 model 凭据，
	// agent run 会因无可用 provider 直接 failed。默认 skip，本地或带了 model
	// 凭据的环境显式设 EOS_TEST_LIVE_MODEL=1 才跑，与 TestRemoteEngineWithRealSidecar
	// 用 EOS_CORE_PATH gate 的模式一致。
	if os.Getenv("EOS_TEST_LIVE_MODEL") != "1" {
		t.Skipf("skipping live-model integration test; set EOS_TEST_LIVE_MODEL=1 with model credentials to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	storeDir := t.TempDir()
	engine, err := StartRemoteEngine(ctx, ProcessOptions{
		VerifyChecksum: true,
		Env: map[string]string{
			"EOS_CORE_STORE_DIR": storeDir,
		},
	})
	if err != nil {
		if errors.Is(err, ErrCoreBinaryNotFound) {
			t.Skipf("no vendored sidecar binary for this target: %v", err)
		}
		t.Fatalf("StartRemoteEngine() error = %v", err)
	}

	session, err := engine.Sessions().Create(ctx, coreapi.CreateSessionRequest{
		WorkspaceRoot: "C:/work-agent-run",
		Title:         "agent run persistence",
	})
	if err != nil {
		_ = engine.Close()
		t.Fatalf("Sessions().Create() error = %v", err)
	}
	agent, err := engine.Agents().Spawn(ctx, coreapi.SpawnAgentRequest{
		RoleID: "planner",
		Task:   "summarize the mailbox input",
	})
	if err != nil {
		_ = engine.Close()
		t.Fatalf("Agents().Spawn() error = %v", err)
	}
	if err := engine.Agents().SendInput(ctx, coreapi.AgentInput{
		AgentID: agent.ID,
		Input:   "remember this public integration message",
	}); err != nil {
		_ = engine.Close()
		t.Fatalf("Agents().SendInput() error = %v", err)
	}
	run, err := engine.Agents().Run(ctx, coreapi.RunAgentRequest{
		AgentID:   agent.ID,
		SessionID: session.ID,
	})
	if err != nil {
		_ = engine.Close()
		t.Fatalf("Agents().Run() error = %v", err)
	}
	if run.Agent.Status != "completed" {
		_ = engine.Close()
		t.Fatalf("Agents().Run().Agent.Status=%q, want completed", run.Agent.Status)
	}
	if len(run.Messages) != 1 || run.Messages[0].Body == "" {
		_ = engine.Close()
		t.Fatalf("Agents().Run().Messages=%+v, want consumed mailbox message", run.Messages)
	}
	messages, err := engine.Sessions().LoadMessages(ctx, coreapi.LoadSessionMessagesRequest{SessionID: session.ID})
	if err != nil {
		_ = engine.Close()
		t.Fatalf("Sessions().LoadMessages() error = %v", err)
	}
	if !hasAssistantMessage(messages) {
		_ = engine.Close()
		t.Fatalf("Sessions().LoadMessages()=%+v, want persisted assistant message", messages)
	}

	turnPath := filepath.Join(storeDir, "sessions", session.ID, "turns", "turn_agent_"+agent.ID+".json")
	storedBytes, err := os.ReadFile(turnPath)
	if err != nil {
		_ = engine.Close()
		t.Fatalf("reading persisted agent turn %s: %v", turnPath, err)
	}
	var storedTurn struct {
		SessionID string       `json:"session_id"`
		Turn      coreapi.Turn `json:"turn"`
	}
	if err := json.Unmarshal(storedBytes, &storedTurn); err != nil {
		_ = engine.Close()
		t.Fatalf("decode persisted agent turn: %v", err)
	}
	if storedTurn.SessionID != session.ID || storedTurn.Turn.SessionID != session.ID || storedTurn.Turn.Status != "completed" {
		_ = engine.Close()
		t.Fatalf("persisted agent turn=%+v, want completed turn for session %q", storedTurn, session.ID)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := engine.Shutdown(shutdownCtx); err != nil {
		_ = engine.Close()
		t.Fatalf("Shutdown() error = %v", err)
	}
	_ = engine.Close()

	restarted, err := StartRemoteEngine(ctx, ProcessOptions{
		VerifyChecksum: true,
		Env: map[string]string{
			"EOS_CORE_STORE_DIR": storeDir,
		},
	})
	if err != nil {
		t.Fatalf("restart StartRemoteEngine() error = %v", err)
	}
	defer restarted.Close()

	if _, err := restarted.Sessions().Resume(ctx, coreapi.ResumeSessionRequest{SessionID: session.ID}); err != nil {
		t.Fatalf("restarted Sessions().Resume() error = %v", err)
	}
	restartedMessages, err := restarted.Sessions().LoadMessages(ctx, coreapi.LoadSessionMessagesRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("restarted Sessions().LoadMessages() error = %v", err)
	}
	if !hasAssistantMessage(restartedMessages) {
		t.Fatalf("restarted messages=%+v, want persisted assistant message", restartedMessages)
	}
	restartShutdownCtx, restartShutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer restartShutdownCancel()
	if err := restarted.Shutdown(restartShutdownCtx); err != nil {
		t.Fatalf("restarted Shutdown() error = %v", err)
	}
}

func hasAssistantMessage(messages []coreapi.SessionMessage) bool {
	for _, message := range messages {
		if message.Role == "assistant" && message.Content != "" {
			return true
		}
	}
	return false
}

func TestRemoteEngineModelCatalogWithVendoredSidecar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	engine, err := StartRemoteEngine(ctx, ProcessOptions{VerifyChecksum: true})
	if err != nil {
		if errors.Is(err, ErrCoreBinaryNotFound) {
			t.Skipf("no vendored sidecar binary for this target: %v", err)
		}
		t.Fatalf("StartRemoteEngine() error = %v", err)
	}
	defer engine.Close()

	catalog, err := engine.Models().Catalog(ctx)
	if err != nil {
		t.Fatalf("Models().Catalog() error = %v", err)
	}
	if len(catalog.Providers) == 0 {
		t.Fatalf("Models().Catalog().Providers is empty")
	}
	if len(catalog.Presets) == 0 {
		t.Fatalf("Models().Catalog().Presets is empty")
	}

	var hasQwen, hasGPTCodex bool
	for _, p := range catalog.Presets {
		if p.ID == "qwen3.6-plus" {
			hasQwen = true
		}
		if p.ID == "gpt-5.3-codex" {
			hasGPTCodex = true
		}
	}
	if !hasQwen {
		t.Errorf("catalog presets missing qwen3.6-plus")
	}
	if !hasGPTCodex {
		t.Errorf("catalog presets missing gpt-5.3-codex")
	}
	if !catalog.AllowCustomProvider {
		t.Errorf("catalog AllowCustomProvider = false, want true")
	}
	if !catalog.AllowCustomModel {
		t.Errorf("catalog AllowCustomModel = false, want true")
	}

	var hasDashscope, hasZhipu, hasMinimax, hasMimo, hasOpenai, hasAnthropic bool
	for _, pr := range catalog.Providers {
		switch pr.ID {
		case "dashscope":
			hasDashscope = true
		case "zhipu":
			hasZhipu = true
		case "minimax":
			hasMinimax = true
		case "mimo":
			hasMimo = true
		case "openai":
			hasOpenai = true
		case "anthropic":
			hasAnthropic = true
		}
	}
	for id, found := range map[string]*bool{
		"dashscope": &hasDashscope,
		"zhipu":     &hasZhipu,
		"minimax":   &hasMinimax,
		"mimo":      &hasMimo,
		"openai":    &hasOpenai,
		"anthropic": &hasAnthropic,
	} {
		if !*found {
			t.Errorf("catalog providers missing %s", id)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := engine.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}
