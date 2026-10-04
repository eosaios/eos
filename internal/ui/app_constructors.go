package ui

// app_constructors.go — AppModel 的构造与生命周期取消函数。
//
// 本文件包含：
//   - NewAppModelFromCoreClient / NewAppModelFromCoreEngine：公开构造器
//   - hydrateCatalogFromAdapter：从适配器加载模型目录并应用到全局状态
//   - newAppModel：内部初始化，创建 shell / 面板 / 视图
//   - resolveShellWelcomeInfo：从适配器解析模型名用于欢迎卡片
//   - setActiveCancel / cancelActiveRequest / markInflightToolsCanceled /
//     cancelProcessingUI：处理中请求的取消与状态清理
//
// 这些代码原位于 app.go，仅做物理拆分，不改行为。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eosaios/eos/internal/ai"
	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/ui/adapter"
	"github.com/eosaios/eos/internal/ui/components/messages"
	"github.com/eosaios/eos/internal/ui/panels"
	"github.com/eosaios/eos/internal/ui/styles"
	"github.com/eosaios/eos/internal/ui/views/help"
	"github.com/eosaios/eos/internal/ui/views/setup"
	"github.com/eosaios/eos/internal/ui/views/shell"
	"github.com/eosaios/eos/pkg/coreapi"
	sidecarclient "github.com/eosaios/eos/pkg/coreapi/sidecar/client"
)

// NewAppModelFromCoreClient 用已 handshake 的 sidecar Client 构造 AppModel。
// 这是生产路径：TUI 通过 pkg/coreapi/sidecar/client 启动 eos-core --app-server --stdio。
// resumeSessionID 非空时（来自 --continue/--resume），Init 会在工作区信任后恢复该会话。
func NewAppModelFromCoreClient(client *sidecarclient.Client, resumeSessionID string) *AppModel {
	return newAppModel(hydrateCatalogFromAdapter(adapter.NewCoreClientAdapter(client)), resumeSessionID)
}

// NewAppModelFromCoreEngine 直接用 coreapi.Engine 构造 AppModel。
// 供测试场景使用（不启动 sidecar 子进程）。
func NewAppModelFromCoreEngine(engine coreapi.Engine) *AppModel {
	return newAppModel(hydrateCatalogFromAdapter(adapter.NewCoreClientAdapterFromEngine(engine)), "")
}

// hydrateCatalogFromAdapter 从适配器加载模型目录并应用到全局状态
func hydrateCatalogFromAdapter(coreAdapter *adapter.CoreClientAdapter) *adapter.CoreClientAdapter {
	if coreAdapter == nil {
		return nil
	}
	catalog, err := coreAdapter.ModelCatalog(context.Background())
	if err != nil {
		ai.ApplyCoreModelCatalog(coreapi.ModelCatalogState{})
		return coreAdapter
	}
	ai.ApplyCoreModelCatalog(catalog)
	return coreAdapter
}

// newAppModel 初始化应用模型，创建所有视图和面板。
// resumeSessionID 非空时存入 pendingResumeSession，由 Init() 在工作区信任后消费。
func newAppModel(adapter *adapter.CoreClientAdapter, resumeSessionID string) *AppModel {
	theme := styles.GetTheme("dark")
	styles := styles.NewStyles(theme)

	// 加载配置
	cfg, _ := config.Load()
	lang := cfg.Language
	if lang == "" {
		lang = "zh"
	}
	predictionEnabled := config.NextMessagePredictionEnabled(&cfg)
	memoryInjectionEnabled := config.MemoryInjectionEnabled(&cfg)
	diffTheme := config.DiffHighlightTheme(&cfg)

	// 创建Shell视图
	shellModel := shell.New(80, 24, styles, lang)
	modelName, modelBase := resolveShellWelcomeInfo(adapter)
	shellModel.SetWelcomeInfo(modelName, modelBase, "")
	shellModel.SetExecutionMode("auto")
	shellModel.SetThinkingExpanded(false)
	_ = adapter.SetExecutionMode(context.Background(), "auto")

	// 创建面板
	panelMap := make(map[string]panels.Panel)
	panelMap["context"] = panels.NewContextPanel(styles, lang)
	panelMap["memory"] = panels.NewMemoryPanel(styles, lang)
	panelMap["rules"] = panels.NewRulesPanel(styles, lang)
	panelMap["workspace"] = panels.NewWorkspacePanel(styles, lang)
	lspPanel := panels.NewLSPPanel(styles, lang)
	panelMap["lsp"] = lspPanel

	// 创建模型面板（内部会自动从配置文件加载当前模型）
	panelMap["models"] = panels.NewModelsPanel(styles, lang)

	panelMap["settings"] = panels.NewSettingsPanel(styles, nil, lang)
	mcpPanel := panels.NewMCPPanel(styles, lang)
	// 加载 MCP 服务器配置
	var mcpServers []panels.MCPServer
	configServers, _ := adapter.MCPServers(context.Background())
	for _, s := range configServers {
		mcpServers = append(mcpServers, panels.MCPServer{
			Name:    s.Name,
			Type:    string(s.Type),
			Enabled: s.Enabled,
		})
	}
	mcpPanel.SetServers(mcpServers)
	panelMap["mcp"] = mcpPanel

	panelMap["cost"] = panels.NewCostPanel(styles, lang)
	panelMap["versions"] = panels.NewVersionsPanel(styles)
	panelMap["tasks"] = panels.NewTasksPanel(styles, lang, adapter)

	setupView := any(setup.NewSetupView(styles))
	activeView := "shell"
	initialSetupFlow := false
	models, _, _ := adapter.ModelEntries(context.Background())
	hasConfiguredModel := len(models) > 0
	if !hasConfiguredModel {
		wizard := setup.NewModelSetupWizard(styles, lang)
		wizard.SetSize(80, 24)
		setupView = wizard
		activeView = "setup"
		initialSetupFlow = true
		shellModel.BlurInput()
	}

	var pendingResume *string
	if id := strings.TrimSpace(resumeSessionID); id != "" {
		idCopy := id
		pendingResume = &idCopy
	}

	model := &AppModel{
		state: AppState{
			Mode:          "ai",
			Language:      lang,
			Theme:         "dark",
			ExecutionMode: "auto",
		},
		adapter:                adapter,
		styles:                 styles,
		msgRenderer:            messages.NewRenderer(styles, 80),
		shell:                  &shellModel,
		panels:                 panelMap,
		helpView:               help.NewHelpView(styles, lang),
		setupView:              setupView,
		activeView:             activeView,
		initialSetupFlow:       initialSetupFlow,
		activePanel:            "",
		toolInflight:           make(map[string]toolTrack),
		history:                make([]historyEntry, 0, 128),
		predictionEnabled:      predictionEnabled,
		memoryInjectionEnabled: memoryInjectionEnabled,
		diffTheme:              diffTheme,
		pendingResumeSession:   pendingResume,
		gitBaselineDirty:         -1,
		gitBaselineAhead:         -1,
	}

	model.msgRenderer.SetChromaTheme(diffTheme)
	return model
}

