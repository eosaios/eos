package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout    = 30 * time.Second
	defaultHTTPMaxBytes   = int64(1 << 20) // 1MB
	defaultRedirectMaxHop = 10
)

type ClientErrorKind string

const (
	ErrInvalidURL        ClientErrorKind = "invalid_url"
	ErrNonHTTPSURL       ClientErrorKind = "non_https_url"
	ErrTooManyRedirects  ClientErrorKind = "too_many_redirects"
	ErrCrossHostRedirect ClientErrorKind = "cross_host_redirect"
	ErrHTTPStatus        ClientErrorKind = "http_status"
)

type ClientError struct {
	Kind         ClientErrorKind
	URL          string
	RedirectURL  string
	StatusCode   int
	Message      string
	BodySnippet  string
	WrappedError error
}

func (e *ClientError) Error() string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case ErrInvalidURL:
		return fmt.Sprintf("invalid URL: %s", e.URL)
	case ErrNonHTTPSURL:
		return fmt.Sprintf("non-HTTPS URL is not allowed: %s", e.URL)
	case ErrTooManyRedirects:
		return fmt.Sprintf("too many redirects for %s", e.URL)
	case ErrCrossHostRedirect:
		return fmt.Sprintf("cross-host redirect from %s to %s is not allowed", e.URL, e.RedirectURL)
	case ErrHTTPStatus:
		if e.Message != "" {
			return e.Message
		}
		return fmt.Sprintf("HTTP request failed with status %d", e.StatusCode)
	default:
		if e.Message != "" {
			return e.Message
		}
		if e.WrappedError != nil {
			return e.WrappedError.Error()
		}
		return "http client error"
	}
}

func (e *ClientError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.WrappedError
}

type RedirectPolicy struct {
	MaxHops        int
	AllowCrossHost bool
}

type RequestSpec struct {
	Method         string
	URL            string
	Headers        http.Header
	Timeout        time.Duration
	MaxBytes       int64
	Accept         string
	UserAgent      string
	RetryPolicy    RetryPolicy
	RedirectPolicy RedirectPolicy
	AllowHTTP      bool
	AllowLocalHTTP bool
}

type ResponseSpec struct {
	StatusCode  int
	Status      string
	Header      http.Header
	Body        []byte
	FinalURL    string
	ContentType string
	Truncated   bool
}

type HTTPClient struct {
	base *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		base: &http.Client{},
	}
}

func (c *HTTPClient) Do(ctx context.Context, spec RequestSpec) (*ResponseSpec, error) {
	spec = withHTTPDefaults(spec)
	if err := validateRequestSpec(spec); err != nil {
		return nil, err
	}

	attempts := max(1, spec.RetryPolicy.MaxAttempts)
	var lastResp *ResponseSpec
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := c.doOnce(ctx, spec)
		lastResp = resp
		lastErr = err
		if !shouldRetry(spec, resp, err, attempt) {
			return resp, err
		}
		delay := calculateDelay(attempt, spec.RetryPolicy)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return lastResp, ctx.Err()
		}
	}

	return lastResp, lastErr
}

func (c *HTTPClient) doOnce(ctx context.Context, spec RequestSpec) (*ResponseSpec, error) {
	currentURL := spec.URL
	redirects := 0
	for {
		parsedURL, err := url.Parse(currentURL)
		if err != nil {
			return nil, &ClientError{Kind: ErrInvalidURL, URL: currentURL, WrappedError: err}
		}

		resp, err := c.sendSingleRequest(ctx, spec, currentURL)
		if err != nil {
			return nil, err
		}
		if isRedirectStatus(resp.StatusCode) {
			location := resp.Header.Get("Location")
			_ = respCloseBody(resp)
			if strings.TrimSpace(location) == "" {
				return nil, &ClientError{
					Kind:       ErrHTTPStatus,
					URL:        currentURL,
					StatusCode: resp.StatusCode,
					Message:    fmt.Sprintf("redirect response missing Location header: %d", resp.StatusCode),
				}
			}
			nextURL, err := parsedURL.Parse(location)
			if err != nil {
				return nil, &ClientError{Kind: ErrInvalidURL, URL: location, WrappedError: err}
			}
			redirects++
			if redirects > spec.RedirectPolicy.MaxHops {
				return nil, &ClientError{Kind: ErrTooManyRedirects, URL: spec.URL}
			}
			if !spec.RedirectPolicy.AllowCrossHost && !sameOriginOrWWW(parsedURL, nextURL) {
				return nil, &ClientError{
					Kind:        ErrCrossHostRedirect,
					URL:         currentURL,
					RedirectURL: nextURL.String(),
					StatusCode:  resp.StatusCode,
				}
			}
			if !isAllowedScheme(nextURL, spec) {
				return nil, &ClientError{Kind: ErrNonHTTPSURL, URL: nextURL.String()}
			}
			currentURL = nextURL.String()
			continue
		}
		return readHTTPResponse(resp, spec.MaxBytes)
	}
}

