package adapter

import (
	"context"
	"github.com/eosaios/eos/pkg/coreapi"
)

// TurnInput carries the user draft and explicitly selected file payloads.
type TurnInput struct {
	Text          string
	ExecutionMode string
	ImagePaths    []string
	Attachments   []coreapi.Attachment
	UseMemory     bool
}

// Invoke 启动一次 turn，订阅事件流直到 RequestDone / RequestFailed，返回 final 文本。
// useMemory 是请求级记忆注入开关，透传给内核的 turn/start.use_memory
// （注入裁决在内核）；source 固定为 cli，仅在日志中标识来源壳层。
func (a *CoreClientAdapter) Invoke(ctx context.Context, query, executionMode string, imagePaths []string, useMemory bool) (string, error) {
	return a.InvokeWithAttachments(ctx, TurnInput{Text: query, ExecutionMode: executionMode, ImagePaths: imagePaths, UseMemory: useMemory})
}