// resolveShellWelcomeInfo 从适配器获取模型信息，用于欢迎卡片显示
func resolveShellWelcomeInfo(adapter *adapter.CoreClientAdapter) (string, string) {
	modelName, modelBase := adapter.GetModelInfo()
	if modelName == "" {
		modelName = "(none)"
	}
	if modelBase == "" {
		modelBase = "(none)"
	}
	return modelName, modelBase
}

// refreshShellWelcomeInfo 从适配器获取模型信息并更新欢迎卡片显示
func (m *AppModel) refreshShellWelcomeInfo() {
	if m == nil || m.shell == nil || m.adapter == nil {
		return
	}
	modelName, modelBase := resolveShellWelcomeInfo(m.adapter)
	m.shell.SetWelcomeInfo(modelName, modelBase, "")
}

// cancelProcessingUI 取消处理状态，清除所有进行中的 UI 状态
func (m *AppModel) cancelProcessingUI() {
	m.state.Processing = false
	m.shell.SetProcessing(false)
	m.shell.ClearLive()
	m.aiLive.Reset()
	m.clearCurrentThinking()
	m.shell.SetStatusHints(false, false)
	m.toolInflight = make(map[string]toolTrack)
	m.activeCancel = nil
	m.stopRequested = false
}

// setActiveCancel 设置当前活跃的取消函数
func (m *AppModel) setActiveCancel(cancel context.CancelFunc) {
	m.activeCancel = cancel
	m.stopRequested = false
}

// cancelActiveRequest 取消当前活跃的请求，返回是否成功取消
func (m *AppModel) cancelActiveRequest() bool {
	if m == nil || m.activeCancel == nil || m.stopRequested {
		return false
	}
	cancel := m.activeCancel
	m.activeCancel = nil
	m.stopRequested = true
	cancel()
	return true
}

// markInflightToolsCanceled 将所有进行中的工具标记为已取消
func (m *AppModel) markInflightToolsCanceled(output string) {
	if len(m.toolInflight) == 0 {
		return
	}
	for _, track := range m.toolInflight {
		if track.idx < 0 || track.idx >= len(m.history) {
			continue
		}
		e := m.history[track.idx]
		e.toolStatus = "canceled"
		e.toolSuccess = false
		if strings.TrimSpace(e.toolOutput) == "" {
			e.toolOutput = output
		}
		e.duration = time.Since(track.started)
		m.history[track.idx] = e
	}
	m.rebuildHistoryContent()
}

// resumeStartupSession 消费 pendingResumeSession：对 --continue/--resume 指定的会话
// 调 ResumeSession（"latest" 交给内核解析为最近会话）+ restoreSessionHistory 把历史
// 回填进 m.history。工作区信任检查通过后调用（Init 直接路径或 workspace_trust 确认后）。
// 幂等：消费后清空 pendingResumeSession，避免重复 resume。
func (m *AppModel) resumeStartupSession() {
	if m == nil || m.adapter == nil || m.pendingResumeSession == nil {
		return
	}
	id := strings.TrimSpace(*m.pendingResumeSession)
	m.pendingResumeSession = nil
	if id == "" {
		return
	}
	if err := m.adapter.ResumeSession(context.Background(), id); err != nil {
		m.appendSystem(err.Error(), "error")
		return
	}
	resolvedID, _ := m.adapter.CurrentSessionID(context.Background())
	if strings.TrimSpace(resolvedID) == "" {
		m.appendSystem(m.localize("未找到可恢复的会话", "No session found to resume"), "warning")
		return
	}
	m.restoreSessionHistory(resolvedID)
	m.refreshContextPanel()
	m.refreshCostPanel()
	m.updateContextUsageUI()
	m.appendSystem(fmt.Sprintf("%s: %s", m.localize("已恢复会话", "Resumed session"), resolvedID), "success")
}
