package webbridge

// Rules 域 DTO：规则文档状态、保存/重置请求。
// JSON 字段语义与前端契约一致，仅做类型归属拆分。

type RulesState struct {
	Global     RuleDocument   `json:"global"`
	Workspaces []RuleDocument `json:"workspaces"`
}

type RuleDocument struct {
	ID            string `json:"id"`
	Scope         string `json:"scope"`
	Title         string `json:"title"`
	Path          string `json:"path"`
	Content       string `json:"content"`
	Exists        bool   `json:"exists"`
	Active        bool   `json:"active"`
	WorkspacePath string `json:"workspacePath,omitempty"`
	WorkspaceName string `json:"workspaceName,omitempty"`
}

type RulesSaveRequest struct {
	Scope         string `json:"scope"`
	WorkspacePath string `json:"workspacePath,omitempty"`
	Value         string `json:"value"`
}

type RulesResetRequest struct {
	Scope         string `json:"scope"`
	WorkspacePath string `json:"workspacePath,omitempty"`
}
