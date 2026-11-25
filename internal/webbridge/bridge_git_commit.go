package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/eosaios/eos/pkg/coreapi"
	protocoljsonrpc "github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

// GitCommitMessageRequest 是一次提交信息的路由询问或 AI 生成。路由策略来自
// 用户的辅助任务设置；requestKey 由界面为每一次逻辑生成新建（重试复用，
// 重新生成换新）。
type GitCommitMessageRequest struct {
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	SessionID     string `json:"sessionId,omitempty"`
	RequestKey    string `json:"requestKey,omitempty"`
	Policy        string `json:"policy"`
	ByokModel     string `json:"byokModel,omitempty"`
	NoSessionMode string `json:"noSessionMode,omitempty"`
	// 用户已确认的托管告知版本。界面只在不需要再确认时才带上它。
	Ack string `json:"ack,omitempty"`
	// 界面展示给用户的费用承担方，取自最近一次 CommitMessageRoute 的结果。
	// 内核的裁定与它不一致时拒绝执行，而不是换一个付费方。
	ExpectedPayer string `json:"expectedPayer,omitempty"`
}

// CommitMessageConsent 是托管生成前须由用户确认的告知。
type CommitMessageConsent struct {
	Disclosure string `json:"disclosure"`
	// 确认后回传给内核的凭据；告知文案或费用上限变化时它会变。
	Ack string `json:"ack"`
	// 单次生成最多消耗多少积分（百万分之一积分）。
	MaxCreditsMicro int64 `json:"maxCreditsMicro"`
}

// GitCommitMessageResult 是生成结果。失败时 Code 是内核给出的稳定原因码
// （AI_*），Error 是供排查的说明；界面按 Code 显示自己的文案。
type GitCommitMessageResult struct {
	Ok           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
	Code         string `json:"code,omitempty"`
	Retryable    bool   `json:"retryable"`
	Message      string `json:"message,omitempty"`
	Payer        string `json:"payer,omitempty"`
	CreditsMicro int64  `json:"creditsMicro,omitempty"`
	InputSHA256  string `json:"inputSha256,omitempty"`
	RequestID    string `json:"requestId,omitempty"`
	// 非空 = 服务端要求先确认告知：这次没有生成，也没有扣费。
	Consent    *CommitMessageConsent `json:"consent,omitempty"`
	Redactions int                   `json:"redactions"`
	Truncated  bool                  `json:"truncated"`
}

// CommitMessageRouteResult 是路由询问的结果：这次生成将由谁付费。
type CommitMessageRouteResult struct {
	Ok          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	Code        string `json:"code,omitempty"`
	Retryable   bool   `json:"retryable"`
	Payer       string `json:"payer,omitempty"`
	InputSHA256 string `json:"inputSha256,omitempty"`
}

// CommitMessageCancelResult 是取消等待的结果；Cancelled=false 表示这次生成已经结束。
type CommitMessageCancelResult struct {
	Cancelled bool `json:"cancelled"`
}

type gitCommitGateway interface {
	CoreGitSuggestRoutingSupportedRPC(context.Context) (bool, error)
	CoreGitCommitMessageRPC(context.Context, coreapi.GitSuggestMessageRequest) (coreapi.GitSuggestMessageResult, error)
}

// commitMessageMode 区分「只问这次由谁付费」与「实际生成」。
type commitMessageMode int

const (
	commitMessageRoute commitMessageMode = iota
	commitMessageGenerate
)

// 进行中的生成，按 requestKey 登记取消函数。取消只让界面停止等待；已经发出的
// 模型调用由内核与服务端照常收尾并按实际用量计费。
var gitCommitCalls = struct {
	sync.Mutex
	cancel map[string]context.CancelFunc
}{cancel: map[string]context.CancelFunc{}}

// 这些原因下再试一次有意义；其余（未登录、未订阅、积分不足、未选模型等）
// 要用户先处理，界面不应把它表现成「重试即可」。
var gitCommitRetryable = map[string]bool{
	"AI_UPSTREAM_FAILED":     true,
	"AI_INVALID_OUTPUT":      true,
	"AI_PENDING":             true,
	"AI_BUSY":                true,
	"AI_MANAGED_UNAVAILABLE": true,
	"AI_MANAGED_UNREACHABLE": true,
	"AI_BYOK_FAILED":         true,
	"AI_BYOK_TIMEOUT":        true,
}

// CommitMessageRoute 问内核：按当前设置，这次生成将由谁付费。不调用任何模型，
// 也不向 EOS 发请求。界面据此标注费用来源，并在生成时把这一结果带回内核核对。
func (s *BridgeService) CommitMessageRoute(request GitCommitMessageRequest) CommitMessageRouteResult {
	if s == nil {
		return CommitMessageRouteResult{}
	}
	result := s.commitMessage(request, commitMessageRoute)
	return CommitMessageRouteResult{
		Ok:          result.Ok,
		Error:       result.Error,
		Code:        result.Code,
		Retryable:   result.Retryable,
		Payer:       result.Payer,
		InputSHA256: result.InputSHA256,
	}
}

// GenerateCommitMessage 让内核按辅助任务路由生成提交信息。壳层不裁决用哪个
// 模型：只转交界面的选择，并把内核的原因码原样带回。
func (s *BridgeService) GenerateCommitMessage(request GitCommitMessageRequest) GitCommitMessageResult {
	if s == nil {
		return GitCommitMessageResult{}
	}
	return s.commitMessage(request, commitMessageGenerate)
}

