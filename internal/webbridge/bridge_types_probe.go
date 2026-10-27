package webbridge

// Probe / Heartbeat 域 DTO：探针事件、心跳载荷。
// JSON 字段语义与前端契约一致，仅做类型归属拆分。

type BridgeProbe struct {
	Source      string       `json:"source"`
	Input       string       `json:"input"`
	Events      []ProbeEvent `json:"events"`
	Completed   bool         `json:"completed"`
	Error       string       `json:"error,omitempty"`
	CompletedAt string       `json:"completedAt"`
}

type ProbeEvent struct {
	Type      string `json:"type"`
	Message   string `json:"message"`
	EventType string `json:"eventType"`
}

type HeartbeatPayload struct {
	Time             string         `json:"time"`
	ActiveWorkspace  string         `json:"activeWorkspace"`
	BridgeMode       string         `json:"bridgeMode"`
	CurrentSessionID string         `json:"currentSessionId"`
	Window           WindowSnapshot `json:"window"`
}
