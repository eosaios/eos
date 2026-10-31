package account

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/eosaios/eos/pkg/credentials"
	"github.com/zalando/go-keyring"
)

// State deliberately contains no credentials or subscription state.
type State struct {
	LoggedIn     bool   `json:"loggedIn"`
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	DisplayName  string `json:"displayName"`
	ProfileError string `json:"profileError,omitempty"`
	AvatarURL    string `json:"avatarUrl,omitempty"`
}
type session struct {
	Access    string    `json:"access_token"`
	Refresh   string    `json:"refresh_token"`
	ExpiresIn int64     `json:"access_token_expires_in"`
	ExpiresAt time.Time `json:"expires_at"`
	Email     string    `json:"email,omitempty"`
}
type flow struct {
	ID          string
	Deadline    time.Time
	nextPoll    time.Time
	rateStrikes int
	ticket      string
}
type secretStore interface {
	Get() (string, error)
	Set(string) error
	Delete() error
}
type systemStore struct{ service string }

func (s systemStore) Get() (string, error) {
	v, e := credentials.Get(s.service, "session")
	if errors.Is(e, keyring.ErrNotFound) {
		return "", nil
	}
	return v, e
}
func (s systemStore) Set(v string) error { return credentials.Set(s.service, "session", v) }
func (s systemStore) Delete() error {
	e := credentials.Delete(s.service, "session")
	if errors.Is(e, keyring.ErrNotFound) {
		return nil
	}
	return e
}

// Manager serializes refresh/login/logout so late replies cannot restore a logged-out session.
type Manager struct {
	mu     sync.Mutex
	client *client
	name   string
	store  secretStore
	flow   *flow
}

func New(name string) (*Manager, error) {
	if name != "eos-desktop" && name != "eos-desktop-beta" && name != "eos-desktop-dev" && name != "eos-cli" {
		return nil, errors.New("ACCOUNT_CLIENT_INVALID")
	}
	c, err := newClient(os.Getenv("EOS_ACCOUNT_BASE_URL"))
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(c.base))
	return &Manager{client: c, name: name, store: systemStore{fmt.Sprintf("com.eosaios.%s.account.%x", name, hash[:8])}}, nil
}

// SetHTTPTransport connects account requests to the owning app's live network policy.
func (m *Manager) SetHTTPTransport(transport http.RoundTripper) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.client.http.Transport = transport
}

