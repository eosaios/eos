package jsonrpc

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// 批十五批测：StreamClient 边界臂（未 Start 拒绝/写失败清理 pending/ctx
// 取消/重复 Start 幂等/nil 防御）/ Call 响应分类 / InProcessClient·Server
// nil 路径 / NewResultResponse·Validate·marshalParams 边界。

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// errWriter 恒定失败（写失败臂注入）。
type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

// blockingReader 永不产出数据（等待由 ctx/Close 解围；Close 解阻塞防泄漏）。
type blockingReader struct {
	ch   chan struct{}
	once sync.Once
}

func newBlockingReader() *blockingReader { return &blockingReader{ch: make(chan struct{})} }

func (r *blockingReader) Read([]byte) (int, error) {
	<-r.ch
	return 0, io.EOF
}

// Close 幂等：t.Cleanup 与 Stream.Close 可能都触发。
func (r *blockingReader) Close() error {
	r.once.Do(func() { close(r.ch) })
	return nil
}

// eofReader 立即 EOF（readLoop 错误路径注入）。
type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }

// replyServer 在 pipe 服务端循环回放预设 Response。
func replyServer(serverStream *Stream, reply func(id RequestID) Response) {
	go func() {
		for {
			msg, err := serverStream.ReadMessage()
			if err != nil {
				return
			}
			if msg.Kind == KindRequest && msg.Request != nil {
				if err := serverStream.WriteMessage(reply(msg.Request.ID)); err != nil {
					return
				}
			}
		}
	}()
}

func TestStreamClientEdgeArms(t *testing.T) {
	t.Run("nil 防御", func(t *testing.T) {
		var c *StreamClient
		if err := c.Start(context.Background()); err == nil {
			t.Fatal("nil Start error = nil")
		}
		if err := c.Call(context.Background(), "m", nil, nil); err == nil {
			t.Fatal("nil Call error = nil")
		}
		if _, err := c.Do(context.Background(), Request{}); err == nil {
			t.Fatal("nil Do error = nil")
		}
	})

	t.Run("未 Start 拒绝与重复 Start 幂等", func(t *testing.T) {
		rd := newBlockingReader()
		t.Cleanup(func() { _ = rd.Close() })
		c := NewStreamClient(NewStream(rd, io.Discard))
		req, _ := NewRequest(NumberID(1), "m", nil)
		if _, err := c.Do(context.Background(), req); err == nil || err.Error() != "jsonrpc stream client is not started" {
			t.Fatalf("not started error = %v", err)
		}
		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start error = %v", err)
		}
		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("second Start error = %v", err)
		}
		defer c.Close()
	})

	t.Run("写失败清理 pending", func(t *testing.T) {
		rd := newBlockingReader()
		t.Cleanup(func() { _ = rd.Close() })
		c := NewStreamClient(NewStream(rd, errWriter{err: errors.New("pipe broken")}))
		if err := c.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		req, _ := NewRequest(NumberID(1), "m", nil)
		if _, err := c.Do(context.Background(), req); err == nil || err.Error() != "pipe broken" {
			t.Fatalf("write failure = %v", err)
		}
	})

	t.Run("ctx 取消释放等待", func(t *testing.T) {
		rd := newBlockingReader()
		t.Cleanup(func() { _ = rd.Close() })
		c := NewStreamClient(NewStream(rd, io.Discard))
		if err := c.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		req, _ := NewRequest(NumberID(2), "m", nil)
		start := time.Now()
		if _, err := c.Do(ctx, req); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ctx cancel error = %v", err)
		}
		if time.Since(start) > time.Second {
			t.Fatal("Do did not respect ctx cancellation promptly")
		}
	})

	t.Run("addPending 空 key 拒绝", func(t *testing.T) {
		rd := newBlockingReader()
		t.Cleanup(func() { _ = rd.Close() })
		c := NewStreamClient(NewStream(rd, io.Discard))
		if err := c.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if err := c.addPending("", make(chan streamClientResponse, 1)); err == nil || err.Error() != "jsonrpc request id is required" {
			t.Fatalf("empty key error = %v", err)
		}
		// removePending 幂等（不存在 key 不 panic）。
		c.removePending("nope")
	})

	t.Run("readLoop 错误关闭客户端", func(t *testing.T) {
		c := NewStreamClient(NewStream(eofReader{}, io.Discard))
		if err := c.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case <-c.done:
				return
			default:
				time.Sleep(5 * time.Millisecond)
			}
		}
		t.Fatal("readLoop error did not close client")
	})
}

