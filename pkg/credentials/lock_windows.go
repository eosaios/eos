//go:build windows

package credentials

import (
	"errors"
)

// macOS is the file-vault rollout target. Native storage remains active on Windows.
func lockDirectory(string) (func(), error) {
	return nil, errors.New("file credentials are unavailable on Windows")
}
func syncDirectory(string) error { return nil }
