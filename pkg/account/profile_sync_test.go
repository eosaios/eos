package account

import (
	"context"
	"errors"
	"net/http"
	"testing"
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
