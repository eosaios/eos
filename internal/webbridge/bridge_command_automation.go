package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"errors"
	"time"
)

func (svc *CommandService) RunAutomationTemplate(templateID string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	template, ok := s.automationTemplateByIDReadOnly(templateID)
	if !ok {
		return s.LoadBootstrap(), errors.New("automation template does not exist")
	}
	now := time.Now().Format(time.RFC3339)
	run := AutomationRunCard{
		ID:         newID("automation"),
		TemplateID: template.ID,
		Title:      template.Title,
		Status:     "queued",
		Detail:     "Creating chat request",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.stateMu.Lock()
	s.automationRuns = append([]AutomationRunCard{run}, s.automationRuns...)
	if len(s.automationRuns) > 20 {
		s.automationRuns = append([]AutomationRunCard(nil), s.automationRuns[:20]...)
	}
	s.emitShellUpdated()
	s.stateMu.Unlock()

	state, err := s.chatService().SendChatWithReasoning("", "", template.Prompt, nil, s.reasoningLevelReadOnly())
	updatedAt := time.Now().Format(time.RFC3339)
	s.stateMu.Lock()
	for index := range s.automationRuns {
		if s.automationRuns[index].ID != run.ID {
			continue
		}
		s.automationRuns[index].UpdatedAt = updatedAt
		if err != nil {
			s.automationRuns[index].Status = "failed"
			s.automationRuns[index].Detail = err.Error()
		} else {
			s.automationRuns[index].Status = "running"
			s.automationRuns[index].Detail = "Sent to chat and continuing in the current session"
			s.automationRuns[index].SessionID = state.CurrentSessionID
		}
		break
	}
	s.emitShellUpdated()
	s.stateMu.Unlock()
	if err != nil {
		return s.LoadBootstrap(), err
	}
	return s.LoadBootstrap(), nil
}
