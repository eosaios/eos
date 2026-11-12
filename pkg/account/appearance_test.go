package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentAppearanceFollowsStoredSession(t *testing.T) {
	requests := make(chan map[string]string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/products/agent/preferences" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("incorrect preference account request")
		}
		theme := "system"
		if r.Method == http.MethodPost {
			var patch map[string]string
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Error(err)
			}
			requests <- patch
			theme = "dark"
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": AgentAppearance{Language: "zh", Theme: theme, DiffTheme: "github"}}); err != nil {
			t.Error(err)
		}
	}))
	client, err := newClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	store := &appearanceMemoryStore{}
	m := &Manager{client: client, store: store}
	if err := m.persist(session{Access: "fixture", Refresh: "refresh-fixture", ExpiresIn: 3600, Email: "a@example.test"}); err != nil {
		t.Fatal(err)
	}
	initial, err := m.SyncAgentAppearance(context.Background(), "", nil)
	if err != nil || initial.Scope == "" || initial.Value != (AgentAppearance{Language: "zh", Theme: "system", DiffTheme: "github"}) {
		t.Fatalf("initial preference: %+v %v", initial, err)
	}
	changed := initial.Value
	changed.Theme = "dark"
	saved, err := m.SyncAgentAppearance(context.Background(), initial.Scope, &changed)
	if err != nil || saved.Value != changed {
		t.Fatalf("saved preference: %+v %v", saved, err)
	}
	if patch := <-requests; len(patch) != 1 || patch["theme"] != "dark" {
		t.Fatalf("unexpected preference patch: %+v", patch)
	}
	server.Close()
	if cached, err := m.CachedAgentAppearance(); err != nil || cached != saved {
		t.Fatalf("offline preference: %+v %v", cached, err)
	}
	if err := m.Logout(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SyncAgentAppearance(context.Background(), initial.Scope, &changed); err == nil {
		t.Fatal("late preference save accepted after logout")
	}
	if err := m.persist(session{Access: "fixture", Refresh: "refresh-fixture", ExpiresIn: 3600, Email: "b@example.test"}); err != nil {
		t.Fatal(err)
	}
	if cached, err := m.CachedAgentAppearance(); err != nil || cached.Scope == initial.Scope || cached.Value != initial.Value {
		t.Fatalf("other account preference: %+v %v", cached, err)
	}
}

type appearanceMemoryStore struct{ value string }

func (m *appearanceMemoryStore) Get() (string, error)   { return m.value, nil }
func (m *appearanceMemoryStore) Set(value string) error { m.value = value; return nil }
func (m *appearanceMemoryStore) Delete() error          { m.value = ""; return nil }
