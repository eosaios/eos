package messages

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"time"

	"github.com/eosaios/eos/internal/ui/render"
	"github.com/eosaios/eos/internal/ui/styles"
)

// Renderer 消息渲染器
type Renderer struct {
	styles       *styles.Styles
	width        int
	agentNameMap map[string]string
	mainAgent    string
	mainLabel    string
	subLabel     string
	mdEnabled    bool
	md           *render.MarkdownRenderer
}

// NewRenderer 创建新的消息渲染器
func NewRenderer(s *styles.Styles, width int) *Renderer {
	r := &Renderer{
		styles:       s,
		width:        width,
		agentNameMap: map[string]string{},
		mainAgent:    "Assistant",
		mainLabel:    "",
		subLabel:     "",
		mdEnabled:    true,
		md:           render.NewMarkdownRenderer(width),
	}
	r.md.SetStyles(render.NewThemeRenderStyles(s))
	return r
}

// SetWidth 设置渲染宽度
func (r *Renderer) SetWidth(width int) {
	r.width = width
	if r.md != nil {
		r.md.SetWidth(width)
	}
}

// SetChromaTheme 设置 markdown 代码块（含 ```diff 围栏）的 chroma 高亮主题。
func (r *Renderer) SetChromaTheme(theme string) {
	if r.md != nil {
		r.md.SetChromaTheme(theme)
	}
}

func (r *Renderer) SetAgentNameMap(m map[string]string) {
	r.agentNameMap = map[string]string{}
	for k, v := range m {
		r.agentNameMap[k] = v
	}
}

func (r *Renderer) SetAgentLabels(mainLabel, subLabel string) {
	r.mainLabel = mainLabel
	r.subLabel = subLabel
}

func (r *Renderer) SetMainAgentName(name string) {
	if name != "" {
		r.mainAgent = name
	}
}

func (r *Renderer) EnableMarkdown(v bool) {
	r.mdEnabled = v
}

func (r *Renderer) displayAgentName(raw string) string {
	if raw == "" {
		return raw
	}
	if v, ok := r.agentNameMap[raw]; ok && v != "" {
		return v
	}
	return raw
}

func (r *Renderer) maybeRenderMarkdown(content string, done bool) string {
	if !r.mdEnabled || r.md == nil {
		return content
	}
	if done {
		// Segment complete: full glamour pass (AST reflow, code highlighting,
		// tables) for the final render.
		return r.md.Render(content)
	}
	// Streaming (done=false): cheap line-by-line styling that is safe on
	// partial input. Glamour is avoided mid-stream because half-written
	// fences/tables would reflow and flicker on every delta.
	return r.md.RenderStreaming(content)
}

func (r *Renderer) RenderUserInputAt(content string, ts time.Time) string {
	msg := &UserMessage{
		Content:   content,
		Timestamp: ts,
	}
	return msg.Render(r.styles, r.width)
}

// RenderUserInput 渲染用户输入
func (r *Renderer) RenderUserInput(content string) string {
	return r.RenderUserInputAt(content, time.Now())
}

func (r *Renderer) RenderAIResponseAt(content string, tokens int, duration time.Duration, done bool, ts time.Time) string {
	msg := &AgentBubbleMessage{
		Name:      r.mainAgent,
		Label:     r.mainLabel,
		Event:     "result",
		IsMain:    true,
		PreStyled: r.mdEnabled,
		Content:   r.maybeRenderMarkdown(content, done),
		Timestamp: ts,
		Tokens:    tokens,
		Duration:  duration,
		Done:      done,
	}
	return msg.Render(r.styles, r.width)
}

func (r *Renderer) RenderAIResponseAtWithCopy(content string, tokens int, duration time.Duration, done bool, ts time.Time, copyLabel string) string {
	return r.RenderAIResponseAtWithActions(content, tokens, duration, done, ts, []BubbleAction{{Kind: "copy", Label: copyLabel}})
}

func (r *Renderer) RenderAIResponseAtWithActions(content string, tokens int, duration time.Duration, done bool, ts time.Time, actions []BubbleAction) string {
	msg := &AgentBubbleMessage{
		Name:      r.mainAgent,
		Label:     r.mainLabel,
		Event:     "result",
		IsMain:    true,
		PreStyled: r.mdEnabled,
		Content:   r.maybeRenderMarkdown(content, done),
		Timestamp: ts,
		Tokens:    tokens,
		Duration:  duration,
		Done:      done,
		Actions:   actions,
	}
	return msg.Render(r.styles, r.width)
}

// RenderAIResponse 渲染AI响应
func (r *Renderer) RenderAIResponse(content string, tokens int, duration time.Duration, done bool) string {
	return r.RenderAIResponseAt(content, tokens, duration, done, time.Now())
}

// RenderToolCall 渲染工具调用
func (r *Renderer) RenderToolCall(name string, params map[string]any) string {
	msg := &ToolCallMessage{
		Name:      name,
		Params:    params,
		Status:    "running",
		Timestamp: time.Now(),
	}
	return msg.Render(r.styles, r.width)
}

