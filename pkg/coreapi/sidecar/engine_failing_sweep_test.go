package sidecar

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// RemoteEngine 错误臂扫荡：全失败 caller + 两级反射（引擎服务 getter →
// 服务方法），批量点亮各 remote*Service 的 call 错误返回臂。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi"

	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
	"github.com/eosaios/eos/pkg/sandbox"
)

type failingSidecarCaller struct{}

func (failingSidecarCaller) Call(context.Context, string, any, any) error {
	return errors.New("sidecar down")
}

var sweepCtxT = reflect.TypeOf((*context.Context)(nil)).Elem()
var sweepErrT = reflect.TypeOf((*error)(nil)).Elem()

// sweepServiceMethods 反射调用服务接口的全部方法：ctx 位传 Background，
// 其余参数零值；error 返回断言非 nil，无 error 方法只要求不 panic。
func sweepServiceMethods(t *testing.T, svc reflect.Value, label string) {
	t.Helper()
	typ := svc.Type()
	called, errs := 0, 0
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		// 绑定后的方法类型不含 receiver（接口 Method 集本就无 receiver）。
		mv := svc.Method(i)
		mt := mv.Type()
		if mt.IsVariadic() {
			continue
		}
		args := make([]reflect.Value, 0, mt.NumIn())
		for j := 0; j < mt.NumIn(); j++ {
			pt := mt.In(j)
			if pt == sweepCtxT {
				args = append(args, reflect.ValueOf(context.Background()))
				continue
			}
			args = append(args, reflect.Zero(pt))
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s.%s panic: %v", label, name, r)
				}
			}()
			results := mv.Call(args)
			called++
			if mt.NumOut() > 0 && mt.Out(mt.NumOut()-1) == sweepErrT {
				if err, _ := results[mt.NumOut()-1].Interface().(error); err == nil {
					t.Errorf("%s.%s 注入失败后应返回 error", label, name)
				}
				errs++
			}
		}()
	}
	t.Logf("%s: called=%d errorArms=%d", label, called, errs)
}

func TestRemoteEngineFailingCallerSweep(t *testing.T) {
	engine := NewRemoteEngine(failingSidecarCaller{})
	ge := reflect.ValueOf(engine)
	gt := ge.Type()
	// 跳过非服务/副作用入口；只扫 0 参返回接口的服务 getter。
	skip := map[string]bool{
		"Caller": true, "Wait": true, "ProcessClient": true,
		"Initialize": true, "Shutdown": true, "Close": true,
	}
	services := 0
	for i := 0; i < gt.NumMethod(); i++ {
		m := gt.Method(i)
		if skip[m.Name] || m.Type.NumIn() != 1 || m.Type.NumOut() != 1 {
			continue
		}
		if m.Type.Out(0).Kind() != reflect.Interface {
			continue
		}
		svc := ge.Method(i).Call(nil)[0]
		if (svc.Kind() == reflect.Ptr || svc.Kind() == reflect.Interface) && svc.IsNil() {
			continue
		}
		sweepServiceMethods(t, svc, m.Name)
		services++
	}
	if services < 25 {
		t.Fatalf("扫到的服务数异常偏少: %d", services)
	}
}

func TestRemoteEngineEdgeArms(t *testing.T) {
	ctx := context.Background()
	engine := NewRemoteEngine(failingSidecarCaller{})

	// Initialize 错误臂（caller 失败透传）。
	if _, err := engine.Initialize(ctx); err == nil {
		t.Fatal("Initialize 失败 caller 应报错")
	}
	// Shutdown 透传 shutdown RPC 错误。
	if err := engine.Shutdown(ctx); err == nil {
		t.Fatal("Shutdown 失败 caller 应报错")
	}
	// Wait 无 client：立即返回 ErrCallerUnavailable。
	ch := engine.Wait()
	select {
	case err, ok := <-ch:
		if !ok || !errors.Is(err, ErrCallerUnavailable) {
			t.Fatalf("Wait = %v, %v", err, ok)
		}
	default:
		t.Fatal("无 client Wait 应立即就绪")
	}
	// Caller() nil receiver 臂。
	var nilEngine *RemoteEngine
	if nilEngine.Caller() != nil {
		t.Fatal("nil receiver Caller 应为 nil")
	}

	// handleNotification 三臂：非事件方法 / 空 params / 坏 JSON。
	if err := engine.handleNotification(ctx, protocoljsonrpc.Notification{Method: "other"}); err != nil {
		t.Fatalf("非事件方法 = %v", err)
	}
	if err := engine.handleNotification(ctx, protocoljsonrpc.Notification{Method: protocoljsonrpc.NotificationEvent}); err != nil {
		t.Fatalf("空 params = %v", err)
	}
	if err := engine.handleNotification(ctx, protocoljsonrpc.Notification{
		Method: protocoljsonrpc.NotificationEvent,
		Params: []byte("{not json"),
	}); err != nil {
		t.Fatalf("坏 JSON = %v", err)
	}
}

