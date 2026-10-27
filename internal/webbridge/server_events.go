package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

// server_events.go 是 WS 事件扇出 hub：BridgeService 的 emitEvent 注入点。
// 每条事件以 {"name": "...", "data": ...} 帧发给所有已连接的浏览器标签页；
// 前端 runtime shim 按 name 分发给 Events.On 订阅者。

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type eventHub struct {
	mu      sync.Mutex
	clients map[*hubClient]struct{}
}

type hubClient struct {
	conn *websocket.Conn
	mu   sync.Mutex // 同一连接的写串行化（coder/websocket 禁止并发写）
}

func newEventHub() *eventHub {
	return &eventHub{clients: map[*hubClient]struct{}{}}
}

func (h *eventHub) add(c *hubClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *eventHub) remove(c *hubClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// emit 把一条桥事件广播给所有浏览器连接。写失败的连接被摘除。
func (h *eventHub) emit(name string, data any) {
	frame, err := json.Marshal(map[string]any{"name": name, "data": data})
	if err != nil {
		slog.Warn("web.events.marshal_failed", "name", name, "error", err)
		return
	}
	h.mu.Lock()
	clients := make([]*hubClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		if err := c.write(frame); err != nil {
			h.remove(c)
		}
	}
}

func (c *hubClient) write(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.conn.Write(ctx, websocket.MessageText, frame)
}

// handleWS 接受浏览器事件订阅连接。服务端只写不读：读循环仅用于感知
// 客户端断开（收到关闭帧或读错误即摘除）。
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.Warn("web.events.accept_failed", "error", err)
		return
	}
	client := &hubClient{conn: conn}
	s.hub.add(client)
	defer func() {
		s.hub.remove(client)
		_ = conn.CloseNow()
	}()
	ctx := r.Context()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}
