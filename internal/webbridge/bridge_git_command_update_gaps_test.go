package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// webbridge 批三批测：git 操作台 / 远程工作区 / 系统对话框 web 模式 /
// bash 命令面板 / 命令面板 / 更新检查与下载状态机。复用 chatSessionsGatewayStub。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/webbridge/adapter"
	"github.com/eosaios/eos/pkg/coreapi"
)

// ---- stub 扩展：git / remote workspace / bash / tasks ----

func (g *chatSessionsGatewayStub) CoreGitReposRPC(_ context.Context, root string) (coreapi.GitReposResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gitErr != nil {
		return coreapi.GitReposResult{}, g.gitErr
	}
	g.gitRepoRoots = append(g.gitRepoRoots, root)
	return g.gitRepos, nil
}

func (g *chatSessionsGatewayStub) CoreGitStageRPC(_ context.Context, root string, paths []string, all, unstage bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gitErr != nil {
		return g.gitErr
	}
	g.stageCalls = append(g.stageCalls, [4]interface{}{root, paths, all, unstage})
	return nil
}

func (g *chatSessionsGatewayStub) CoreGitCommitRPC(_ context.Context, root, message string) (coreapi.GitCommitResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gitErr != nil {
		return coreapi.GitCommitResult{}, g.gitErr
	}
	g.commitMessages = append(g.commitMessages, [2]string{root, message})
	return coreapi.GitCommitResult{Hash: "abc1234", Branch: "main"}, nil
}

func (g *chatSessionsGatewayStub) CoreGitPushRPC(_ context.Context, root string) (coreapi.GitPushResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gitErr != nil {
		return coreapi.GitPushResult{}, g.gitErr
	}
	g.pushRoots = append(g.pushRoots, root)
	return g.gitPushResult, nil
}

func (g *chatSessionsGatewayStub) CoreGitMergeAbortRPC(_ context.Context, root string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gitErr != nil {
		return g.gitErr
	}
	g.abortRoots = append(g.abortRoots, root)
	return nil
}

func (g *chatSessionsGatewayStub) CoreGitSuggestMessageRPC(_ context.Context, root string) (coreapi.GitSuggestMessageResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gitErr != nil {
		return coreapi.GitSuggestMessageResult{}, g.gitErr
	}
	g.suggestRoots = append(g.suggestRoots, root)
	return coreapi.GitSuggestMessageResult{Message: "feat: 建议信息"}, nil
}

func (g *chatSessionsGatewayStub) CoreOpenRemoteWorkspaceRPC(_ context.Context, idOrPath string) (adapter.RemoteWorkspace, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.remoteErr != nil {
		return adapter.RemoteWorkspace{}, g.remoteErr
	}
	g.openRemoteCalls = append(g.openRemoteCalls, idOrPath)
	return g.remoteWorkspace, nil
}

func (g *chatSessionsGatewayStub) OpenRemoteWorkspace(idOrPath string) (adapter.RemoteWorkspace, error) {
	return g.CoreOpenRemoteWorkspaceRPC(context.Background(), idOrPath)
}

func (g *chatSessionsGatewayStub) CoreForgetRemoteWorkspaceRPC(_ context.Context, idOrPath string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.remoteErr != nil {
		return g.remoteErr
	}
	g.forgetRemoteCalls = append(g.forgetRemoteCalls, idOrPath)
	return nil
}

func (g *chatSessionsGatewayStub) ForgetRemoteWorkspace(idOrPath string) error {
	return g.CoreForgetRemoteWorkspaceRPC(context.Background(), idOrPath)
}

func (g *chatSessionsGatewayStub) CoreClearRemoteWorkspaceCacheRPC(_ context.Context, idOrPath string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.remoteErr != nil {
		return g.remoteErr
	}
	g.clearRemoteCalls = append(g.clearRemoteCalls, idOrPath)
	return nil
}

func (g *chatSessionsGatewayStub) ClearRemoteWorkspaceCache(idOrPath string) error {
	return g.CoreClearRemoteWorkspaceCacheRPC(context.Background(), idOrPath)
}

