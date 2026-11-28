// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

package webbridge

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/eosaios/eos/pkg/coreapi"
)

const requestUserInputAnswerLevel = "user_input_answer"

type QuestionAnswer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func requestUserInputDisplayAnswers(prompt *promptState, decision, note string) ([]QuestionAnswer, error) {
	if prompt == nil || prompt.Source != "request-user-input" {
		return nil, nil
	}
	reason, err := requestUserInputReasonJSON(prompt, decision, note)
	if err != nil {
		return nil, err
	}
	var response coreapi.RequestUserInputResponse
	if err := json.Unmarshal([]byte(reason), &response); err != nil {
		return nil, err
	}
	if len(prompt.Questions) == 0 {
		answer := strings.TrimSpace(strings.Join(response.Answers[firstQuestionID(prompt)].Answers, "\n"))
		if answer == "" {
			return nil, nil
		}
		return []QuestionAnswer{{Question: prompt.Message, Answer: answer}}, nil
	}
	entries := make([]QuestionAnswer, 0, len(prompt.Questions))
	for _, question := range prompt.Questions {
		answer := strings.TrimSpace(strings.Join(response.Answers[question.ID].Answers, "\n"))
		if answer == "" {
			continue
		}
		label := strings.TrimSpace(question.Question)
		if label == "" {
			label = question.Header
		}
		entries = append(entries, QuestionAnswer{Question: label, Answer: answer})
	}
	return entries, nil
}

func (s *BridgeService) showRequestUserInputAnswerLocked(prompt *promptState, answers []QuestionAnswer) error {
	session := s.sessions[prompt.SessionID]
	if session == nil {
		return nil
	}
	message := findSessionMessageByID(session, prompt.AssistantMessageID)
	if message == nil {
		return nil
	}

	statusKey := promptStatusKey(prompt.ID)
	var text []string
	for _, answer := range answers {
		text = append(text, strings.TrimSpace(answer.Question+"\n"+answer.Answer))
	}
	item := ThreadItem{
		ID: statusKey, Kind: "status", Text: strings.Join(text, "\n\n"),
		Level: requestUserInputAnswerLevel, Status: "completed", QuestionAnswers: answers,
	}
	if index := findStatusItemIndex(message.Items, statusKey); index >= 0 {
		message.Items[index] = item
	} else {
		message.Items = append(message.Items, item)
	}
	session.UpdatedAt = time.Now()
	s.emitPromptStatusDeltaLocked(session, message.ID, statusKey)
	return s.persistSessionForTurnLocked(session, message.turnID)
}

func encodeQuestionAnswers(answers []QuestionAnswer) []any {
	encoded := make([]any, 0, len(answers))
	for _, entry := range answers {
		encoded = append(encoded, map[string]any{"question": entry.Question, "answer": entry.Answer})
	}
	return encoded
}

func decodeQuestionAnswers(raw any) []QuestionAnswer {
	entries, ok := raw.([]any)
	if !ok {
		return nil
	}
	answers := make([]QuestionAnswer, 0, len(entries))
	for _, entry := range entries {
		value, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		question, _ := value["question"].(string)
		answer, _ := value["answer"].(string)
		if question != "" || answer != "" {
			answers = append(answers, QuestionAnswer{Question: question, Answer: answer})
		}
	}
	return answers
}

// Structured questions stay pending until Core accepts the response. The same
// lock serializes duplicate clicks and resolution events from the turn stream.
func (s *BridgeService) resolveStructuredQuestionLocked(prompt *promptState, decision, note string, answers []QuestionAnswer) error {
	if prompt.Background {
		return s.resolveAsyncQuestionLocked(prompt, note)
	}
	if err := s.respondPromptRPC(prompt, decision, note); err != nil {
		return err
	}
	text, level := s.resolvedStatusTextAndLevel(prompt, decision)
	session := s.settlePromptLocked(nil, prompt, text, level, "streaming")
	if session == nil {
		return nil
	}
	if err := s.showRequestUserInputAnswerLocked(prompt, answers); err != nil {
		s.pushNotificationLocked(s.t("notification.request_failed.title"), err.Error(), "danger")
	}
	return nil
}
