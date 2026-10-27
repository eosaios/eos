package messages

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eosaios/eos/internal/ui/styles"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// MessageType 消息类型
type MessageType string

const (
	MsgTypeUser     MessageType = "user"
	MsgTypeAI       MessageType = "ai"
	MsgTypeTool     MessageType = "tool"
	MsgTypeAgent    MessageType = "agent"
	MsgTypePlan     MessageType = "plan"
	MsgTypeThinking MessageType = "thinking"
	MsgTypeSystem   MessageType = "system"
	MsgTypeError    MessageType = "error"
	MsgTypeWarning  MessageType = "warning"
	MsgTypeInfo     MessageType = "info"
)

// Message 消息接口
type Message interface {
	Type() MessageType
	Render(s *styles.Styles, width int) string
}

// UserMessage 用户消息
type UserMessage struct {
	Content   string
	Timestamp time.Time
}

func (m *UserMessage) Type() MessageType { return MsgTypeUser }

func (m *UserMessage) Render(s *styles.Styles, width int) string {
	// 文本流布局：首行 "› " 前缀，续行缩进，无气泡边框。
	return renderUserStream(s, m.Content, width)
}

// BubbleAction 描述消息可触发的操作（复制/下载）。
// 文本流布局下不再渲染为内联按钮，仅用于标记可点击性，
// 由 app 层在点击消息文本时弹出操作弹框。
type BubbleAction struct {
	Kind  string
	Label string
}

type AgentBubbleMessage struct {
	Name       string
	Label      string
	Event      string
	AgentID    string
	SourceName string
	SourceID   string
	IsMain     bool
	PreStyled  bool
	Content    string
	Timestamp  time.Time
	Tokens     int
	Duration   time.Duration
	Done       bool
	Actions    []BubbleAction
}

func (m *AgentBubbleMessage) Type() MessageType { return MsgTypeAgent }

func (m *AgentBubbleMessage) Render(s *styles.Styles, width int) string {
	// 文本流布局：头部行 "● [event] source -> name <id> ts" + 内容缩进 + 可选元信息。
	// 不再渲染内联操作按钮；Actions 仅标记可点击性，由 app 层弹框承载。
	return renderAgentFinalStream(s, m.Event, m.SourceName, m.Name, m.AgentID,
		m.Content, m.PreStyled, m.Tokens, m.Duration, m.Done, m.Timestamp, width)
}

func splitAndWrapANSI(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		out = append(out, wrapLine(line, maxWidth)...)
	}
	return out
}

type AgentDispatchMessage struct {
	AgentName  string
	AgentID    string
	SourceName string
	SourceID   string
	Event      string
	Task       string
	Timestamp  time.Time
}

func (m *AgentDispatchMessage) Type() MessageType { return MsgTypeAgent }

func (m *AgentDispatchMessage) Render(s *styles.Styles, width int) string {
	// 文本流布局：头部行 + 任务文本缩进，无气泡边框。
	return renderAgentTaskStream(s, m.Event, m.SourceName, m.AgentName, m.AgentID, m.Task, m.Timestamp, width)
}

// ToolCallMessage 工具调用消息
type ToolCallMessage struct {
	Name      string
	Params    map[string]any
	Status    string // "running", "success", "error"
	Result    string
	Duration  time.Duration
	Timestamp time.Time
}

func (m *ToolCallMessage) Type() MessageType { return MsgTypeTool }

func (m *ToolCallMessage) Render(s *styles.Styles, width int) string {
	return renderToolStream(s, m.Name, m.Params, m.Status, m.Result, m.Duration, width)
}

