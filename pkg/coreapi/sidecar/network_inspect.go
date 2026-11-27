package sidecar

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
)

// DisableNetworkInspect also protects release hosts that select an older or
// development sidecar via an explicit local path. Windows environment keys
// are case-insensitive.
func DisableNetworkInspect(env []string) []string {
	return setDiagnosticEnvironment(env, "EOS_NETWORK_INSPECT", "0")
}

type AgentReviewMode uint8

const (
	AgentReviewDisabled AgentReviewMode = iota
	AgentReviewDev
)

// SetAgentReviewMode overrides local environment case variants. A release
// host that explicitly selects a Dev sidecar still leaves collection off.
func SetAgentReviewMode(env []string, mode AgentReviewMode) []string {
	value := "0"
	if mode == AgentReviewDev {
		value = "1"
	}
	return setDiagnosticEnvironment(env, "EOS_DEV_AGENT_REVIEW", value)
}

func setDiagnosticEnvironment(env []string, name, value string) []string {
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, name) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, name+"="+value)
}

// HasNetworkInspectMetadata recognizes a Core that can retain the safe
// diagnostics schema. Legacy Dev cores captured arbitrary request bodies.
func HasNetworkInspectMetadata(binaryPath string) (bool, error) {
	file, err := os.Open(binaryPath)
	if err != nil {
		return false, err
	}
	defer file.Close()
	marker := []byte("dev_network_inspect_metadata_v1")
	buffer := make([]byte, 64<<10)
	carry := 0
	for {
		n, err := io.ReadFull(file, buffer[carry:])
		size := carry + n
		if bytes.Contains(buffer[:size], marker) {
			return true, nil
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		carry = len(marker) - 1
		copy(buffer[:carry], buffer[size-carry:size])
	}
}
