package account

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFlowSurvivesRateLimitAndNetworkFailure(t *testing.T) {
	for _, fault := range []string{"rate-limit", "network"} {
		t.Run(fault, func(t *testing.T) {
			polls, exchanges := 0, 0
			m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/auth/desktop/ticket-exchange" {
					exchanges++
					reply(w, map[string]any{"access_token": "access", "refresh_token": "refresh", "access_token_expires_in": 3600})
					return
				}
				polls++
				if fault == "rate-limit" && polls <= 3 {
					w.WriteHeader(http.StatusTooManyRequests)
					reply(w, nil)
					return
				}
				reply(w, map[string]string{"status": "completed", "ticket": "ticket", "client": "eos-cli"})
			})
			m.flow = &flow{ID: "fixture", Deadline: time.Now().Add(time.Minute)}
			if fault == "network" {
				m.SetHTTPTransport(roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private network detail") }))
			}
			ctx := context.Background()
			strikes := 1
			if fault == "rate-limit" {
				strikes = 3
			}
			for i := 0; i < strikes; i++ {
				before := time.Now()
				done, err := m.Poll(ctx)
				if done || err != nil || store.value != "" || m.flow == nil {
					t.Fatalf("abandoned flow: %v %v", done, err)
				}
				want := 2 * time.Second
				if fault == "rate-limit" {
					want = time.Duration(1<<min(i+1, 3)) * time.Second
				}
				if m.flow.nextPoll.Before(before.Add(want)) {
					t.Fatal("retry window not respected")
				}
				calls := polls
				if done, err = m.Poll(ctx); done || err != nil || polls != calls {
					t.Fatal("polled within retry window")
				}
				m.flow.nextPoll = time.Time{}
			}
			m.SetHTTPTransport(http.DefaultTransport)
			if done, err := m.Poll(ctx); !done || err != nil || exchanges != 1 || store.value == "" {
				t.Fatalf("completion %v %v exchanges=%d", done, err, exchanges)
			}
			if _, err := m.Poll(ctx); err == nil || exchanges != 1 {
				t.Fatal("replayed completed exchange")
			}
		})
	}
}

func TestRetryExpiresAndRejectsHardErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusBadRequest, http.StatusTooManyRequests} {
		m, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); reply(w, nil) })
		m.flow = &flow{ID: "fixture", Deadline: time.Now().Add(time.Minute)}
		_, err := m.Poll(context.Background())
		if status == http.StatusTooManyRequests {
			if err != nil {
				t.Fatal(err)
			}
			m.flow.Deadline = time.Now().Add(-time.Second)
			_, err = m.Poll(context.Background())
		}
		if err == nil || m.flow != nil {
			t.Fatalf("status %d did not terminate", status)
		}
	}
}

func TestExchangeStorageFailureDoesNotReplayOrExposeCredentials(t *testing.T) {
	exchanges := 0
	m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/desktop/ticket-exchange" {
			exchanges++
			reply(w, map[string]any{"access_token": "private-access", "refresh_token": "private-refresh", "access_token_expires_in": 3600})
			return
		}
		reply(w, map[string]string{"status": "completed", "ticket": "private-ticket", "client": "eos-cli"})
	})
	store.fail = errors.New("storage rejected private details")
	m.flow = &flow{ID: "fixture", Deadline: time.Now().Add(time.Minute)}
	done, err := m.Poll(context.Background())
	if done || err == nil || err.Error() != "ACCOUNT_STORAGE_ERROR" || store.value != "" {
		t.Fatalf("storage outcome %v %v", done, err)
	}
	if _, err = m.Poll(context.Background()); err == nil || exchanges != 1 {
		t.Fatal("exchange replayed after storage failure")
	}
}
