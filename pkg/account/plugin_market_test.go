package account

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPluginMarketBindsAuthoritativeIdentityAndCachedLocalSelection(t *testing.T) {
	m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/me":
			reply(w, map[string]string{"user_id": "alice"})
		case "/api/v1/plugin-market/sessions":
			if r.Header.Get("Authorization") != "Bearer private-access" {
				t.Error("missing account bearer")
			}
			reply(w, map[string]any{"token": "private-market-session", "subjectId": "alice", "applicationId": "eos-agent", "environmentId": "fixture", "expiresAt": time.Now().Add(5 * time.Minute).Unix()})
		default:
			t.Error("unexpected identity request")
			w.WriteHeader(404)
		}
	})
	if err := m.persist(session{Access: "private-access", Refresh: "private-refresh", ExpiresIn: 3600}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PluginMarketLocalAccess("eos-agent"); err == nil {
		t.Fatal("unverified cached identity must not select installations")
	}
	access, err := m.PluginMarketAccess(context.Background(), "eos-agent")
	if err != nil {
		t.Fatal(err)
	}
	if access.SubjectID != "alice" || access.ApplicationID != "eos-agent" || access.MarketToken != "private-market-session" || access.LocalOnly {
		t.Fatalf("bad live context: subject=%s app=%s local=%v", access.SubjectID, access.ApplicationID, access.LocalOnly)
	}
	if strings.Contains(store.value, "private-market-session") {
		t.Fatal("market bearer persisted")
	}
	m.SetHTTPTransport(roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline fixture") }))
	local, err := m.PluginMarketLocalAccess("eos-agent")
	if err != nil {
		t.Fatal(err)
	}
	if !local.LocalOnly || local.SubjectID != "alice" || local.AccountToken != "" || local.MarketToken != "" {
		t.Fatal("local selection received credentials or wrong identity")
	}
	if err := m.Logout(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PluginMarketLocalAccess("eos-agent"); err == nil {
		t.Fatal("logout retained local identity")
	}
}

func TestPluginMarketRejectsMismatchedSessionAndDistinguishesDenial(t *testing.T) {
	m, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/me" {
			reply(w, map[string]string{"user_id": "alice"})
			return
		}
		reply(w, map[string]string{"token": "secret", "subjectId": "bob", "applicationId": "eos-agent", "environmentId": "fixture"})
	})
	if err := m.persist(session{Access: "access", Refresh: "refresh", ExpiresIn: 3600}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PluginMarketAccess(context.Background(), "eos-agent"); err == nil || err.Error() != "PLUGIN_MARKET_IDENTITY_MISMATCH" {
		t.Fatal("accepted another subject")
	}
	if !PluginMarketAccessDenied(&apiError{Status: 403}) || PluginMarketAccessDenied(errNetwork) {
		t.Fatal("denial and outage conflated")
	}
}
