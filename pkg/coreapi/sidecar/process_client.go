package sidecar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	coreapijsonrpc "github.com/eosaios/eos/pkg/coreapi/jsonrpc"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

type ProcessOptions struct {
	BinaryPath          string
	Args                []string
	Env                 map[string]string
	Dir                 string
	Stderr              io.Writer
	NotificationHandler protocoljsonrpc.NotificationHandler
	Resolve             ResolveOptions
	VerifyChecksum      bool
	RequireSignature    bool
	RequiredFeatures    []string
	PublicKeyPath       string
	AllowDevPlaceholder bool
}

type ProcessClient struct {
	cmd    *exec.Cmd
	client *protocoljsonrpc.StreamClient
	waitCh chan error

	// waitDone/waitErr：进程收割后关闭/落账。waitCh 是单值通道，
	// 会被 Wait() 消费者抽干（如 StartRemoteEngine 的退出监看），
	// Close 需要一个可多读者共享的退出信号才能不误等超时。
	waitDoneOnce sync.Once
	waitDone     chan struct{}
	waitMu       sync.Mutex
	waitErr      error

	closeOnce sync.Once
	closeErr  error
}

func StartProcess(ctx context.Context, opts ProcessOptions) (*ProcessClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	binaryPath := opts.BinaryPath
	if binaryPath == "" {
		resolveOpts := opts.Resolve
		resolveOpts.VerifyChecksum = opts.VerifyChecksum
		resolveOpts.RequireSignature = opts.RequireSignature
		if len(opts.RequiredFeatures) > 0 {
			resolveOpts.RequiredFeatures = append([]string(nil), opts.RequiredFeatures...)
		}
		if opts.PublicKeyPath != "" {
			resolveOpts.PublicKeyPath = opts.PublicKeyPath
		}
		if opts.AllowDevPlaceholder {
			resolveOpts.AllowDevPlaceholder = true
		}
		resolved, err := ResolveBinary(resolveOpts)
		if err != nil {
			return nil, err
		}
		binaryPath = resolved.Path
	}

	cmd := exec.Command(binaryPath, opts.Args...)
	HideConsole(cmd)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Env = mergedEnv(os.Environ(), opts.Env)
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = io.Discard
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	stream := protocoljsonrpc.NewStream(stdout, stdin)
	client := protocoljsonrpc.NewStreamClient(stream, protocoljsonrpc.WithNotificationHandler(opts.NotificationHandler))
	if err := client.Start(ctx); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}

	pc := &ProcessClient{
		cmd:      cmd,
		client:   client,
		waitCh:   make(chan error, 1),
		waitDone: make(chan struct{}),
	}
	go func() {
		err := cmd.Wait()
		pc.waitMu.Lock()
		pc.waitErr = err
		pc.waitMu.Unlock()
		pc.waitDoneOnce.Do(func() { close(pc.waitDone) })
		pc.waitCh <- err
	}()
	return pc, nil
}

func (c *ProcessClient) Call(ctx context.Context, method string, params any, out any) error {
	if c == nil || c.client == nil {
		return errors.New("core sidecar process client is nil")
	}
	return c.client.Call(ctx, method, params, out)
}

func (c *ProcessClient) Do(ctx context.Context, req protocoljsonrpc.Request) (protocoljsonrpc.Response, error) {
	if c == nil || c.client == nil {
		return protocoljsonrpc.Response{}, errors.New("core sidecar process client is nil")
	}
	return c.client.Do(ctx, req)
}

func (c *ProcessClient) Initialize(ctx context.Context) (coreapijsonrpc.InitializeResult, error) {
	var result coreapijsonrpc.InitializeResult
	if err := c.Call(ctx, protocoljsonrpc.MethodInitialize, nil, &result); err != nil {
		return coreapijsonrpc.InitializeResult{}, err
	}
	return result, nil
}

func (c *ProcessClient) Shutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}
	var result map[string]any
	return c.Call(ctx, protocoljsonrpc.MethodShutdown, nil, &result)
}

func (c *ProcessClient) Close() error {
	return c.CloseWithTimeout(2 * time.Second)
}

func (c *ProcessClient) Wait() <-chan error {
	if c == nil {
		ch := make(chan error, 1)
		ch <- errors.New("nil process client")
		close(ch)
		return ch
	}
	return c.waitCh
}

// readWaitErr 在 waitDone 已关后读取收割结果（waitCh 可能已被 Wait() 消费者取走）。
func (c *ProcessClient) readWaitErr() {
	c.waitMu.Lock()
	c.closeErr = c.waitErr
	c.waitMu.Unlock()
}

func (c *ProcessClient) CloseWithTimeout(timeout time.Duration) error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.client != nil {
			_ = c.client.Close()
		}
		if timeout <= 0 {
			timeout = 2 * time.Second
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case err := <-c.waitCh:
			c.closeErr = err
		case <-c.waitDone:
			c.readWaitErr()
		case <-timer.C:
			if c.cmd != nil && c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
			select {
			case err := <-c.waitCh:
				c.closeErr = err
			case <-c.waitDone:
				c.readWaitErr()
			case <-time.After(500 * time.Millisecond):
				c.closeErr = fmt.Errorf("core sidecar process did not exit after kill")
			}
		}
	})
	return c.closeErr
}

func mergedEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return append([]string(nil), base...)
	}
	out := append([]string(nil), base...)
	index := map[string]int{}
	for i, item := range out {
		if key, _, ok := strings.Cut(item, "="); ok {
			index[strings.ToUpper(key)] = i
		}
	}
	for key, value := range extra {
		if key == "" {
			continue
		}
		env := key + "=" + value
		upper := strings.ToUpper(key)
		if i, ok := index[upper]; ok {
			out[i] = env
			continue
		}
		index[upper] = len(out)
		out = append(out, env)
	}
	return out
}