func (g *chatSessionsGatewayStub) CoreRunBashStreamRPC(_ context.Context, command string) (<-chan adapter.Event, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.bashErr != nil {
		return nil, g.bashErr
	}
	g.bashCommands = append(g.bashCommands, command)
	if g.bashEvents == nil {
		g.bashEvents = make(chan adapter.Event, 8)
	}
	return g.bashEvents, nil
}

func (g *chatSessionsGatewayStub) RunBash(ctx context.Context, command string) (<-chan adapter.Event, error) {
	return g.CoreRunBashStreamRPC(ctx, command)
}

func (g *chatSessionsGatewayStub) CoreKillTaskRPC(_ context.Context, taskID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.killErr != nil {
		return g.killErr
	}
	g.killCalls = append(g.killCalls, taskID)
	return nil
}

func (g *chatSessionsGatewayStub) CoreCleanupTasksRPC(context.Context) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleanupCalls++
	return 2, nil
}

func (g *chatSessionsGatewayStub) CoreRespondApprovalWithReasonRPC(_ context.Context, approvalID string, decision coreapi.ApprovalDecision, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.respondErr != nil {
		return g.respondErr
	}
	g.respondWithReason = append(g.respondWithReason, [3]interface{}{approvalID, decision, reason})
	return nil
}

// ---- git 操作台 ----

func TestGitOpsForwardingArms(t *testing.T) {
	t.Run("repos 投影", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{
			gitRepos: coreapi.GitReposResult{Repos: []coreapi.GitRepoSummary{{
				Root: " /ws/repo ", Name: " repo ", Primary: true,
				Summary: coreapi.GitSummaryResult{
					Branch: " main ", Upstream: " origin/main ", Ahead: 2, Behind: 1,
					Changes: []coreapi.GitChange{{Path: " a.go ", State: " modified ", Staged: true}},
				},
			}}},
		}
		s := &BridgeService{runtimeGateway: gateway}
		repos := s.GetWorkspaceGitRepos("/ws")
		if !repos.Ok || len(repos.Repos) != 1 {
			t.Fatalf("repos = %+v", repos)
		}
		card := repos.Repos[0]
		if card.Root != "/ws/repo" || card.Name != "repo" || !card.Primary {
			t.Fatalf("card = %+v", card)
		}
		if card.Summary.Branch != "main" || card.Summary.Ahead != 2 || card.Summary.Behind != 1 {
			t.Fatalf("summary = %+v", card.Summary)
		}
		if len(card.Summary.Changes) != 1 || card.Summary.Changes[0].Path != "a.go" || !card.Summary.Changes[0].Staged {
			t.Fatalf("changes = %+v", card.Summary.Changes)
		}
	})

	t.Run("repos nil/无 gateway/错误", func(t *testing.T) {
		var nilBridge *BridgeService
		if got := nilBridge.GetWorkspaceGitRepos("/ws"); got.Ok {
			t.Fatalf("nil bridge repos = %+v", got)
		}
		if got := (&BridgeService{}).GetWorkspaceGitRepos("/ws"); got.Ok || got.Error != "" {
			t.Fatalf("no gateway repos = %+v", got)
		}
		gateway := &chatSessionsGatewayStub{gitErr: errors.New("git missing")}
		if got := (&BridgeService{runtimeGateway: gateway}).GetWorkspaceGitRepos("/ws"); got.Ok || !strings.Contains(got.Error, "git missing") {
			t.Fatalf("error repos = %+v", got)
		}
	})

	t.Run("stage/commit/push/abort/suggest", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s := &BridgeService{runtimeGateway: gateway}
		workspace := t.TempDir()

		if result := s.GitStage(workspace, []string{"a.go"}, false, false); !result.Ok {
			t.Fatalf("GitStage = %+v", result)
		}
		if result := s.GitStage(workspace, nil, true, true); !result.Ok {
			t.Fatalf("GitStage(unstage all) = %+v", result)
		}
		gateway.mu.Lock()
		stages := gateway.stageCalls
		gateway.mu.Unlock()
		if len(stages) != 2 || stages[0][1].([]string)[0] != "a.go" || stages[1][2] != true || stages[1][3] != true {
			t.Fatalf("stageCalls = %+v", stages)
		}

		commit := s.GitCommit(workspace, "feat: x")
		if !commit.Ok || commit.Hash != "abc1234" || commit.Branch != "main" {
			t.Fatalf("GitCommit = %+v", commit)
		}

		gateway.gitPushResult = coreapi.GitPushResult{Status: "pushed", Branch: "main"}
		push := s.GitPush(workspace)
		if !push.Ok || push.Status != "pushed" {
			t.Fatalf("GitPush = %+v", push)
		}
		gateway.gitPushResult = coreapi.GitPushResult{Status: "conflict", Conflicts: []string{"a.go"}}
		push = s.GitPush(workspace)
		if !push.Ok || push.Status != "conflict" || len(push.Conflicts) != 1 || push.Conflicts[0] != "a.go" {
			t.Fatalf("GitPush(conflict) = %+v", push)
		}

		if result := s.GitAbortMerge(workspace); !result.Ok {
			t.Fatalf("GitAbortMerge = %+v", result)
		}
		suggest := s.SuggestGitCommitMessage(workspace)
		if !suggest.Ok || suggest.Message != "feat: 建议信息" {
			t.Fatalf("SuggestGitCommitMessage = %+v", suggest)
		}

		gateway.gitErr = errors.New("git down")
		if result := s.GitStage(workspace, nil, true, false); result.Ok || !strings.Contains(result.Error, "git down") {
			t.Fatalf("GitStage error arm = %+v", result)
		}
		if result := s.GitCommit(workspace, "x"); result.Ok {
			t.Fatalf("GitCommit error arm = %+v", result)
		}
		if result := s.GitPush(workspace); result.Ok {
			t.Fatalf("GitPush error arm = %+v", result)
		}
		if result := s.GitAbortMerge(workspace); result.Ok {
			t.Fatalf("GitAbortMerge error arm = %+v", result)
		}
		if result := s.SuggestGitCommitMessage(workspace); result.Ok {
			t.Fatalf("Suggest error arm = %+v", result)
		}
	})
}

