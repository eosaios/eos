package jsonrpc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

const DefaultMaxFrameBytes = 16 * 1024 * 1024

type Stream struct {
	reader          *bufio.Reader
	writer          io.Writer
	readCloser      io.Closer
	writeCloser     io.Closer
	closeOnce       sync.Once
	writeMu         sync.Mutex
	MaxMessageBytes int
}

func NewStream(reader io.Reader, writer io.Writer) *Stream {
	var br *bufio.Reader
	if existing, ok := reader.(*bufio.Reader); ok {
		br = existing
	} else if reader != nil {
		br = bufio.NewReader(reader)
	}
	s := &Stream{
		reader:          br,
		writer:          writer,
		MaxMessageBytes: DefaultMaxFrameBytes,
	}
	if closer, ok := reader.(io.Closer); ok {
		s.readCloser = closer
	}
	if closer, ok := writer.(io.Closer); ok {
		s.writeCloser = closer
	}
	return s
}

func (s *Stream) ReadMessage() (DecodedMessage, error) {
	payload, err := s.ReadFrame()
	if err != nil {
		return DecodedMessage{}, err
	}
	return Decode(payload)
}

func (s *Stream) ReadFrame() ([]byte, error) {
	if s == nil || s.reader == nil {
		return nil, errors.New("jsonrpc stream reader is nil")
	}
	contentLength := -1
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid jsonrpc stream header: %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid jsonrpc content length: %q", value)
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, errors.New("jsonrpc content length is required")
	}
	limit := s.MaxMessageBytes
	if limit <= 0 {
		limit = DefaultMaxFrameBytes
	}
	if contentLength > limit {
		return nil, fmt.Errorf("jsonrpc frame too large: %d > %d", contentLength, limit)
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Stream) WriteMessage(v any) error {
	payload, err := Marshal(v)
	if err != nil {
		return err
	}
	return s.WriteFrame(payload)
}

func (s *Stream) WriteFrame(payload []byte) error {
	if s == nil || s.writer == nil {
		return errors.New("jsonrpc stream writer is nil")
	}
	if len(payload) == 0 {
		return errors.New("jsonrpc payload is empty")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	if _, err := s.writer.Write(payload); err != nil {
		return err
	}
	if flusher, ok := s.writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

func (s *Stream) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.closeOnce.Do(func() {
		// Signal EOF to the peer before waiting for an in-flight pipe read to
		// finish. Windows synchronous pipe Close waits for ReadFile to return.
		if s.writeCloser != nil {
			err = s.writeCloser.Close()
		}
		if s.readCloser != nil {
			if closeErr := s.readCloser.Close(); err == nil {
				err = closeErr
			}
		}
	})
	return err
}

type StreamNotifier struct {
	Stream *Stream
}

func (n StreamNotifier) Notify(ctx context.Context, notification Notification) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if n.Stream == nil {
		return errors.New("jsonrpc stream is nil")
	}
	return n.Stream.WriteMessage(notification)
}

func ServeStream(ctx context.Context, router *Router, stream *Stream) error {
	if router == nil {
		return errors.New("jsonrpc router is nil")
	}
	if stream == nil {
		return errors.New("jsonrpc stream is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		readCh := make(chan streamReadResult, 1)
		go func() {
			message, err := stream.ReadMessage()
			readCh <- streamReadResult{message: message, err: err}
		}()
		var result streamReadResult
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result = <-readCh:
		}
		if result.err != nil {
			if errors.Is(result.err, io.EOF) {
				return nil
			}
			return result.err
		}
		if result.message.Kind != KindRequest || result.message.Request == nil {
			continue
		}
		response := router.Handle(ctx, *result.message.Request)
		if err := stream.WriteMessage(response); err != nil {
			return err
		}
	}
}

type streamReadResult struct {
	message DecodedMessage
	err     error
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
