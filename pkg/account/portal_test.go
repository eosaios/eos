package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPortalUsesOneUseFlowWithoutExposingSessionTokens(t *testing.T) {
	var m *Manager
	var authenticated int
	m, _ = fixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/me":
			if r.Header.Get("Authorization") != "Bearer private-access" {
				t.Error("missing profile auth")
			}
			authenticated++
			reply(w, map[string]string{"portal_url": m.client.base})
		case "/api/v1/auth/desktop/flows":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["client"] != "web-portal" || r.Header.Get("Authorization") != "" {
				t.Error("wrong portal flow")
			}
			reply(w, map[string]string{"flow_id": "fixture-portal"})
		case "/api/v1/auth/desktop/flows/fixture-portal/authorize":
			if r.Header.Get("Authorization") != "Bearer private-access" {
				t.Error("missing authorize auth")
			}
			authenticated++
			reply(w, map[string]string{"ticket": "one-use-ticket"})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	})
	if err := m.persist(session{Access: "private-access", Refresh: "private-refresh", ExpiresIn: 3600}); err != nil {
		t.Fatal(err)
	}
	target, err := m.AccountCenterURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	fragment, _ := url.ParseQuery(u.Fragment)
	if u.Path != "/desktop-session" || u.RawQuery != "" || fragment.Get("portal_flow") != "fixture-portal" || fragment.Get("portal_ticket") != "one-use-ticket" || fragment.Get("portal_target") != "/" || authenticated != 2 {
		t.Fatal("invalid handoff")
	}
	if strings.Contains(target, "private-") {
		t.Fatal("long-lived credential leaked into URL")
	}
}

func TestPortalRejectsForeignDestinationBeforeAuthorizing(t *testing.T) {
	calls := 0
	m, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		reply(w, map[string]string{"portal_url": "https://foreign.invalid"})
	})
	if err := m.persist(session{Access: "a", Refresh: "r", ExpiresIn: 3600}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AccountCenterURL(context.Background()); err == nil || calls != 1 {
		t.Fatal("foreign portal was authorized")
	}
}
