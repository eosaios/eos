//go:build !darwin || !cgo

package credentials

import (
	"errors"
)

func legacyGet(string, string) (string, error) {
	return "", errors.New("legacy migration unavailable; sign in again")
}
