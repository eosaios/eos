package identitypilot

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

func TestCheckWithTLSProviderAndNativeCallback(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		wrongNonce bool
		want       error
	}{
		{name: "original account from resource"},
		{name: "nonce mismatch prevents account lookup", wrongNonce: true, want: ErrProvider},
	} {
		t.Run(test.name, func(t *testing.T) {
			const clientID = "eos-fixture-cli"
			const originalAccount = "34fe72c6-6307-4f98-870f-d66dd7e7e461"
			var issuer string
			var captured struct {
				sync.Mutex
				authorization url.Values
				accountCalls  int
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured.Lock()
				defer captured.Unlock()
				w.Header().Set("Content-Type", "application/json")
				var reply any
				switch r.URL.Path {
				case "/.well-known/openid-configuration":
					reply = map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
						"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks",
						"id_token_signing_alg_values_supported":          []string{"RS256"},
						"authorization_response_iss_parameter_supported": true}
				case "/jwks":
					reply = jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: key.Public(), KeyID: "fixture", Use: "sig", Algorithm: "RS256"}}}
				case "/token":
					query := captured.authorization
					if r.ParseForm() != nil || r.Method != http.MethodPost || r.Header.Get("Authorization") != "" ||
						r.Form.Get("client_id") != clientID || r.Form.Has("client_secret") ||
						r.Form.Get("code") != "fixture-code" || r.Form.Get("redirect_uri") != query.Get("redirect_uri") ||
						oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != query.Get("code_challenge") {
						t.Error("invalid PKCE token exchange")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					nonce := query.Get("nonce")
					if test.wrongNonce {
						nonce = "different-nonce"
					}
					now := time.Now()
					claims, err := json.Marshal(map[string]any{"iss": issuer, "sub": "opaque-subject",
						"aud": clientID, "nonce": nonce, "exp": now.Add(time.Minute).Unix(), "iat": now.Unix()})
					if err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					reply = map[string]any{"access_token": "fixture-access", "token_type": "Bearer", "expires_in": 60,
						"id_token": oidctest.SignIDToken(key, "fixture", "RS256", string(claims))}
				case "/api/v1/identity/me":
					captured.accountCalls++
					if r.Header.Get("Authorization") != "Bearer fixture-access" {
						t.Error("account request lacks the OIDC access token")
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					reply = map[string]any{"data": map[string]string{"user_id": originalAccount}}
				default:
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if err := json.NewEncoder(w).Encode(reply); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			issuer = server.URL
			open := func(target string) error {
				authorization, err := url.Parse(target)
				if err != nil {
					return err
				}
				query := authorization.Query()
				if query.Get("scope") != "openid account.read" || query.Get("audience") != audience ||
					query.Get("code_challenge_method") != "S256" || query.Get("nonce") == query.Get("state") {
					t.Errorf("incorrect authorization parameters: %s %s", query.Get("scope"), query.Get("audience"))
					return ErrConfig
				}
				captured.Lock()
				captured.authorization = query
				captured.Unlock()
				callback, err := url.Parse(query.Get("redirect_uri"))
				if err != nil || callback.Hostname() != "127.0.0.1" || callback.Port() == "" {
					return ErrCallback
				}
				callback.RawQuery = url.Values{"state": {query.Get("state")}, "code": {"fixture-code"}, "iss": {issuer}}.Encode()
				browser := http.Client{Timeout: 3 * time.Second}
				response, err := browser.Get(callback.String())
				if err != nil {
					return err
				}
				data, readErr := io.ReadAll(response.Body)
				closeErr := response.Body.Close()
				if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK ||
					string(data) != "Finished" || response.Header.Get("Cache-Control") != "no-store" {
					return ErrCallback
				}
				return nil
			}
			got, err := check(context.Background(), Settings{
				Issuer: issuer, APIOrigin: issuer, ClientID: clientID, Confirmation: "Finished",
			}, open, server.Client().Transport)
			captured.Lock()
			calls := captured.accountCalls
			captured.Unlock()
			if err != test.want {
				t.Fatalf("identity result = %v, want %v", err, test.want)
			}
			if err == nil && (got.String() != originalAccount || calls != 1) {
				t.Fatalf("account = %s with %d calls", got, calls)
			}
			if err != nil && (got != uuid.Nil || calls != 0) {
				t.Fatalf("unexpected account after credential rejection: %s with %d calls", got, calls)
			}
		})
	}
}

func TestNativeCallbackRejectsWrongAndDuplicateParametersBeforeConsuming(t *testing.T) {
	results := make(chan callbackResult, 1)
	handler := (callbackVerification{"state", "https://issuer.example.test", true, "Finished"}).handler("127.0.0.1:53319", results)
	for _, query := range []string{
		"state=wrong&code=code&iss=https%3A%2F%2Fissuer.example.test",
		"state=state&state=state&code=code&iss=https%3A%2F%2Fissuer.example.test",
		"state=state&code=code",
		"state=state&code=code&iss=https%3A%2F%2Fother.example.test",
		"state=state&code=code&error=denied&iss=https%3A%2F%2Fissuer.example.test",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:53319/oauth2/callback?"+query, nil))
		if response.Code != http.StatusBadRequest || len(results) != 0 {
			t.Fatalf("invalid callback was consumed: %s", query)
		}
	}
	target := "http://127.0.0.1:53319/oauth2/callback?state=state&code=code&iss=https%3A%2F%2Fissuer.example.test"
	first, replay := httptest.NewRecorder(), httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, target, nil))
	handler.ServeHTTP(replay, httptest.NewRequest(http.MethodGet, target, nil))
	if first.Code != http.StatusOK || replay.Code != http.StatusBadRequest || len(results) != 1 ||
		!strings.Contains(first.Body.String(), "Finished") {
		t.Fatal("expected one successful response and one refused replay")
	}
}