func (s *BridgeService) commitMessage(request GitCommitMessageRequest, mode commitMessageMode) GitCommitMessageResult {
	gateway, ok := s.runtimeGatewayClient().(gitCommitGateway)
	if !ok {
		return GitCommitMessageResult{Code: "AI_CORE_UNSUPPORTED", Error: "this Core cannot route auxiliary AI tasks"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := strings.TrimSpace(request.RequestKey)
	if key != "" {
		gitCommitCalls.Lock()
		gitCommitCalls.cancel[key] = cancel
		gitCommitCalls.Unlock()
		defer func() {
			gitCommitCalls.Lock()
			delete(gitCommitCalls.cancel, key)
			gitCommitCalls.Unlock()
		}()
	}
	supported, err := gateway.CoreGitSuggestRoutingSupportedRPC(ctx)
	if err == nil && !supported {
		// 旧内核会忽略路由并使用全局默认模型：费用承担方不明，不发起调用。
		return GitCommitMessageResult{Code: "AI_CORE_UNSUPPORTED", Error: "this Core does not route auxiliary AI tasks"}
	}
	if err == nil {
		var result coreapi.GitSuggestMessageResult
		result, err = gateway.CoreGitCommitMessageRPC(ctx, commitWireRequest(request, mode))
		if err == nil {
			out := commitResult(result)
			// 只有真的生成过才记：待确认的答复不代表用户用过哪一方。
			if mode == commitMessageGenerate && out.Payer != "" && out.Consent == nil {
				s.rememberCommitPayer(out.Payer)
			}
			return out
		}
	}
	if errors.Is(err, context.Canceled) {
		return GitCommitMessageResult{Code: "AI_CANCELLED", Error: "generation cancelled"}
	}
	return commitFailure(err)
}

// CancelCommitMessage 让界面停止等待某次生成。已经发出的调用不会被撤回。
func (s *BridgeService) CancelCommitMessage(requestKey string) CommitMessageCancelResult {
	key := strings.TrimSpace(requestKey)
	gitCommitCalls.Lock()
	cancel, waiting := gitCommitCalls.cancel[key]
	if waiting {
		delete(gitCommitCalls.cancel, key)
	}
	gitCommitCalls.Unlock()
	if waiting {
		cancel()
	}
	return CommitMessageCancelResult{Cancelled: waiting}
}

// commitWireRequest 把界面的请求翻译成内核字段。
func commitWireRequest(request GitCommitMessageRequest, mode commitMessageMode) coreapi.GitSuggestMessageRequest {
	// 内核拒绝未知策略；没说就用「自动」，与设置页的默认值一致。
	policy := auxiliaryModeName(request.Policy)
	if policy == "" {
		policy = AuxiliaryPolicyAuto
	}
	routing := &coreapi.GitSuggestRouting{
		Policy:        policy,
		ByokModel:     strings.TrimSpace(request.ByokModel),
		SessionID:     strings.TrimSpace(request.SessionID),
		NoSessionMode: strings.TrimSpace(request.NoSessionMode),
		RequestKey:    strings.TrimSpace(request.RequestKey),
		Ack:           strings.TrimSpace(request.Ack),
		ExpectedPayer: strings.TrimSpace(request.ExpectedPayer),
	}
	if mode == commitMessageRoute {
		// 询问只是把裁定告诉界面，界面展示的就是这个结果，无需核对。
		routing.Probe = true
		routing.ExpectedPayer = ""
	}
	return coreapi.GitSuggestMessageRequest{
		WorkspaceRoot: strings.TrimSpace(request.WorkspaceRoot),
		Routing:       routing,
	}
}

func commitResult(result coreapi.GitSuggestMessageResult) GitCommitMessageResult {
	out := GitCommitMessageResult{
		Ok:           true,
		Message:      strings.TrimSpace(result.Message),
		Payer:        strings.TrimSpace(result.Payer),
		CreditsMicro: result.CreditsMicro,
		RequestID:    strings.TrimSpace(result.RequestID),
		InputSHA256:  strings.TrimSpace(result.InputSha256),
		Redactions:   int(result.Redactions),
		Truncated:    result.Truncated,
	}
	if result.Consent != nil {
		out.Consent = &CommitMessageConsent{
			Disclosure:      result.Consent.Disclosure,
			Ack:             result.Consent.Ack,
			MaxCreditsMicro: result.Consent.MaxRequestCreditsMicro,
		}
	}
	return out
}

// commitFailure 取出内核原因码：内核把稳定码写在 reason 的开头（AI_*: 说明）。
func commitFailure(err error) GitCommitMessageResult {
	result := GitCommitMessageResult{Error: err.Error()}
	var rpcErr *protocoljsonrpc.RPCError
	if !errors.As(err, &rpcErr) {
		result.Retryable = true
		return result
	}
	name, detail, found := strings.Cut(strings.TrimSpace(rpcErr.Reason), ":")
	if found && strings.HasPrefix(name, "AI_") {
		result.Code = name
		result.Error = strings.TrimSpace(detail)
	} else if rpcErr.Reason != "" {
		result.Error = rpcErr.Reason
	}
	result.Retryable = result.Code == "" || gitCommitRetryable[result.Code]
	return result
}
