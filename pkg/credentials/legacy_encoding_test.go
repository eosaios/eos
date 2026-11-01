//go:build !windows

package credentials

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestLegacyKeyringImportDecodesBeforeVaultPersistence(t *testing.T) {
	want := `{"access_token":"synthetic","refresh_token":"synthetic-refresh"}`
	for _, encoded := range []string{want, "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(want)), "go-keyring-encoded:" + hex.EncodeToString([]byte(want))} {
		v := Vault{Dir: t.TempDir(), LegacyGet: func(string, string) (string, error) { return decodeLegacySecret(encoded) }}
		got, err := v.Get("fixture", "session")
		if err != nil || got != want {
			t.Fatalf("import did not restore session: %v", err)
		}
		v.LegacyGet = func(string, string) (string, error) {
			t.Fatal("import repeated after vault persistence")
			return "", nil
		}
		got, err = v.Get("fixture", "session")
		if err != nil || got != want {
			t.Fatalf("restored wrong session: %v", err)
		}
	}
	for _, malformed := range []string{"go-keyring-base64:!", "go-keyring-encoded:xx"} {
		if value, err := decodeLegacySecret(malformed); err == nil || value != "" {
			t.Fatal("invalid encoding accepted")
		}
	}
}
