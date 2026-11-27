package account

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type AgentAppearance struct {
	Language  string `json:"language"`
	Theme     string `json:"theme"`
	DiffTheme string `json:"diffTheme"`
}

type AgentAppearanceSnapshot struct {
	Scope string
	Value AgentAppearance
}

func (m *Manager) agentSession() (*session, error) {
	raw, err := m.store.Get()
	if err != nil || raw == "" {
		return nil, err
	}
	var current session
	if err := json.Unmarshal([]byte(raw), &current); err != nil || current.Access == "" || current.Refresh == "" {
		return nil, errors.New("ACCOUNT_SESSION_INVALID")
	}
	return &current, nil
}

func appearanceSnapshot(current *session) AgentAppearanceSnapshot {
	if current == nil {
		return AgentAppearanceSnapshot{}
	}
	value := AgentAppearance{Language: "zh", Theme: "system", DiffTheme: "github"}
	if current.AgentAppearance != nil {
		value = *current.AgentAppearance
	}
	identity := current.DeviceID
	if identity == "" {
		identity = strings.ToLower(strings.TrimSpace(current.Email))
	}
	if identity == "" {
		identity = current.Refresh
	}
	id := sha256.Sum256([]byte(identity))
	return AgentAppearanceSnapshot{Scope: fmt.Sprintf("%x", id), Value: value}
}

func (m *Manager) CachedAgentAppearance() (AgentAppearanceSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.agentSession()
	return appearanceSnapshot(current), err
}

// SyncAgentAppearance refreshes or patches the active account's appearance.
func (m *Manager) SyncAgentAppearance(ctx context.Context, expectedScope string, update *AgentAppearance) (AgentAppearanceSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.agentSession()
	if err != nil {
		return AgentAppearanceSnapshot{}, err
	}
	snapshot := appearanceSnapshot(current)
	if update != nil && snapshot.Scope != expectedScope {
		return snapshot, errors.New("ACCOUNT_PREFERENCE_SCOPE_CHANGED")
	}
	if current == nil {
		return snapshot, nil
	}
	method := http.MethodGet
	var body any
	if update != nil {
		patch := make(map[string]string)
		if update.Language != snapshot.Value.Language {
			patch["language"] = update.Language
		}
		if update.Theme != snapshot.Value.Theme {
			patch["theme"] = update.Theme
		}
		if update.DiffTheme != snapshot.Value.DiffTheme {
			patch["diffTheme"] = update.DiffTheme
		}
		if len(patch) == 0 {
			return snapshot, nil
		}
		method, body = http.MethodPost, patch
	}
	var result AgentAppearance
	const path = "/api/v1/products/agent/preferences"
	if time.Now().Add(30 * time.Second).After(current.ExpiresAt) {
		if err := m.refresh(ctx, current); err != nil {
			return snapshot, err
		}
	}
	err = m.client.call(ctx, method, path, body, current.Access, &result)
	var rejected *apiError
	if errors.As(err, &rejected) && rejected.Status == 401 {
		if err := m.refresh(ctx, current); err != nil {
			return snapshot, err
		}
		err = m.client.call(ctx, method, path, body, current.Access, &result)
	}
	if err != nil {
		return snapshot, err
	}
	if result.Language == "" || result.Theme == "" || result.DiffTheme == "" {
		return snapshot, errors.New("ACCOUNT_PROTOCOL_ERROR")
	}
	current.AgentAppearance = &result
	encoded, err := json.Marshal(current)
	if err != nil {
		return snapshot, err
	}
	if err := m.store.Set(string(encoded)); err != nil {
		return snapshot, err
	}
	return appearanceSnapshot(current), nil
}
