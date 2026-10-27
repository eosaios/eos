package webbridge

// Permission 域 DTO：执行权限/沙箱状态、Bash 命令状态。
// JSON 字段语义与前端契约一致，仅做类型归属拆分。

type PermissionState struct {
	ExecutionMode     string   `json:"executionMode"`
	SandboxMode       string   `json:"sandboxMode"`
	ApprovalMode      string   `json:"approvalMode"`
	AllowAll          bool     `json:"allowAll"`
	AllowedCategories []string `json:"allowedCategories"`
	HasPendingDiff    bool     `json:"hasPendingDiff"`
	PendingDiffPath   string   `json:"pendingDiffPath"`
}

type BashState struct {
	Command   string   `json:"command"`
	Output    []string `json:"output"`
	Status    string   `json:"status"`
	UpdatedAt string   `json:"updatedAt"`
}