// ---- 远程工作区 ----

func TestRemoteWorkspaceServiceArms(t *testing.T) {
	t.Run("nil bridge 全方法", func(t *testing.T) {
		ws := NewWorkspaceService(nil)
		if got := ws.ListRemoteWorkspaces(); got == nil || len(got) != 0 {
			t.Fatalf("ListRemoteWorkspaces(nil) = %+v", got)
		}
		if _, err := ws.OpenRemoteWorkspace("x"); err == nil {
			t.Fatal("OpenRemoteWorkspace(nil) error = nil")
		}
		if _, err := ws.ForgetRemoteWorkspace("x"); err == nil {
			t.Fatal("ForgetRemoteWorkspace(nil) error = nil")
		}
		if _, err := ws.ClearRemoteWorkspaceCache("x"); err == nil {
			t.Fatal("ClearRemoteWorkspaceCache(nil) error = nil")
		}
		if _, err := ws.StartRemoteRepoFlow(RemoteRepoFlowRequest{}); err == nil {
			t.Fatal("StartRemoteRepoFlow(nil) error = nil")
		}
	})

	t.Run("open 失败/成功切换", func(t *testing.T) {
		workspace := t.TempDir()
		gateway := &chatSessionsGatewayStub{
			defaultWorkspace: workspace,
			remoteWorkspace:  adapter.RemoteWorkspace{LocalPath: workspace},
		}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		ws := NewWorkspaceService(chatSvc.bridge)

		gateway.remoteErr = errors.New("clone failed")
		if _, err := ws.OpenRemoteWorkspace("repo-id"); err == nil || !strings.Contains(err.Error(), "clone failed") {
			t.Fatalf("OpenRemoteWorkspace error = %v", err)
		}

		gateway.remoteErr = nil
		if _, err := ws.OpenRemoteWorkspace(" repo-id "); err != nil {
			t.Fatalf("OpenRemoteWorkspace error = %v", err)
		}
		if gateway.openRemoteCalls[0] != "repo-id" {
			t.Fatalf("openRemoteCalls = %v", gateway.openRemoteCalls)
		}
	})

	t.Run("forget/clear 成功与失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		ws := NewWorkspaceService(chatSvc.bridge)
		if _, err := ws.ForgetRemoteWorkspace("repo"); err != nil {
			t.Fatalf("ForgetRemoteWorkspace error = %v", err)
		}
		if _, err := ws.ClearRemoteWorkspaceCache("repo"); err != nil {
			t.Fatalf("ClearRemoteWorkspaceCache error = %v", err)
		}
		gateway.remoteErr = errors.New("busy")
		if _, err := ws.ForgetRemoteWorkspace("repo"); err == nil {
			t.Fatal("ForgetRemoteWorkspace error = nil on failure")
		}
		if _, err := ws.ClearRemoteWorkspaceCache("repo"); err == nil {
			t.Fatal("ClearRemoteWorkspaceCache error = nil on failure")
		}
	})

	t.Run("StartRemoteRepoFlow 空 URL 拒绝", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		_, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		ws := NewWorkspaceService(chatSvc.bridge)
		if _, err := ws.StartRemoteRepoFlow(RemoteRepoFlowRequest{RepoURL: "   "}); err == nil || !strings.Contains(err.Error(), "URL 不能为空") {
			t.Fatalf("StartRemoteRepoFlow('') error = %v", err)
		}
	})
}

