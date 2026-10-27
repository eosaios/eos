package webbridge

import (
	"strings"
	"time"
)

const maxMessageRuntimeEvents = 200

func (s *BridgeService) applyStructuredResultToMessageLocked(session *sessionState, messageID string, payload map[string]any) {
	if session == nil || len(payload) == 0 {
		return
	}
	for index := range session.Messages {
		if session.Messages[index].ID != messageID {
			continue
		}
		implementation, verification, verdict, summary, covered, risks, evidence := structuredResultsFromPayload(payload)
		if implementation != "" {
			session.Messages[index].ImplementationResult = implementation
		}
		if verification != "" {
			session.Messages[index].VerificationResult = verification
		}
		if verdict != "" {
			session.Messages[index].VerificationVerdict = verdict
		}
		if summary != "" {
			session.Messages[index].VerificationSummary = summary
		}
		if len(covered) > 0 {
			session.Messages[index].VerificationCovered = covered
		}
		if len(risks) > 0 {
			session.Messages[index].VerificationOpenRisks = risks
		}
		if len(evidence) > 0 {
			session.Messages[index].VerificationEvidence = evidence
		}
		session.Messages[index].RuntimeSummary = runtimeSummaryForMessage(session.Messages[index])
		return
	}
}

func (s *BridgeService) appendRuntimeEventLocked(session *sessionState, messageID, eventType, title, detail, status string) {
	if session == nil {
		return
	}
	for index := range session.Messages {
		if session.Messages[index].ID != messageID {
			continue
		}
		event := newRuntimeEvent(session.Messages[index], eventType, title, detail, status)
		appendRuntimeEventToMessage(&session.Messages[index], event)
		return
	}
}

func appendRuntimeEventToMessage(message *ChatMessage, event RuntimeEvent) {
	if message == nil {
		return
	}
	if strings.TrimSpace(event.ID) == "" {
		event.ID = newID("runtime")
	}
	if strings.TrimSpace(event.Timestamp) == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}
	if strings.TrimSpace(event.Status) == "" {
		event.Status = "running"
	}
	message.RuntimeEvents = append(message.RuntimeEvents, event)
	if len(message.RuntimeEvents) > maxMessageRuntimeEvents {
		message.RuntimeEvents = append([]RuntimeEvent(nil), message.RuntimeEvents[len(message.RuntimeEvents)-maxMessageRuntimeEvents:]...)
	}
	message.RuntimeEvents = nonNilSlice(message.RuntimeEvents)
	message.UpdatedAt = event.Timestamp
	message.RuntimeSummary = runtimeSummaryForMessage(*message)
}

func newRuntimeEvent(message ChatMessage, eventType, title, detail, status string) RuntimeEvent {
	now := time.Now()
	createdAt := parseTime(message.CreatedAt)
	duration := int64(0)
	if !createdAt.IsZero() {
		duration = max(now.Sub(createdAt).Milliseconds(), 0)
	}
	return RuntimeEvent{
		ID:         newID("runtime"),
		Type:       fallbackText(strings.TrimSpace(eventType), "progress"),
		Title:      fallbackText(strings.TrimSpace(title), "姝ｅ湪杩愯"),
		Detail:     strings.TrimSpace(detail),
		Status:     fallbackText(strings.TrimSpace(status), "running"),
		Timestamp:  now.Format(time.RFC3339),
		DurationMS: duration,
	}
}

func structuredResultsFromPayload(payload map[string]any) (implementation string, verification string, verdict string, summary string, covered []string, risks []string, evidence []string) {
	if len(payload) == 0 {
		return "", "", "", "", nil, nil, nil
	}
	implementation = runtimeNestedStringValue(payload, "implementation_result")
	verification = runtimeNestedStringValue(payload, "verification_result")
	verdict = strings.ToUpper(strings.TrimSpace(runtimeNestedStringValue(payload, "verification_verdict")))
	summary = runtimeNestedStringValue(payload, "verification_summary")
	covered = runtimeNestedStringSliceValue(payload, "verification_covered_checks")
	risks = runtimeNestedStringSliceValue(payload, "verification_open_risks")
	evidence = runtimeNestedStringSliceValue(payload, "verification_evidence")
	return strings.TrimSpace(implementation), strings.TrimSpace(verification), verdict, strings.TrimSpace(summary), compactStrings(covered), compactStrings(risks), compactStrings(evidence)
}

func runtimeTimeoutMessage(session *sessionState, messageID string) string {
	lastTitle := lastRuntimeStepTitle(session, messageID)
	if lastTitle == "" {
		return "璇锋眰瓒呮椂"
	}
	return "璇锋眰瓒呮椂\n鏈€杩戜竴姝ワ細" + lastTitle
}

func runtimeClosedStreamMessage(session *sessionState, messageID string) string {
	lastTitle := lastRuntimeStepTitle(session, messageID)
	if lastTitle == "" {
		return requestFailureMessage("")
	}
	return requestFailureMessage("娴佸紡鍝嶅簲寮傚父缁撴潫\n鏈€杩戜竴姝ワ細" + lastTitle)
}

func lastRuntimeEventTitle(session *sessionState, messageID string) string {
	item := findSessionMessageByID(session, messageID)
	if item == nil || len(item.RuntimeEvents) == 0 {
		return ""
	}
	for index := len(item.RuntimeEvents) - 1; index >= 0; index-- {
		title := strings.TrimSpace(item.RuntimeEvents[index].Title)
		if title != "" {
			return title
		}
	}
	return ""
}

func lastRuntimeStepTitle(session *sessionState, messageID string) string {
	item := findSessionMessageByID(session, messageID)
	if item == nil || len(item.RuntimeEvents) == 0 {
		return ""
	}
	for index := len(item.RuntimeEvents) - 1; index >= 0; index-- {
		event := item.RuntimeEvents[index]
		if strings.EqualFold(strings.TrimSpace(event.Status), "failed") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(event.Type), "error") {
			continue
		}
		title := strings.TrimSpace(event.Title)
		if title != "" {
			return title
		}
	}
	return ""
}
