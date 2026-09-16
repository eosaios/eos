package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// server.go 是 eos web 模式的 HTTP 服务壳：把浏览器里的桌面前端桥接到
// BridgeService。协议与 Wails v3 资产服务器对齐：
//   - GET  /wails/runtime.js  —— 运行时 shim（Call.ByName / Events.On / Window.*）
//   - POST /wails/call        —— BridgeService.<Method> 反射分发
//   - GET  /wails/ws          —— 事件推送（shell-updated / conversation-delta / …）
//   - GET  /eos/attachment-image —— 附件图片路由（与桌面 asset server 同路径）
//   - GET  /                  —— 前端静态产物（注入 web 运行时标记）

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ServerOptions 是 eos web 模式的启动参数（由 internal/cli/web.go 装配）。
type ServerOptions struct {
	ListenAddr string
	// UIDir 是前端构建产物目录（frontend/dist）。空值触发自动解析。
	UIDir string
	// StartupWorkspace 传给内核的初始工作区。
	StartupWorkspace string
	// CorePath / CoreManifestPath 来自 CLI 侧 sidecar 解析器；空值让桥
	// 回退到自身搜索（EOS_GUI_* env / exe 同级 core/）。
	CorePath         string
	CoreManifestPath string
	// NoOpenBrowser 禁止启动后自动打开浏览器。
	NoOpenBrowser bool
	// HeartbeatInterval 为心跳发射周期，0 用默认 1s。
	HeartbeatInterval time.Duration
}

// Server 是 web 模式的运行时容器：桥 + 事件扇出 hub + HTTP 路由。
type Server struct {
	opts   ServerOptions
	bridge *BridgeService
	hub    *eventHub
	uiDir  string
	server *http.Server
}

// Run 装配并阻塞运行 web 模式，直到 ctx 取消或收到 SIGINT/SIGTERM。
func Run(ctx context.Context, opts ServerOptions) error {
	uiDir, err := resolveWebUIDir(opts.UIDir)
	if err != nil {
		return err
	}
	bridge := NewBridgeServiceWithOptions(BridgeServiceOptions{
		LogFile:          DefaultLogDir(),
		StartupWorkspace: opts.StartupWorkspace,
		CorePath:         opts.CorePath,
		CoreManifestPath: opts.CoreManifestPath,
	})
	if bridge.runtimeGatewayStartError != "" {
		bridge.Close()
		return fmt.Errorf("start eos-core sidecar: %s", bridge.runtimeGatewayStartError)
	}

	hub := newEventHub()
	bridge.emitEvent = hub.emit
	bridge.Start()

	srv := &Server{opts: opts, bridge: bridge, hub: hub, uiDir: uiDir}
	addr := opts.ListenAddr
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	srv.server = httpSrv

	url := fmt.Sprintf("http://%s", addr)
	slog.Info("web.server.ready", "addr", addr, "ui_dir", uiDir, "url", url)
	fmt.Println(webServerReadyMessage(url, uiDir))
	if !opts.NoOpenBrowser {
		openBrowser(url)
	}

	errCh := make(chan error, 1)
	go func() {
		serveErr := httpSrv.ListenAndServe()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		bridge.Close()
		return err
	case <-sigCh:
		fmt.Println(webServerShutdownMessage())
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	bridge.Close()
	return nil
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /wails/runtime.js", s.handleRuntimeJS)
	mux.HandleFunc("POST /wails/call", s.handleCall)
	mux.HandleFunc("GET /wails/ws", s.handleWS)
	mux.Handle("GET "+AttachmentImageRoutePath, http.HandlerFunc(s.bridge.serveAttachmentImage))
	mux.Handle("GET "+BrowserFrameRoutePath, http.HandlerFunc(s.bridge.serveBrowserFrame))
	mux.HandleFunc("GET /", s.handleStatic)
	return mux
}

// openBrowser 用平台默认浏览器打开 url，失败只告警（URL 已打印到 stdout）。
func openBrowser(url string) {
	var name string
	var args []string
	switch runtimeGOOS() {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "cmd", []string{"/c", "start", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	if err := runBackgroundCommand(name, args...); err != nil {
		slog.Warn("web.server.open_browser_failed", "url", url, "error", err)
	}
}