// buildToolDetailBlocks 按工具类型从 params 提取要展示的明细块，
// 返回 blocks 与已被消费的参数键集合 shown（供调用方做未识别参数回退）。
func buildToolDetailBlocks(name string, params map[string]any) []toolDetailBlock {
	toolLower := strings.ToLower(strings.TrimSpace(name))
	var blocks []toolDetailBlock
	shown := map[string]bool{}
	getStr := func(keys ...string) string {
		for _, k := range keys {
			if params == nil {
				continue
			}
			if v, ok := params[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					shown[k] = true
					return s
				}
			}
		}
		return ""
	}
	getInt := func(key string) (int, bool) {
		if params == nil {
			return 0, false
		}
		v, ok := params[key]
		if !ok {
			return 0, false
		}
		switch n := v.(type) {
		case int:
			shown[key] = true
			return n, true
		case int64:
			shown[key] = true
			return int(n), true
		case float64:
			shown[key] = true
			return int(n), true
		default:
			return 0, false
		}
	}
	getStrList := func(keys ...string) string {
		for _, k := range keys {
			if params == nil {
				continue
			}
			v, ok := params[k]
			if !ok || v == nil {
				continue
			}
			switch vv := v.(type) {
			case []string:
				if len(vv) == 0 {
					continue
				}
				shown[k] = true
				max := 3
				if len(vv) < max {
					max = len(vv)
				}
				out := strings.Join(vv[:max], "\n")
				if len(vv) > max {
					out += fmt.Sprintf("\n...(+%d)", len(vv)-max)
				}
				return out
			case []any:
				if len(vv) == 0 {
					continue
				}
				var ss []string
				for _, it := range vv {
					if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
						ss = append(ss, s)
					}
					if len(ss) >= 3 {
						break
					}
				}
				if len(ss) == 0 {
					continue
				}
				shown[k] = true
				out := strings.Join(ss, "\n")
				if len(vv) > len(ss) {
					out += fmt.Sprintf("\n...(+%d)", len(vv)-len(ss))
				}
				return out
			}
		}
		return ""
	}

	switch toolLower {
	case "read", "functions.read":
		if p := getStr("file_path", "path", "file"); p != "" {
			blocks = append(blocks, toolDetailBlock{label: "Path", value: p, kind: "path"})
		}
		if off, okOff := getInt("offset"); okOff {
			if lim, okLim := getInt("limit"); okLim {
				blocks = append(blocks, toolDetailBlock{label: "Range", value: fmt.Sprintf("offset=%d limit=%d", off, lim), kind: "meta"})
			} else {
				blocks = append(blocks, toolDetailBlock{label: "Range", value: fmt.Sprintf("offset=%d", off), kind: "meta"})
			}
		} else if lim, okLim := getInt("limit"); okLim {
			blocks = append(blocks, toolDetailBlock{label: "Range", value: fmt.Sprintf("limit=%d", lim), kind: "meta"})
		}
	case "grep", "functions.grep":
		if pat := getStr("pattern"); pat != "" {
			blocks = append(blocks, toolDetailBlock{label: "Pattern", value: pat, kind: "pattern"})
		}
		if p := getStr("path"); p != "" {
			blocks = append(blocks, toolDetailBlock{label: "In", value: p, kind: "path"})
		}
		if g := getStr("glob"); g != "" {
			blocks = append(blocks, toolDetailBlock{label: "Glob", value: g, kind: "meta"})
		}
	case "glob", "functions.glob":
		if pat := getStr("pattern"); pat != "" {
			blocks = append(blocks, toolDetailBlock{label: "Pattern", value: pat, kind: "pattern"})
		}
		if p := getStr("path"); p != "" {
			blocks = append(blocks, toolDetailBlock{label: "In", value: p, kind: "path"})
		}
	case "searchcodebase", "functions.searchcodebase":
		if req := getStr("information_request"); req != "" {
			blocks = append(blocks, toolDetailBlock{label: "Query", value: req, kind: "pattern"})
		}
	case "runcommand", "functions.runcommand":
		if cmd := getStr("command"); cmd != "" {
			cmd = strings.ReplaceAll(cmd, "\r\n", "\n")
			blocks = append(blocks, toolDetailBlock{label: "Command", value: cmd, kind: "command"})
		}
		if cwd := getStr("cwd"); cwd != "" {
			blocks = append(blocks, toolDetailBlock{label: "Cwd", value: cwd, kind: "path"})
		}
		if ct := getStr("command_type"); ct != "" {
			blocks = append(blocks, toolDetailBlock{label: "Type", value: ct, kind: "meta"})
		}
		if b, ok := params["blocking"].(bool); ok {
			shown["blocking"] = true
			blocks = append(blocks, toolDetailBlock{label: "Blocking", value: fmt.Sprintf("%v", b), kind: "meta"})
		}
	case "webfetch", "functions.webfetch":
		if url := getStr("url"); url != "" {
			blocks = append(blocks, toolDetailBlock{label: "URL", value: url, kind: "path"})
		}
	case "websearch", "functions.websearch":
		if q := getStr("query"); q != "" {
			blocks = append(blocks, toolDetailBlock{label: "Query", value: q, kind: "pattern"})
		}
	case "deletefile", "functions.deletefile":
		if fp := getStrList("file_paths", "paths", "files"); fp != "" {
			blocks = append(blocks, toolDetailBlock{label: "Paths", value: fp, kind: "path"})
		}
	default:
		if cmd := getStr("command"); cmd != "" {
			blocks = append(blocks, toolDetailBlock{label: "Command", value: cmd, kind: "command"})
		} else if p := getStr("path", "file_path", "file"); p != "" {
			blocks = append(blocks, toolDetailBlock{label: "Path", value: p, kind: "path"})
		} else if fp := getStrList("file_paths", "paths", "files"); fp != "" {
			blocks = append(blocks, toolDetailBlock{label: "Paths", value: fp, kind: "path"})
		}
	}

	// 未被明细块消费的参数作为通用 meta 块追加，保证信息不丢。
	if len(params) > 0 {
		var keys []string
		for k := range params {
			if shown[k] {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			blocks = append(blocks, toolDetailBlock{label: k, value: fmt.Sprintf("%v", params[k]), kind: "meta"})
		}
	}

	return blocks
}

