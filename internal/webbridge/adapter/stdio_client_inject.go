package adapter

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// This file is intentionally outside the legacy build tag so that
// bridge-service tests can construct a StdioClient backed by a custom
// io.ReadWriteCloser (e.g. a net.Pipe) instead of an external rust-core
// process. Production callers should still use NewStdioClient + Start().

import (
	"bufio"
	"context"
	"errors"
	"io"
)

// NewStdioClientWithStream returns a StdioClient whose transport is the
// supplied ReadWriteCloser rather than a child rust-core process. The
// returned client is NOT started; the caller must invoke StartWithStream
// to wire the JSON-RPC stream and start the read loop.
//
// This is the seam used by bridge-service integration tests to drive the
// production StdioGateway from an in-memory mock core without requiring
// the actual eos-core binary on the test machine.
func NewStdioClientWithStream(opts StdioClientOptions, stream io.ReadWriteCloser) *StdioClient {
	return &StdioClient{
		opts: opts,
		done: make(chan struct{}),
	}
}

// StartWithStream wires the JSON-RPC stream over the supplied
// ReadWriteCloser and starts the read loop. It is the stream-injection
// counterpart to Start(); production callers should keep using Start() so
// that the actual rust-core child process is launched and supervised.
func (sc *StdioClient) StartWithStream(ctx context.Context, stream io.ReadWriteCloser) error {
	if stream == nil {
		return errors.New("stdio client: stream is nil")
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cmd != nil || sc.stream != nil {
		return errors.New("stdio client already started")
	}
	br := bufio.NewReader(stream)
	sa := &streamAdapter{
		reader: br,
		writer: stream,
		closer: stream,
	}
	sc.stream = sa
	sc.client = newStdioRPCClient(sa)
	if err := sc.client.Start(ctx); err != nil {
		return err
	}
	// We are not supervising an external process, so close the done
	// channel when the stream is closed.
	go func() {
		<-ctx.Done()
		_ = stream.Close()
	}()
	return nil
}
