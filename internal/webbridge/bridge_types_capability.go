package webbridge

// Capability 域 DTO：MCP / LSP / Skill / Plugin 集成卡片。
// JSON 字段语义与前端契约一致，仅做类型归属拆分。

type MCPServerCard struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
}

type LSPServerCard struct {
	Language string `json:"language"`
	Status   string `json:"status"`
	Command  string `json:"command"`
}

type SkillCard struct {
	Name                   string   `json:"name"`
	Description            string   `json:"description"`
	Source                 string   `json:"source"`
	ArgumentHint           string   `json:"argumentHint"`
	BaseDir                string   `json:"baseDir"`
	AllowedTools           []string `json:"allowedTools"`
	Enabled                bool     `json:"enabled"`
	Active                 bool     `json:"active"`
	DisableModelInvocation bool     `json:"disableModelInvocation"`
	UserInvocable          bool     `json:"userInvocable"`
}

type PluginCard struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Command     string `json:"command"`
	Enabled     bool   `json:"enabled"`
}