func displayToolName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if i := strings.LastIndex(raw, "."); i >= 0 && i+1 < len(raw) {
		raw = raw[i+1:]
	}
	if !strings.Contains(raw, "_") && !strings.Contains(raw, "-") {
		if raw != strings.ToLower(raw) {
			return raw
		}
	}
	l := strings.ToLower(raw)
	known := map[string]string{
		"fs":             "FS",
		"mcp":            "MCP",
		"lsp":            "LSP",
		"ui":             "UI",
		"time_now":       "TimeNow",
		"runcommand":     "RunCommand",
		"searchcodebase": "SearchCodebase",
		"websearch":      "WebSearch",
		"webfetch":       "WebFetch",
	}
	if v, ok := known[l]; ok {
		return v
	}
	raw = strings.ReplaceAll(raw, "-", "_")
	parts := strings.Split(raw, "_")
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pl := strings.ToLower(p)
		switch pl {
		case "fs", "mcp", "lsp", "ui", "url", "http", "https", "json", "yaml", "xml", "sql":
			b.WriteString(strings.ToUpper(pl))
		default:
			runes := []rune(pl)
			b.WriteString(strings.ToUpper(string(runes[:1])))
			if len(runes) > 1 {
				b.WriteString(string(runes[1:]))
			}
		}
	}
	if b.Len() == 0 {
		return raw
	}
	return b.String()
}

// PlanMessage 计划消息
type PlanMessage struct {
	Title       string
	Description string
	Steps       []PlanStep
	Status      string // "pending", "running", "completed"
}

// PlanStep 计划步骤
type PlanStep struct {
	Number      int
	Description string
	Status      string // "pending", "running", "completed", "failed"
}

func (m *PlanMessage) Type() MessageType { return MsgTypePlan }

func (m *PlanMessage) Render(s *styles.Styles, width int) string {
	var result strings.Builder

	// 头部
	header := s.MsgPlanHeader.Render(fmt.Sprintf("📋 Plan: %s", m.Title))
	result.WriteString(header)
	result.WriteString("\n")

	// 描述
	if m.Description != "" {
		result.WriteString(m.Description)
		result.WriteString("\n\n")
	}

	// 步骤
	for _, step := range m.Steps {
		statusIcon := "⏸"
		statusStyle := s.TextMuted
		switch step.Status {
		case "completed":
			statusIcon = "✓"
			statusStyle = s.TextSuccess
		case "running":
			statusIcon = "🔄"
			statusStyle = s.TextInfo
		case "failed":
			statusIcon = "✗"
			statusStyle = s.TextError
		}

		stepLine := fmt.Sprintf("%s %d. %s", statusStyle.Render(statusIcon), step.Number, step.Description)
		result.WriteString(s.MsgPlanStep.Render(stepLine))
		result.WriteString("\n")
	}

	return result.String()
}

// ThinkingMessage 思考过程消息
type ThinkingMessage struct {
	Content    string
	Duration   time.Duration
	Expanded   bool
	Steps      []ThinkingStep
	ToggleHint string
}

// ThinkingStep 思考步骤
type ThinkingStep struct {
	Description string
	Status      string // "done", "doing", "pending"
}

func (m *ThinkingMessage) Type() MessageType { return MsgTypeThinking }