// Start creates a browser flow. Only the latest flow may establish a session.
func (m *Manager) Start(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flow = nil
	var out struct {
		ID  string `json:"flow_id"`
		URL string `json:"login_url"`
		TTL int64  `json:"expires_in"`
	}
	if err := m.client.call(ctx, "POST", "/api/v1/auth/desktop/flows", map[string]string{"client": m.name}, "", &out); err != nil {
		return "", err
	}
	u, err := url.Parse(out.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || out.ID == "" || out.TTL <= 0 {
		return "", errors.New("ACCOUNT_FLOW_INVALID")
	}
	m.flow = &flow{ID: out.ID, Deadline: time.Now().Add(time.Duration(out.TTL) * time.Second)}
	return out.URL, nil
}

// Poll checks once; pending returns false. Call at intervals of at least two seconds.
func (m *Manager) Poll(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.flow
	if f == nil {
		return false, errors.New("ACCOUNT_NO_LOGIN_FLOW")
	}
	if time.Now().After(f.Deadline) {
		m.flow = nil
		return false, errors.New("ACCOUNT_LOGIN_EXPIRED")
	}
	if time.Now().Before(f.nextPoll) {
		return false, nil
	}
	if f.ticket != "" {
		return m.exchange(ctx, f)
	}
	var out struct {
		Status string `json:"status"`
		Ticket string `json:"ticket"`
		Client string `json:"client"`
	}
	if err := m.client.call(ctx, "GET", "/api/v1/auth/desktop/flows/"+url.PathEscape(f.ID), nil, "", &out); err != nil {
		if retryableFlowPoll(err) {
			f.nextPoll = time.Now().Add(flowPollDelay(f, err))
			return false, nil
		}
		m.flow = nil
		return false, err
	}
	f.rateStrikes = 0
	f.nextPoll = time.Now().Add(2 * time.Second)
	if out.Client != "" && out.Client != m.name {
		m.flow = nil
		return false, errors.New("ACCOUNT_FLOW_INVALID")
	}
	if out.Status == "pending" {
		return false, nil
	}
	if out.Status != "completed" || out.Ticket == "" {
		m.flow = nil
		return false, errors.New("ACCOUNT_LOGIN_EXPIRED")
	}
	f.ticket = out.Ticket
	return m.exchange(ctx, f)
}

func (m *Manager) exchange(ctx context.Context, f *flow) (bool, error) {
	// A slow status request must not spend the one-use ticket with only a
	// fraction of the HTTP timeout left. The next poll has a fresh bridge
	// budget and exchanges the retained ticket without another status GET.
	// Check again for retained tickets: waiting on another account operation
	// can also spend the caller's budget before it acquires the manager lock.
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < m.client.http.Timeout {
		slog.Info("account.login.exchange_deferred", "client", m.name)
		return false, nil
	}
	// Never replay an exchange: the server may have consumed the ticket even
	// when its response is lost. Surface a retryable-login action to the user.
	m.flow = nil
	host, err := os.Hostname()
	if err != nil {
		return false, err
	}
	var sess session
	if err := m.client.call(ctx, "POST", "/api/v1/auth/desktop/ticket-exchange", map[string]string{"flow_id": f.ID, "ticket": f.ticket, "device_name": m.name + " / " + host}, "", &sess); err != nil {
		slog.Warn("account.login.exchange_failed", "client", m.name, "error", err)
		return false, err
	}
	if err := m.persist(sess); err != nil {
		slog.Warn("account.login.persist_failed", "client", m.name)
		return false, errors.New("ACCOUNT_STORAGE_ERROR")
	}
	slog.Info("account.login.completed", "client", m.name)
	return true, nil
}
func (m *Manager) persist(s session) error {
	if s.Access == "" || s.Refresh == "" || s.ExpiresIn <= 0 {
		return errors.New("ACCOUNT_SESSION_INVALID")
	}
	s.ExpiresAt = time.Now().Add(time.Duration(s.ExpiresIn) * time.Second)
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return m.store.Set(string(b))
}

// Status restores a session, rotates expired tokens and reads the authoritative profile.
func (m *Manager) Status(ctx context.Context) (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, err := m.store.Get()
	if err != nil {
		return State{}, errors.New("ACCOUNT_STORAGE_ERROR")
	}
	if raw == "" {
		return State{}, nil
	}
	var sess session
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return State{}, errors.New("ACCOUNT_SESSION_INVALID")
	}
	if sess.Access == "" || sess.Refresh == "" {
		return State{}, errors.New("ACCOUNT_SESSION_INVALID")
	}
	if time.Now().Add(30 * time.Second).After(sess.ExpiresAt) {
		if err := m.refresh(ctx, &sess); err != nil {
			return identitySyncFailure(sess, err)
		}
	}
	state, err := m.profile(ctx, sess.Access)
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.Status == 401 {
		if err = m.refresh(ctx, &sess); err != nil {
			return identitySyncFailure(sess, err)
		}
		state, err = m.profile(ctx, sess.Access)
	}
	if err != nil {
		return identitySyncFailure(sess, err)
	}
	return state, nil
}

// Cached identity is not an authorization grant. Only a definitive refresh
// rejection removes the stored session; a network outage must not look like
// sign-out after relaunch merely because the access token needs rotation.
func identitySyncFailure(sess session, err error) (State, error) {
	if retryableProfileSync(err) {
		return State{LoggedIn: true, Email: sess.Email, ProfileError: "ACCOUNT_PROFILE_SYNC_FAILED"}, nil
	}
	return State{}, err
}
func (m *Manager) refresh(ctx context.Context, s *session) error {
	var next session
	err := m.client.call(ctx, "POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": s.Refresh}, "", &next)
	if err != nil {
		var rejected *apiError
		if errors.As(err, &rejected) && rejected.Status == 401 && rejected.Code == "AUTH_INVALID_TOKEN" {
			if delErr := m.store.Delete(); delErr != nil {
				return delErr
			}
		}
		return err
	}
	next.Email = s.Email
	if err = m.persist(next); err != nil {
		return err
	}
	*s = next
	return nil
}
func (m *Manager) profile(ctx context.Context, token string) (State, error) {
	var p struct {
		ID     string `json:"user_id"`
		Email  string `json:"email"`
		Name   string `json:"display_name"`
		Avatar string `json:"avatar_url"`
	}
	if err := m.client.call(ctx, "GET", "/api/v1/auth/me", nil, token, &p); err != nil {
		return State{}, err
	}
	if p.ID == "" {
		return State{}, errors.New("ACCOUNT_PROFILE_INVALID")
	}
	if len(p.Avatar) > 0 && p.Avatar[0] == '/' {
		p.Avatar = m.client.base + p.Avatar
	}
	return State{LoggedIn: true, UserID: p.ID, Email: p.Email, DisplayName: p.Name, AvatarURL: p.Avatar}, nil
}

// Logout cancels pending login and removes the local credentials.
func (m *Manager) Logout() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flow = nil
	return m.store.Delete()
}
