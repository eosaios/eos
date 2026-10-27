package architecture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi/generated"
)

func isReleaseArtifactCheck() bool {
	return os.Getenv("EOS_RELEASE_ARTIFACT_CHECK") == "1"
}

func TestClosedRustCoreSourceIsNotVendored(t *testing.T) {
	root := moduleRoot(t)
	var problems []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build", "output", "release", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		base := strings.ToLower(filepath.Base(path))
		ext := strings.ToLower(filepath.Ext(path))
		switch {
		case ext == ".rs":
			problems = append(problems, rel+" is Rust source")
		case base == "cargo.toml" || base == "cargo.lock":
			problems = append(problems, rel+" is Rust Cargo metadata")
		case ext == ".pdb" || ext == ".dsym":
			problems = append(problems, rel+" is a debug symbol artifact")
		case ext == ".rlib" || ext == ".rmeta":
			problems = append(problems, rel+" is a Rust compiled library artifact")
		case ext == ".dwarf":
			problems = append(problems, rel+" is a DWARF debug symbol")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository: %v", err)
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("closed Rust core source or debug artifacts must not be committed:\n%s", strings.Join(problems, "\n"))
	}
}

func TestNoSecretsOrPrivateKeysVendored(t *testing.T) {
	root := moduleRoot(t)
	var problems []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build", "output", "release", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		base := strings.ToLower(filepath.Base(path))
		ext := strings.ToLower(filepath.Ext(path))

		switch {
		case ext == ".pem" || ext == ".key" || ext == ".p12" || ext == ".pfx":
			content, readErr := os.ReadFile(path)
			if readErr == nil {
				text := string(content)
				if strings.Contains(text, "PRIVATE KEY") {
					problems = append(problems, rel+" contains private key material")
				}
			}
		case strings.Contains(base, "private") && (ext == ".pem" || ext == ".key"):
			problems = append(problems, rel+" filename suggests private key file")
		case base == "id_rsa" || base == "id_ed25519" || base == "id_ecdsa" || base == "id_dsa":
			problems = append(problems, rel+" is an SSH private key")
		}

		if ext == ".go" || ext == ".json" || ext == ".yaml" || ext == ".yml" || ext == ".toml" || ext == ".md" || ext == ".txt" || ext == ".ps1" || ext == ".sh" {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			text := string(content)
			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "-----BEGIN") && strings.Contains(line, "PRIVATE KEY") {
					problems = append(problems, rel+" contains PEM private key block")
					break
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("scan repository: %v", err)
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("private keys or secret markers must not be committed to the public repository:\n%s", strings.Join(problems, "\n"))
	}
}

func TestVendoredManifestsAreNotDevelopmentPlaceholder(t *testing.T) {
	root := moduleRoot(t)
	binariesDir := filepath.Join(root, "pkg", "coreapi", "sidecar", "core")
	if _, err := os.Stat(binariesDir); os.IsNotExist(err) {
		t.Skip("no core directory found")
	}
	var placeholderManifests []string
	err := filepath.WalkDir(binariesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.ToLower(d.Name()) != "manifest.json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var manifest struct {
			Signature string `json:"signature"`
		}
		if jsonErr := json.Unmarshal(data, &manifest); jsonErr != nil {
			return jsonErr
		}
		if strings.TrimSpace(manifest.Signature) == "unsigned-development-placeholder" {
			rel, _ := filepath.Rel(root, path)
			placeholderManifests = append(placeholderManifests, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan core directory: %v", err)
	}
	if len(placeholderManifests) > 0 {
		releaseCheck := isReleaseArtifactCheck()
		msg := fmt.Sprintf("%d vendored manifest(s) use unsigned-development-placeholder signature:", len(placeholderManifests))
		for _, m := range placeholderManifests {
			msg += "\n  - " + m
		}
		if releaseCheck {
			msg += "\nEOS_RELEASE_ARTIFACT_CHECK=1: release gate requires Ed25519-signed manifests."
			msg += "\nTo sign, run package-public-artifact.ps1 with -PrivateKeyPath pointing to the Ed25519 signing key."
			t.Fatalf("RELEASE GATE FAILED: %s", msg)
		}
		t.Logf("DEVELOPMENT-ONLY: %s", msg)
		t.Logf("These are development artifacts. Production release requires Ed25519-signed manifests.")
		t.Logf("To enforce, set EOS_RELEASE_ARTIFACT_CHECK=1 in CI before running tests.")
	}
}

func TestVendoredManifestsCoverGeneratedCoreMethods(t *testing.T) {
	root := moduleRoot(t)
	binariesDir := filepath.Join(root, "pkg", "coreapi", "sidecar", "core")
	if _, err := os.Stat(binariesDir); os.IsNotExist(err) {
		t.Skip("no core directory found")
	}

	required := generated.CoreMethods()
	var checked int
	var staleManifests []string
	err := filepath.WalkDir(binariesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.ToLower(d.Name()) != "manifest.json" {
			return nil
		}
		checked++
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var manifest struct {
			Features []string `json:"features"`
		}
		if jsonErr := json.Unmarshal(data, &manifest); jsonErr != nil {
			return jsonErr
		}
		missing := missingManifestFeatures(manifest.Features, required)
		if len(missing) == 0 {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		staleManifests = append(staleManifests, fmt.Sprintf(
			"%s missing %d generated core method(s): %s",
			filepath.ToSlash(rel),
			len(missing),
			strings.Join(firstStrings(missing, 12), ", "),
		))
		return nil
	})
	if err != nil {
		t.Fatalf("scan core directory: %v", err)
	}
	if checked == 0 {
		t.Skip("no vendored manifests found")
	}
	if len(staleManifests) == 0 {
		return
	}

	sort.Strings(staleManifests)
	msg := "vendored sidecar manifest feature sets do not cover generated.CoreMethods():\n" + strings.Join(staleManifests, "\n")
	if isReleaseArtifactCheck() {
		msg += "\nEOS_RELEASE_ARTIFACT_CHECK=1: release artifacts must be rebuilt from eos-core-protocol and signed after manifest update."
		t.Fatalf("RELEASE GATE FAILED: %s", msg)
	}
	t.Logf("DEVELOPMENT-ONLY: %s", msg)
	t.Logf("To enforce full protocol coverage in CI, set EOS_RELEASE_ARTIFACT_CHECK=1 before running tests.")
}

func missingManifestFeatures(features []string, required []string) []string {
	seen := map[string]struct{}{}
	for _, feature := range features {
		feature = strings.TrimSpace(feature)
		if feature != "" {
			seen[feature] = struct{}{}
		}
	}
	var missing []string
	for _, feature := range required {
		feature = strings.TrimSpace(feature)
		if feature == "" {
			continue
		}
		if _, ok := seen[feature]; !ok {
			missing = append(missing, feature)
		}
	}
	return missing
}

func firstStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	out := append([]string(nil), values[:limit]...)
	out = append(out, "...")
	return out
}