func (m *ThinkingMessage) Render(s *styles.Styles, width int) string {
	var result strings.Builder

	// 头部
	expandIcon := "[+]"
	if m.Expanded {
		expandIcon = "[-]"
	}
	headerText := fmt.Sprintf("💭 Thinking · %.1fs ─── %s", m.Duration.Seconds(), expandIcon)
	if strings.TrimSpace(m.ToggleHint) != "" {
		headerText = fmt.Sprintf("%s · %s", headerText, strings.TrimSpace(m.ToggleHint))
	}
	header := s.MsgThinkingHeader.Render(headerText)
	result.WriteString(header)

	// 展开时显示内容
	if m.Expanded {
		result.WriteString("\n")

		// 步骤
		for _, step := range m.Steps {
			statusIcon := "○"
			switch step.Status {
			case "done":
				statusIcon = s.TextSuccess.Render("✓")
			case "doing":
				statusIcon = s.TextInfo.Render("●")
			}
			fmt.Fprintf(&result, "  %s %s\n", statusIcon, step.Description)
		}

		// 详细内容
		if m.Content != "" {
			result.WriteString("\n")
			lines := wrapText(m.Content, width-6)
			for _, line := range lines {
				result.WriteString("  " + s.TextMuted.Render(line))
				result.WriteString("\n")
			}
		}
	} else {
		summary := lastNonEmptyLine(m.Content)
		if summary != "" {
			result.WriteString("\n")
			summary = truncateInline(summary, 160)
			for _, line := range wrapText(summary, width-6) {
				result.WriteString("  " + s.TextMuted.Render(line))
				result.WriteString("\n")
			}
		}
	}

	return result.String()
}

// LastNonEmptyLine returns the last non-empty, non-whitespace line of text.
// Exported so other packages (e.g. the shell's history renderer) can reuse the
// same summary logic used by the collapsed ThinkingMessage.
func LastNonEmptyLine(text string) string {
	return lastNonEmptyLine(text)
}

func lastNonEmptyLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

// SystemMessage 系统消息
type SystemMessage struct {
	Content string
	Level   string // "info", "warning", "error", "success"
	// PreStyled 为 true 时内容已含 ANSI 码（如 diff 高亮），跳过按宽度
	// 折行（wrapText 不感知 ANSI，会把转义序列拆断）。
	PreStyled bool
}

func (m *SystemMessage) Type() MessageType {
	switch m.Level {
	case "error":
		return MsgTypeError
	case "warning":
		return MsgTypeWarning
	case "info":
		return MsgTypeInfo
	default:
		return MsgTypeSystem
	}
}

func (m *SystemMessage) Render(s *styles.Styles, width int) string {
	icon := "ℹ️"
	style := s.MsgInfo

	switch m.Level {
	case "error":
		icon = "❌"
		style = s.MsgError
	case "warning":
		icon = "⚠️"
		style = s.MsgWarning
	case "success":
		icon = "✅"
		style = s.TextSuccess
	}
	lines := wrapText(fmt.Sprintf("%s %s", icon, m.Content), width)
	if m.PreStyled {
		// 预格式化内容（含 ANSI）：只按换行拆行，不做宽度折行，
		// 避免把已有 ANSI 序列当可见字符折断。
		lines = strings.Split(fmt.Sprintf("%s %s", icon, m.Content), "\n")
	}
	var out strings.Builder
	for i, line := range lines {
		out.WriteString(style.Render(line))
		if i < len(lines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
}

// 辅助函数

func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}

	var lines []string
	for _, src := range strings.Split(text, "\n") {
		if src == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapLine(src, maxWidth)...)
	}
	return lines
}

func bubbleWidth(width int) int {
	if width < 24 {
		return width
	}
	if width <= 100 {
		return width
	}
	if width <= 140 {
		if width-2 < 120 {
			return width - 2
		}
		return 120
	}
	if width-2 < 144 {
		return width - 2
	}
	return 144
}

