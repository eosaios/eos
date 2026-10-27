package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

// server_dispatch.go 实现 POST /wails/call：Wails v3 Call.ByName 协议的
// 服务端等价物。前端按 "<FQN>.BridgeService.<Method>" 调用，这里用反射
// 分发到 BridgeService 的导出方法：JSON 数组按位置反序列化为参数，
// 返回 (result, error) 或 (error)。

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
)

// dispatchDeniedMethods 是反射分发拒绝的生命周期方法：它们由 server 自身
// 调用，不向浏览器暴露。
var dispatchDeniedMethods = map[string]bool{
	"Start": true,
	"Close": true,
}

type callRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

type callResponse struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) handleCall(w http.ResponseWriter, r *http.Request) {
	var req callRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCallResponse(w, http.StatusBadRequest, callResponse{OK: false, Error: "invalid request body: " + err.Error()})
		return
	}
	result, err := s.dispatchCall(req.Method, req.Args)
	if err != nil {
		slog.Warn("web.call.failed", "method", req.Method, "error", err)
		writeCallResponse(w, http.StatusOK, callResponse{OK: false, Error: err.Error()})
		return
	}
	writeCallResponse(w, http.StatusOK, callResponse{OK: true, Result: result})
}

func writeCallResponse(w http.ResponseWriter, status int, resp callResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// dispatchCall 按 Wails FQN（或裸方法名）反射调用 BridgeService 导出方法。
func (s *Server) dispatchCall(method string, rawArgs []json.RawMessage) (any, error) {
	name := strings.TrimSpace(method)
	if name == "" {
		return nil, errors.New("method is required")
	}
	// Wails v3 FQN = <完整包路径>.<类型>.<方法>（包路径本身含点），
	// 取最后一个点之后的部分即 "<类型>.<方法>"。
	short := name[strings.LastIndex(name, ".")+1:]
	short = strings.TrimPrefix(short, "BridgeService.")
	if dispatchDeniedMethods[short] {
		return nil, fmt.Errorf("method %q is not callable from the browser", short)
	}
	target := reflect.ValueOf(s.bridge)
	m := target.MethodByName(short)
	if !m.IsValid() {
		return nil, fmt.Errorf("unknown bridge method %q", short)
	}
	mt := m.Type()
	if mt.NumIn() != len(rawArgs) {
		return nil, fmt.Errorf("method %q expects %d argument(s), got %d", short, mt.NumIn(), len(rawArgs))
	}
	in := make([]reflect.Value, mt.NumIn())
	for i, raw := range rawArgs {
		ptr := reflect.New(mt.In(i))
		if err := json.Unmarshal(raw, ptr.Interface()); err != nil {
			return nil, fmt.Errorf("method %q argument %d: %w", short, i, err)
		}
		in[i] = ptr.Elem()
	}
	out := m.Call(in)
	// 约定：最后一个返回值若是 error，则作为调用错误上抛；首个非 error
	// 返回值作为结果。
	if n := len(out); n > 0 {
		if errOut, ok := out[n-1].Interface().(error); ok && out[n-1].Type().Implements(errType) {
			if errOut != nil {
				return nil, errOut
			}
			if n == 1 {
				return nil, nil
			}
		}
	}
	for _, v := range out {
		if v.Type().Implements(errType) {
			continue
		}
		return v.Interface(), nil
	}
	return nil, nil
}

var errType = reflect.TypeOf((*error)(nil)).Elem()
