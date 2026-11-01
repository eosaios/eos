package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/version"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, data string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(data))}
}

func TestChannelIdentityAndInstallation(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		want Channel
	}{{"v1.0.0-beta.34", Stable}, {"1.0.0-beta.34", Stable}, {"v1.0.0-beta.35", Beta}, {"v1.0.0-beta.36", Beta}, {"v1.0.0", Stable}, {"v2.4.1", Stable}} {
		got, err := VersionChannel(tc.tag)
		if err != nil || got != tc.want {
			t.Fatalf("%s: %s %v", tc.tag, got, err)
		}
	}
	if _, err := VersionChannel("v1.0.0-rc.1"); err == nil {
		t.Fatal("unsupported channel accepted")
	}
	exe := filepath.Join(t.TempDir(), "eos")
	want := Installation{Channel: Beta, Method: "script"}
	if err := WriteInstallation(exe, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInstallation(exe, "v1.0.0-beta.36")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
	if _, err := ReadInstallation(exe, "v1.0.0-beta.34"); err == nil {
		t.Fatal("legacy stable moved into beta")
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(exe), InstallationFile), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadInstallation(exe, "v1.0.0-beta.36"); err == nil {
		t.Fatal("corrupt marker ignored")
	}
}

func TestBetaReleaseSelectionAndPagination(t *testing.T) {
	releases := []release{
		release{Tag: "v1.0.0-beta.34", Prerelease: false, Draft: false}, release{Tag: "v1.0.0-beta.35", Prerelease: true, Draft: false},
		release{Tag: "v1.0.0-beta.100", Prerelease: true, Draft: true}, release{Tag: "v9.0.0", Prerelease: false, Draft: false},
		release{Tag: "v1.0.0-beta.9", Prerelease: true, Draft: false}, release{Tag: "v1.0.0-beta.40", Prerelease: false, Draft: false},
		release{Tag: "v1.0.0-rc.999", Prerelease: true, Draft: false},
	}
	for len(releases) < 100 {
		releases = append(releases, release{Tag: "v0.0.1", Prerelease: false, Draft: false})
	}
	calls := 0
	client := &http.Client{Transport: releaseTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.github.com" {
			t.Fatal("beta used stable latest", r.URL)
		}
		data := releases
		if r.URL.Query().Get("page") == "2" {
			data = []release{release{Tag: "v1.0.0-beta.36", Prerelease: true, Draft: false}, release{Tag: "v1.0.0-beta.10", Prerelease: true, Draft: false}}
		}
		raw, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		return response(200, string(raw)), nil
	})}
	got, err := latestChannelTag(context.Background(), Beta, client)
	if err != nil || got != "v1.0.0-beta.36" || calls != 2 {
		t.Fatal(got, calls, err)
	}
}

func TestStableLatestContract(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		fail bool
	}{{"v1.0.0-beta.34", false}, {"v1.0.0", false}, {"v1.1.0", false}, {"v1.0.0-beta.36", true}} {
		client := &http.Client{Transport: releaseTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != latestReleasesPageURL() {
				t.Fatal(r.URL)
			}
			resp := response(302, "")
			resp.Header.Set("Location", "https://github.com/eosaios/eos/releases/tag/"+tc.tag)
			return resp, nil
		})}
		got, err := latestChannelTag(context.Background(), Stable, client)
		if (err != nil) != tc.fail || (!tc.fail && got != tc.tag) {
			t.Fatal(tc, got, err)
		}
	}
}

func TestBetaFailuresDoNotSelectStable(t *testing.T) {
	for _, tc := range []struct {
		name         string
		code         int
		body         string
		transportErr error
	}{
		{name: "empty", code: 200, body: "[]"},
		{name: "only stable", code: 200, body: `[{"tag_name":"v1.0.0-beta.34","prerelease":false,"draft":false}]`},
		{name: "malformed", code: 200, body: "{"},
		{name: "rate limit", code: 403, body: `{"message":"rate limit"}`},
		{name: "server", code: 503, body: "unavailable"},
		{name: "network", transportErr: errors.New("fixture network failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: releaseTransport(func(*http.Request) (*http.Response, error) {
				if tc.transportErr != nil {
					return nil, tc.transportErr
				}
				return response(tc.code, tc.body), nil
			})}
			if got, err := latestChannelTag(context.Background(), Beta, client); err == nil || got != "" {
				t.Fatal(got, err)
			}
		})
	}
}

func TestChannelCheckResults(t *testing.T) {
	old := version.AppVersion
	t.Cleanup(func() { version.AppVersion = old })
	for _, tc := range []struct {
		current, latest string
		has             bool
	}{{"v1.0.0-beta.35", "v1.0.0-beta.36", true}, {"v1.0.0-beta.36", "v1.0.0-beta.36", false}, {"v1.0.0-beta.40", "v1.0.0-beta.36", false}, {"v1.0.0-beta.9", "v1.0.0-beta.10", true}} {
		version.AppVersion = tc.current
		client := &http.Client{Transport: releaseTransport(func(*http.Request) (*http.Response, error) {
			return response(200, fmt.Sprintf(`[{"tag_name":%q,"prerelease":true,"draft":false}]`, tc.latest)), nil
		})}
		got, err := checkChannelFor(context.Background(), Beta, platform{"windows", "amd64"}, client)
		if err != nil || got.Channel != Beta || got.HasUpdate != tc.has || got.LatestVersion != tc.latest || !strings.HasSuffix(got.AssetName, "_windows-amd64.zip") {
			t.Fatal(got, err)
		}
	}
}

func TestPackageManagerBoundary(t *testing.T) {
	for _, path := range []string{"/opt/homebrew/Cellar/eos/1.0/bin/eos", `C:\Users\fixture\scoop\apps\eos\current\eos.exe`, `C:\ProgramData\chocolatey\bin\eos.exe`, "/nix/store/id-eos/bin/eos", "/snap/eos/current/eos"} {
		if !ManagedInstallation(path, Installation{Method: "archive"}) {
			t.Fatal(path)
		}
	}
	if ManagedInstallation("/home/fixture/.local/share/eos/eos", Installation{Method: "script"}) {
		t.Fatal("script installation blocked")
	}
	if !ManagedInstallation("/usr/bin/eos", Installation{Method: "package-manager"}) {
		t.Fatal("package-manager marker ignored")
	}
}

func TestApplyRejectsCrossChannelBeforeReplacement(t *testing.T) {
	tmp := t.TempDir()
	binary := "eos"
	if runtime.GOOS == "windows" {
		binary = "eos.exe"
	}
	archivePath, asset, sum := buildTestArchive(t, tmp, binary, "eos-cross-channel")
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	exe, err := currentExecutable()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: releaseTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/sums") {
			return response(200, sum+"  "+asset+"\n"), nil
		}
		return response(200, string(archive)), nil
	})}
	_, err = ApplyWithClient(context.Background(), &CheckResult{LatestVersion: "v1.0.0", AssetName: asset, DownloadURL: "https://fixture/archive", ChecksumURL: "https://fixture/sums"}, nil, client)
	if err == nil || !strings.Contains(err.Error(), "cross-channel") {
		t.Fatalf("expected channel rejection, got %v", err)
	}
	after, err := os.ReadFile(exe)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("cross-channel attempt changed executable", err)
	}
}
