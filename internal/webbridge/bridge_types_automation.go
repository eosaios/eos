package webbridge

// Automation 域 DTO：自动化模板卡片、保存请求、运行记录。
// JSON 字段语义与前端契约一致，仅做类型归属拆分。

type AutomationTemplateCard struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Prompt        string `json:"prompt"`
	Schedule      string `json:"schedule"`      // cron 表达式（空 = 手动模板，不参与定时）
	Enabled       bool   `json:"enabled"`       // 是否参与定时调度
	Preset        bool   `json:"preset"`        // 是否预设模板（用户不可删除，可一键启用）
	WorkspacePath string `json:"workspacePath"` // 绑定的工作区（空 = 当前活动工作区）
	NextRunAt     string `json:"nextRunAt"`     // 下次运行时间（展示用）
}

// AutomationSaveRequest 用于新增/编辑用户自定义自动化模板。
type AutomationSaveRequest struct {
	OriginalID    string `json:"originalId"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Prompt        string `json:"prompt"`
	Schedule      string `json:"schedule"`      // cron 表达式，空 = 手动模板
	Enabled       bool   `json:"enabled"`       // 是否启用定时
	WorkspacePath string `json:"workspacePath"` // 绑定工作区，空 = 当前
}

type AutomationRunCard struct {
	ID         string `json:"id"`
	TemplateID string `json:"templateId"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	SessionID  string `json:"sessionId"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}
