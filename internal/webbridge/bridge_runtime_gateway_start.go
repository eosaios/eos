package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"context"

	"github.com/eosaios/eos/internal/webbridge/adapter"
)

func startBridgeStdioGatewayProcess(ctx context.Context, opts adapter.StdioClientOptions) (bridgeRuntimeGateway, func() error, adapter.StdioResolvedBinary, error) {
	client := adapter.NewStdioClient(opts)
	if err := client.Start(ctx); err != nil {
		return nil, nil, client.ResolvedBinary(), err
	}
	return adapter.NewStdioGateway(client), client.Close, client.ResolvedBinary(), nil
}