// ---- 系统对话框 web 模式 ----

func TestSystemServiceWebModeArms(t *testing.T) {
	t.Run("web 模式不支持的原生对话框", func(t *testing.T) {
		svc := NewSystemService(nil)
		if _, err := svc.OpenWorkspaceDialog(); err == nil {
			t.Fatal("OpenWorkspaceDialog(web) error = nil")
		}
		if _, err := svc.ChooseLogDirectory(); err == nil {
			t.Fatal("ChooseLogDirectory(web) error = nil")
		}
		if _, err := svc.ExportDiagnosticsBundle(); err == nil {
			t.Fatal("ExportDiagnosticsBundle(web) error = nil")
		}
		result, err := svc.SaveTextFileDialog("a.txt", "内容")
		if err == nil || !result.Cancelled {
			t.Fatalf("SaveTextFileDialog(web) = %+v %v", result, err)
		}
		result, err = svc.SaveZipFileDialog("a.zip", nil)
		if err == nil || !result.Cancelled {
			t.Fatalf("SaveZipFileDialog(web) = %+v %v", result, err)
		}
	})

	t.Run("RevealInFileManager 校验", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewSystemService(s)
		if err := svc.RevealInFileManager(""); err == nil {
			t.Fatal("RevealInFileManager('') error = nil")
		}
		// 相对路径无活动工作区 → 拒绝；有活动工作区 → join 后交底层 RevealPath。
		s.stateMu.Lock()
		s.activeWorkspace = ""
		s.stateMu.Unlock()
		if err := svc.RevealInFileManager("rel/path.go"); err == nil {
			t.Fatal("relative path without workspace error = nil")
		}
		workspace := t.TempDir()
		s.stateMu.Lock()
		s.activeWorkspace = workspace
		s.stateMu.Unlock()
		if err := svc.RevealInFileManager("missing-file.go"); err == nil {
			t.Fatal("nonexistent joined path should fail in RevealPath")
		}
	})

	t.Run("OpenInExternalApp 校验", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewSystemService(s)
		if err := svc.OpenInExternalApp("vscode", "  "); err == nil {
			t.Fatal("OpenInExternalApp('') error = nil")
		}
		s.stateMu.Lock()
		s.activeWorkspace = ""
		s.stateMu.Unlock()
		if err := svc.OpenInExternalApp("vscode", "rel"); err == nil {
			t.Fatal("relative path without workspace error = nil")
		}
		s.stateMu.Lock()
		s.activeWorkspace = t.TempDir()
		s.stateMu.Unlock()
		// 未知 appID 在目录存在时也应失败（目录存在但应用不可用）。
		if err := svc.OpenInExternalApp("no-such-app", "."); err == nil {
			t.Fatal("unknown app id error = nil")
		}
	})

	t.Run("剪贴板与崩溃确认", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewSystemService(s)
		if got := svc.ReadClipboardText(); got != "" {
			t.Fatalf("ReadClipboardText(web) = %q", got)
		}
		state := svc.WriteClipboardText("text")
		if state.AppVersion == "" {
			t.Fatal("WriteClipboardText should return bootstrap state")
		}
		if _, err := svc.AcknowledgeCrashReport(); err != nil {
			t.Fatalf("AcknowledgeCrashReport error = %v", err)
		}
	})

	t.Run("ListExternalApps 返回目录", func(t *testing.T) {
		svc := NewSystemService(nil)
		apps, err := svc.ListExternalApps()
		if err != nil {
			t.Fatalf("ListExternalApps error = %v", err)
		}
		for _, app := range apps {
			if strings.TrimSpace(app.ID) == "" || strings.TrimSpace(app.Name) == "" {
				t.Fatalf("external app missing id/name: %+v", app)
			}
		}
	})
}

