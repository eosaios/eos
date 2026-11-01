package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestEstablishedSessionSurvivesProfileNetworkFailure(t *testing.T) {
	for _, fault := range []string{"network", "server"} {
		m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503); reply(w, nil) })
		if err := m.persist(session{Access: "private-access", Refresh: "private-refresh", ExpiresIn: 3600, Email: "fixture@example.com"}); err != nil {
			t.Fatal(err)
		}
		if fault == "network" {
			m.SetHTTPTransport(roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("fixture offline") }))
		}
		before := store.value
		state, err := m.Status(context.Background())
		want := State{LoggedIn: true, Email: "fixture@example.com", ProfileError: "ACCOUNT_PROFILE_SYNC_FAILED"}
		if err != nil || state != want || store.value != before {
			t.Fatalf("lost established session: %+v %v", state, err)
		}
	}
}

func TestRelaunchRetainsExpiredIdentityDuringTransientRefreshFailure(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "profile-401", true: "expired-before-relaunch"}[expired], func(t *testing.T) {
			offline := true
			m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/auth/refresh" {
					if offline {
						w.WriteHeader(503)
						reply(w, nil)
						return
					}
					reply(w, map[string]any{"access_token": "rotated", "refresh_token": "rotated-refresh", "access_token_expires_in": 3600})
					return
				}
				if r.Header.Get("Authorization") != "Bearer rotated" {
					w.WriteHeader(401)
					reply(w, nil)
					return
				}
				reply(w, map[string]string{"user_id": "user", "email": "fixture@example.com", "display_name": "Fixture"})
			})
			if err := m.persist(session{Access: "old", Refresh: "refresh", ExpiresIn: 3600, Email: "fixture@example.com"}); err != nil {
				t.Fatal(err)
			}
			if expired {
				var s session
				if err := json.Unmarshal([]byte(store.value), &s); err != nil {
					t.Fatal(err)
				}
				s.ExpiresAt = time.Now().Add(-time.Hour)
				raw, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				store.value = string(raw)
			}
			before := store.value
			restored := &Manager{client: m.client, name: m.name, store: store}
			state, err := restored.Status(context.Background())
			want := State{LoggedIn: true, Email: "fixture@example.com", ProfileError: "ACCOUNT_PROFILE_SYNC_FAILED"}
			if err != nil || state != want || store.value != before {
				t.Fatalf("offline relaunch: %+v %v", state, err)
			}
			offline = false
			state, err = restored.Status(context.Background())
			if err != nil || state != (State{LoggedIn: true, UserID: "user", Email: "fixture@example.com", DisplayName: "Fixture"}) || store.value == before {
				t.Fatalf("reconnected: %+v %v", state, err)
			}
		})
	}
}

func TestInvalidSessionIsNotPresentedAsProfileSyncFailure(t *testing.T) {
	m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if _, err := w.Write([]byte(`{"error":{"code":"AUTH_INVALID_TOKEN"}}`)); err != nil {
			t.Error(err)
		}
	})
	if err := m.persist(session{Access: "revoked", Refresh: "revoked-refresh", ExpiresIn: 3600}); err != nil {
		t.Fatal(err)
	}
	state, err := m.Status(context.Background())
	if err == nil || state.LoggedIn || store.value != "" {
		t.Fatalf("revoked session retained: %+v %v", state, err)
	}
}
