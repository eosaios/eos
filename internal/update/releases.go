package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/eosaios/eos/internal/version"
)

type release struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

const releasesAPI = "https://api.github.com/repos/eosaios/eos/releases"

func (r *release) UnmarshalJSON(data []byte) error {
	var fields struct {
		Tag        string `json:"tag_name"`
		Draft      *bool  `json:"draft"`
		Prerelease *bool  `json:"prerelease"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields.Tag == "" || fields.Draft == nil || fields.Prerelease == nil {
		return fmt.Errorf("release metadata is missing tag_name, draft or prerelease")
	}
	*r = release{Tag: fields.Tag, Draft: *fields.Draft, Prerelease: *fields.Prerelease}
	return nil
}

func (r release) channel() (Channel, error) {
	channel, err := VersionChannel(r.Tag)
	if err != nil {
		return "", err
	}
	if r.Draft || r.Prerelease != (channel == Beta) {
		return "", fmt.Errorf("release %s has inconsistent channel metadata", r.Tag)
	}
	return channel, nil
}

func readReleaseJSON(ctx context.Context, endpoint string, client *http.Client, dest any) error {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "EOS-Update-Check")
	req.Header.Set("Accept", "application/vnd.github+json")
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("release metadata HTTP %d (GitHub rate limits or connectivity may prevent checking)", resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 16<<20))
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("release metadata: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("invalid trailing release metadata")
	}
	return nil
}

func latestChannelTag(ctx context.Context, channel Channel, client *http.Client) (string, error) {
	if channel == Stable {
		tag, err := fetchLatestTag(ctx, latestReleasesPageURL(), client)
		if err != nil {
			return "", err
		}
		// latest must stay on the formal line even if a maintainer accidentally
		// marks a future beta as latest. No cross-channel fallback is permitted.
		actual, err := VersionChannel(tag)
		if err != nil {
			return "", err
		}
		if actual != Stable {
			return "", fmt.Errorf("latest points to beta release %s", tag)
		}
		return tag, nil
	}
	if channel != Beta {
		return "", fmt.Errorf("unknown update channel %q", channel)
	}
	newest := ""
	for page := 1; page <= 100; page++ {
		var releases []release
		if err := readReleaseJSON(ctx, fmt.Sprintf("%s?per_page=100&page=%d", releasesAPI, page), client, &releases); err != nil {
			return "", err
		}
		for _, candidate := range releases {
			actual, err := candidate.channel()
			if err != nil || actual != Beta {
				continue
			}
			if newest == "" || isNewer(newest, candidate.Tag) {
				newest = candidate.Tag
			}
		}
		if len(releases) < 100 {
			if newest == "" {
				return "", fmt.Errorf("no published beta release found")
			}
			return newest, nil
		}
	}
	return "", fmt.Errorf("release pagination exceeded safety limit")
}

func CheckChannelWithClient(ctx context.Context, channel Channel, client *http.Client) (*CheckResult, error) {
	return checkChannelFor(ctx, channel, platform{runtime.GOOS, runtime.GOARCH}, client)
}

type platform struct{ goos, goarch string }

func checkChannelFor(ctx context.Context, channel Channel, target platform, client *http.Client) (*CheckResult, error) {
	latest, err := latestChannelTag(ctx, channel, client)
	if err != nil {
		return nil, err
	}
	result := buildCheckResult(strings.TrimSpace(version.AppVersion), latest, target.goos, target.goarch)
	result.Channel = channel
	return result, nil
}