// ---- bash 命令面板 ----

func TestRunBashCommandArms(t *testing.T) {
	t.Run("nil bridge / 空命令", func(t *testing.T) {
		if _, err := NewCommandService(nil).RunBashCommand(""); err == nil {
			t.Fatal("RunBashCommand(nil bridge) error = nil")
		}
		gateway := &chatSessionsGatewayStub{}
		s, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		if _, err := NewCommandService(s).RunBashCommand("   "); err == nil || !strings.Contains(err.Error(), "bash command is required") {
			t.Fatalf("RunBashCommand('') error = %v", err)
		}
		_ = chatSvc
	})

	t.Run("起跑态与 RPC 失败收尾", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{bashErr: errors.New("no shell")}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewCommandService(s)
		if _, err := svc.RunBashCommand("ls -la"); err != nil {
			t.Fatalf("RunBashCommand error = %v", err)
		}
		eventually(t, "bash failed status", func() bool {
			s.stateMu.RLock()
			defer s.stateMu.RUnlock()
			return s.bash.Status == "failed" && len(s.bash.Output) >= 3
		})
		s.stateMu.RLock()
		output := append([]string(nil), s.bash.Output...)
		s.stateMu.RUnlock()
		if !strings.Contains(strings.Join(output, "\n"), "no shell") {
			t.Fatalf("output missing error: %v", output)
		}
	})

	t.Run("事件流驱动输出与完成", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		gateway.bashEvents = make(chan adapter.Event, 8)
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewCommandService(s)
		if _, err := svc.RunBashCommand("echo hi"); err != nil {
			t.Fatalf("RunBashCommand error = %v", err)
		}
		gateway.bashEvents <- adapter.Event{Type: "bash.output", Data: map[string]any{"message": "hi"}}
		gateway.bashEvents <- adapter.Event{Type: "turn.completed", Data: map[string]any{"message": "done"}}
		close(gateway.bashEvents)
		eventually(t, "bash completed", func() bool {
			s.stateMu.RLock()
			defer s.stateMu.RUnlock()
			return s.bash.Status == "completed"
		})
		s.stateMu.RLock()
		output := strings.Join(s.bash.Output, "\n")
		s.stateMu.RUnlock()
		if !strings.Contains(output, "echo hi") || !strings.Contains(output, "hi") {
			t.Fatalf("output = %q", output)
		}
	})
}

// ---- 命令面板 ----