func (c *HTTPClient) sendSingleRequest(ctx context.Context, spec RequestSpec, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, spec.Method, rawURL, nil)
	if err != nil {
		return nil, &ClientError{Kind: ErrInvalidURL, URL: rawURL, WrappedError: err}
	}

	for k, values := range spec.Headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	if strings.TrimSpace(spec.Accept) != "" && req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", spec.Accept)
	}
	if strings.TrimSpace(spec.UserAgent) != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", spec.UserAgent)
	}

	client := *c.base
	client.Timeout = spec.Timeout
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func readHTTPResponse(resp *http.Response, maxBytes int64) (*ResponseSpec, error) {
	defer func() { _ = resp.Body.Close() }()
	limited, truncated, err := readLimited(resp.Body, maxBytes)
	if err != nil {
		return nil, err
	}

	result := &ResponseSpec{
		StatusCode:  resp.StatusCode,
		Status:      resp.Status,
		Header:      resp.Header.Clone(),
		Body:        limited,
		ContentType: resp.Header.Get("Content-Type"),
		Truncated:   truncated,
	}
	if resp.Request != nil && resp.Request.URL != nil {
		result.FinalURL = resp.Request.URL.String()
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(limited))
		if len(snippet) > 240 {
			snippet = snippet[:240] + "..."
		}
		return result, &ClientError{
			Kind:        ErrHTTPStatus,
			URL:         result.FinalURL,
			StatusCode:  resp.StatusCode,
			BodySnippet: snippet,
			Message:     fmt.Sprintf("HTTP request failed: %s", resp.Status),
		}
	}

	return result, nil
}

func withHTTPDefaults(spec RequestSpec) RequestSpec {
	if strings.TrimSpace(spec.Method) == "" {
		spec.Method = http.MethodGet
	}
	if spec.Timeout <= 0 {
		spec.Timeout = defaultHTTPTimeout
	}
	if spec.MaxBytes <= 0 {
		spec.MaxBytes = defaultHTTPMaxBytes
	}
	if spec.Headers == nil {
		spec.Headers = make(http.Header)
	}
	if spec.RedirectPolicy.MaxHops <= 0 {
		spec.RedirectPolicy.MaxHops = defaultRedirectMaxHop
	}
	if spec.RetryPolicy.MaxAttempts <= 0 {
		spec.RetryPolicy = NoRetry
	}
	return spec
}

func validateRequestSpec(spec RequestSpec) error {
	parsed, err := url.Parse(spec.URL)
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Host) == "" {
		return &ClientError{Kind: ErrInvalidURL, URL: spec.URL, WrappedError: err}
	}
	if !isAllowedScheme(parsed, spec) {
		return &ClientError{Kind: ErrNonHTTPSURL, URL: spec.URL}
	}
	return nil
}

func isAllowedScheme(parsed *url.URL, spec RequestSpec) bool {
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return true
	case "http":
		if spec.AllowHTTP {
			return true
		}
		if spec.AllowLocalHTTP && isLocalHost(parsed.Hostname()) {
			return true
		}
		return false
	default:
		return false
	}
}

func shouldRetry(spec RequestSpec, resp *ResponseSpec, err error, attempt int) bool {
	if spec.RetryPolicy.MaxAttempts <= 1 || attempt >= spec.RetryPolicy.MaxAttempts {
		return false
	}
	method := strings.ToUpper(strings.TrimSpace(spec.Method))
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	if err != nil {
		var clientErr *ClientError
		if errors.As(err, &clientErr) {
			// 结构性错误（非法 URL/跨主机重定向等）不重试。
			// HTTP 状态错误要按状态码判断——否则 429/503 永远重试不到。
			if clientErr.Kind == ErrHTTPStatus {
				return IsRetryableHTTPError(clientErr.StatusCode)
			}
			return false
		}
		var netErr net.Error
		if errors.As(err, &netErr) {
			return true
		}
		return IsRetryableError(err)
	}
	if resp == nil {
		return false
	}
	return IsRetryableHTTPError(resp.StatusCode)
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		body, err := io.ReadAll(r)
		return body, false, err
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > maxBytes {
		return body[:maxBytes], true, nil
	}
	return body, false, nil
}

func isRedirectStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func sameOriginOrWWW(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		a.Port() == b.Port() &&
		stripWWW(a.Hostname()) == stripWWW(b.Hostname())
}

func stripWWW(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.TrimPrefix(host, "www.")
}

func isLocalHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func respCloseBody(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	return resp.Body.Close()
}
