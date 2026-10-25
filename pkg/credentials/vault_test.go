//go:build !windows

package credentials

import (
	"errors"
	"fmt"
	gokeyring "github.com/zalando/go-keyring"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMigrationAndLogout(t *testing.T) {
	var reads atomic.Int32
	v := Vault{Dir: t.TempDir(), LegacyGet: func(string, string) (string, error) { reads.Add(1); return "synthetic-secret", nil }}
	for range 3 {
		got, err := v.Get("fixture", "account")
		if err != nil || got != "synthetic-secret" {
			t.Fatal("migration read failed", err)
		}
	}
	if err := v.Delete("fixture", "account"); err != nil {
		t.Fatal(err)
	}
	got, err := v.Get("fixture", "account")
	if err != nil || got != "" || reads.Load() != 1 {
		t.Fatal("logout resurrected legacy", err)
	}
	raw, err := os.ReadFile(filepath.Join(v.Dir, "credentials.enc"))
	if err != nil || strings.Contains(string(raw), "synthetic-secret") {
		t.Fatal("plaintext persisted", err)
	}
}
func TestMissingLegacyOnlyReadOnce(t *testing.T) {
	var reads int
	v := Vault{Dir: t.TempDir(), LegacyGet: func(string, string) (string, error) { reads++; return "", gokeyring.ErrNotFound }}
	for range 3 {
		got, err := v.Get("fixture", "absent")
		if err != nil || got != "" {
			t.Fatal(err)
		}
	}
	if reads != 1 {
		t.Fatalf("legacy queried %d times", reads)
	}
}
func TestConcurrentInstancesKeepAllWrites(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for n := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := Vault{Dir: dir}
			if err := v.Set("fixture", fmt.Sprint(n), "value"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	v := Vault{Dir: dir}
	for n := range 32 {
		value, err := v.Get("fixture", fmt.Sprint(n))
		if err != nil || value != "value" {
			t.Fatal("lost credential", n, err)
		}
	}
}
func TestCorruptionDoesNotEraseVault(t *testing.T) {
	v := Vault{Dir: t.TempDir()}
	if err := v.Set("s", "a", "v"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(v.Dir, "credentials.enc")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get("s", "a"); err == nil {
		t.Fatal("corruption hidden")
	}
	if err := v.Set("s", "b", "v"); err == nil {
		t.Fatal("corruption overwritten")
	}
	if err := v.Delete("s", "a"); err == nil {
		t.Fatal("corruption overwritten")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "broken" {
		t.Fatal("vault changed")
	}
}
func TestMissingKeyDoesNotRegenerate(t *testing.T) {
	v := Vault{Dir: t.TempDir()}
	if err := v.Set("s", "a", "v"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(v.Dir, "machine-id")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := v.Set("s", "b", "v"); err == nil {
		t.Fatal("missing key reset")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("key recreated")
	}
}
func TestPermissionsAndAccountBinding(t *testing.T) {
	v := Vault{Dir: filepath.Join(t.TempDir(), "private")}
	if err := v.Set("s", "a", "v"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"credentials.enc", "machine-id", "credentials.lock"} {
		info, err := os.Stat(filepath.Join(v.Dir, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("unsafe permissions", name, err)
		}
	}
	file, err := v.load()
	if err != nil {
		t.Fatal(err)
	}
	file.Entries[entryKey("s", "b")] = file.Entries[entryKey("s", "a")]
	if err = v.save(file); err != nil {
		t.Fatal(err)
	}
	if _, err = v.Get("s", "b"); err == nil {
		t.Fatal("account swap accepted")
	}
}

func TestSharedGoRustVectorsAndLegacyV1(t *testing.T) {
	v := Vault{Dir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(v.Dir, "machine-id"), []byte("000102030405060708090a0b0c0d0e0f"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, encoded := range []string{"enc:v1:000102030405060708090a0b628c02ded63339dc88796ae7f4c747827e306ce9f44db061062f64429b82d61c570bc9fc71d9c416", "enc:v2:000102030405060708090a0b628c02ded63339dc88796ae7f4c747827e306ce9f44db061437da84ace6a505d9dfe3526016b2bea"} {
		file := vaultFile{Entries: map[string]*string{entryKey("fixture", "account"): &encoded}}
		if err := v.save(file); err != nil {
			t.Fatal(err)
		}
		value, err := v.Get("fixture", "account")
		if err != nil || value != "synthetic-interop-凭据" {
			t.Fatal("incompatible vault", err)
		}
	}
}
