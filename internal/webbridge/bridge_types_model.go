package webbridge

// Model 域 DTO：模型卡片、上下文快照、provider/preset 目录、保存请求。
// JSON 字段语义与前端契约一致，仅做类型归属拆分。

type ModelCard struct {
	Name                    string `json:"name"`
	APIBase                 string `json:"apiBase"`
	APIKeyMasked            string `json:"apiKeyMasked"`
	Model                   string `json:"model"`
	Source                  string `json:"source"`
	Active                  bool   `json:"active"`
	SupportsReasoningEffort bool   `json:"supportsReasoningEffort"`
	// ReasoningLevels 思考档位（空 = 未标注，前端回落通用四档；
	// wire 档位词汇：off/auto/minimal/low/medium/high/xhigh/max）。
	ReasoningLevels []string `json:"reasoningLevels"`
	SupportsVision  bool     `json:"supportsVision"`
	SupportsTools   bool     `json:"supportsTools"`
	ProviderID      string   `json:"providerId"`
	Format          string   `json:"format"`
	PresetID        string   `json:"presetId"`
	ContextWindow   int64    `json:"contextWindow"`
	EditKind        string   `json:"editKind"`
	CanEdit         bool     `json:"canEdit"`
	CanDelete       bool     `json:"canDelete"`
}

type ModelContextSnapshot struct {
	WorkspaceRoot      string `json:"workspaceRoot"`
	SessionID          string `json:"sessionId"`
	GlobalDefaultName  string `json:"globalDefaultName"`
	WorkspaceModelName string `json:"workspaceModelName"`
	SessionModelName   string `json:"sessionModelName"`
	ResolvedModelName  string `json:"resolvedModelName"`
	ResolvedScope      string `json:"resolvedScope"`
}

type ModelProviderOption struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Website       string             `json:"website"`
	APIKeyEnv     string             `json:"apiKeyEnv"`
	Endpoints     []ProviderEndpoint `json:"endpoints"`
	DefaultModels []string           `json:"defaultModels"`
}

type ProviderEndpoint struct {
	Plan    string `json:"plan"`
	Format  string `json:"format"`
	APIBase string `json:"apiBase"`
}

// PlanModel 是套餐类 preset 内可选的模型项（如方舟 Agent Plan 含多厂商模型）。
// 能力字段为模型级标注：nil = 未标注（前端回落 preset 级能力）。
type PlanModel struct {
	ModelID                 string `json:"modelId"`
	Label                   string `json:"label"`
	ContextWindow           int64  `json:"contextWindow"`
	SupportsReasoningEffort *bool  `json:"supportsReasoningEffort,omitempty"`
	SupportsVision          *bool  `json:"supportsVision,omitempty"`
	SupportsTools           *bool  `json:"supportsTools,omitempty"`
	// ReasoningLevels 模型级思考档位（nil = 未标注，回落 preset 级）。
	ReasoningLevels []string `json:"reasoningLevels,omitempty"`
}

type ModelPresetOption struct {
	ID                      string   `json:"id"`
	Name                    string   `json:"name"`
	ProviderID              string   `json:"providerId"`
	ModelName               string   `json:"modelName"`
	Plan                    string   `json:"plan"`
	Format                  string   `json:"format"`
	ContextWindow           int      `json:"contextWindow"`
	Tags                    []string `json:"tags"`
	Description             string   `json:"description"`
	SupportsReasoningEffort bool     `json:"supportsReasoningEffort"`
	// ReasoningLevels 思考档位（空 = 不支持思考强度）。
	ReasoningLevels []string    `json:"reasoningLevels,omitempty"`
	SupportsVision  bool        `json:"supportsVision"`
	SupportsTools   bool        `json:"supportsTools"`
	PlanModels      []PlanModel `json:"planModels"`
}

type ModelCatalogState struct {
	Providers           []ModelProviderOption `json:"providers"`
	Presets             []ModelPresetOption   `json:"presets"`
	AllowCustomProvider bool                  `json:"allowCustomProvider"`
	AllowCustomModel    bool                  `json:"allowCustomModel"`
}

type ModelSaveRequest struct {
	OriginalName string `json:"originalName"`
	Mode         string `json:"mode"`
	ProviderID   string `json:"providerId"`
	PresetID     string `json:"presetId"`
	Name         string `json:"name"`
	APIKey       string `json:"apiKey"`
	APIBase      string `json:"apiBase"`
	Model        string `json:"model"`
	// 自定义模型的能力开关。可选；nil/缺省 = 用 core 的默认值（推理+工具开、视觉关）。
	SupportsReasoningEffort *bool `json:"supportsReasoningEffort,omitempty"`
	SupportsVision          *bool `json:"supportsVision,omitempty"`
	SupportsTools           *bool `json:"supportsTools,omitempty"`
}

// ModelVerifyResult 是 model/verify 连通测试的结果（只读，不改任何状态）。
// ok=false 时 message 是面向用户的失败原因，前端渲染在向导错误区。
type ModelVerifyResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latencyMs"`
	Message   string `json:"message"`
}
