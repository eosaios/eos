// Package account provides EOS identity login without subscription or license checks.
package account

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ProductionURL = "https://api.eosaios.com"

var errNetwork = errors.New("ACCOUNT_NETWORK_ERROR")

type apiError struct {
	Status int
	Code   string
}

func (e *apiError) Error() string { return fmt.Sprintf("account: %s (HTTP %d)", e.Code, e.Status) }

type client struct {
	base string
	http *http.Client
}

func newClient(base string) (*client, error) {
	if base == "" {
		base = ProductionURL
	} // Identity only; no commerce APIs are called.
	base = strings.TrimRight(base, "/")
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, errors.New("ACCOUNT_ENDPOINT_INVALID")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(local && u.Scheme == "http") {
		return nil, errors.New("ACCOUNT_HTTPS_REQUIRED")
	}
	return &client{base, &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("ACCOUNT_REDIRECT_REJECTED") }}}, nil
}
func (c *client) call(ctx context.Context, method, path string, body any, token string, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errNetwork
	}
	defer resp.Body.Close()
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || env.Error != nil {
		code := "ACCOUNT_REQUEST_FAILED"
		if env.Error != nil {
			code = env.Error.Code
		}
		return &apiError{resp.StatusCode, code}
	}
	if decodeErr != nil {
		return errors.New("ACCOUNT_RESPONSE_INVALID")
	}
	if out == nil {
		return nil
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return errors.New("ACCOUNT_RESPONSE_INVALID")
	}
	return json.Unmarshal(env.Data, out)
}
