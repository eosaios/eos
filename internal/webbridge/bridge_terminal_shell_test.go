//go:build windows

package webbridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBashBesideGit(t *testing.T) {
	cases := map[string]string{
		`C:\Program Files\Git\cmd\git.exe`: `C:\Program Files\Git\bin\bash.exe`,
		`D:\tools\Git\cmd\git.exe`:         `D:\tools\Git\bin\bash.exe`,
		`C:\Program Files\Git\bin\git.exe`: "", // 非标准布局（bin 下无 cmd）
		`C:\git.exe`:                       "",
	}
	for gitExe, want := range cases {
		if got := bashBesideGit(gitExe); got != want {
			t.Errorf("bashBesideGit(%q) = %q, want %q", gitExe, got, want)
		}
	}
}

func TestAppManagedBashPath(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\u\AppData\Local`)
	want := `C:\Users\u\AppData\Local\eos\shell\git\bin\bash.exe`
	if got := appManagedBashPath(); got != want {
		t.Fatalf("appManagedBashPath() = %q, want %q", got, want)
	}
}

func TestTerminalBashCandidateResolveRequiresFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bash.exe")
	if err := os.WriteFile(file, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, ok := (terminalBashCandidate{path: file, source: "path"}).resolve(); !ok {
		t.Fatal("existing file should resolve")
	}
	if _, ok := (terminalBashCandidate{path: filepath.Join(dir, "missing.exe"), source: "path"}).resolve(); ok {
		t.Fatal("missing file should not resolve")
	}
	if _, ok := (terminalBashCandidate{path: dir, source: "path"}).resolve(); ok {
		t.Fatal("directory should not resolve")
	}
	if _, ok := (terminalBashCandidate{path: "", source: "path"}).resolve(); ok {
		t.Fatal("empty path should not resolve")
	}
}

func TestProbeTerminalShellPrefersAppManaged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	// 程序指定目录就位 → app-managed 优先于一切系统来源。
	managed := filepath.Join(dir, "eos", "shell", "git", "bin", "bash.exe")
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	status := probeTerminalShell()
	if !status.Available || status.Source != terminalShellSourceAppManaged || status.Path != managed {
		t.Fatalf("probe = %+v, want app-managed at %s", status, managed)
	}
}

func TestTerminalShellSnapshotCachesAndInvalidates(t *testing.T) {
	s := &BridgeService{}
	first := s.terminalShellSnapshot()
	if !first.Available {
		t.Fatal("test host has bash somewhere; probe should succeed")
	}
	s.terminalShell.mu.Lock()
	cached := s.terminalShell.resolved
	s.terminalShell.mu.Unlock()
	if !cached {
		t.Fatal("snapshot should cache the probe result")
	}
	s.terminalShellInvalidate()
	s.terminalShell.mu.Lock()
	cached = s.terminalShell.resolved
	s.terminalShell.mu.Unlock()
	if cached {
		t.Fatal("invalidate should drop the cache")
	}
}

func TestDownloadPercent(t *testing.T) {
	if got := downloadPercent(0, 100); got != 0 {
		t.Errorf("downloadPercent(0,100) = %d", got)
	}
	if got := downloadPercent(50, 100); got != 50 {
		t.Errorf("downloadPercent(50,100) = %d", got)
	}
	if got := downloadPercent(100, 100); got != 99 {
		t.Errorf("downloadPercent(100,100) = %d, want capped 99 until done", got)
	}
	if got := downloadPercent(10, 0); got != 0 {
		t.Errorf("downloadPercent(10,0) = %d, want 0 for unknown total", got)
	}
}

func TestPortableGitFetchURLMirrorOverride(t *testing.T) {
	if got := portableGitFetchURL(); got != portableGitDownloadURL {
		t.Errorf("default URL = %q", got)
	}
	t.Setenv("EOS_PORTABLEGIT_MIRROR", "https://mirror.example.com/dl/")
	want := "https://mirror.example.com/dl/" + portableGitAssetName
	if got := portableGitFetchURL(); got != want {
		t.Errorf("mirror URL = %q, want %q", got, want)
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "asset.bin")
	payload := []byte("portable git payload")
	if err := os.WriteFile(file, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	good := hex.EncodeToString(sum[:])
	if err := verifyFileSHA256(file, good); err != nil {
		t.Fatalf("matching hash rejected: %v", err)
	}
	if err := verifyFileSHA256(file, strings.Repeat("0", 64)); err == nil {
		t.Fatal("mismatched hash must be rejected")
	}
	if err := verifyFileSHA256(filepath.Join(dir, "missing"), good); err == nil {
		t.Fatal("missing file must be rejected")
	}
}

// localShellAssetFetcher 从本地文件源"下载"（测试不打网络）。
func localShellAssetFetcher(path string) shellAssetFetcher {
	return func(ctx context.Context, url string, progress func(received, total int64)) (string, error) {
		return path, nil
	}
}

// fakeShellExtract 模拟 SFX 解压：把载荷复制为 staging 的 bin\bash.exe。
func fakeShellExtract(payload []byte) func(string, string) error {
	return func(sfxPath string, stagingDir string) error {
		bin := filepath.Join(stagingDir, "bin", "bash.exe")
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			return err
		}
		return os.WriteFile(bin, payload, 0o755)
	}
}

func TestRunTerminalShellInstallHappyPath(t *testing.T) {
	s := &BridgeService{}
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	ws := newTerminalShellWorkspace()

	payload := []byte("fake bash binary")
	sfx := filepath.Join(dir, "portablegit.7z.exe")
	if err := os.WriteFile(sfx, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)

	s.runTerminalShellInstall(terminalShellInstaller{
		fetch:       localShellAssetFetcher(sfx),
		extract:     fakeShellExtract(payload),
		ws:          ws,
		expectedSHA: hex.EncodeToString(sum[:]),
	})

	installed := filepath.Join(ws.installDir, "bin", "bash.exe")
	data, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("installed bash missing: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatal("installed payload mismatch")
	}
	status := s.terminalShellSnapshot()
	if !status.Available || status.Source != terminalShellSourceAppManaged {
		t.Fatalf("status after install = %+v, want app-managed available", status)
	}
}

func TestRunTerminalShellInstallRejectsBadHash(t *testing.T) {
	s := &BridgeService{}
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	ws := newTerminalShellWorkspace()

	payload := []byte("tampered payload")
	sfx := filepath.Join(dir, "portablegit.7z.exe")
	if err := os.WriteFile(sfx, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	// 官方 pin 哈希 + 篡改载荷 → 必须拒绝且不得执行解压。
	s.runTerminalShellInstall(terminalShellInstaller{
		fetch:       localShellAssetFetcher(sfx),
		extract:     fakeShellExtract(payload),
		ws:          ws,
		expectedSHA: portableGitSHA256,
	})

	if _, err := os.Stat(filepath.Join(ws.installDir, "bin", "bash.exe")); !os.IsNotExist(err) {
		t.Fatal("hash mismatch must not produce an installation")
	}
}
