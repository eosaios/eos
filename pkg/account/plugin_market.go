package account

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eosaios/eos/pkg/pluginprotocol"
	"time"
)

// PluginMarketAccess is transport-only. It must never be exposed to frontend/plugin contexts or persisted.
type PluginMarketAccess = pluginprotocol.MarketAccess

func (m *Manager) PluginMarketAccess(ctx context.Context, applicationID string) (PluginMarketAccess, error) {
	state, err := m.Status(ctx)
	if err != nil {
		return PluginMarketAccess{}, err
	}
	if state.ProfileError != "" {
		return PluginMarketAccess{}, errors.New(state.ProfileError)
	}
	if !state.LoggedIn || state.UserID == "" {
		return PluginMarketAccess{}, errors.New("PLUGIN_MARKET_SIGN_IN_REQUIRED")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, err := m.store.Get()
	if err != nil {
		return PluginMarketAccess{}, errors.New("ACCOUNT_STORAGE_ERROR")
	}
	var current session
	if err := json.Unmarshal([]byte(raw), &current); err != nil || current.Access == "" {
		return PluginMarketAccess{}, errors.New("ACCOUNT_SESSION_INVALID")
	}
	var scoped pluginprotocol.MarketSession
	if err := m.client.call(ctx, "POST", "/api/v1/plugin-market/sessions", map[string]string{"applicationId": applicationID}, current.Access, &scoped); err != nil {
		return PluginMarketAccess{}, err
	}
	if scoped.SubjectID != state.UserID || scoped.ApplicationID != applicationID || scoped.Token == "" || scoped.EnvironmentID == "" || scoped.ExpiresAt <= uint64(time.Now().Unix()) {
		return PluginMarketAccess{}, errors.New("PLUGIN_MARKET_IDENTITY_MISMATCH")
	}
	if current.PluginMarkets == nil {
		current.PluginMarkets = make(map[string]pluginMarketIdentity)
	}
	current.PluginMarkets[applicationID] = pluginMarketIdentity{Origin: m.client.base, SubjectID: state.UserID, EnvironmentID: scoped.EnvironmentID}
	encoded, err := json.Marshal(current)
	if err != nil {
		return PluginMarketAccess{}, err
	}
	if err := m.store.Set(string(encoded)); err != nil {
		return PluginMarketAccess{}, errors.New("ACCOUNT_STORAGE_ERROR")
	}
	return PluginMarketAccess{Origin: m.client.base, SubjectID: state.UserID, AccountToken: current.Access, MarketToken: scoped.Token, ApplicationID: applicationID, EnvironmentID: scoped.EnvironmentID}, nil
}

// PluginMarketAccessDenied distinguishes an authoritative denial from an outage.
func PluginMarketAccessDenied(err error) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403)
}

// This cached identity selects existing local records; Core alone validates their signed leases.
// It cannot create sessions, authorize downloads or install/enable plugins.
type pluginMarketIdentity struct {
	Origin        string `json:"origin"`
	SubjectID     string `json:"subjectId"`
	EnvironmentID string `json:"environmentId"`
}

func (m *Manager) PluginMarketLocalAccess(applicationID string) (PluginMarketAccess, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, err := m.store.Get()
	if err != nil {
		return PluginMarketAccess{}, errors.New("ACCOUNT_STORAGE_ERROR")
	}
	var current session
	if raw == "" || json.Unmarshal([]byte(raw), &current) != nil || current.Access == "" || current.Refresh == "" {
		return PluginMarketAccess{}, errors.New("PLUGIN_MARKET_SIGN_IN_REQUIRED")
	}
	identity, exists := current.PluginMarkets[applicationID]
	if !exists || identity.Origin != m.client.base || identity.SubjectID == "" || identity.EnvironmentID == "" {
		return PluginMarketAccess{}, errors.New("PLUGIN_MARKET_ONLINE_SYNC_REQUIRED")
	}
	return PluginMarketAccess{Origin: identity.Origin, SubjectID: identity.SubjectID, EnvironmentID: identity.EnvironmentID, ApplicationID: applicationID, LocalOnly: true}, nil
}
