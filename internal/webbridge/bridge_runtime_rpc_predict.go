package webbridge

import (
	"context"
	"strings"
)

// 消息预测 RPC：根据当前 draft 预测下一条用户消息文本。

func (s *BridgeService) predictNextUserMessageRPC(ctx context.Context, draft string) (string, error) {
	gateway, err := requireRuntimeGateway(s)
	if err != nil {
		return "", err
	}
	text, err := coreOnlyResult(
		gateway,
		func(g bridgeRuntimeGateway) (string, error) { return g.CorePredictNextUserMessageRPC(ctx, draft) },
	)
	return strings.TrimSpace(text), err
}
