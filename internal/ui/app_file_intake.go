package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/eosaios/eos/internal/i18n"
	"github.com/eosaios/eos/internal/ui/views/shell"
	"github.com/eosaios/eos/pkg/attachments"
	"github.com/eosaios/eos/pkg/coreapi"
)

func (m *AppModel) handleFilePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	if m.activeView == "shell" && m.shell != nil && m.shell.GetMode() == shell.ModeAI {
		text, paths := attachments.ExtractPaths(msg.Content)
		if len(paths) > 0 && strings.TrimSpace(text) == "" {
			m.queueAttachmentPaths(paths)
			return m, m.finalizeUpdate(nil)
		}
	}
	// Paste ordinary prose and code through the existing textarea path.
	if m.activeView == "shell" && m.shell != nil {
		updated, cmd := m.shell.Update(msg)
		*m.shell = updated
		m.syncPredictionState()
		return m, m.finalizeUpdate(cmd)
	}
	return m, m.finalizeUpdate(nil)
}

func (m *AppModel) queueAttachmentPaths(paths []string) {
	seen := make(map[string]struct{}, len(m.pendingAttachmentPaths))
	for _, path := range m.pendingAttachmentPaths {
		seen[path] = struct{}{}
	}
	for _, path := range paths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		m.pendingAttachmentPaths = append(m.pendingAttachmentPaths, path)
		m.appendSystem(fmt.Sprintf(i18n.T("attachment.added", m.state.Language), path), "info")
	}
	m.clearPrediction()
}

// Prepare before clearing the draft: failed reads keep both text and attachments.
func (m *AppModel) prepareFileAttachments(text string) (string, []coreapi.Attachment, error) {
	prompt, dropped := attachments.ExtractPaths(text)
	paths := append(append([]string(nil), m.pendingAttachmentPaths...), dropped...)
	encoded := make([]coreapi.Attachment, 0, len(paths))
	seen := map[string]struct{}{}
	for _, path := range paths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		file, err := attachments.Encode(attachments.File{Path: path})
		if err != nil {
			return text, nil, err
		}
		encoded = append(encoded, file)
	}
	return prompt, encoded, nil
}
