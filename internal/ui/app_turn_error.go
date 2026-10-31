package ui

import "strings"

// The request RPC and runtime event stream both report a failed turn. Render
// that outcome once; a subsequent user entry starts a new, independently visible turn.
func (m *AppModel) appendTurnError(message string) {
	text := strings.TrimSpace(message)
	if strings.Contains(text, "401") || strings.Contains(strings.ToLower(text), "invalid api key") {
		text += "\n" + m.localize("请用 /model 切换模型，或打开 /config 检查服务商地址和 API Key。", "Use /model to switch models, or open /config to check the provider URL and API key.")
	}
	if len(m.history) > 0 {
		last := m.history[len(m.history)-1]
		if last.kind == "system" && last.level == "error" && last.content == text {
			return
		}
	}
	m.appendHistory(historyEntry{kind: "system", content: text, level: "error"})
}
