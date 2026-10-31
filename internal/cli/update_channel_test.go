package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/version"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpdateCommandKeepsInstalledChannel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	savedVersion := version.AppVersion
	savedTransport := http.DefaultTransport
	t.Cleanup(func() { version.AppVersion = savedVersion; http.DefaultTransport = savedTransport })
	for _, tc := range []struct{ current, channel, latest string }{{"v1.0.0-beta.34", "stable", "v1.0.0"}, {"v1.0.0-beta.36", "beta", "v1.0.0-beta.37"}} {
		version.AppVersion = tc.current
		requests := 0
		http.DefaultTransport = updateTransport(func(r *http.Request) (*http.Response, error) {
			requests++
			response := &http.Response{StatusCode: 200, Header: make(http.Header)}
			if tc.channel == "stable" {
				if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
					t.Fatal("stable queried beta", r.URL)
				}
				response.StatusCode = 302
				response.Header.Set("Location", "https://github.com/eosaios/eos/releases/tag/"+tc.latest)
				response.Body = io.NopCloser(strings.NewReader(""))
			} else {
				if r.URL.Host != "api.github.com" {
					t.Fatal("beta queried stable", r.URL)
				}
				response.Body = io.NopCloser(strings.NewReader(`[{"tag_name":"v9.0.0","draft":false,"prerelease":false},{"tag_name":"v1.0.0-beta.37","draft":false,"prerelease":true}]`))
			}
			return response, nil
		})
		var runErr error
		output := captureStdout(t, func() { cmd := newUpdateCmd(); cmd.SetArgs([]string{"--check"}); runErr = cmd.Execute() })
		if runErr != nil || requests != 1 || !strings.Contains(output, tc.latest) || !strings.Contains(output, tc.channel) {
			t.Fatalf("%s: %q requests=%d err=%v", tc.current, output, requests, runErr)
		}
	}
}
