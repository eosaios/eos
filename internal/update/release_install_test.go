package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/eosaios/eos/pkg/coreapi/sidecar"
)

const testReleaseVersion = "v1.0.0-beta.40"

func releaseFixture(t *testing.T) map[string]string {
	t.Helper()
	binary := "eos"
	coreBinary := "eos-core"
	if runtime.GOOS == "windows" {
		binary += ".exe"
		coreBinary += ".exe"
	}
	digest := sha256.Sum256([]byte("new-core"))
	pair := releasePair{CLIVersion: testReleaseVersion, CLICommit: strings.Repeat("a", 40), CoreSourceCommit: strings.Repeat("b", 40), CoreTarget: sidecar.CurrentTarget(), CoreSHA256: hex.EncodeToString(digest[:])}
	pairData, err := json.Marshal(pair)
	if err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"schema_version":"1.0","core_version":"0.1.0","api_version":"v1","target":%q,"binary":%q,"sha256":%q,"source_commit":%q}`, pair.CoreTarget, coreBinary, pair.CoreSHA256, pair.CoreSourceCommit)
	return map[string]string{
		binary: "new-binary", releasePairFile: string(pairData),
		"core/" + pair.CoreTarget + "/manifest.json": manifest,
		"core/" + pair.CoreTarget + "/" + coreBinary: "new-core",
	}
}

func fixtureServer(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "release/" + filepath.ToSlash(name), Mode: 0o755, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive.Bytes())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		if r.URL.Path == "/SHA256SUMS.txt" {
			_, err = fmt.Fprintf(w, "%x  release.tar.gz\n", digest)
		} else {
			_, err = w.Write(archive.Bytes())
		}
		if err != nil {
			t.Errorf("serve fixture: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func readFixture(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		files[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestApplySynchronizesReleasePair(t *testing.T) {
	for _, oldPair := range []string{"", `{"cli_version":"v1.0.0-beta.38"}`} {
		t.Run("old-pair="+oldPair, func(t *testing.T) {
			files := releaseFixture(t)
			binary := "eos"
			if runtime.GOOS == "windows" {
				binary += ".exe"
			}
			dir := t.TempDir()
			old := map[string]string{binary: "old-binary", "core/old-core": "old-core", InstallationFile: `{"channel":"beta","method":"script"}`, "unrelated.txt": "keep"}
			if oldPair != "" {
				old[releasePairFile] = oldPair
			}
			writeFixture(t, dir, old)
			server := fixtureServer(t, files)
			result, err := applyToInstallation(context.Background(), &CheckResult{LatestVersion: testReleaseVersion, DownloadURL: server.URL + "/release.tar.gz", AssetName: "release.tar.gz", ChecksumURL: server.URL + "/SHA256SUMS.txt"}, nil, server.Client(), filepath.Join(dir, binary))
			if err != nil {
				t.Fatal(err)
			}
			wantOutcome := &ApplyOutcome{BinaryPath: filepath.Join(dir, binary), CoreUpdated: true}
			if !reflect.DeepEqual(result, wantOutcome) {
				t.Fatalf("outcome=%+v want=%+v", result, wantOutcome)
			}
			files[InstallationFile] = "{\"channel\":\"beta\",\"method\":\"script\"}\n"
			files["unrelated.txt"] = "keep"
			if got := readFixture(t, dir); !reflect.DeepEqual(got, files) {
				t.Fatalf("installed package=%v want=%v", got, files)
			}
		})
	}
}

func TestApplyRejectsIncoherentPairBeforeReplacing(t *testing.T) {
	cases := map[string]func(map[string]string){
		"missing pair":   func(f map[string]string) { delete(f, releasePairFile) },
		"malformed pair": func(f map[string]string) { f[releasePairFile] = "{" },
		"wrong version": func(f map[string]string) {
			f[releasePairFile] = strings.ReplaceAll(f[releasePairFile], testReleaseVersion, "v1.0.0-beta.39")
		},
		"wrong source": func(f map[string]string) {
			f[releasePairFile] = strings.ReplaceAll(f[releasePairFile], strings.Repeat("b", 40), strings.Repeat("c", 40))
		},
		"invalid commit": func(f map[string]string) {
			f[releasePairFile] = strings.ReplaceAll(f[releasePairFile], strings.Repeat("a", 40), "unknown")
		},
		"wrong platform": func(f map[string]string) {
			f[releasePairFile] = strings.ReplaceAll(f[releasePairFile], sidecar.CurrentTarget(), "../outside")
		},
		"wrong digest": func(f map[string]string) {
			f[releasePairFile] = strings.ReplaceAll(f[releasePairFile], `"core_sha256":"`, `"core_sha256":"0`)
		},
		"missing core": func(f map[string]string) {
			for name := range f {
				if strings.HasPrefix(name, "core") {
					delete(f, name)
				}
			}
		},
		"damaged core": func(f map[string]string) {
			for name := range f {
				if strings.HasPrefix(name, "core") && !strings.HasSuffix(name, "json") {
					f[name] = "damaged"
				}
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			files := releaseFixture(t)
			mutate(files)
			dir := t.TempDir()
			binary := "eos"
			if runtime.GOOS == "windows" {
				binary += ".exe"
			}
			old := map[string]string{binary: "old-binary", "core/old-core": "old-core", releasePairFile: "old-pair", InstallationFile: `{"channel":"beta","method":"script"}`}
			writeFixture(t, dir, old)
			server := fixtureServer(t, files)
			if _, err := applyToInstallation(context.Background(), &CheckResult{LatestVersion: testReleaseVersion, DownloadURL: server.URL + "/release.tar.gz", AssetName: "release.tar.gz", ChecksumURL: server.URL + "/SHA256SUMS.txt"}, nil, server.Client(), filepath.Join(dir, binary)); err == nil {
				t.Fatal("invalid release accepted")
			}
			if got := readFixture(t, dir); !reflect.DeepEqual(got, old) {
				t.Fatalf("installation changed after rejection: %v", got)
			}
		})
	}
}

func TestReleaseTransactionRollsBackEveryReplacement(t *testing.T) {
	for failAt := 1; failAt <= 8; failAt++ {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			dir, stage := t.TempDir(), t.TempDir()
			old := map[string]string{"eos": "old-cli", "core/old": "old-core", releasePairFile: "old-pair", InstallationFile: "old-channel"}
			writeFixture(t, dir, old)
			writeFixture(t, stage, map[string]string{"eos": "new-cli", "core/new": "new-core", releasePairFile: "new-pair", InstallationFile: "new-channel"})
			calls := 0
			tx := releaseTransaction{dir: dir, stage: stage, rename: func(from, to string) error {
				calls++
				if calls == failAt {
					return errors.New("permission denied")
				}
				return os.Rename(from, to)
			}}
			if err := tx.commit([]string{"eos", "core", releasePairFile, InstallationFile}); err == nil || !strings.Contains(err.Error(), "permission denied") {
				t.Fatalf("expected replacement failure, got %v", err)
			}
			if got := readFixture(t, dir); !reflect.DeepEqual(got, old) {
				t.Fatalf("rollback left mixed package: %v", got)
			}
			if tx.recoveryRequired {
				t.Fatal("successful rollback marked recovery required")
			}
		})
	}
}

func TestReleaseTransactionRollbackPreservesAbsentMetadata(t *testing.T) {
	dir, stage := t.TempDir(), t.TempDir()
	old := map[string]string{"eos": "old-cli"}
	writeFixture(t, dir, old)
	writeFixture(t, stage, map[string]string{"eos": "new-cli", "core/new": "new-core", releasePairFile: "new-pair", InstallationFile: "new-channel"})
	tx := releaseTransaction{dir: dir, stage: stage, rename: func(from, to string) error {
		if to == filepath.Join(dir, InstallationFile) {
			return errors.New("cannot install channel marker")
		}
		return os.Rename(from, to)
	}}
	if err := tx.commit([]string{"eos", "core", releasePairFile, InstallationFile}); err == nil {
		t.Fatal("expected channel marker failure")
	}
	if got := readFixture(t, dir); !reflect.DeepEqual(got, old) {
		t.Fatalf("rollback invented files in a standalone install: %v", got)
	}
}

func TestReleaseTransactionReportsRecoveryFailure(t *testing.T) {
	dir, stage := t.TempDir(), t.TempDir()
	writeFixture(t, dir, map[string]string{"eos": "old-cli", releasePairFile: "old-pair"})
	writeFixture(t, stage, map[string]string{"eos": "new-cli", releasePairFile: "new-pair"})
	tx := releaseTransaction{dir: dir, stage: stage, rename: func(from, to string) error {
		if from == filepath.Join(stage, releasePairFile) || from == filepath.Join(stage, "previous-eos") {
			return errors.New("filesystem unavailable")
		}
		return os.Rename(from, to)
	}}
	err := tx.commit([]string{"eos", releasePairFile})
	if err == nil || !tx.recoveryRequired || !strings.Contains(err.Error(), "recovery files preserved at "+stage) {
		t.Fatalf("unreported recovery failure: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(stage, "previous-eos"))
	if err != nil || string(data) != "old-cli" {
		t.Fatalf("old CLI recovery copy lost: %q %v", data, err)
	}
}

func TestInstallReleaseRejectsUnwritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("POSIX write permissions require a non-root Unix user")
	}
	root, dir := t.TempDir(), t.TempDir()
	files := releaseFixture(t)
	writeFixture(t, root, files)
	old := map[string]string{"eos": "old-cli", releasePairFile: "old-pair"}
	writeFixture(t, dir, old)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Error(err)
		}
	})
	if err := installRelease(root, filepath.Join(root, "eos"), filepath.Join(dir, "eos"), Installation{Channel: Beta, Method: "script"}); err == nil {
		t.Fatal("unwritable install directory accepted")
	}
	if got := readFixture(t, dir); !reflect.DeepEqual(got, old) {
		t.Fatalf("unwritable install changed: %v", got)
	}
}