// TestRemoteEngineSuccessArmsWithResults 剩余成功臂：带 results 的 caller
// 直调 MCP/LSP/SetMeta/Turns.Resume/ToolCatalog/Sandbox.Policy 等服务方法。
func TestRemoteEngineSuccessArmsWithResults(t *testing.T) {
	caller := &fakeEngineCaller{results: map[string]any{
		protocoljsonrpc.MethodSessionSetMeta:        coreapi.Session{ID: "s-meta"},
		protocoljsonrpc.MethodTurnResume:            coreapi.Turn{ID: "t-r", Status: "completed"},
		protocoljsonrpc.MethodToolCatalog:           []coreapi.ToolDefinition{{Name: "bash"}},
		protocoljsonrpc.MethodSandboxPolicy:         sandbox.Policy{Mode: "read-only"},
		protocoljsonrpc.MethodSandboxDerivePolicy:   sandbox.Policy{Mode: "workspace-write"},
		protocoljsonrpc.MethodMCPList:               []coreapi.MCPServer{{Name: "fetch"}},
		protocoljsonrpc.MethodMCPUpsert:             map[string]any{"ok": true},
		protocoljsonrpc.MethodMCPImportJSON:         map[string]any{"ok": true},
		protocoljsonrpc.MethodMCPDelete:             map[string]any{"ok": true},
		protocoljsonrpc.MethodMCPSetEnabled:         map[string]any{"ok": true},
		protocoljsonrpc.MethodLSPList:               []coreapi.LSPServer{{Language: "go"}},
		protocoljsonrpc.MethodLSPDetect:             "installed",
		protocoljsonrpc.MethodLSPStart:              "started",
		protocoljsonrpc.MethodLSPInstall:            "installed",
		protocoljsonrpc.MethodLSPDiagnostics:        []string{"diag"},
		protocoljsonrpc.MethodLSPDiagnosticsSummary: coreapi.LSPDiagnosticsSummary{Files: 2, Errors: 1},
	}}
	engine := NewRemoteEngine(caller)
	ctx := context.Background()

	if out, err := engine.Sessions().SetMeta(ctx, coreapi.SetSessionMetaRequest{}); err != nil || out.ID != "s-meta" {
		t.Fatalf("SetMeta = %+v, %v", out, err)
	}
	if out, err := engine.Turns().Resume(ctx, coreapi.TurnRef{}); err != nil || out.ID != "t-r" {
		t.Fatalf("Turns.Resume = %+v, %v", out, err)
	}
	if out, err := engine.ToolCatalog().List(ctx, coreapi.ListToolCatalogRequest{}); err != nil || len(out) != 1 {
		t.Fatalf("ToolCatalog.List = %+v, %v", out, err)
	}
	if out, err := engine.Sandbox().Policy(ctx, coreapi.SessionRef{}); err != nil || out.Mode != "read-only" {
		t.Fatalf("Sandbox.Policy = %+v, %v", out, err)
	}
	if out, err := engine.Sandbox().DerivePolicy(ctx, coreapi.DeriveSandboxPolicyRequest{}); err != nil || out.Mode != "workspace-write" {
		t.Fatalf("Sandbox.DerivePolicy = %+v, %v", out, err)
	}

	if out, err := engine.MCP().List(ctx); err != nil || len(out) != 1 {
		t.Fatalf("MCP.List = %+v, %v", out, err)
	}
	if err := engine.MCP().Upsert(ctx, coreapi.UpsertMCPRequest{}); err != nil {
		t.Fatalf("MCP.Upsert: %v", err)
	}
	if err := engine.MCP().ImportJSON(ctx, coreapi.ImportMCPJSONRequest{}); err != nil {
		t.Fatalf("MCP.ImportJSON: %v", err)
	}
	if err := engine.MCP().Delete(ctx, coreapi.MCPNameRequest{}); err != nil {
		t.Fatalf("MCP.Delete: %v", err)
	}
	if err := engine.MCP().SetEnabled(ctx, coreapi.SetMCPEnabledRequest{}); err != nil {
		t.Fatalf("MCP.SetEnabled: %v", err)
	}

	if out, err := engine.LSP().List(ctx); err != nil || len(out) != 1 {
		t.Fatalf("LSP.List = %+v, %v", out, err)
	}
	if out, err := engine.LSP().Detect(ctx, coreapi.LSPLanguageRequest{}); err != nil || out != "installed" {
		t.Fatalf("LSP.Detect = %q, %v", out, err)
	}
	if out, err := engine.LSP().Start(ctx, coreapi.LSPLanguageRequest{}); err != nil || out != "started" {
		t.Fatalf("LSP.Start = %q, %v", out, err)
	}
	if out, err := engine.LSP().Install(ctx, coreapi.LSPLanguageRequest{}); err != nil || out != "installed" {
		t.Fatalf("LSP.Install = %q, %v", out, err)
	}
	if out, err := engine.LSP().Diagnostics(ctx); err != nil || len(out) != 1 {
		t.Fatalf("LSP.Diagnostics = %+v, %v", out, err)
	}
	if out, err := engine.LSP().DiagnosticsSummary(ctx); err != nil || out.Files != 2 {
		t.Fatalf("LSP.DiagnosticsSummary = %+v, %v", out, err)
	}
}

