package webbridge

// System 域 DTO：通知、命令面板、诊断、剪贴板、窗口、文件对话框、导出、资源检查。
// JSON 字段语义与前端契约一致，仅做类型归属拆分。

type NotificationItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Tone      string `json:"tone"`
	CreatedAt string `json:"createdAt"`
}

type CommandAction struct {
	ID          string `json:"id"`
	Command     string `json:"command"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Target      string `json:"target"`
}

type DiagnosticsState struct {
	LogFile           string   `json:"logFile"`
	LogTail           []string `json:"logTail"`
	LSPDiagnostics    []string `json:"lspDiagnostics"`
	ContextPreview    []string `json:"contextPreview"`
	ContextSummary    string   `json:"contextSummary"`
	CostSummary       string   `json:"costSummary"`
	PendingReviewPath string   `json:"pendingReviewPath"`
	PendingReviewDiff string   `json:"pendingReviewDiff"`
	PendingCrashPath  string   `json:"pendingCrashPath"`
	PendingCrashPanic string   `json:"pendingCrashPanic"`
	ExportDirectory   string   `json:"exportDirectory"`
}

type ClipboardState struct {
	Supported bool   `json:"supported"`
	Text      string `json:"text"`
}

type WindowSnapshot struct {
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	X          int  `json:"x"`
	Y          int  `json:"y"`
	Maximised  bool `json:"maximised"`
	Minimised  bool `json:"minimised"`
	Fullscreen bool `json:"fullscreen"`
	Visible    bool `json:"visible"`
}

type FileDialogResult struct {
	Paths     []string `json:"paths"`
	Cancelled bool     `json:"cancelled"`
}

type ExportResult struct {
	Path      string `json:"path"`
	Cancelled bool   `json:"cancelled"`
}

// ZipEntry 是 SaveZipFileDialog 的单条打包项（与桌面端 eos-app 同契约）。
type ZipEntry struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type ResourceCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}
