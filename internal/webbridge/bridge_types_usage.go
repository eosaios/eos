package webbridge

// Usage 域 DTO：成本明细、用量汇总、版本记录卡片。
// JSON 字段语义与前端契约一致，仅做类型归属拆分。

type CostItemCard struct {
	Time              string `json:"time"`
	Model             string `json:"model"`
	InputTokens       *int   `json:"inputTokens"`
	ReplyTokens       *int   `json:"replyTokens"`
	CachedInputTokens *int   `json:"cachedInputTokens"`
	TotalTokens       *int   `json:"totalTokens"`
	// ContextInputTokens 是该 turn 最近一次请求的真实 prompt tokens ≈ 上下文规模；
	// InputTokens 是各步累加的计费口径（多轮 ReAct 会远超窗口，不能当占用展示）。
	ContextInputTokens *int     `json:"contextInputTokens"`
	CostUSD            *float64 `json:"costUsd"`
	UsageKnown         bool     `json:"usageKnown"`
	CostKnown          bool     `json:"costKnown"`
}

type UsageSummaryCard struct {
	Rounds             int      `json:"rounds"`
	InputTokens        *int     `json:"inputTokens"`
	ReplyTokens        *int     `json:"replyTokens"`
	CachedInputTokens  *int     `json:"cachedInputTokens"`
	TotalTokens        *int     `json:"totalTokens"`
	CostUSD            *float64 `json:"costUsd"`
	UnknownUsageRounds int      `json:"unknownUsageRounds"`
	UnknownCostRounds  int      `json:"unknownCostRounds"`
}

type VersionCard struct {
	ID        string `json:"id"`
	File      string `json:"file"`
	CreatedAt string `json:"createdAt"`
	Summary   string `json:"summary"`
}