func (r *Renderer) RenderToolEvent(name string, params map[string]any, status string, result string, duration time.Duration) string {
	msg := &ToolCallMessage{
		Name:     name,
		Params:   params,
		Status:   status,
		Result:   result,
		Duration: duration,
	}
	return msg.Render(r.styles, r.width)
}

// RenderToolResult 渲染工具结果
func (r *Renderer) RenderToolResult(name string, result string, success bool, duration time.Duration) string {
	status := "success"
	if !success {
		status = "error"
	}
	msg := &ToolCallMessage{
		Name:     name,
		Status:   status,
		Result:   result,
		Duration: duration,
	}
	return msg.Render(r.styles, r.width)
}

// RenderAgentTask 渲染子Agent任务（蓝色圆点 - 调度）
func (r *Renderer) RenderAgentTask(name, task, goal string, progress, step, totalSteps int, status string, duration time.Duration, results []string) string {
	return r.RenderAgentTaskAt(name, "", "assistant", "", "dispatch", task, time.Now())
}

func (r *Renderer) RenderAgentTaskAt(name, agentID, sourceName, sourceID, event, task string, ts time.Time) string {
	msg := &AgentDispatchMessage{
		AgentName:  r.displayAgentName(name),
		AgentID:    strings.TrimSpace(agentID),
		SourceName: r.displayAgentName(sourceName),
		SourceID:   strings.TrimSpace(sourceID),
		Event:      strings.TrimSpace(event),
		Task:       task,
		Timestamp:  ts,
	}
	return msg.Render(r.styles, r.width)
}

// RenderAgentFinal 渲染子Agent最终结果（绿色圆点）
func (r *Renderer) RenderAgentFinal(agentName, content string) string {
	return r.RenderAgentFinalAt(agentName, "", "assistant", "", "result", content, time.Now())
}

func (r *Renderer) RenderAgentFinalAt(agentName, agentID, sourceName, sourceID, event, content string, ts time.Time) string {
	msg := &AgentBubbleMessage{
		Name:       r.displayAgentName(agentName),
		Label:      r.subLabel,
		Event:      firstNonEmptyString(strings.TrimSpace(event), "result"),
		AgentID:    strings.TrimSpace(agentID),
		SourceName: r.displayAgentName(sourceName),
		SourceID:   strings.TrimSpace(sourceID),
		IsMain:     false,
		PreStyled:  r.mdEnabled,
		Content:    r.maybeRenderMarkdown(content, true),
		Timestamp:  ts,
		Done:       true,
	}
	return msg.Render(r.styles, r.width)
}

func (r *Renderer) RenderAgentFinalAtWithCopy(agentName, content string, ts time.Time, copyLabel string) string {
	return r.RenderAgentFinalAtWithActions(agentName, "", "assistant", "", "result", content, ts, []BubbleAction{{Kind: "copy", Label: copyLabel}})
}

func (r *Renderer) RenderAgentFinalAtWithActions(agentName, agentID, sourceName, sourceID, event, content string, ts time.Time, actions []BubbleAction) string {
	msg := &AgentBubbleMessage{
		Name:       r.displayAgentName(agentName),
		Label:      r.subLabel,
		Event:      firstNonEmptyString(strings.TrimSpace(event), "result"),
		AgentID:    strings.TrimSpace(agentID),
		SourceName: r.displayAgentName(sourceName),
		SourceID:   strings.TrimSpace(sourceID),
		IsMain:     false,
		PreStyled:  r.mdEnabled,
		Content:    r.maybeRenderMarkdown(content, true),
		Timestamp:  ts,
		Done:       true,
		Actions:    actions,
	}
	return msg.Render(r.styles, r.width)
}

// RenderPlan 渲染计划
func (r *Renderer) RenderPlan(title, description string, steps []PlanStep) string {
	msg := &PlanMessage{
		Title:       title,
		Description: description,
		Steps:       steps,
		Status:      "pending",
	}
	return msg.Render(r.styles, r.width)
}

// RenderThinking 渲染思考过程
func (r *Renderer) RenderThinking(content string, duration time.Duration, expanded bool, steps []ThinkingStep) string {
	return r.RenderThinkingWithHint(content, duration, expanded, steps, "")
}

func (r *Renderer) RenderThinkingWithHint(content string, duration time.Duration, expanded bool, steps []ThinkingStep, toggleHint string) string {
	msg := &ThinkingMessage{
		Content:    content,
		Duration:   duration,
		Expanded:   expanded,
		Steps:      steps,
		ToggleHint: toggleHint,
	}
	return msg.Render(r.styles, r.width)
}

// RenderSystem 渲染系统消息
func (r *Renderer) RenderSystem(content, level string) string {
	msg := &SystemMessage{
		Content: content,
		Level:   level,
	}
	return msg.Render(r.styles, r.width)
}

// RenderSystemPreStyled 渲染已含 ANSI 码的系统消息（跳过宽度折行）。
func (r *Renderer) RenderSystemPreStyled(content, level string) string {
	msg := &SystemMessage{
		Content:   content,
		Level:     level,
		PreStyled: true,
	}
	return msg.Render(r.styles, r.width)
}