func TestCommandServicePaletteAndTasks(t *testing.T) {
	t.Run("palette 目录与幂等分支", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, chatSvc, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewCommandService(s)
		palette := svc.DefaultCommandPalette()
		if len(palette) == 0 {
			t.Fatal("DefaultCommandPalette empty")
		}
		// 未知命令 → 幂等成功返回 bootstrap。
		if _, err := svc.RunCommandPalette("unknown.command"); err != nil {
			t.Fatalf("RunCommandPalette(unknown) error = %v", err)
		}
		if _, err := svc.RunCommandPalette("notifications.clear"); err != nil {
			t.Fatalf("RunCommandPalette(notifications.clear) error = %v", err)
		}
		s.stateMu.Lock()
		s.notifications = []NotificationItem{{ID: "n1"}, {ID: "n2"}}
		s.stateMu.Unlock()
		_ = svc.DismissNotification(" n1 ")
		s.stateMu.RLock()
		stillN1 := false
		for _, item := range s.notifications {
			if item.ID == "n1" {
				stillN1 = true
			}
		}
		s.stateMu.RUnlock()
		if stillN1 {
			t.Fatal("n1 still present after dismiss")
		}
		_ = chatSvc
	})

	t.Run("tasks.cleanup", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewCommandService(s)
		if _, err := svc.RunCommandPalette("tasks.cleanup"); err != nil {
			t.Fatalf("RunCommandPalette(tasks.cleanup) error = %v", err)
		}
		gateway.mu.Lock()
		calls := gateway.cleanupCalls
		gateway.mu.Unlock()
		if calls != 1 {
			t.Fatalf("cleanupCalls = %d, want 1", calls)
		}
	})

	t.Run("KillTask 成功与失败", func(t *testing.T) {
		gateway := &chatSessionsGatewayStub{}
		s, _, _ := newChatSessionsTestBridge(t, gateway)
		svc := NewCommandService(s)
		if _, err := svc.KillTask("task-1"); err != nil {
			t.Fatalf("KillTask error = %v", err)
		}
		gateway.mu.Lock()
		killCalls := append([]string(nil), gateway.killCalls...)
		gateway.mu.Unlock()
		if len(killCalls) != 1 || killCalls[0] != "task-1" {
			t.Fatalf("killCalls = %v", killCalls)
		}
		gateway.killErr = errors.New("task gone")
		if _, err := svc.KillTask("task-1"); err == nil || !strings.Contains(err.Error(), "task gone") {
			t.Fatalf("KillTask error = %v", err)
		}
		if _, err := NewCommandService(nil).KillTask("x"); err == nil {
			t.Fatal("KillTask(nil) error = nil")
		}
	})
}

// ---- 更新检查与下载 ----

func TestUpdateCheckPureHelpers(t *testing.T) {
	if got := releaseLatestPageURL(); !strings.Contains(got, "eosaios/eos-app/releases/latest") {
		t.Fatalf("releaseLatestPageURL = %q", got)
	}
}

func TestFetchAssetDigestFromSumsFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "1111111111111111111111111111111111111111111111111111111111111111  EOS-1.0.0-macos-aarch64.zip\n"+
			"2222222222222222222222222222222222222222222222222222222222222222  other-asset.zip\n")
	}))
	defer server.Close()

	digest, err := fetchAssetDigest(context.Background(), server.URL, "EOS-1.0.0-macos-aarch64.zip", nil)
	if err != nil {
		t.Fatalf("fetchAssetDigest error = %v", err)
	}
	if digest != "1111111111111111111111111111111111111111111111111111111111111111" {
		t.Fatalf("digest = %q", digest)
	}
	if _, err := fetchAssetDigest(context.Background(), server.URL, "missing-asset.zip", nil); err == nil {
		t.Fatal("missing asset digest error = nil")
	}
}

func withUpdateCache(t *testing.T, result *UpdateCheckResult) {
	t.Helper()
	updateCacheMu.Lock()
	oldResult, oldAt := cachedUpdateResult, lastCheckAt
	cachedUpdateResult, lastCheckAt = result, time.Now()
	updateCacheMu.Unlock()
	t.Cleanup(func() {
		updateCacheMu.Lock()
		cachedUpdateResult, lastCheckAt = oldResult, oldAt
		updateCacheMu.Unlock()
	})
}