func wrapLine(s string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{s}
	}
	if xansi.StringWidth(s) <= maxWidth {
		return []string{s}
	}

	type tok struct {
		s     string
		width int
		space bool
	}
	var toks []tok
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) {
					c := s[j]
					if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
						j++
						break
					}
					j++
				}
				toks = append(toks, tok{s: s[i:j], width: 0})
				i = j
				continue
			}
			toks = append(toks, tok{s: s[i : i+1], width: 0})
			i++
			continue
		}
		r, size := rune(s[i]), 1
		if r >= 0x80 {
			r, size = utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				size = 1
			}
		}
		w := runewidth.RuneWidth(r)
		space := r == ' ' || r == '\t'
		if r == '\t' {
			w = 4
		}
		toks = append(toks, tok{s: s[i : i+size], width: w, space: space})
		i += size
	}

	var out []string
	curW := 0
	lastSpace := -1
	widthAtSpace := 0
	start := 0

	emit := func(end int) {
		if end < start {
			end = start
		}
		partToks := toks[start:end]
		for len(partToks) > 0 && partToks[len(partToks)-1].space {
			partToks = partToks[:len(partToks)-1]
		}
		var b strings.Builder
		for _, t := range partToks {
			b.WriteString(t.s)
		}
		out = append(out, b.String())
		start = end
		for start < len(toks) && toks[start].space {
			start++
		}
		curW = 0
		lastSpace = -1
		widthAtSpace = 0
		for i := start; i < len(toks); i++ {
			if toks[i].space {
				lastSpace = i
				widthAtSpace = curW
			}
			curW += toks[i].width
			if curW >= maxWidth {
				break
			}
		}
	}

	curW = 0
	for i := 0; i < len(toks); i++ {
		curW += toks[i].width
		if toks[i].space {
			lastSpace = i
			widthAtSpace = curW - toks[i].width
		}
		if curW > maxWidth && i > start {
			if lastSpace >= start && widthAtSpace > 0 {
				emit(lastSpace)
				i = start - 1
				curW = 0
				continue
			}
			emit(i)
			i = start - 1
			curW = 0
			continue
		}
	}
	if start < len(toks) {
		var b strings.Builder
		for _, t := range toks[start:] {
			b.WriteString(t.s)
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}

func renderAgentEventDot(s *styles.Styles, event string, isMain bool, done bool) string {
	switch strings.ToLower(strings.TrimSpace(event)) {
	case "failed":
		return s.TextError.Render("●")
	case "cancelled":
		return s.TextWarning.Render("●")
	case "dispatch", "started", "progress", "update":
		return s.TextInfo.Render("●")
	default:
		if isMain && !done {
			return s.TextInfo.Render("●")
		}
		return s.TextSuccess.Render("●")
	}
}

func renderAgentEventLabel(s *styles.Styles, event string) string {
	event = strings.ToLower(strings.TrimSpace(event))
	if event == "" {
		event = "result"
	}
	return s.TextMuted.Render("[" + event + "]")
}

func renderAgentRoute(s *styles.Styles, sourceName string, agentName string) string {
	sourceName = strings.TrimSpace(sourceName)
	agentName = strings.TrimSpace(agentName)
	switch {
	case sourceName != "" && agentName != "":
		return s.MsgAgentHeader.Render(sourceName + " -> " + agentName)
	case agentName != "":
		return s.MsgAgentHeader.Render(agentName)
	default:
		return s.MsgAgentHeader.Render(sourceName)
	}
}

func renderAgentID(s *styles.Styles, agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	return s.TextMuted.Render(shortID(agentID))
}

func shortID(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= 28 {
		return string(runes)
	}
	return string(runes[:12]) + "..." + string(runes[len(runes)-8:])
}

func truncateInline(text string, limit int) string {
	clipped, truncated, total := truncateRunes(text, limit)
	if !truncated {
		return text
	}
	remaining := total - len([]rune(clipped))
	if remaining < 0 {
		remaining = 0
	}
	return clipped + fmt.Sprintf("...(+%d chars)", remaining)
}

func truncateBlockValue(text string, limit int) string {
	clipped, truncated, total := truncateRunes(text, limit)
	if !truncated {
		return text
	}
	shown := len([]rune(clipped))
	return clipped + fmt.Sprintf("\n[truncated: showing first %d of %d chars]", shown, total)
}

func truncateRunes(text string, limit int) (string, bool, int) {
	runes := []rune(text)
	total := len(runes)
	if limit <= 0 || total <= limit {
		return text, false, total
	}
	return string(runes[:limit]), true, total
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func renderProgressBar(progress int, width int, s *styles.Styles) string {
	if width < 3 {
		return ""
	}

	filled := (progress * (width - 2)) / 100
	if filled < 0 {
		filled = 0
	}
	if filled > width-2 {
		filled = width - 2
	}

	empty := (width - 2) - filled

	bar := s.TextSuccess.Render(strings.Repeat("█", filled)) +
		s.TextMuted.Render(strings.Repeat("░", empty))

	return fmt.Sprintf("[%s]", bar)
}
