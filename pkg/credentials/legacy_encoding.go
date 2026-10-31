package credentials

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// go-keyring wraps native values on write. Decode once during read-only import,
// before encrypting the actual secret in the shared vault.
func decodeLegacySecret(value string) (string, error) {
	var decoded []byte
	var err error
	switch {
	case strings.HasPrefix(value, "go-keyring-base64:"):
		decoded, err = base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "go-keyring-base64:"))
	case strings.HasPrefix(value, "go-keyring-encoded:"):
		decoded, err = hex.DecodeString(strings.TrimPrefix(value, "go-keyring-encoded:"))
	default:
		return value, nil
	}
	if err != nil {
		return "", errors.New("legacy credential encoding invalid")
	}
	return string(decoded), nil
}
