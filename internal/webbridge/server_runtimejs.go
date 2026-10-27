package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import "net/http"

// server_runtimejs.go 在 /wails/runtime.js 提供浏览器端运行时 shim。
// 桌面前端只通过 workbench-runtime.ts 的三个出口使用 Wails 运行时：
// Call.ByName（→ POST /wails/call）、Events.On（→ /wails/ws 事件流）、
// Window.*（浏览器窗口不受服务端管理，能力位如实返回不支持）。
// 服务接口签名与 Wails 内置运行时一致，桌面前端代码无需感知差异。

const runtimeJSSource = `// eos web runtime shim — serves the desktop frontend in a browser.
const listeners = new Map();
let ws = null;
let wsBackoffMs = 500;

function connectWS() {
  if (ws) return;
  const proto = location.protocol === "https:" ? "wss" : "ws";
  const socket = new WebSocket(proto + "://" + location.host + "/wails/ws");
  ws = socket;
  socket.onopen = () => { wsBackoffMs = 500; };
  socket.onmessage = (e) => {
    let frame;
    try { frame = JSON.parse(e.data); } catch { return; }
    if (!frame || typeof frame.name !== "string") return;
    const event = { name: frame.name, data: frame.data };
    const subs = listeners.get(frame.name);
    if (subs) for (const cb of [...subs]) {
      try { cb(event); } catch (err) { console.error("[eos-web] event handler failed", frame.name, err); }
    }
  };
  socket.onclose = () => {
    if (ws === socket) ws = null;
    setTimeout(connectWS, wsBackoffMs);
    wsBackoffMs = Math.min(wsBackoffMs * 2, 8000);
  };
  socket.onerror = () => { try { socket.close(); } catch {} };
}

async function callByName(method, args) {
  const res = await fetch("/wails/call", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ method, args }),
  });
  let payload;
  try { payload = await res.json(); } catch {
    throw new Error("web bridge returned non-JSON response (HTTP " + res.status + ")");
  }
  if (payload && payload.ok) return payload.result;
  throw new Error((payload && payload.error) || ("web bridge call failed (HTTP " + res.status + ")"));
}

function addListener(name, cb) {
  if (typeof cb !== "function") return () => {};
  let subs = listeners.get(name);
  if (!subs) { subs = new Set(); listeners.set(name, subs); }
  subs.add(cb);
  return () => { subs.delete(cb); };
}

window.__EOS_WEB_RUNTIME__ = true;
connectWS();

export const Call = {
  ByName: (method, ...args) => callByName(method, args),
};

export const Events = {
  On: (name, cb) => { connectWS(); return addListener(name, cb); },
  Off: (name, cb) => {
    const subs = listeners.get(name);
    if (subs) subs.delete(cb);
  },
  Once: (name, cb) => {
    const off = addListener(name, (event) => { off(); cb(event); });
    return off;
  },
  Emit: async (name, data) => {
    // 浏览器 → 服务端没有自定义事件通道；桥状态全部由 RPC 驱动。
    console.info("[eos-web] Events.Emit is a no-op in web mode:", name, data);
  },
};

const unsupported = (op) => () => {
  console.info("[eos-web] window control unavailable in web mode:", op);
  return Promise.resolve();
};

export const Window = {
  IsMaximised: async () => true,
  IsFullscreen: async () => false,
  IsMinimised: async () => false,
  Close: unsupported("close"),
  Minimise: unsupported("minimise"),
  ToggleMaximise: unsupported("toggle-maximise"),
  ToggleFullscreen: unsupported("toggle-fullscreen"),
  Hide: unsupported("hide"),
};
`

func (s *Server) handleRuntimeJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(runtimeJSSource))
}
