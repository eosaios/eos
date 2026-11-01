// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

package account

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestSlowPollDefersOneUseExchangeToFreshRequest(t *testing.T) {
	gets, exchanges := 0, 0
	m, store := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/desktop/ticket-exchange" {
			exchanges++
			time.Sleep(80 * time.Millisecond)
			reply(w, map[string]any{"access_token": "access", "refresh_token": "refresh", "access_token_expires_in": 3600})
			return
		}
		gets++
		time.Sleep(120 * time.Millisecond)
		reply(w, map[string]string{"status": "completed", "ticket": "one-use", "client": "eos-cli"})
	})
	m.client.http.Timeout = 180 * time.Millisecond
	m.flow = &flow{ID: "slow-flow", Deadline: time.Now().Add(time.Minute)}
	ctx, cancel := context.WithTimeout(context.Background(), 190*time.Millisecond)
	defer cancel()
	if done, err := m.Poll(ctx); done || err != nil || exchanges != 0 || store.value != "" || m.flow == nil {
		t.Fatalf("premature exchange: done=%v error=%v exchanges=%d", done, err, exchanges)
	}
	m.flow.nextPoll = time.Time{}
	short, finish := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer finish()
	if done, err := m.Poll(short); done || err != nil || exchanges != 0 || m.flow == nil {
		t.Fatalf("short queued request consumed ticket: %v %v", done, err)
	}
	fresh, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if done, err := m.Poll(fresh); !done || err != nil || gets != 1 || exchanges != 1 || store.value == "" {
		t.Fatalf("fresh exchange: done=%v error=%v gets=%d exchanges=%d", done, err, gets, exchanges)
	}
	if _, err := m.Poll(fresh); err == nil || exchanges != 1 {
		t.Fatal("completed ticket replayed")
	}
}
