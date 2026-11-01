package jsonrpc

import (
	"io"
	"sync"
	"testing"
	"time"
)

type waitForEOFReader struct{ eof <-chan struct{} }

func (r waitForEOFReader) Read([]byte) (int, error) { return 0, io.EOF }
func (r waitForEOFReader) Close() error             { <-r.eof; return nil }

type signalEOFWriter struct {
	eof  chan struct{}
	once *sync.Once
}

func (w signalEOFWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w signalEOFWriter) Close() error                { w.once.Do(func() { close(w.eof) }); return nil }

func TestStreamSignalsEOFBeforeClosingBlockingRead(t *testing.T) {
	eof := make(chan struct{})
	once := &sync.Once{}
	t.Cleanup(func() { once.Do(func() { close(eof) }) })
	stream := NewStream(waitForEOFReader{eof}, signalEOFWriter{eof, once})
	done := make(chan error, 1)
	go func() { done <- stream.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("read close blocked before the peer received EOF")
	}
}
