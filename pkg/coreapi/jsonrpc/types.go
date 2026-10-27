package jsonrpc

import (
	"context"

	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// Notifier sends protocol notifications to connected clients.
type Notifier interface {
	Notify(context.Context, protocoljsonrpc.Notification) error
}

type NotifierFunc func(context.Context, protocoljsonrpc.Notification) error

func (f NotifierFunc) Notify(ctx context.Context, notification protocoljsonrpc.Notification) error {
	return f(ctx, notification)
}

// Options configures the JSON-RPC server registration.
type Options struct {
	ServerName      string         `json:"server_name,omitempty"`
	ProtocolVersion string         `json:"protocol_version,omitempty"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	Notifier        Notifier       `json:"-"`
}

// InitializeResult is the response to the initialize JSON-RPC method.
type InitializeResult struct {
	ServerName      string         `json:"server_name"`
	ProtocolVersion string         `json:"protocol_version"`
	Methods         []string       `json:"methods"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
}
