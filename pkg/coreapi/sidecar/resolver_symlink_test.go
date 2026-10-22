package sidecar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// install.sh installs the complete release under share/eos, then links bin/eos.
// Run a real process through that link: source-tree and embedded cores must not
// hide a failure to select the sidecar shipped beside the installed executable.
func TestResolveBinaryFromSymlinkedInstallation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix installer layout; Windows does not install a symlink")
	}
	root := t.TempDir()
	install := filepath.Join(root, "share", "eos")
	binDir := filepath.Join(root, "bin")
	for _, dir := range []string{install, binDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	installedExe := filepath.Join(install, "eos")
	if err := os.WriteFile(installedExe, data, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(binDir, "eos")
	if err := os.Symlink(filepath.Join("..", "share", "eos", "eos"), link); err != nil {
		t.Fatal(err)
	}

	target := TargetTriple(runtime.GOOS, runtime.GOARCH)
	coreDir := filepath.Join(install, "core", target)
	if err := os.MkdirAll(coreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	coreBytes := []byte("installed sidecar selected, never executed")
	wantPath := filepath.Join(coreDir, "eos-core")
	if err := os.WriteFile(wantPath, coreBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(coreBytes)
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion, CoreVersion: "0.1.0",
		APIVersion: DefaultAPIVersion, Target: target, Binary: "eos-core",
		SHA256:    "sha256:" + hex.EncodeToString(sum[:]),
		Signature: "unsigned-development-placeholder", Features: []string{"initialize"},
	}
	manifestPath := filepath.Join(coreDir, DefaultManifestName)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{EnvCorePath, EnvCoreManifest, EnvCoreBinDir, EnvReleaseArtifactCheck, EnvSignaturePublicKey} {
		t.Setenv(name, "")
	}
	t.Setenv("EOS_TEST_SYMLINK_RESOLVE", "1")
	cmd := exec.Command(link, "-test.run=^TestSymlinkResolveHelper$")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve through installer symlink: %v\n%s", err, out)
	}
	var got struct{ Path, ManifestPath, Source string }
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode child result: %v\n%s", err, out)
	}
	wantPath, err = filepath.EvalSymlinks(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	wantManifest, err := filepath.EvalSymlinks(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	want := struct{ Path, ManifestPath, Source string }{wantPath, wantManifest, "search"}
	if got != want {
		t.Fatalf("resolved = %+v, want installed release %+v", got, want)
	}

	// A corrupt installed core must fail checksum validation, never select the
	// source-tree or embedded core instead.
	if err := os.WriteFile(wantPath, []byte("corrupt"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(link, "-test.run=^TestSymlinkResolveHelper$")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), ErrChecksumMismatch.Error()) {
		t.Fatalf("corrupt installed core: got %v, %s; want checksum mismatch", err, out)
	}
}

func TestSymlinkResolveHelper(t *testing.T) {
	if os.Getenv("EOS_TEST_SYMLINK_RESOLVE") != "1" {
		return
	}
	resolved, err := ResolveBinary(ResolveOptions{
		VerifyChecksum: true, RequireSignature: true, AllowDevPlaceholder: true,
		RequiredFeatures: []string{"initialize"},
	})
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
	err = json.NewEncoder(os.Stdout).Encode(struct{ Path, ManifestPath, Source string }{
		resolved.Path, resolved.ManifestPath, resolved.Source,
	})
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