func TestStreamClientCallResponseClassification(t *testing.T) {
	t.Run("RPC 错误响应", func(t *testing.T) {
		clientStream, serverStream, cleanup := newStreamPipe(t)
		defer cleanup()
		replyServer(serverStream, func(id RequestID) Response {
			resp, _ := NewErrorResponse(id, CodeMethodNotFound, "no such method", nil)
			return resp
		})
		c := NewStreamClient(clientStream)
		if err := c.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		err := c.Call(context.Background(), "m", nil, nil)
		if err == nil {
			t.Fatal("RPC error response not surfaced")
		}
		rpcErr, ok := err.(*RPCError)
		if !ok || rpcErr.Code != CodeMethodNotFound {
			t.Fatalf("error = %+v", err)
		}
	})

	t.Run("非法响应（result 与 error 并存）", func(t *testing.T) {
		clientStream, serverStream, cleanup := newStreamPipe(t)
		defer cleanup()
		replyServer(serverStream, func(id RequestID) Response {
			return Response{ID: id, Result: json.RawMessage(`{}`), Error: &Error{Code: 1, Message: "x"}}
		})
		c := NewStreamClient(clientStream)
		if err := c.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if err := c.Call(context.Background(), "m", nil, nil); err == nil || err.Error() != "jsonrpc response cannot include both result and error" {
			t.Fatalf("invalid response error = %v", err)
		}
	})
}

func TestInProcessClientAndServerNilArms(t *testing.T) {
	var c *InProcessClient
	if err := c.Call(context.Background(), "m", nil, nil); err == nil || err.Error() != "jsonrpc requester is nil" {
		t.Fatalf("nil InProcessClient error = %v", err)
	}
	empty := InProcessClient{}
	if err := empty.Call(context.Background(), "m", nil, nil); err == nil {
		t.Fatal("empty requester error = nil")
	}

	// router nil → InternalError 响应（不是 panic）。
	server := InProcessServer{}
	resp, err := server.Do(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if resp.Error == nil || resp.Error.Code != CodeInternalError {
		t.Fatalf("nil router response = %+v", resp)
	}

	// 正常往返：router 错误透传为 RPC 错误。
	router := NewRouter()
	if err := router.Register("boom", func(context.Context, Request) (any, *Error) {
		return nil, &Error{Code: CodeInvalidParams, Message: "bad params"}
	}); err != nil {
		t.Fatal(err)
	}
	client := NewInProcessClient(InProcessServer{Router: router})
	callErr := client.Call(context.Background(), "boom", nil, nil)
	if callErr == nil {
		t.Fatal("handler error not surfaced")
	}
	if rpcErr, ok := callErr.(*RPCError); !ok || rpcErr.Code != CodeInvalidParams {
		t.Fatalf("error = %+v", callErr)
	}

	// 正常结果解包 + 空 result 不解包。
	if err := router.Register("ok", func(context.Context, Request) (any, *Error) {
		return map[string]int{"n": 7}, nil
	}); err != nil {
		t.Fatal(err)
	}
	var out map[string]int
	if err := client.Call(context.Background(), "ok", nil, &out); err != nil || out["n"] != 7 {
		t.Fatalf("ok call = %v %+v", err, out)
	}
	if err := router.Register("null", func(context.Context, Request) (any, *Error) {
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.Call(context.Background(), "null", nil, nil); err != nil {
		t.Fatalf("null result call = %v", err)
	}
}

func TestMessageConstructionEdges(t *testing.T) {
	// NewResultResponse nil → "null"。
	resp, err := NewResultResponse(NumberID(1), nil)
	if err != nil || string(resp.Result) != "null" {
		t.Fatalf("nil result = %+v %v", resp, err)
	}
	// 不可序列化结果 → 错误。
	if _, err := NewResultResponse(NumberID(1), make(chan int)); err == nil {
		t.Fatal("unmarshalable result error = nil")
	}
	// NewErrorResponse 不可序列化 data → 错误。
	if _, err := NewErrorResponse(NumberID(1), 1, "m", make(chan int)); err == nil {
		t.Fatal("unmarshalable data error = nil")
	}

	// Request/Notification/Validate 词表。
	req, _ := NewRequest(NumberID(1), "m", nil)
	if err := req.Validate(); err != nil {
		t.Fatalf("valid request = %v", err)
	}
	emptyReq, _ := NewRequest(NumberID(1), "", nil)
	if err := emptyReq.Validate(); err == nil || err.Error() != "jsonrpc method is required" {
		t.Fatalf("empty method = %v", err)
	}
	if err := (Notification{Method: " x "}).Validate(); err != nil {
		t.Fatalf("valid notification = %v", err)
	}
	if err := (Notification{}).Validate(); err == nil {
		t.Fatal("empty notification method = nil")
	}

	// marshalParams RawMessage 透传（空→nil）。
	raw, err := marshalParams(json.RawMessage{})
	if err != nil || raw != nil {
		t.Fatalf("empty raw = %v %v", raw, err)
	}
	raw2, err := marshalParams(json.RawMessage(`{"a":1}`))
	if err != nil || string(raw2) != `{"a":1}` {
		t.Fatalf("raw passthrough = %v %v", raw2, err)
	}
	if raw3, err := marshalParams(nil); err != nil || raw3 != nil {
		t.Fatalf("nil marshal = %v %v", raw3, err)
	}
}