func TestUpdateDownloadStateMachineArms(t *testing.T) {
	newBridge := func(t *testing.T) (*BridgeService, *chatSessionsGatewayStub) {
		gateway := &chatSessionsGatewayStub{}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		s := &BridgeService{
			runtimeGateway: gateway,
			sessions:       map[string]*sessionState{},
			prompts:        map[string]*promptState{},
			emitEvent:      func(string, any) {},
		}
		return s, gateway
	}

	t.Run("Cancel 幂等", func(t *testing.T) {
		s, _ := newBridge(t)
		state := s.CancelUpdateDownload()
		if state.Stage != "" {
			t.Fatalf("Cancel on idle = %+v", state)
		}
	})

	t.Run("检查失败走 fail 态", func(t *testing.T) {
		s, _ := newBridge(t)
		// configPath 指向不存在文件 → updateProxyRaw 报读配置失败 → 检查失败。
		withUpdateCache(t, nil)
		state := s.StartUpdateDownload()
		if state.Stage != updateStageFailed && state.Error == "" {
			t.Fatalf("state = %+v, want failure with error", state)
		}
	})

	t.Run("已是最新无需下载", func(t *testing.T) {
		s, _ := newBridge(t)
		withUpdateCache(t, &UpdateCheckResult{HasUpdate: false, LatestVersion: BuildVersion})
		state := s.StartUpdateDownload()
		if state.Stage != updateStageFailed || !strings.Contains(state.Error, "已是最新") {
			t.Fatalf("state = %+v", state)
		}
	})

	t.Run("缺平台安装包与缺 digest 拒绝", func(t *testing.T) {
		s, _ := newBridge(t)
		withUpdateCache(t, &UpdateCheckResult{HasUpdate: true, LatestVersion: "9.9.9"})
		if state := s.StartUpdateDownload(); state.Stage != updateStageFailed || !strings.Contains(state.Error, "安装包") {
			t.Fatalf("no-asset state = %+v", state)
		}
		withUpdateCache(t, &UpdateCheckResult{
			HasUpdate: true, LatestVersion: "9.9.9",
			DownloadURL: "https://example.com/x.zip", AssetName: "x.zip",
		})
		if state := s.StartUpdateDownload(); state.Stage != updateStageFailed || !strings.Contains(state.Error, "digest") {
			t.Fatalf("no-digest state = %+v", state)
		}
	})

	t.Run("downloading 早退与 ready 复用", func(t *testing.T) {
		s, _ := newBridge(t)
		s.updateMu.Lock()
		s.updateDownload = UpdateDownloadState{Stage: updateStageDownloading, Version: "1.0.0", Percent: 42}
		s.updateMu.Unlock()
		if state := s.StartUpdateDownload(); state.Percent != 42 {
			t.Fatalf("downloading early return = %+v", state)
		}

		file := filepath.Join(t.TempDir(), "pkg.zip")
		if err := os.WriteFile(file, []byte("pkg"), 0o644); err != nil {
			t.Fatal(err)
		}
		s.updateMu.Lock()
		s.updateDownload = UpdateDownloadState{Stage: updateStageReady, LocalPath: file}
		s.updateMu.Unlock()
		if state := s.StartUpdateDownload(); state.Stage != updateStageReady {
			t.Fatalf("ready early return = %+v", state)
		}
	})

	t.Run("InstallUpdate 未就绪拒绝", func(t *testing.T) {
		s, _ := newBridge(t)
		if err := s.InstallUpdate(); err == nil || !strings.Contains(err.Error(), "尚未就绪") {
			t.Fatalf("InstallUpdate(idle) error = %v", err)
		}
		file := filepath.Join(t.TempDir(), "gone.zip")
		s.updateMu.Lock()
		s.updateDownload = UpdateDownloadState{Stage: updateStageReady, LocalPath: file}
		s.updateMu.Unlock()
		if err := s.InstallUpdate(); err == nil || !strings.Contains(err.Error(), "已不存在") {
			t.Fatalf("InstallUpdate(missing file) error = %v", err)
		}
	})
}