// TestStartRemoteEngineBadPathArm StartProcess 失败快速返回臂；
// firstAgent 空列表回退构造。
func TestStartRemoteEngineBadPathArmAndFirstAgent(t *testing.T) {
	t.Setenv("EOS_CORE_PATH", "")
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")
	if _, err := StartRemoteEngine(context.Background(), ProcessOptions{
		Resolve: ResolveOptions{BinaryPath: "/definitely/missing/eos-core"},
	}); err == nil {
		t.Fatal("坏 CorePath StartRemoteEngine 应报错")
	}
	if got := firstAgent(remoteAgentControlResponse{}); got.ID != "" || got.Status != "" {
		t.Fatalf("空列表回退 = %+v", got)
	}
	if got := firstAgent(remoteAgentControlResponse{AgentID: "a", Status: "running"}); got.ID != "a" || got.Status != "running" {
		t.Fatalf("字段回退 = %+v", got)
	}
}

// TestStartRemoteEngineInitializeFailAndFullStart 错误模式：进程起、
// initialize 失败快速返回；成功模式：引擎全程可用 + Wait 走 client 通道。
func TestStartRemoteEngineInitializeFailAndFullStart(t *testing.T) {
	t.Setenv("EOS_CORE_PATH", "")
	t.Setenv("EOS_CORE_MANIFEST", "")
	t.Setenv("EOS_CORE_BIN_DIR", "")
	ctx := context.Background()

	dir := t.TempDir()
	core := filepath.Join(dir, "eos-core")
	py := `#!/usr/bin/env python3
import sys, json
mode = sys.argv[1] if len(sys.argv) > 1 else 'ok'

def read_frame():
    length = 0
    while True:
        line = sys.stdin.readline()
        if not line:
            return None
        line = line.strip()
        if not line:
            break
        if line.lower().startswith('content-length:'):
            length = int(line.split(':', 1)[1].strip())
    if length <= 0:
        return None
    return sys.stdin.read(length)

while True:
    raw = read_frame()
    if raw is None:
        break
    try:
        req = json.loads(raw)
    except Exception:
        continue
    resp = {'jsonrpc': '2.0', 'id': req.get('id')}
    if mode == 'error':
        resp['error'] = {'code': -32000, 'message': 'kernel stub failure'}
    else:
        resp['result'] = {'ok': True}
    out = json.dumps(resp)
    sys.stdout.write('Content-Length: %d\n\n%s' % (len(out), out))
    sys.stdout.flush()
`
	if err := os.WriteFile(core, []byte(py), 0o755); err != nil {
		t.Fatal(err)
	}

	// 错误模式：StartProcess 成功、Initialize 失败 → 66-69 臂。
	if _, err := StartRemoteEngine(ctx, ProcessOptions{BinaryPath: core, Args: []string{"error"}}); err == nil {
		t.Fatal("错误模式内核 Initialize 应失败")
	}

	// 成功模式：全链启动 + Wait 走 client 通道（128 臂）+ Close 收口。
	engine, err := StartRemoteEngine(ctx, ProcessOptions{BinaryPath: core, Args: []string{"ok"}})
	if err != nil {
		t.Fatalf("成功模式启动: %v", err)
	}
	select {
	case err := <-engine.Wait():
		t.Fatalf("存活内核 Wait 不应提前返回: %v", err)
	default:
	}
	if _, err := engine.State().Snapshot(ctx, coreapi.StateSnapshotRequest{}); err != nil {
		t.Fatalf("State.Snapshot 经脚本内核: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// 注：RemoteEngine.Wait() 的 waitCh 已被 StartRemoteEngine 的退出
	// 监看 goroutine 消费（单值通道语义），Close 的及时收口由 ProcessClient
	// 的 waitDone 广播保证（修复前 Close 必等 2.5s 后报假错）。
}
