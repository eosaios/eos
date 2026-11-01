// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

package account

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestFlowSurvivesNonJSONGatewayError(t *testing.T) {
	for _, status := range []int{429, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					w.WriteHeader(status)
					_, err := w.Write([]byte("temporary gateway failure"))
					if err != nil {
						t.Error(err)
					}
					return
				}
				if r.URL.Path == "/api/v1/auth/desktop/ticket-exchange" {
					reply(w, map[string]any{"access_token": "access", "refresh_token": "refresh", "access_token_expires_in": 3600})
					return
				}
				reply(w, map[string]string{"status": "completed", "ticket": "ticket", "client": "eos-cli"})
			})
			m.flow = &flow{ID: "flow", Deadline: time.Now().Add(time.Minute)}
			if done, err := m.Poll(context.Background()); done || err != nil || m.flow == nil || store.value != "" {
				t.Fatalf("gateway failure terminated login: %v %v", done, err)
			}
			m.flow.nextPoll = time.Time{}
			if done, err := m.Poll(context.Background()); !done || err != nil || store.value == "" || calls != 3 {
				t.Fatalf("recovery: %v %v calls=%d", done, err, calls)
			}
		})
	}
}
