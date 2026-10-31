package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

type memoryStore struct {
	value string
	fail  error
}

func (s *memoryStore) Get() (string, error) { return s.value, s.fail }
func (s *memoryStore) Set(v string) error {
	if s.fail != nil {
		return s.fail
	}
	s.value = v
	return nil
}
func (s *memoryStore) Delete() error {
	if s.fail != nil {
		return s.fail
	}
	s.value = ""
	return nil
}
func fixture(t *testing.T, handler http.HandlerFunc) (*Manager, *memoryStore) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	return &Manager{client: c, name: "eos-cli", store: store}, store
}
func reply(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(map[string]any{"data": v}); err != nil {
		panic(err)
	}
}
func TestLoginRestoreLogoutWithoutCommerce(t *testing.T) {
	calls := []string{}
	m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/auth/desktop/flows":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(body, map[string]string{"client": "eos-cli"}) {
				t.Errorf("body=%v", body)
			}
			reply(w, map[string]any{"flow_id": "flow1", "login_url": "https://account.eosaios.com/?flow=flow1", "expires_in": 600})
		case "/api/v1/auth/desktop/flows/flow1":
			reply(w, map[string]string{"status": "completed", "ticket": "ticket1", "client": "eos-cli"})
		case "/api/v1/auth/desktop/ticket-exchange":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["flow_id"] != "flow1" || body["ticket"] != "ticket1" || body["device_name"] == "" {
				t.Errorf("exchange=%v", body)
			}
			reply(w, map[string]any{"access_token": "access1", "refresh_token": "refresh1", "access_token_expires_in": 3600})
		case "/api/v1/auth/me":
			if r.Header.Get("Authorization") != "Bearer access1" {
				t.Error("wrong auth")
			}
			reply(w, map[string]string{"user_id": "user1", "email": "fixture@example.com", "display_name": "Fixture"})
		default:
			t.Errorf("unexpected API %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	link, err := m.Start(ctx)
	if err != nil || link != "https://account.eosaios.com/?flow=flow1" {
		t.Fatalf("start %s %v", link, err)
	}
	done, err := m.Poll(ctx)
	if !done || err != nil {
		t.Fatalf("poll %v %v", done, err)
	}
	restored := &Manager{client: m.client, name: m.name, store: store}
	got, err := restored.Status(ctx)
	want := State{LoggedIn: true, UserID: "user1", Email: "fixture@example.com", DisplayName: "Fixture"}
	if err != nil || got != want {
		t.Fatalf("status=%+v %v", got, err)
	}
	if err = m.Logout(); err != nil {
		t.Fatal(err)
	}
	got, err = restored.Status(ctx)
	if err != nil || got != (State{}) {
		t.Fatalf("logout=%+v %v", got, err)
	}
	if _, err = m.Poll(ctx); err == nil {
		t.Fatal("late login restored session")
	}
	wantCalls := []string{"POST /api/v1/auth/desktop/flows", "GET /api/v1/auth/desktop/flows/flow1", "POST /api/v1/auth/desktop/ticket-exchange", "GET /api/v1/auth/me"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%v", calls)
	}
}
func TestRefreshRotationAndFailure(t *testing.T) {
	for _, mode := range []string{"success", "rejected", "network", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/auth/me" {
					if r.Header.Get("Authorization") != "Bearer new-access" {
						t.Error("stale access")
					}
					reply(w, map[string]string{"user_id": "u"})
					return
				}
				if r.URL.Path != "/api/v1/auth/refresh" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["refresh_token"] != "old-refresh" {
					t.Error("wrong refresh")
				}
				switch mode {
				case "success":
					reply(w, map[string]any{"access_token": "new-access", "refresh_token": "new-refresh", "access_token_expires_in": 3600})
				case "rejected":
					w.WriteHeader(401)
					w.Write([]byte(`{"error":{"code":"AUTH_INVALID_TOKEN"}}`))
				case "network":
					w.WriteHeader(503)
					w.Write([]byte(`{"error":{"code":"UNAVAILABLE"}}`))
				case "invalid":
					reply(w, map[string]any{"access_token": "new-access"})
				}
			})
			raw, _ := json.Marshal(session{Access: "old-access", Refresh: "old-refresh", ExpiresAt: time.Now().Add(-time.Hour)})
			store.value = string(raw)
			before := store.value
			state, err := m.Status(context.Background())
			if mode == "success" {
				if err != nil || state != (State{LoggedIn: true, UserID: "u"}) {
					t.Fatalf("state=%+v %v", state, err)
				}
				var s session
				json.Unmarshal([]byte(store.value), &s)
				if s.Refresh != "new-refresh" {
					t.Fatal("rotation not persisted")
				}
			} else if mode == "network" {
				if err != nil || state != (State{LoggedIn: true, ProfileError: "ACCOUNT_PROFILE_SYNC_FAILED"}) || store.value != before {
					t.Fatalf("transient refresh lost stored identity: %+v %v", state, err)
				}
			} else {
				if err == nil {
					t.Fatal("expected failure")
				}
				if mode == "rejected" {
					if store.value != "" {
						t.Fatal("rejected session retained")
					}
				} else if store.value != before {
					t.Fatal("transient failure erased credentials")
				}
			}
		})
	}
}
func TestPendingExpiredAndMismatchedFlow(t *testing.T) {
	for _, status := range []string{"pending", "expired", "consumed", "mismatch"} {
		t.Run(status, func(t *testing.T) {
			m, s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				client := "eos-cli"
				if status == "mismatch" {
					client = "eos-desktop"
				}
				reply(w, map[string]string{"status": status, "client": client})
			})
			m.flow = &flow{ID: "f", Deadline: time.Now().Add(time.Minute)}
			done, err := m.Poll(context.Background())
			if done || s.value != "" {
				t.Fatal("unexpected session")
			}
			if (err == nil) != (status == "pending") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
func TestKeychainFailureAndInvalidEndpoint(t *testing.T) {
	m, s := fixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("network called") })
	s.fail = errors.New("keychain locked")
	if _, err := m.Status(context.Background()); err == nil || err.Error() != "ACCOUNT_STORAGE_ERROR" {
		t.Fatal(err)
	}
	if err := m.persist(session{Access: "a", Refresh: "r", ExpiresIn: 60}); !errors.Is(err, s.fail) {
		t.Fatal(err)
	}
	for _, base := range []string{"http://remote.example.com", "https://user:pass@remote.example.com", "https://remote.example.com/path", "https://remote.example.com?token=1"} {
		if _, err := newClient(base); err == nil {
			t.Errorf("accepted %s", base)
		}
	}
}
