package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Channel string

const (
	Stable           Channel = "stable"
	Beta             Channel = "beta"
	InstallationFile         = "eos-install.json"
)

var releaseVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-beta\.[0-9]+)?$`)

// VersionChannel describes EOS's published channel contract. beta.34 is the
// historical ordinary/latest release and deliberately stays on the stable line.
func VersionChannel(tag string) (Channel, error) {
	if !releaseVersion.MatchString(tag) {
		return "", fmt.Errorf("unsupported release version: %q", tag)
	}
	if strings.TrimPrefix(tag, "v") == "1.0.0-beta.34" || !strings.Contains(tag, "-beta.") {
		return Stable, nil
	}
	return Beta, nil
}

type Installation struct {
	Channel Channel `json:"channel"`
	Method  string  `json:"method"`
}

// ReadInstallation uses per-install metadata, never a shared desktop config.
// A standalone binary has no marker, so its immutable version identifies its
// channel. Malformed or contradictory metadata must fail without changing files.
func ReadInstallation(executable, version string) (Installation, error) {
	channel, err := VersionChannel(version)
	if err != nil {
		return Installation{}, err
	}
	install := Installation{Channel: channel, Method: "archive"}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(executable), InstallationFile))
	if os.IsNotExist(err) {
		return install, nil
	}
	if err != nil {
		return Installation{}, err
	}
	install = Installation{}
	if err := json.Unmarshal(data, &install); err != nil {
		return Installation{}, fmt.Errorf("invalid %s: %w", InstallationFile, err)
	}
	if install.Channel != channel {
		return Installation{}, fmt.Errorf("installation channel %q conflicts with version %s (%s)", install.Channel, version, channel)
	}
	switch install.Method {
	case "archive", "script", "package-manager":
		return install, nil
	default:
		return Installation{}, fmt.Errorf("invalid installation method %q", install.Method)
	}
}

func ManagedInstallation(executable string, install Installation) bool {
	if install.Method == "package-manager" {
		return true
	}
	path := strings.ToLower(strings.ReplaceAll(executable, "\\", "/"))
	for _, part := range []string{"/cellar/", "/homebrew/caskroom/", "/scoop/apps/", "/chocolatey/", "/nix/store/", "/snap/"} {
		if strings.Contains(path, part) {
			return true
		}
	}
	return false
}

func WriteInstallation(executable string, install Installation) error {
	data, err := json.Marshal(install)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(filepath.Dir(executable), InstallationFile), append(data, '\n'), 0o644)
}
