package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"log/slog"
)

func requireRuntimeGateway(s *BridgeService) (bridgeRuntimeGateway, error) {
	if s == nil || s.runtimeGatewayClient() == nil {
		return nil, errors.New("runtime core unavailable")
	}
	return s.runtimeGatewayClient(), nil
}

func runtimeGatewayOrNil(s *BridgeService) bridgeRuntimeGateway {
	if s == nil {
		return nil
	}
	return s.runtimeGatewayClient()
}

func coreOnlyErr(gateway bridgeRuntimeGateway, core func(bridgeRuntimeGateway) error) error {
	return core(gateway)
}

// coreOnlyValue invokes the core RPC and returns its value; on error it returns
// the caller-supplied zero and logs the failure so a swallowed read does not
// become invisible. Most callers feed the result into BootstrapState, which has
// no error channel — the slog entry is the operator-visible observability path,
// and user-facing degradation notices are emitted by the load* projections that
// detect the resulting empty payload (see bridge_degraded_notify.go).
func coreOnlyValue[T any](gateway bridgeRuntimeGateway, zero T, core func(bridgeRuntimeGateway) (T, error)) T {
	if value, err := core(gateway); err == nil {
		return value
	} else {
		slog.Warn("bridge.core_rpc.read_failed", "error", err)
	}
	return zero
}

func coreOnlyResult[T any](gateway bridgeRuntimeGateway, core func(bridgeRuntimeGateway) (T, error)) (T, error) {
	return core(gateway)
}

// coreValueOrNil collapses the common "gateway-or-nil guard + coreOnlyValue"
// pattern used by read-only snapshots: returns zero when no core is wired,
// otherwise delegates to coreOnlyValue (which logs RPC errors). Prefer this
// over spelling out the three steps inline.
func coreValueOrNil[T any](s *BridgeService, zero T, core func(bridgeRuntimeGateway) (T, error)) T {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return zero
	}
	return coreOnlyValue(gateway, zero, core)
}

// coreValueOrRequire collapses "requireRuntimeGateway + coreOnlyResult" for
// callers that return (T, error) to their own caller.
func coreValueOrRequire[T any](s *BridgeService, core func(bridgeRuntimeGateway) (T, error)) (T, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		var zero T
		return zero, err
	}
	return coreOnlyResult(gateway, core)
}

// coreErrOrRequire collapses "requireRuntimeGateway + coreOnlyErr" for write
// operations whose only result is an error.
func coreErrOrRequire(s *BridgeService, core func(bridgeRuntimeGateway) error) error {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return err
	}
	return coreOnlyErr(gateway, core)
}
