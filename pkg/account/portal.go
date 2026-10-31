package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"
)

// AccountCenterURL follows Todo's one-use portal handoff. Long-lived tokens
// remain in this process; the caller must open the URL without logging it.
func (m *Manager) AccountCenterURL(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, err := m.store.Get()
	if err != nil {
		return "", errors.New("ACCOUNT_STORAGE_ERROR")
	}
	var sess session
	if raw == "" || json.Unmarshal([]byte(raw), &sess) != nil || sess.Access == "" || sess.Refresh == "" {
		return "", errors.New("ACCOUNT_SIGN_IN_REQUIRED")
	}
	if time.Now().Add(30 * time.Second).After(sess.ExpiresAt) {
		if err := m.refresh(ctx, &sess); err != nil {
			return "", err
		}
	}
	var profile struct {
		PortalURL string `json:"portal_url"`
	}
	err = m.client.call(ctx, "GET", "/api/v1/auth/me", nil, sess.Access, &profile)
	var rejected *apiError
	if errors.As(err, &rejected) && rejected.Status == 401 {
		if err = m.refresh(ctx, &sess); err != nil {
			return "", err
		}
		err = m.client.call(ctx, "GET", "/api/v1/auth/me", nil, sess.Access, &profile)
	}
	if err != nil {
		return "", err
	}
	u, err := url.Parse(profile.PortalURL)
	base, _ := url.Parse(m.client.base)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host == "" {
		return "", errors.New("ACCOUNT_ENDPOINT_INVALID")
	}
	if m.client.base == ProductionURL {
		if u.Scheme != "https" || (u.Host != "account.eosaios.com" && u.Host != "eosaios.com") {
			return "", errors.New("ACCOUNT_ENDPOINT_INVALID")
		}
	} else if u.Scheme != base.Scheme || u.Host != base.Host {
		return "", errors.New("ACCOUNT_ENDPOINT_INVALID")
	}
	var flow struct {
		ID string `json:"flow_id"`
	}
	if err := m.client.call(ctx, "POST", "/api/v1/auth/desktop/flows", map[string]string{"client": "web-portal"}, "", &flow); err != nil {
		return "", err
	}
	if flow.ID == "" {
		return "", errors.New("ACCOUNT_FLOW_INVALID")
	}
	var authorized struct {
		Ticket string `json:"ticket"`
	}
	if err := m.client.call(ctx, "POST", "/api/v1/auth/desktop/flows/"+url.PathEscape(flow.ID)+"/authorize", nil, sess.Access, &authorized); err != nil {
		return "", err
	}
	if authorized.Ticket == "" {
		return "", errors.New("ACCOUNT_FLOW_INVALID")
	}
	u.Path = "/desktop-session"
	u.RawPath = ""
	u.Fragment = url.Values{"portal_flow": {flow.ID}, "portal_ticket": {authorized.Ticket}, "portal_target": {"/"}}.Encode()
	return u.String(), nil
}
