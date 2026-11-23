package identitypilot

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

const (
	callbackPath = "/oauth2/callback"
	audience     = "eos-account-api"
	readScope    = "account.read"
)

var (
	ErrConfig   = errors.New("identity.check.invalid_config")
	ErrProvider = errors.New("identity.check.provider")
	ErrBrowser  = errors.New("identity.check.browser")
	ErrCallback = errors.New("identity.check.callback")
	ErrAccount  = errors.New("identity.check.account")
)

type Settings struct {
	Issuer       string
	APIOrigin    string
	ClientID     string
	Confirmation string
}

// Check completes a read-only identity check using the system browser.
func Check(ctx context.Context, settings Settings, open func(string) error) (uuid.UUID, error) {
	return check(ctx, settings, open, http.DefaultTransport)
}

func check(ctx context.Context, settings Settings, open func(string) error, transport http.RoundTripper) (uuid.UUID, error) {
	issuer, err := httpsOrigin(settings.Issuer)
	if err != nil {
		return uuid.Nil, ErrConfig
	}
	api, err := httpsOrigin(settings.APIOrigin)
	if err != nil || !validClientID(settings.ClientID) {
		return uuid.Nil, ErrConfig
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	ctx, cancel := context.WithTimeout(oidc.ClientContext(ctx, client), 5*time.Minute)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, issuer.String())
	if err != nil {
		return uuid.Nil, ErrProvider
	}
	var metadata struct {
		JWKS                   string `json:"jwks_uri"`
		SupportsResponseIssuer bool   `json:"authorization_response_iss_parameter_supported"`
	}
	endpoint := provider.Endpoint()
	if provider.Claims(&metadata) != nil || !providerEndpoint(issuer, endpoint.AuthURL) ||
		!providerEndpoint(issuer, endpoint.TokenURL) || !providerEndpoint(issuer, metadata.JWKS) {
		return uuid.Nil, ErrProvider
	}
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	config := oauth2.Config{ClientID: settings.ClientID, Endpoint: endpoint, Scopes: []string{oidc.ScopeOpenID, readScope}}
	state, nonce, verifier := oauth2.GenerateVerifier(), oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
	callback := callbackVerification{
		state: state, issuer: issuer.String(), requireIssuer: metadata.SupportsResponseIssuer,
		confirmation: settings.Confirmation,
	}
	code, err := nativeCode(ctx, &config, open, callback, nonce, verifier)
	if err != nil {
		return uuid.Nil, err
	}
	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil || token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") {
		return uuid.Nil, ErrProvider
	}
	idToken, ok := token.Extra("id_token").(string)
	if !ok || idToken == "" {
		return uuid.Nil, ErrProvider
	}
	identity, err := provider.VerifierContext(ctx, &oidc.Config{
		ClientID: settings.ClientID, SupportedSigningAlgs: []string{oidc.RS256},
	}).Verify(ctx, idToken)
	if err != nil || !validIdentity(identity, settings.ClientID, nonce, token.AccessToken, time.Now()) {
		return uuid.Nil, ErrProvider
	}
	api.Path = "/api/v1/identity/me"
	return fetchAccount(ctx, client, api, token)
}

func httpsOrigin(raw string) (*url.URL, error) {
	origin, err := url.Parse(raw)
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil ||
		(origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.ForceQuery ||
		origin.Fragment != "" || origin.RawPath != "" {
		return nil, ErrConfig
	}
	origin.Path = ""
	return origin, nil
}

func providerEndpoint(issuer *url.URL, raw string) bool {
	endpoint, err := url.Parse(raw)
	return err == nil && endpoint.Scheme == issuer.Scheme && endpoint.Host == issuer.Host &&
		endpoint.User == nil && endpoint.Path != "" && endpoint.Fragment == ""
}

func validClientID(value string) bool {
	return value != "" && len(value) <= 160 && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:-", r))
	}) == -1
}

func validIdentity(identity *oidc.IDToken, clientID, nonce, accessToken string, now time.Time) bool {
	if identity.Subject == "" || identity.Nonce != nonce || len(identity.Audience) != 1 ||
		identity.Audience[0] != clientID || identity.IssuedAt.IsZero() ||
		identity.IssuedAt.After(now.Add(30*time.Second)) {
		return false
	}
	var claims struct {
		AuthorizedParty string `json:"azp"`
	}
	return identity.Claims(&claims) == nil &&
		(claims.AuthorizedParty == "" || claims.AuthorizedParty == clientID) &&
		(identity.AccessTokenHash == "" || identity.VerifyAccessToken(accessToken) == nil)
}

func fetchAccount(ctx context.Context, client *http.Client, api *url.URL, token *oauth2.Token) (uuid.UUID, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, api.String(), nil)
	if err != nil {
		return uuid.Nil, ErrAccount
	}
	token.SetAuthHeader(request)
	response, err := client.Do(request)
	if err != nil {
		return uuid.Nil, ErrAccount
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return uuid.Nil, ErrAccount
	}
	var account struct {
		Data struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8192))
	if decoder.Decode(&account) != nil {
		return uuid.Nil, ErrAccount
	}
	id, err := uuid.Parse(account.Data.UserID)
	if err != nil || id == uuid.Nil || id.String() != account.Data.UserID {
		return uuid.Nil, ErrAccount
	}
	return id, nil
}

type callbackVerification struct {
	state         string
	issuer        string
	requireIssuer bool
	confirmation  string
}

type callbackResult struct {
	code string
	err  error
}

func nativeCode(ctx context.Context, config *oauth2.Config, open func(string) error, callback callbackVerification, nonce, verifier string) (string, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", ErrCallback
	}
	host := listener.Addr().String()
	config.RedirectURL = (&url.URL{Scheme: "http", Host: host, Path: callbackPath}).String()
	results := make(chan callbackResult, 1)
	server := &http.Server{
		Handler:           callback.handler(host, results),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       time.Second,
		MaxHeaderBytes:    8192,
	}
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	target := config.AuthCodeURL(callback.state, oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("audience", audience))
	if open(target) != nil {
		return "", ErrBrowser
	}
	select {
	case result := <-results:
		return result.code, result.err
	case <-ctx.Done():
		return "", ErrCallback
	case <-done:
		return "", ErrCallback
	}
}

func (callback callbackVerification) handler(host string, results chan<- callbackResult) http.Handler {
	var consumed atomic.Bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		params, err := url.ParseQuery(r.URL.RawQuery)
		state, code, issuer, failure := params["state"], params["code"], params["iss"], params["error"]
		if r.Method != http.MethodGet || r.Host != host || r.URL.EscapedPath() != callbackPath ||
			err != nil || len(state) != 1 || !sameValue(state[0], callback.state) ||
			(len(issuer) != 0 || callback.requireIssuer) && (len(issuer) != 1 || issuer[0] != callback.issuer) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var result callbackResult
		switch {
		case len(failure) == 1 && failure[0] != "" && len(code) == 0:
			result.err = ErrCallback
		case len(failure) == 0 && len(code) == 1 && code[0] != "" && len(code[0]) <= 2048:
			result.code = code[0]
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !consumed.CompareAndSwap(false, true) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if _, err := io.WriteString(w, callback.confirmation); err != nil {
			result.err = ErrCallback
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			result.err = ErrCallback
		}
		results <- result
	})
}

func sameValue(actual, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
